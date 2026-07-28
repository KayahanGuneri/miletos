package executionhttp

import (
	"context"
	"errors"
	"fmt"
	"miletos-go/internal/engine"
	"miletos-go/internal/features/execution"
	executionfeature "miletos-go/internal/features/execution/application"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/infra/http/transport"
	repository "miletos-go/internal/ports/persistence"
	"net/http"
	"time"
)

func mapAsyncExecutionApplicationError(err error,
) *APIError {
	switch {
	case errors.Is(
		err, context.DeadlineExceeded):
		return newAPIError(http.StatusGatewayTimeout, errorCodeRequestTimeout,
			"Workflow execution exceeded the allowed request deadline.")
	case errors.Is(err, engine.ErrAsyncExecutionUnavailable):
		return newAPIError(http.StatusServiceUnavailable,
			errorCodeExecutionUnavailable, "Requested execution capability is currently unavailable.")
	case errors.Is(err,
		executionfeature.ErrIdempotencyKeyReused):
		return newAPIError(
			http.StatusConflict, errorCodeIdempotencyKeyReused, "Idempotency-Key was already used for a different request.",
		)
	case errors.Is(
		err, executionfeature.ErrIdempotencyRequestInProgress):
		return newAPIError(http.StatusConflict, errorCodeIdempotencyRequestInProgress,
			"The idempotent request has not completed yet.")
	default:
		return newAPIError(http.StatusInternalServerError,
			errorCodeInternalServerError, "An unexpected internal error occurred.")
	}
}

func mapPartialRecoveryApplicationError(err error) *APIError {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return newAPIError(
			http.StatusGatewayTimeout, errorCodeRequestTimeout,
			"Partial recovery exceeded the allowed request deadline.")
	case repository.IsNotFound(err):
		return newAPIError(
			http.StatusNotFound, errorCodeNotFound,
			"The source execution was not found.")
	case errors.Is(err, engine.ErrPartialRecoveryUnavailable):
		return newAPIError(
			http.StatusServiceUnavailable, errorCodeRecoveryUnavailable,
			"Partial recovery is currently unavailable.")
	case errors.Is(err, engine.ErrPartialRecoveryUnsupported):
		return newAPIError(
			http.StatusConflict, errorCodeRecoveryNotSupported,
			"The source execution is not eligible for partial recovery.")
	case errors.Is(err, engine.ErrPartialRecoveryUnsafe):
		return newAPIError(
			http.StatusConflict, errorCodeUnsafeRecovery,
			"The persisted execution state cannot be recovered without violating workflow invariants.")
	case errors.Is(err, engine.ErrPartialRecoveryKeyReused):
		return newAPIError(
			http.StatusConflict, errorCodeIdempotencyKeyReused,
			"Idempotency-Key was already used for a different recovery request.")
	default:
		return newAPIError(
			http.StatusInternalServerError, errorCodeInternalServerError,
			"An unexpected internal error occurred.")
	}
}

func mapExecutionQueryError(err error,
) *APIError {
	if err == nil {
		return newAPIError(
			http.StatusInternalServerError, errorCodeInternalServerError, "An unexpected internal error occurred.",
		)
	}
	if repository.IsNotFound(err) {
		return newAPIError(http.StatusNotFound, errorCodeNotFound,
			"The requested execution was not found.")
	}
	var validationError *repository.ValidationError
	if errors.As(err, &validationError) {
		return newAPIError(http.StatusBadRequest,
			errorCodeInvalidExecutionQuery, "Execution query parameters are invalid.")
	}
	return newAPIError(
		http.StatusInternalServerError, errorCodeInternalServerError, "An unexpected internal error occurred.",
	)
}

func mapExecutionApplicationError(
	err error) *APIError {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return newAPIError(http.StatusGatewayTimeout,
			errorCodeRequestTimeout, "Workflow execution exceeded the allowed request deadline.")
	case errors.Is(err,
		engine.ErrAsyncExecutionUnavailable):
		return newAPIError(
			http.StatusServiceUnavailable, errorCodeExecutionUnavailable, "Requested execution capability is currently unavailable.",
		)
	default:
		return newAPIError(http.StatusInternalServerError, errorCodeInternalServerError,
			"An unexpected internal error occurred.")
	}
}

func isSyncTerminalStatus(
	status string) bool {
	switch status {
	case execution.WorkflowExecutionStatusSucceeded.String(),
		execution.WorkflowExecutionStatusFailed.
			String(), execution.
			WorkflowExecutionStatusCancelled.String(),
		execution.WorkflowExecutionStatusTimedOut.String():
		return true
	default:
		return false
	}
}

const (
	HeaderRequestID     = transport.HeaderRequestID
	HeaderCorrelationID = transport.HeaderCorrelationID
	HeaderCompanyID     = transport.HeaderCompanyID
)

