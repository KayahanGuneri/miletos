package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/infra/http/openapi"
	"miletos-go/internal/infra/http/transport"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	HeaderRequestID     = transport.HeaderRequestID
	HeaderCorrelationID = transport.HeaderCorrelationID
	HeaderCompanyID     = transport.HeaderCompanyID
)

func RequestIDFromContext(ctx context.Context,
) (string, bool,
) {
	return transport.RequestIDFromContext(ctx)
}
func CorrelationIDFromContext(ctx context.Context,
) (string, bool,
) {
	return transport.CorrelationIDFromContext(ctx)
}
func isInternalAuthenticated(
	ctx context.Context) bool {
	return transport.IsInternalAuthenticated(
		ctx)
}
func CompanyIDFromContext(
	ctx context.Context) (workflow.CompanyID,
	bool) {
	return transport.CompanyIDFromContext(
		ctx)
}

const (
	errorCodeBadRequest            = transport.ErrorCodeBadRequest
	errorCodeMethodNotAllowed      = transport.ErrorCodeMethodNotAllowed
	errorCodeNotFound              = transport.ErrorCodeNotFound
	errorCodeUnauthorized          = transport.ErrorCodeUnauthorized
	errorCodeInternalServerError   = transport.ErrorCodeInternalServerError
	errorCodeServiceUnavailable    = transport.ErrorCodeServiceUnavailable
	errorCodeUnsupportedMediaType  = transport.ErrorCodeUnsupportedMediaType
	errorCodeInvalidJSON           = transport.ErrorCodeInvalidJSON
	errorCodeEmptyRequestBody      = transport.ErrorCodeEmptyRequestBody
	errorCodePayloadTooLarge       = transport.ErrorCodePayloadTooLarge
	errorCodeMissingCompanyContext = transport.ErrorCodeMissingCompanyContext
	errorCodeInvalidCompanyContext = transport.ErrorCodeInvalidCompanyContext
	errorCodeRequestTimeout        = transport.ErrorCodeRequestTimeout
)

type ErrorDetail = transport.ErrorDetail
type APIErrorResponse = transport.APIErrorResponse
type APIError = transport.APIError

func newAPIError(status int, code string,
	message string, details ...ErrorDetail) *APIError {
	return transport.NewAPIError(status, code,
		message, details...)
}
func writeAPIError(
	writer http.ResponseWriter, request *http.Request, apiError *APIError,
) {
	transport.WriteRequestAPIError(writer,
		request, apiError)
}

const maximumTraceIDLength = 128

var fallbackIDCounter atomic.Uint64

type Middleware func(
	http.Handler) http.Handler

