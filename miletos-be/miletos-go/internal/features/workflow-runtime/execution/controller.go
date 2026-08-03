package execution

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/shared/requestcontext"

	"github.com/go-chi/chi/v5"
)

const maximumRequestBody = 1 << 20

type ExecutionController struct {
	service  *ExecutionService
	recovery *RecoveryService
	queries  *ExecutionQueryService
}

func NewExecutionController(
	service *ExecutionService,
	recovery *RecoveryService,
	queries *ExecutionQueryService,
) *ExecutionController {
	return &ExecutionController{
		service: service, recovery: recovery, queries: queries,
	}
}

func (controller *ExecutionController) ExecuteSync(writer http.ResponseWriter, request *http.Request) {
	prepared, ok := controller.executionCommand(writer, request, "SYNC")
	if !ok {
		return
	}
	outcome, err := controller.service.ExecuteSyncCommand(request.Context(), prepared)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapExecutionOutcome(outcome))
}

func (controller *ExecutionController) ExecuteAsync(writer http.ResponseWriter, request *http.Request) {
	prepared, ok := controller.executionCommand(writer, request, "ASYNC")
	if !ok {
		return
	}
	outcome, err := controller.service.ExecuteAsyncCommand(request.Context(), prepared)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, mapExecutionOutcome(outcome))
}

func (controller *ExecutionController) Recover(writer http.ResponseWriter, request *http.Request) {
	key, err := idempotencyKey(request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
		return
	}
	companyID, executionID := executionScope(request)
	outcome, err := controller.recovery.Recover(
		request.Context(), companyID, executionID, key,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, mapRecoveryOutcome(outcome))
}

func (controller *ExecutionController) List(writer http.ResponseWriter, request *http.Request) {
	after, limit, err := pagination(request, 100)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY", err.Error())
		return
	}
	workflowID, status, err := executionFilters(request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY", err.Error())
		return
	}
	page, err := controller.queries.List(
		request.Context(), requestcontext.CompanyID(request.Context()),
		workflowID, status, after, limit,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapExecutionPage(page))
}

func (controller *ExecutionController) Get(writer http.ResponseWriter, request *http.Request) {
	companyID, executionID := executionScope(request)
	execution, err := controller.queries.Get(
		request.Context(), companyID, executionID,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapExecutionSummary(execution))
}

func (controller *ExecutionController) GetDefinition(
	writer http.ResponseWriter,
	request *http.Request,
) {
	companyID, executionID := executionScope(request)
	snapshot, err := controller.queries.Definition(
		request.Context(), companyID, executionID,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapDefinition(executionID, snapshot))
}

func (controller *ExecutionController) GetNodes(writer http.ResponseWriter, request *http.Request) {
	controller.writeObservation(writer, request, "nodes")
}

func (controller *ExecutionController) GetEvents(writer http.ResponseWriter, request *http.Request) {
	controller.writeObservation(writer, request, "events")
}

func (controller *ExecutionController) GetLogs(writer http.ResponseWriter, request *http.Request) {
	controller.writeObservation(writer, request, "logs")
}

func (controller *ExecutionController) GetErrors(writer http.ResponseWriter, request *http.Request) {
	controller.writeObservation(writer, request, "errors")
}

func (controller *ExecutionController) writeObservation(
	writer http.ResponseWriter,
	request *http.Request,
	rawResource string,
) {
	resource, err := parseObservationResource(rawResource)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_RESOURCE", err.Error())
		return
	}
	after, limit, err := pagination(request, 200)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY", err.Error())
		return
	}
	companyID, executionID := executionScope(request)
	page, err := controller.queries.Observations(
		request.Context(), companyID, executionID, resource, after, limit,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func parseObservationResource(raw string) (model.ObservationResource, error) {
	resource := model.ObservationResource(strings.TrimSpace(raw))
	switch resource {
	case model.ObservationNodes,
		model.ObservationEvents,
		model.ObservationLogs,
		model.ObservationErrors:
		return resource, nil
	default:
		return "", ErrInvalidObservationResource
	}
}

func (controller *ExecutionController) executionCommand(
	writer http.ResponseWriter,
	request *http.Request,
	mode string,
) (ExecutionCommand, bool) {
	key, err := idempotencyKey(request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
		return ExecutionCommand{}, false
	}
	body, err := decodeExecutionRequest(writer, request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST", err.Error())
		return ExecutionCommand{}, false
	}
	return mapExecutionCommand(
		body,
		requestcontext.CompanyID(request.Context()),
		requestcontext.CorrelationID(request.Context()),
		key,
		mode,
	), true
}

func decodeExecutionRequest(writer http.ResponseWriter, request *http.Request) (executionRequest, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, maximumRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var body executionRequest
	if err := decoder.Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return executionRequest{}, fmt.Errorf("request body must not be empty")
		}
		return executionRequest{}, fmt.Errorf("request body must contain valid JSON")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return executionRequest{}, fmt.Errorf("request body must contain one JSON object")
	}
	return body, nil
}

func idempotencyKey(request *http.Request) (string, error) {
	values := request.Header.Values("Idempotency-Key")
	if len(values) != 1 {
		return "", fmt.Errorf("exactly one Idempotency-Key header is required")
	}
	key := strings.TrimSpace(values[0])
	if key == "" || utf8.RuneCountInString(key) > 255 {
		return "", fmt.Errorf("Idempotency-Key header is invalid")
	}
	return key, nil
}

func pagination(request *http.Request, maximum int) (string, int, error) {
	query := request.URL.Query()
	after := query.Get("after")
	raw := query.Get("limit")
	if raw == "" {
		return after, 50, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maximum {
		return "", 0, fmt.Errorf("limit must be between 1 and %d", maximum)
	}
	return after, limit, nil
}

func executionFilters(request *http.Request) (string, model.ExecutionStatus, error) {
	query := request.URL.Query()
	workflowID := query.Get("workflowId")
	if _, exists := query["workflowId"]; exists && strings.TrimSpace(workflowID) == "" {
		return "", "", fmt.Errorf("workflowId must not be blank")
	}
	status := query.Get("status")
	if _, exists := query["status"]; !exists {
		return workflowID, "", nil
	}
	parsed, err := ParseExecutionStatus(status)
	if err != nil {
		return "", "", err
	}
	return workflowID, parsed, nil
}

func executionScope(request *http.Request) (string, string) {
	return requestcontext.CompanyID(request.Context()), chi.URLParam(request, "executionID")
}
