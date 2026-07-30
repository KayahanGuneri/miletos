package httptrigger

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"miletos-go/internal/features/workflowruntime/execution"
	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/shared/requestcontext"
)

const maximumWebhookBody = 1 << 20

var tokenSyntax = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)

type PublicController struct {
	service *Service
}

func NewPublicController(service *Service) *PublicController {
	return &PublicController{service: service}
}

func (controller *PublicController) Invoke(
	writer http.ResponseWriter,
	request *http.Request,
) {
	rawToken := chi.URLParam(request, "token")
	if !tokenSyntax.MatchString(rawToken) {
		writePublicError(writer, request, http.StatusNotFound, "HOOK_NOT_FOUND",
			"The requested webhook was not found.")
		return
	}
	body, err := decodeWebhookBody(writer, request)
	if err != nil {
		status := http.StatusBadRequest
		code := "INVALID_WEBHOOK_BODY"
		if errors.Is(err, errBodyTooLarge) {
			status, code = http.StatusRequestEntityTooLarge, "WEBHOOK_BODY_TOO_LARGE"
		} else if errors.Is(err, errUnsupportedMedia) {
			status, code = http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE"
		}
		writePublicError(writer, request, status, code, err.Error())
		return
	}
	key, err := optionalIdempotencyKey(request)
	if err != nil {
		writePublicError(writer, request, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY",
			"Idempotency-Key is invalid.")
		return
	}
	requestID := requestcontext.RequestID(request.Context())
	correlationID := requestcontext.CorrelationID(request.Context())
	payload := map[string]any{
		"method":        request.Method,
		"path":          "/hooks/{redacted}",
		"query":         request.URL.Query(),
		"headers":       safeHeaders(request.Header),
		"body":          body,
		"requestId":     requestID,
		"correlationId": correlationID,
	}
	outcome, allowedMethod, err := controller.service.Invoke(
		request.Context(), rawToken, request.Method, payload, correlationID, key,
	)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			writePublicError(writer, request, http.StatusNotFound, "HOOK_NOT_FOUND",
				"The requested webhook was not found.")
		case errors.Is(err, ErrMethodNotAllowed):
			writer.Header().Set("Allow", allowedMethod)
			writePublicError(writer, request, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
				"The requested method is not allowed for this webhook.")
		case errors.Is(err, repository.ErrIdempotencyConflict):
			writePublicError(writer, request, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED",
				"Idempotency-Key was already used for a different request.")
		case errors.Is(err, execution.ErrAsyncUnavailable):
			writePublicError(writer, request, http.StatusServiceUnavailable,
				"EXECUTION_UNAVAILABLE", "Workflow execution is currently unavailable.")
		default:
			writePublicError(writer, request, http.StatusInternalServerError,
				"INTERNAL_SERVER_ERROR", "The webhook could not be accepted.")
		}
		return
	}
	writePublicJSON(writer, http.StatusAccepted, map[string]any{
		"executionId":    outcome.Execution.ID,
		"mode":           outcome.Execution.Mode,
		"status":         outcome.Execution.Status,
		"correlationId":  outcome.Execution.CorrelationID,
		"scheduledRoots": outcome.ScheduledRoots,
		"replayed":       outcome.Replayed,
	})
}

var (
	errBodyTooLarge     = errors.New("Webhook request body exceeds the allowed size.")
	errUnsupportedMedia = errors.New("Webhook request body media type is not supported.")
	errMalformedBody    = errors.New("Webhook request body is malformed.")
)

func decodeWebhookBody(writer http.ResponseWriter, request *http.Request) (any, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, maximumWebhookBody)
	encoded, err := io.ReadAll(request.Body)
	if err != nil {
		var maximumBytesError *http.MaxBytesError
		if errors.As(err, &maximumBytesError) {
			return nil, errBodyTooLarge
		}
		return nil, errMalformedBody
	}
	if len(encoded) == 0 {
		return nil, nil
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		return nil, errUnsupportedMedia
	}
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		decoder := json.NewDecoder(strings.NewReader(string(encoded)))
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, errMalformedBody
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return nil, errMalformedBody
		}
		return value, nil
	}
	if strings.HasPrefix(mediaType, "text/") && utf8.Valid(encoded) {
		return string(encoded), nil
	}
	return nil, errUnsupportedMedia
}

func optionalIdempotencyKey(request *http.Request) (string, error) {
	values := request.Header.Values("Idempotency-Key")
	if len(values) == 0 {
		return "", nil
	}
	if len(values) != 1 {
		return "", errors.New("invalid idempotency key")
	}
	key := strings.TrimSpace(values[0])
	if key == "" || utf8.RuneCountInString(key) > 255 {
		return "", errors.New("invalid idempotency key")
	}
	return key, nil
}

func safeHeaders(headers http.Header) map[string][]string {
	sensitive := map[string]bool{
		"authorization":                    true,
		"proxy-authorization":              true,
		"cookie":                           true,
		"set-cookie":                       true,
		"x-api-key":                        true,
		"x-auth-token":                     true,
		"idempotency-key":                  true,
		"x-correlation-id":                 true,
		"x-request-id":                     true,
		"traceparent":                      true,
		"x-miletos-company-id":             true,
		"x-miletos-internal-authorization": true,
	}
	result := make(map[string][]string)
	for name, values := range headers {
		normalized := strings.ToLower(name)
		if sensitive[normalized] || strings.HasPrefix(normalized, "x-miletos-") {
			continue
		}
		result[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
	}
	return result
}

func writePublicError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code string,
	message string,
) {
	writePublicJSON(writer, status, map[string]any{
		"status":        status,
		"code":          code,
		"message":       message,
		"requestId":     requestcontext.RequestID(request.Context()),
		"correlationId": requestcontext.CorrelationID(request.Context()),
	})
}

func writePublicJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