func Chain(handler http.Handler, middlewares ...Middleware,
) http.Handler {
	for index := len(middlewares) - 1; index >= 0; index-- {
		handler = middlewares[index](handler)
	}
	return handler
}
func RecoveryMiddleware(
	logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter,
			request *http.Request) {
			defer func() {
				if recover() == nil {
					return
				}
				logger.Error("http request panic recovered",
					"method", request.Method, "path",
					request.URL.Path, "request_id", writer.Header().
						Get(HeaderRequestID), "correlation_id", writer.Header().
						Get(HeaderCorrelationID))
				writeAPIError(writer, request,
					newAPIError(http.StatusInternalServerError, errorCodeInternalServerError,
						"An unexpected internal error occurred."))
			}()
			next.ServeHTTP(
				writer, request)
		})
	}
}
func RequestIDMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter,
			request *http.Request) {
			requestID := resolveTraceID(
				request.Header.Get(HeaderRequestID),
				"req_")
			writer.Header().Set(HeaderRequestID, requestID)
			ctx := transport.WithRequestID(
				request.Context(), requestID)
			next.ServeHTTP(writer,
				request.WithContext(ctx))
		},
		)
	}
}
func CorrelationIDMiddleware() Middleware {
	return func(
		next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(writer http.ResponseWriter, request *http.Request,
			) {
				correlationID := resolveTraceID(request.Header.Get(
					HeaderCorrelationID), "corr_",
				)
				writer.Header().Set(
					HeaderCorrelationID, correlationID)
				ctx := transport.WithCorrelationID(request.Context(),
					correlationID)
				next.ServeHTTP(writer, request.WithContext(ctx))
			})
	}
}
func TechnicalLoggingMiddleware(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler,
	) http.Handler {
		return http.HandlerFunc(func(
			writer http.ResponseWriter, request *http.Request) {
			startedAt := time.Now()
			recorder := &statusRecorder{
				ResponseWriter: writer}
			next.ServeHTTP(recorder, request)
			statusCode := recorder.statusCode
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
			logger.Info(
				"http request completed", "method", request.Method,
				"path", request.URL.Path, "status",
				statusCode, "duration", time.Since(startedAt),
				"request_id", recorder.Header().Get(HeaderRequestID),
				"correlation_id", recorder.Header().Get(HeaderCorrelationID),
			)
		})
	}
}
func SecurityHeadersMiddleware() Middleware {
	return func(next http.Handler,
	) http.Handler {
		return http.HandlerFunc(func(
			writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("X-Content-Type-Options", "nosniff")
			writer.Header().Set(
				"X-Frame-Options", "DENY")
			writer.Header().Set("Referrer-Policy",
				"no-referrer")
			writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			next.ServeHTTP(
				writer, request)
		})
	}
}
func BodyLimitMiddleware(
	maximumBytes int64) Middleware {
	return func(
		next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(writer http.ResponseWriter, request *http.Request,
			) {
				if maximumBytes <= 0 {
					next.ServeHTTP(
						writer, request)
					return
				}
				if request.ContentLength > maximumBytes {
					writeAPIError(writer,
						request, newAPIError(http.StatusRequestEntityTooLarge,
							errorCodePayloadTooLarge, "Request body exceeds the configured size limit."),
					)
					return
				}
				if request.Body != nil &&
					request.Body != http.NoBody {
					request.Body =
						http.MaxBytesReader(writer, request.Body,
							maximumBytes)
				}
				next.ServeHTTP(writer,
					request)
			},
		)
	}
}
func TrustedCompanyContextMiddleware() Middleware {
	return func(
		next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(writer http.ResponseWriter, request *http.Request,
			) {
				if !isInternalAuthenticated(request.Context()) {
					writeAPIError(writer,
						request, newAPIError(http.StatusUnauthorized,
							errorCodeUnauthorized, "Trusted company context requires authenticated internal service access."),
					)
					return
				}
				rawCompanyID := strings.TrimSpace(
					request.Header.Get(HeaderCompanyID),
				)
				if rawCompanyID == "" {
					writeAPIError(writer, request,
						newAPIError(http.StatusBadRequest, errorCodeMissingCompanyContext,
							"Trusted company context is required.", ErrorDetail{Field: HeaderCompanyID,
								Reason: "header is required"},
						))
					return
				}
				companyID, err := workflow.NewCompanyID(rawCompanyID)
				if err != nil {
					writeAPIError(writer, request,
						newAPIError(http.StatusBadRequest, errorCodeInvalidCompanyContext,
							"Trusted company context is invalid.", ErrorDetail{Field: HeaderCompanyID,
								Reason: "header does not contain a valid company identifier"},
						))
					return
				}
				ctx := transport.WithCompanyID(request.Context(), companyID)
				next.ServeHTTP(
					writer, request.WithContext(ctx))
			})
	}
}
func HandlerTimeoutMiddleware(
	timeout time.Duration) Middleware {
	return func(
		next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(writer http.ResponseWriter, request *http.Request,
			) {
				if timeout <= 0 {
					next.ServeHTTP(
						writer, request)
					return
				}
				ctx, cancel := context.WithTimeout(
					request.Context(), timeout)
				defer cancel()
				next.ServeHTTP(writer, request.WithContext(ctx))
			})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (recorder *statusRecorder) WriteHeader(
	statusCode int) {
	if recorder.statusCode != 0 {
		return
	}
	recorder.statusCode = statusCode
	recorder.ResponseWriter.WriteHeader(
		statusCode)
}
func (recorder *statusRecorder,
) Write(body []byte) (
	int, error) {
	if recorder.statusCode == 0 {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(body)
}
func (
	recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}
func resolveTraceID(
	provided string, prefix string) string {
	normalized := strings.TrimSpace(provided)
	if validTraceID(normalized) {
		return normalized
	}
	return newOpaqueID(prefix)
}
func validTraceID(value string) bool {
	if value == "" || len(value) > maximumTraceIDLength {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'a' &&
			character <= 'z':
		case character >= 'A' &&
			character <= 'Z':
		case character >= '0' &&
			character <= '9':
		case character == '-',
			character == '_', character == '.', character == ':',
			character == '/':
		default:
			return false
		}
	}
	return true
}
func newOpaqueID(prefix string,
) string {
	randomBytes := make([]byte,
		16)
	if _, err := rand.Read(randomBytes); err == nil {
		return prefix + hex.EncodeToString(randomBytes)
	}
	return fmt.Sprintf("%s%x%x", prefix,
		time.Now().UTC().UnixNano(), fallbackIDCounter.Add(1))
}
func openAPIHandler(
	writer http.ResponseWriter, request *http.Request) {
	openapi.OpenAPI(writeAPIError, writer,
		request)
}
func swaggerRedirectHandler(writer http.ResponseWriter,
	request *http.Request) {
	openapi.SwaggerRedirect(
		writeAPIError, writer, request,
	)
}
func swaggerUIHandler(writer http.ResponseWriter, request *http.Request,
) {
	openapi.SwaggerUI(writeAPIError,
		writer, request)
}
func decodeJSONBody(
	writer http.ResponseWriter, request *http.Request, destination any,
) *APIError {
	return transport.DecodeJSONBody(writer,
		request, destination)
}
func writeJSON(
	writer http.ResponseWriter, statusCode int, payload any,
) error {
	return transport.WriteJSON(writer,
		statusCode, payload)
}
