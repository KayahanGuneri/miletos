package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"miletos-go/internal/engine"
	"miletos-go/internal/middleware"
	"miletos-go/internal/repository"
)

const maximumRequestBody = 1 << 20

type ExecutionController struct {
	service    *engine.ExecutionService
	recovery   *engine.RecoveryService
	workflows  *repository.WorkflowRepository
	executions *repository.ExecutionRepository
}

func NewExecutionController(
	service *engine.ExecutionService,
	recovery *engine.RecoveryService,
	workflows *repository.WorkflowRepository,
	executions *repository.ExecutionRepository,
) *ExecutionController {
	return &ExecutionController{
		service: service, recovery: recovery,
		workflows: workflows, executions: executions,
	}
}

func (controller *ExecutionController) ExecuteSync(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST",
			"Workflow execution does not accept query parameters.")
		return
	}
	if !hasJSONContentType(request) {
		WriteAPIError(writer, request, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE",
			"Content-Type must be application/json.")
		return
	}
	body, err := decodeExecutionRequest(writer, request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST", err.Error())
		return
	}
	workflow := mapWorkflowRequest(body.Definition, middleware.CompanyID(request.Context()))
	outcome, err := controller.service.ExecuteSync(
		request.Context(), workflow, body.InitialVariables,
		middleware.CorrelationID(request.Context()),
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapExecutionOutcome(outcome))
}

func (controller *ExecutionController) ExecuteAsync(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST",
			"Workflow execution does not accept query parameters.")
		return
	}
	key, err := idempotencyKey(request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
		return
	}
	if !hasJSONContentType(request) {
		WriteAPIError(writer, request, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE",
			"Content-Type must be application/json.")
		return
	}
	body, err := decodeExecutionRequest(writer, request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST", err.Error())
		return
	}
	workflow := mapWorkflowRequest(body.Definition, middleware.CompanyID(request.Context()))
	outcome, err := controller.service.ExecuteAsync(
		request.Context(), workflow, body.InitialVariables,
		middleware.CorrelationID(request.Context()), key, fingerprint(body),
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, mapExecutionOutcome(outcome))
}

func (controller *ExecutionController) Recover(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" || request.ContentLength != 0 || len(request.TransferEncoding) > 0 {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST",
			"Partial recovery does not accept query parameters or a request body.")
		return
	}
	key, err := idempotencyKey(request)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
		return
	}
	executionID := chi.URLParam(request, "executionID")
	companyID := middleware.CompanyID(request.Context())
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
	limit, err := queryLimit(request, 100, "workflowId", "status")
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
		request.Context(), middleware.CompanyID(request.Context()),
		workflowID, status,
		request.URL.Query().Get("after"), limit,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapExecutionPage(page))
}

func (controller *ExecutionController) Get(writer http.ResponseWriter, request *http.Request) {
	if request.URL.RawQuery != "" {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY",
			"Execution details do not accept query parameters.")
		return
	}
	execution, err := controller.executions.FindByID(
		request.Context(), middleware.CompanyID(request.Context()),
		chi.URLParam(request, "executionID"),
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
	if request.URL.RawQuery != "" {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY",
			"Execution definition does not accept query parameters.")
		return
	}
	executionID := chi.URLParam(request, "executionID")
	snapshot, err := controller.workflows.FindByExecutionID(
		request.Context(), middleware.CompanyID(request.Context()), executionID,
	)
	if err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, mapDefinition(executionID, snapshot))
}

func (controller *ExecutionController) GetNodes(writer http.ResponseWriter, request *http.Request) {
	limit, err := queryLimit(request, 200)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY", err.Error())
		return
	}
	companyID := middleware.CompanyID(request.Context())
	executionID := chi.URLParam(request, "executionID")
	if _, err := controller.executions.FindByID(request.Context(), companyID, executionID); err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	page, err := controller.executions.ListNodes(
		request.Context(), companyID, executionID, request.URL.Query().Get("after"), limit,
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
	limit, err := queryLimit(request, 200)
	if err != nil {
		WriteAPIError(writer, request, http.StatusBadRequest, "INVALID_EXECUTION_QUERY", err.Error())
		return
	}
	companyID := middleware.CompanyID(request.Context())
	executionID := chi.URLParam(request, "executionID")
	if _, err := controller.executions.FindByID(request.Context(), companyID, executionID); err != nil {
		writeExecutionError(writer, request, err)
		return
	}
	after := request.URL.Query().Get("after")
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

func hasJSONContentType(request *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

func queryLimit(request *http.Request, maximum int, filters ...string) (int, error) {
	allowed := map[string]bool{"after": true, "limit": true}
	for _, filter := range filters {
		allowed[filter] = true
	}
	for name, values := range request.URL.Query() {
		if !allowed[name] {
			return 0, fmt.Errorf("query parameter %q is not supported", name)
		}
		if len(values) != 1 {
			return 0, fmt.Errorf("query parameter %q must be supplied once", name)
		}
	}
	raw := request.URL.Query().Get("limit")
	if raw == "" {
		return 50, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maximum {
		return 0, fmt.Errorf("limit must be between 1 and %d", maximum)
	}
	return limit, nil
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
	valid := map[string]bool{
		"CREATED": true, "VALIDATING": true, "REJECTED": true,
		"QUEUED": true, "RUNNING": true, "SUCCEEDED": true,
		"FAILED": true, "CANCELLED": true, "TIMED_OUT": true,
	}
	if !valid[status] {
		return "", "", fmt.Errorf("execution status filter is invalid")
	}
	return workflowID, status, nil
}
