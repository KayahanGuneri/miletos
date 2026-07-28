package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"miletos-go/internal/features/workflow"
	"mime"
	"net/http"
	"strings"
	"time"
)

const (
	HeaderRequestID     = "X-Request-ID"
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderCompanyID     = "X-Miletos-Company-ID"
)

type contextKey uint8

const (
	contextKeyRequestID contextKey = iota + 1
	contextKeyCorrelationID
	contextKeyInternalAuthenticated
	contextKeyCompanyID
)

func WithRequestID(ctx context.Context, requestID string,
) context.Context {
	return context.WithValue(ctx,
		contextKeyRequestID, requestID)
}
func RequestIDFromContext(
	ctx context.Context) (string,
	bool) {
	if ctx == nil {
		return "", false
	}
	value, exists := ctx.Value(contextKeyRequestID).(string)
	return value,
		exists && value != ""
}
func WithCorrelationID(ctx context.Context, correlationID string,
) context.Context {
	return context.WithValue(ctx,
		contextKeyCorrelationID, correlationID)
}
func CorrelationIDFromContext(
	ctx context.Context) (string,
	bool) {
	if ctx == nil {
		return "", false
	}
	value, exists := ctx.Value(contextKeyCorrelationID).(string)
	return value,
		exists && value != ""
}
func WithInternalAuthentication(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyInternalAuthenticated,
		true)
}
func IsInternalAuthenticated(ctx context.Context,
) bool {
	if ctx == nil {
		return false
	}
	value, exists :=
		ctx.Value(contextKeyInternalAuthenticated).(bool)
	return exists && value
}
func WithCompanyID(ctx context.Context,
	companyID workflow.CompanyID) context.Context {
	return context.WithValue(
		ctx, contextKeyCompanyID, companyID,
	)
}
func CompanyIDFromContext(ctx context.Context) (
	workflow.CompanyID, bool) {
	if ctx == nil {
		return "", false
	}
	value, exists := ctx.Value(
		contextKeyCompanyID).(workflow.CompanyID)
	if !exists || value.String() == "" {
		return "", false
	}
	return value, true
}

const (
	ErrorCodeBadRequest            = "BAD_REQUEST"
	ErrorCodeMethodNotAllowed      = "METHOD_NOT_ALLOWED"
	ErrorCodeNotFound              = "NOT_FOUND"
	ErrorCodeUnauthorized          = "UNAUTHORIZED"
	ErrorCodeInternalServerError   = "INTERNAL_SERVER_ERROR"
	ErrorCodeServiceUnavailable    = "SERVICE_UNAVAILABLE"
	ErrorCodeUnsupportedMediaType  = "UNSUPPORTED_MEDIA_TYPE"
	ErrorCodeInvalidJSON           = "INVALID_JSON"
	ErrorCodeEmptyRequestBody      = "EMPTY_REQUEST_BODY"
	ErrorCodePayloadTooLarge       = "PAYLOAD_TOO_LARGE"
	ErrorCodeMissingCompanyContext = "MISSING_COMPANY_CONTEXT"
	ErrorCodeInvalidCompanyContext = "INVALID_COMPANY_CONTEXT"
	ErrorCodeRequestTimeout        = "REQUEST_TIMEOUT"
)

