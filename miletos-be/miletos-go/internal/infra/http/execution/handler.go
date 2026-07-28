package executionhttp

import (
	"miletos-go/internal/features/execution"
	executionfeature "miletos-go/internal/features/execution/application"
	"miletos-go/internal/infra/http/transport"
	"net/http"
)

func (handler Handler,
) ExecuteAsync(writer http.ResponseWriter, request *http.Request,
) {
	if !transport.RequireMethod(writer, request, http.MethodPost) {
		return
	}
	if handler.executionApplication == nil {
		writeAPIError(writer,
			request, newAPIError(http.StatusServiceUnavailable,
				errorCodeExecutionUnavailable, "Workflow execution is currently unavailable."),
		)
		return
	}
	asyncApplication, exists :=
		handler.executionApplication.(executionfeature.AsyncExecutionApplication)
	if !exists || !handler.executionApplication.
		AsyncEnabled() {
		writeAPIError(
			writer, request, newAPIError(
				http.StatusServiceUnavailable, errorCodeExecutionUnavailable, "Async workflow execution is currently unavailable.",
			))
		return
	}
	companyID, exists := CompanyIDFromContext(request.Context())
	if !exists {
		writeAPIError(writer, request,
			newAPIError(http.StatusInternalServerError, errorCodeInternalServerError,
				"An unexpected internal error occurred."))
		return
	}
	correlationID, exists := CorrelationIDFromContext(
		request.Context())
	if !exists {
		writeAPIError(writer,
			request, newAPIError(http.StatusInternalServerError,
				errorCodeInternalServerError, "An unexpected internal error occurred."),
		)
		return
	}
	idempotencyKey, apiError :=
		readIdempotencyKey(request)
	if apiError != nil {
		writeAPIError(
			writer, request, apiError,
		)
		return
	}
	var body ExecutionRequestBody
	if apiError := decodeJSONBody(
		writer, request, &body,
	); apiError != nil {
		writeAPIError(
			writer, request, apiError,
		)
		return
	}
	requestFingerprint, apiError :=
		fingerprintAsyncExecutionRequest(body)
	if apiError != nil {
		writeAPIError(
			writer, request, apiError,
		)
		return
	}
	executionRequest, apiError :=
		mapAsyncExecutionRequest(body, companyID,
			correlationID)
	if apiError != nil {
		writeAPIError(writer,
			request, apiError)
		return
	}
	outcome, _, err := asyncApplication.
		ExecuteAsync(request.Context(), executionRequest,
			idempotencyKey, requestFingerprint)
	if err != nil {
		writeAPIError(
			writer, request, mapAsyncExecutionApplicationError(
				err))
		return
	}
	if outcome.Rejected {
		writeAPIError(
			writer, request, newAPIError(
				http.StatusUnprocessableEntity, errorCodeWorkflowValidationFailed, "Workflow execution was rejected by validation.",
				mapExecutionValidationDetails(outcome.ValidationDetails)...))
		return
	}
	if outcome.Mode != execution.
		ExecutionModeAsync.String() {
		writeAPIError(writer, request,
			newAPIError(http.StatusInternalServerError, errorCodeInternalServerError,
				"An unexpected internal error occurred."))
		return
	}
	_ = writeJSON(writer,
		http.StatusAccepted, mapExecutionResponse(outcome))
}

// Handler owns only execution HTTP controller dependencies.
//
// Health, readiness and plugin responsibilities intentionally
// remain outside this feature controller.
type Handler struct {
	executionApplication       executionfeature.ExecutionApplication
	executionQueryApplication  executionfeature.ExecutionQueryApplication
	partialRecoveryApplication executionfeature.PartialRecoveryApplication
}

func NewHandler(
	executionApplication executionfeature.ExecutionApplication,
	executionQueryApplication executionfeature.ExecutionQueryApplication,
	partialRecoveryApplication executionfeature.PartialRecoveryApplication,
) Handler {
	return Handler{
		executionApplication:       executionApplication,
		executionQueryApplication:  executionQueryApplication,
		partialRecoveryApplication: partialRecoveryApplication,
	}
}

