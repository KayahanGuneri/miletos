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

	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/workflow"
	"miletos-go/internal/shared/requestcontext"

	"github.com/go-chi/chi/v5"
)

const maximumRequestBody = 1 << 20

type ExecutionController struct {
	service    *ExecutionService
	recovery   *RecoveryService
	workflows  *workflow.WorkflowRepository
	executions *repository.ExecutionRepository
}

type preparedExecutionRequest struct {
	definition       workflow.Workflow
	initialVariables map[string]any
	correlationID    string
	idempotencyKey   string
	fingerprint      string
}

func NewExecutionController(
	service *ExecutionService,
	recovery *RecoveryService,
	workflows *workflow.WorkflowRepository,
	executions *repository.ExecutionRepository,
) *ExecutionController {
	return &ExecutionController{
		service: service, recovery: recovery,
		workflows: workflows, executions: executions,
	}
}

func (controller *ExecutionController) ExecuteSync(writer http.ResponseWriter, request *http.Request) {
	prepared, ok := prepareExecutionRequest(writer, request, "SYNC")
	if !ok {
		return
	}
	outcome, err := controller.service.ExecuteSync(
		request.Context(), prepared.definition, prepared.initialVariables,
		prepared.correlationID, prepared.idempotencyKey, prepared.fingerprint,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapExecutionOutcome(outcome))
}

func (controller *ExecutionController) ExecuteAsync(writer http.ResponseWriter, request *http.Request) {
	prepared, ok := prepareExecutionRequest(writer, request, "ASYNC")
	if !ok {
		return
	}
	outcome, err := controller.service.ExecuteAsync(
		request.Context(), prepared.definition, prepared.initialVariables,
		prepared.correlationID, prepared.idempotencyKey, prepared.fingerprint,
	)
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
		fingerprint(map[string]string{"companyId": companyID, "executionId": executionID}),
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
	page, err := controller.executions.List(
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
	execution, err := controller.executions.FindByID(
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
	snapshot, err := controller.workflows.FindByExecutionID(
		request.Context(), companyID, executionID,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapDefinition(executionID, snapshot))
}

func (controller *ExecutionController) GetNodes(writer http.ResponseWriter, request *http.Request) {
	after, limit, err := pagination(request, 200)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY", err.Error())
		return
	}
	companyID, executionID := executionScope(request)
	if _, err := controller.executions.FindByID(request.Context(), companyID, executionID); err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	page, err := controller.executions.ListNodes(
		request.Context(), companyID, executionID, after, limit,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func (controller *ExecutionController) GetEvents(writer http.ResponseWriter, request *http.Request) {
	controller.writeTimeline(writer, request, "events")
}

func (controller *ExecutionController) GetLogs(writer http.ResponseWriter, request *http.Request) {
	controller.writeTimeline(writer, request, "logs")
}

func (controller *ExecutionController) GetErrors(writer http.ResponseWriter, request *http.Request) {
	controller.writeTimeline(writer, request, "errors")
}

func (controller *ExecutionController) writeTimeline(
	writer http.ResponseWriter,
	request *http.Request,
	resource string,
) {
	after, limit, err := pagination(request, 200)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY", err.Error())
		return
	}
	companyID, executionID := executionScope(request)
	if _, err := controller.executions.FindByID(request.Context(), companyID, executionID); err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	var page any
	switch resource {
	case "events":
		page, err = controller.executions.ListEvents(request.Context(), companyID, executionID, after, limit)
	case "logs":
		page, err = controller.executions.ListLogs(request.Context(), companyID, executionID, after, limit)
	case "errors":
		page, err = controller.executions.ListErrors(request.Context(), companyID, executionID, after, limit)
	}
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, page)
}

func prepareExecutionRequest(
	writer http.ResponseWriter,
	request *http.Request,
	mode string,
) (preparedExecutionRequest, bool) {
	key, err := idempotencyKey(request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
		return preparedExecutionRequest{}, false
	}
	body, err := decodeExecutionRequest(writer, request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST", err.Error())
		return preparedExecutionRequest{}, false
	}
	return preparedExecutionRequest{
		definition: mapWorkflowRequest(
			body.Definition,
			requestcontext.CompanyID(request.Context()),
		),
		initialVariables: body.InitialVariables,
		correlationID:    requestcontext.CorrelationID(request.Context()),
		idempotencyKey:   key,
		fingerprint:      fingerprint(map[string]any{"request": body, "mode": mode}),
	}, true
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

func executionFilters(request *http.Request) (string, string, error) {
	query := request.URL.Query()
	workflowID := query.Get("workflowId")
	if _, exists := query["workflowId"]; exists && strings.TrimSpace(workflowID) == "" {
		return "", "", fmt.Errorf("workflowId must not be blank")
	}
	status := query.Get("status")
	if _, exists := query["status"]; !exists {
		return workflowID, "", nil
	}
	switch status {
	case "CREATED", "VALIDATING", "REJECTED",
		"QUEUED", "RUNNING", "SUCCEEDED",
		"FAILED", "CANCELLED", "TIMED_OUT":
		return workflowID, status, nil
	default:
		return "", "", fmt.Errorf("execution status filter is invalid")
	}
}

func executionScope(request *http.Request) (string, string) {
	return requestcontext.CompanyID(request.Context()), chi.URLParam(request, "executionID")
}