const (
	errorCodeBadRequest                   = transport.ErrorCodeBadRequest
	errorCodeMethodNotAllowed             = transport.ErrorCodeMethodNotAllowed
	errorCodeNotFound                     = transport.ErrorCodeNotFound
	errorCodeUnauthorized                 = transport.ErrorCodeUnauthorized
	errorCodeInternalServerError          = transport.ErrorCodeInternalServerError
	errorCodeServiceUnavailable           = transport.ErrorCodeServiceUnavailable
	errorCodeUnsupportedMediaType         = transport.ErrorCodeUnsupportedMediaType
	errorCodeInvalidJSON                  = transport.ErrorCodeInvalidJSON
	errorCodeEmptyRequestBody             = transport.ErrorCodeEmptyRequestBody
	errorCodePayloadTooLarge              = transport.ErrorCodePayloadTooLarge
	errorCodeMissingCompanyContext        = transport.ErrorCodeMissingCompanyContext
	errorCodeInvalidCompanyContext        = transport.ErrorCodeInvalidCompanyContext
	errorCodeRequestTimeout               = transport.ErrorCodeRequestTimeout
	errorCodeInvalidExecutionRequest      = "INVALID_EXECUTION_REQUEST"
	errorCodeWorkflowValidationFailed     = "WORKFLOW_VALIDATION_FAILED"
	errorCodeExecutionUnavailable         = "EXECUTION_UNAVAILABLE"
	errorCodeInvalidIdempotencyKey        = "INVALID_IDEMPOTENCY_KEY"
	errorCodeIdempotencyKeyReused         = "IDEMPOTENCY_KEY_REUSED"
	errorCodeIdempotencyRequestInProgress = "IDEMPOTENCY_REQUEST_IN_PROGRESS"
	errorCodeExecutionQueryUnavailable    = "EXECUTION_QUERY_UNAVAILABLE"
	errorCodeInvalidExecutionQuery        = "INVALID_EXECUTION_QUERY"
	errorCodeInvalidExecutionID           = "INVALID_EXECUTION_ID"
	errorCodeRecoveryUnavailable          = "RECOVERY_UNAVAILABLE"
	errorCodeRecoveryNotSupported         = "RECOVERY_NOT_SUPPORTED"
	errorCodeUnsafeRecovery               = "UNSAFE_RECOVERY"
)

type ErrorDetail struct {
	Code          string   `json:"code,omitempty"`
	Field         string   `json:"field,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	NodeID        string   `json:"nodeId,omitempty"`
	EdgeID        string   `json:"edgeId,omitempty"`
	PluginType    string   `json:"pluginType,omitempty"`
	PluginVersion string   `json:"pluginVersion,omitempty"`
	Expected      string   `json:"expected,omitempty"`
	Actual        string   `json:"actual,omitempty"`
	CyclePath     []string `json:"cyclePath,omitempty"`
}

type APIErrorResponse struct {
	Timestamp     string        `json:"timestamp"`
	Status        int           `json:"status"`
	Code          string        `json:"code"`
	Message       string        `json:"message"`
	RequestID     string        `json:"requestId,omitempty"`
	CorrelationID string        `json:"correlationId,omitempty"`
	Details       []ErrorDetail `json:"details,omitempty"`
}

type APIError struct {
	Status  int
	Code    string
	Message string
	Details []ErrorDetail
}

func (apiError *APIError) Error() string {
	if apiError == nil {
		return "api error"
	}
	if apiError.Code == "" {
		return apiError.Message
	}
	if apiError.Message == "" {
		return apiError.Code
	}
	return fmt.Sprintf("%s: %s", apiError.Code, apiError.Message)
}

func newAPIError(
	status int, code string, message string,
	details ...ErrorDetail) *APIError {
	return &APIError{
		Status: status, Code: code, Message: message,
		Details: append([]ErrorDetail(nil), details...)}
}

func writeAPIError(writer http.ResponseWriter,
	request *http.Request, apiError *APIError) {
	if apiError == nil {
		apiError = newAPIError(http.StatusInternalServerError,
			errorCodeInternalServerError, "An unexpected internal error occurred.")
	}
	status := apiError.Status
	if status < 100 || status > 599 {
		status = http.StatusInternalServerError
	}
	code := apiError.Code
	if code == "" {
		code = errorCodeInternalServerError
	}
	message := apiError.Message
	if message == "" {
		message = http.StatusText(status)
	}
	requestID, _ := RequestIDFromContext(request.Context())
	correlationID, _ := CorrelationIDFromContext(request.Context())
	response := APIErrorResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Status: status, Code: code,
		Message: message, RequestID: requestID, CorrelationID: correlationID,
		Details: append([]ErrorDetail(nil), apiError.Details...)}
	_ = transport.WriteJSON(writer, status, response)
}

func writeJSON(writer http.ResponseWriter, statusCode int,
	payload any) error {
	return transport.WriteJSON(
		writer, statusCode, payload,
	)
}

func decodeJSONBody(writer http.ResponseWriter, request *http.Request,
	destination any) *APIError {
	transportError :=
		transport.DecodeJSONBody(writer, request,
			destination)
	if transportError == nil {
		return nil
	}
	details := make([]ErrorDetail, 0, len(transportError.Details))
	for _, detail := range transportError.Details {
		details = append(details,
			ErrorDetail{Code: detail.Code, Field: detail.Field,
				Reason: detail.Reason, Expected: detail.Expected, Actual: detail.Actual,
			})
	}
	return &APIError{Status: transportError.Status,
		Code: transportError.Code, Message: transportError.Message, Details: details,
	}
}

func RequestIDFromContext(ctx context.Context) (
	string, bool) {
	return transport.RequestIDFromContext(ctx)
}

func CorrelationIDFromContext(ctx context.Context) (
	string, bool) {
	return transport.CorrelationIDFromContext(ctx)
}

func CompanyIDFromContext(ctx context.Context) (
	workflow.CompanyID, bool) {
	return transport.CompanyIDFromContext(ctx)
}

func mapExecutionValidationDetails(details []executionfeature.ValidationDetail,
) []ErrorDetail {
	if len(details) == 0 {
		return nil
	}
	mapped :=
		make([]ErrorDetail, 0,
			len(details))
	for _, detail := range details {
		mapped =
			append(mapped, ErrorDetail{
				Code: detail.Code, Field: detail.Field,
				Reason: detail.Reason,
				NodeID: detail.NodeID, EdgeID: detail.EdgeID,
				PluginType:    detail.PluginType,
				PluginVersion: detail.PluginVersion, Expected: detail.Expected,
				Actual: detail.Actual,
				CyclePath: append([]string(nil), detail.CyclePath...,
				)})
	}
	return mapped
}