func (handler Handler) RecoverExecution(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if !transport.RequireMethod(writer, request, http.MethodPost) {
		return
	}
	if request.URL.RawQuery != "" || request.ContentLength != 0 ||
		len(request.TransferEncoding) > 0 {
		writeAPIError(writer, request, newAPIError(
			http.StatusBadRequest, errorCodeInvalidExecutionRequest,
			"Partial recovery does not accept query parameters or a request body."))
		return
	}
	if handler.partialRecoveryApplication == nil {
		writeAPIError(writer, request, newAPIError(
			http.StatusServiceUnavailable, errorCodeRecoveryUnavailable,
			"Partial recovery is currently unavailable."))
		return
	}
	companyID, exists := CompanyIDFromContext(request.Context())
	if !exists {
		writeAPIError(writer, request, newAPIError(
			http.StatusInternalServerError, errorCodeInternalServerError,
			"An unexpected internal error occurred."))
		return
	}
	sourceExecutionID, apiError := executionIDFromSubresourcePath(
		request.URL.Path, executionRecoverySubresource)
	if apiError != nil {
		writeAPIError(writer, request, apiError)
		return
	}
	idempotencyKey, apiError := readIdempotencyKey(request)
	if apiError != nil {
		writeAPIError(writer, request, apiError)
		return
	}
	outcome, replayed, err := handler.partialRecoveryApplication.Recover(
		request.Context(), companyID, sourceExecutionID, idempotencyKey,
		fingerprintPartialRecoveryRequest(companyID, sourceExecutionID))
	if err != nil {
		writeAPIError(writer, request, mapPartialRecoveryApplicationError(err))
		return
	}
	_ = writeJSON(writer, http.StatusAccepted, PartialRecoveryResponse{
		SourceExecutionID:   outcome.SourceExecutionID.String(),
		RecoveryExecutionID: outcome.RecoveryExecutionID.String(),
		Status:              outcome.Status.String(),
		PreservedNodeCount:  outcome.PreservedNodeCount,
		ScheduledNodeCount:  outcome.ScheduledNodeCount,
		ResetNodeCount:      outcome.ResetNodeCount,
		CreatedAt:           outcome.CreatedAt.UTC().Format(timeRFC3339Nano),
		Replayed:            replayed,
	})
}

func (handler Handler,
) NotFound(writer http.ResponseWriter, request *http.Request,
) {
	writeAPIError(writer,
		request, newAPIError(http.StatusNotFound,
			errorCodeNotFound, "The requested resource was not found."),
	)
}

func (handler Handler) GetExecutionDefinition(
	writer http.ResponseWriter, request *http.Request) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	if request.URL.RawQuery != "" {
		writeAPIError(writer,
			request, newAPIError(http.StatusBadRequest,
				errorCodeInvalidExecutionQuery, "Execution definition does not accept query parameters."),
		)
		return
	}
	if handler.executionQueryApplication == nil {
		writeAPIError(writer, request,
			newAPIError(http.StatusServiceUnavailable, errorCodeExecutionQueryUnavailable,
				"Execution query service is currently unavailable."))
		return
	}
	companyID, exists := CompanyIDFromContext(
		request.Context())
	if !exists {
		writeAPIError(writer,
			request, newAPIError(http.StatusInternalServerError,
				errorCodeInternalServerError, "An unexpected internal error occurred."),
		)
		return
	}
	workflowExecutionID, apiError :=
		executionIDFromSubresourcePath(request.URL.Path, executionDefinitionSubresource)
	if apiError != nil {
		writeAPIError(writer, request,
			apiError)
		return
	}
	snapshot, err := handler.executionQueryApplication.
		GetExecutionDefinition(request.Context(), companyID,
			workflowExecutionID)
	if err != nil {
		writeAPIError(writer,
			request, mapExecutionQueryError(err))
		return
	}
	_ = writeJSON(writer, http.StatusOK,
		mapExecutionDefinitionResponse(workflowExecutionID, snapshot))
}

func (handler Handler,
) GetExecutionErrors(writer http.ResponseWriter, request *http.Request,
) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	if handler.executionQueryApplication == nil {
		writeAPIError(writer, request,
			newAPIError(http.StatusServiceUnavailable, errorCodeExecutionQueryUnavailable,
				"Execution query service is currently unavailable."))
		return
	}
	companyID, exists := CompanyIDFromContext(
		request.Context())
	if !exists {
		writeAPIError(writer,
			request, newAPIError(http.StatusInternalServerError,
				errorCodeInternalServerError, "An unexpected internal error occurred."),
		)
		return
	}
	workflowExecutionID, apiError :=
		executionIDFromSubresourcePath(request.URL.Path, executionErrorsSubresource)
	if apiError != nil {
		writeAPIError(writer, request,
			apiError)
		return
	}
	pageRequest, apiError := parseExecutionErrorsQuery(request)
	if apiError != nil {
		writeAPIError(writer, request,
			apiError)
		return
	}
	page, err := handler.executionQueryApplication.
		ListExecutionErrors(request.Context(), companyID,
			workflowExecutionID, pageRequest)
	if err != nil {
		writeAPIError(
			writer, request, mapExecutionQueryError(
				err))
		return
	}
	_ = writeJSON(writer,
		http.StatusOK, mapExecutionErrorPageResponse(page))
}