// ErrorDetail contains transport-generic validation metadata only.
type ErrorDetail struct {
	Code     string `json:"code,omitempty"`
	Field    string `json:"field,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
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
func NewAPIError(status int,
	code string, message string, details ...ErrorDetail,
) *APIError {
	return &APIError{Status: status,
		Code: code, Message: message, Details: append([]ErrorDetail(nil), details...),
	}
}
func WriteAPIError(writer http.ResponseWriter, requestID string,
	correlationID string, apiError *APIError) {
	if apiError == nil {
		apiError = NewAPIError(http.StatusInternalServerError,
			ErrorCodeInternalServerError, "An unexpected internal error occurred.")
	}
	status := apiError.Status
	if status < 100 || status > 599 {
		status = http.StatusInternalServerError
	}
	code := apiError.Code
	if code == "" {
		code = ErrorCodeInternalServerError
	}
	message := apiError.Message
	if message == "" {
		message = http.StatusText(status)
	}
	response := APIErrorResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Status: status, Code: code,
		Message: message, RequestID: requestID, CorrelationID: correlationID,
		Details: append([]ErrorDetail(nil), apiError.Details...)}
	_ = WriteJSON(writer, status, response)
}
func DecodeJSONBody(writer http.ResponseWriter, request *http.Request,
	destination any) *APIError {
	if destination == nil {
		return NewAPIError(http.StatusInternalServerError, ErrorCodeInternalServerError,
			"JSON request destination is unavailable.")
	}
	if request == nil || request.Body == nil ||
		request.Body == http.NoBody {
		return NewAPIError(
			http.StatusBadRequest, ErrorCodeEmptyRequestBody, "Request body must contain exactly one JSON document.",
		)
	}
	if apiError := validateJSONContentType(request.Header.Get("Content-Type")); apiError != nil {
		return apiError
	}
	decoder := json.NewDecoder(
		request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(
		destination); err != nil {
		return mapJSONDecodeError(
			err)
	}
	var extra any
	err := decoder.Decode(&extra)
	if err == nil {
		return NewAPIError(
			http.StatusBadRequest, ErrorCodeInvalidJSON, "Request body must contain exactly one JSON document.",
		)
	}
	if err != io.EOF {
		return mapJSONDecodeError(err)
	}
	_ = writer
	return nil
}
func validateJSONContentType(
	value string) *APIError {
	value = strings.TrimSpace(
		value)
	if value == "" {
		return NewAPIError(http.StatusUnsupportedMediaType,
			ErrorCodeUnsupportedMediaType, "Content-Type must be application/json.")
	}
	mediaType, _, err := mime.ParseMediaType(
		value)
	if err != nil || !strings.EqualFold(mediaType,
		"application/json") {
		return NewAPIError(http.StatusUnsupportedMediaType, ErrorCodeUnsupportedMediaType,
			"Content-Type must be application/json.")
	}
	return nil
}
func mapJSONDecodeError(err error,
) *APIError {
	if err == nil {
		return nil
	}
	var maxBytesError *http.MaxBytesError
	if errors.As(err,
		&maxBytesError) {
		return NewAPIError(
			http.StatusRequestEntityTooLarge, ErrorCodePayloadTooLarge, "Request body exceeds the configured size limit.",
		)
	}
	if errors.Is(err, io.EOF) {
		return NewAPIError(http.StatusBadRequest,
			ErrorCodeEmptyRequestBody, "Request body must contain exactly one JSON document.")
	}
	var syntaxError *json.SyntaxError
	if errors.As(err,
		&syntaxError) {
		return NewAPIError(
			http.StatusBadRequest, ErrorCodeInvalidJSON, "Request body contains malformed JSON.",
		)
	}
	var typeError *json.UnmarshalTypeError
	if errors.As(
		err, &typeError) {
		detail := ErrorDetail{Field: typeError.Field, Reason: "contains an incompatible JSON value"}
		return NewAPIError(
			http.StatusBadRequest, ErrorCodeInvalidJSON, "Request body contains an invalid JSON value.",
			detail)
	}
	if field, exists := unknownJSONField(err); exists {
		return NewAPIError(http.StatusBadRequest,
			ErrorCodeInvalidJSON, "Request body contains an unknown field.", ErrorDetail{
				Field: field, Reason: "field is not supported by this API contract"},
		)
	}
	return NewAPIError(http.StatusBadRequest, ErrorCodeInvalidJSON,
		"Request body contains invalid JSON.")
}
func unknownJSONField(err error,
) (string, bool,
) {
	if err == nil {
		return "", false
	}
	const prefix = `json: unknown field "`
	message := err.Error()
	if !strings.HasPrefix(message, prefix) || !strings.HasSuffix(message,
		`"`) {
		return "", false
	}
	field := strings.TrimSuffix(strings.TrimPrefix(message,
		prefix), `"`,
	)
	field = strings.TrimSpace(
		field)
	if field == "" {
		return "", false
	}
	return field, true
}

// WriteRequestAPIError writes an API error while resolving request and
// correlation identifiers from the shared HTTP request context.
func WriteRequestAPIError(writer http.ResponseWriter, request *http.Request,
	apiError *APIError) {
	requestID := ""
	correlationID := ""
	if request != nil {
		if value, exists := RequestIDFromContext(request.Context()); exists {
			requestID =
				value
		}
		if value, exists := CorrelationIDFromContext(request.Context()); exists {
			correlationID =
				value
		}
	}
	if writer != nil && requestID == "" {
		requestID = writer.
			Header().Get(HeaderRequestID)
	}
	if writer != nil && correlationID == "" {
		correlationID = writer.Header().
			Get(HeaderCorrelationID)
	}
	WriteAPIError(
		writer, requestID, correlationID,
		apiError)
}

// WriteJSON writes a JSON response using the supplied HTTP status code.
func WriteJSON(
	writer http.ResponseWriter, statusCode int, payload any,
) error {
	writer.Header().Set("Content-Type",
		"application/json")
	writer.WriteHeader(statusCode)
	return json.NewEncoder(writer).Encode(payload)
}

func RequireMethod(
	writer http.ResponseWriter,
	request *http.Request,
	allowed string,
) bool {
	if request.Method == allowed {
		return true
	}
	writer.Header().Set("Allow", allowed)
	WriteRequestAPIError(
		writer,
		request,
		NewAPIError(
			http.StatusMethodNotAllowed,
			ErrorCodeMethodNotAllowed,
			"The requested HTTP method is not allowed for this resource.",
		),
	)
	return false
}