func (handler Handler) GetExecutionEvents(
	writer http.ResponseWriter, request *http.Request) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	if handler.executionQueryApplication == nil {
		writeAPIError(writer,
			request, newAPIError(http.StatusServiceUnavailable,
				errorCodeExecutionQueryUnavailable, "Execution query service is currently unavailable."),
		)
		return
	}
	companyID, exists :=
		CompanyIDFromContext(request.Context())
	if !exists {
		writeAPIError(
			writer, request, newAPIError(
				http.StatusInternalServerError, errorCodeInternalServerError, "An unexpected internal error occurred.",
			))
		return
	}
	workflowExecutionID, apiError := executionIDFromSubresourcePath(request.URL.Path,
		executionEventsSubresource)
	if apiError != nil {
		writeAPIError(writer,
			request, apiError)
		return
	}
	pageRequest, apiError := parseExecutionEventsQuery(
		request)
	if apiError != nil {
		writeAPIError(writer,
			request, apiError)
		return
	}
	page, err := handler.executionQueryApplication.ListExecutionEvents(request.Context(),
		companyID, workflowExecutionID, pageRequest,
	)
	if err != nil {
		writeAPIError(writer, request,
			mapExecutionQueryError(err),
		)
		return
	}
	_ = writeJSON(
		writer, http.StatusOK, mapExecutionEventPageResponse(
			page))
}

func (handler Handler) GetExecutionLogs(
	writer http.ResponseWriter, request *http.Request) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	if handler.executionQueryApplication == nil {
		writeAPIError(
			writer, request, newAPIError(
				http.StatusServiceUnavailable, errorCodeExecutionQueryUnavailable, "Execution query service is currently unavailable.",
			))
		return
	}
	companyID, exists := CompanyIDFromContext(request.Context())
	if !exists {
		writeAPIError(writer, request,
			newAPIError(http.StatusInternalServerError, errorCodeInternalServerError,
				"An unexpected internal error occurred."))
		return
	}
	workflowExecutionID, apiError := executionIDFromSubresourcePath(
		request.URL.Path, executionLogsSubresource)
	if apiError != nil {
		writeAPIError(
			writer, request, apiError,
		)
		return
	}
	pageRequest, apiError :=
		parseExecutionLogsQuery(request)
	if apiError != nil {
		writeAPIError(
			writer, request, apiError,
		)
		return
	}
	page, err :=
		handler.executionQueryApplication.ListExecutionLogs(
			request.Context(), companyID, workflowExecutionID,
			pageRequest)
	if err != nil {
		writeAPIError(writer,
			request, mapExecutionQueryError(err))
		return
	}
	_ = writeJSON(writer, http.StatusOK,
		mapExecutionLogPageResponse(page),
	)
}

func (handler Handler) GetExecutionNodes(
	writer http.ResponseWriter, request *http.Request) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	if handler.executionQueryApplication == nil {
		writeAPIError(writer,
			request, newAPIError(http.StatusServiceUnavailable,
				errorCodeExecutionQueryUnavailable, "Execution query service is currently unavailable."),
		)
		return
	}
	companyID, exists :=
		CompanyIDFromContext(request.Context())
	if !exists {
		writeAPIError(
			writer, request, newAPIError(
				http.StatusInternalServerError, errorCodeInternalServerError, "An unexpected internal error occurred.",
			))
		return
	}
	workflowExecutionID, apiError := executionIDFromSubresourcePath(request.URL.Path,
		executionNodesSubresource)
	if apiError != nil {
		writeAPIError(writer,
			request, apiError)
		return
	}
	pageRequest, apiError := parseExecutionNodesQuery(
		request)
	if apiError != nil {
		writeAPIError(writer,
			request, apiError)
		return
	}
	page, err := handler.executionQueryApplication.ListExecutionNodes(request.Context(),
		companyID, workflowExecutionID, pageRequest,
	)
	if err != nil {
		writeAPIError(writer, request,
			mapExecutionQueryError(err),
		)
		return
	}
	_ = writeJSON(
		writer, http.StatusOK, mapNodeExecutionPageResponse(
			page))
}

func (handler Handler) ListExecutions(
	writer http.ResponseWriter, request *http.Request) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	if handler.executionQueryApplication == nil {
		writeAPIError(
			writer, request, newAPIError(
				http.StatusServiceUnavailable, errorCodeExecutionQueryUnavailable, "Execution query service is currently unavailable.",
			))
		return
	}
	companyID, exists := CompanyIDFromContext(request.Context())
	if !exists {
		writeAPIError(writer, request,
			newAPIError(http.StatusInternalServerError, errorCodeInternalServerError,
				"An unexpected internal error occurred."))
		return
	}
	filter, pageRequest, apiError := parseExecutionListQuery(
		request)
	if apiError != nil {
		writeAPIError(writer,
			request, apiError)
		return
	}
	page, err := handler.
		executionQueryApplication.ListExecutions(request.Context(),
		companyID, filter, pageRequest,
	)
	if err != nil {
		writeAPIError(writer, request,
			mapExecutionQueryError(err),
		)
		return
	}
	_ = writeJSON(
		writer, http.StatusOK, mapExecutionPageResponse(
			page))
}

func (
	handler Handler) GetExecution(writer http.ResponseWriter,
	request *http.Request) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	if handler.executionQueryApplication == nil {
		writeAPIError(writer,
			request, newAPIError(http.StatusServiceUnavailable,
				errorCodeExecutionQueryUnavailable, "Execution query service is currently unavailable."),
		)
		return
	}
	companyID, exists :=
		CompanyIDFromContext(request.Context())
	if !exists {
		writeAPIError(
			writer, request, newAPIError(
				http.StatusInternalServerError, errorCodeInternalServerError, "An unexpected internal error occurred.",
			))
		return
	}
	workflowExecutionID, apiError := executionIDFromRequestPath(request.URL.Path)
	if apiError != nil {
		writeAPIError(writer, request,
			apiError)
		return
	}
	record, err := handler.executionQueryApplication.
		GetExecution(request.Context(), companyID,
			workflowExecutionID)
	if err != nil {
		writeAPIError(writer,
			request, mapExecutionQueryError(err))
		return
	}
	_ = writeJSON(writer, http.StatusOK,
		mapExecutionSummaryResponse(record),
	)
}

func (handler Handler,
) ExecuteSync(writer http.ResponseWriter, request *http.Request,
) {
	if !transport.RequireMethod(writer, request, http.MethodPost) {
		return
	}
	if handler.executionApplication == nil {
		writeAPIError(writer,
			request, newAPIError(http.StatusServiceUnavailable,
				errorCodeExecutionUnavailable, "Workflow execution is currently unavailable."),
		)
		return
	}
	companyID, exists :=
		CompanyIDFromContext(request.Context())
	if !exists {
		writeAPIError(
			writer, request, newAPIError(
				http.StatusInternalServerError, errorCodeInternalServerError, "An unexpected internal error occurred.",
			))
		return
	}
	correlationID, exists := CorrelationIDFromContext(request.Context())
	if !exists {
		writeAPIError(writer, request,
			newAPIError(http.StatusInternalServerError, errorCodeInternalServerError,
				"An unexpected internal error occurred."))
		return
	}
	var body ExecutionRequestBody
	if apiError := decodeJSONBody(writer,
		request, &body); apiError != nil {
		writeAPIError(writer,
			request, apiError)
		return
	}
	executionRequest, apiError := mapSyncExecutionRequest(
		body, companyID, correlationID,
	)
	if apiError != nil {
		writeAPIError(writer, request,
			apiError)
		return
	}
	outcome, err := handler.executionApplication.
		Execute(request.Context(), executionRequest)
	if err != nil {
		writeAPIError(writer, request,
			mapExecutionApplicationError(err),
		)
		return
	}
	if outcome.Rejected {
		writeAPIError(writer, request,
			newAPIError(http.StatusUnprocessableEntity, errorCodeWorkflowValidationFailed,
				"Workflow execution was rejected by validation.", mapExecutionValidationDetails(outcome.ValidationDetails)...),
		)
		return
	}
	if outcome.Mode !=
		execution.ExecutionModeSync.String() ||
		!isSyncTerminalStatus(outcome.Status) {
		writeAPIError(writer,
			request, newAPIError(http.StatusInternalServerError,
				errorCodeInternalServerError, "An unexpected internal error occurred."),
		)
		return
	}
	_ = writeJSON(
		writer, http.StatusOK, mapExecutionResponse(
			outcome))
}
