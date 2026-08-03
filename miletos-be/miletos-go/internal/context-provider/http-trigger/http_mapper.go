package httptrigger

import (
	"net/http"
	"strings"

	"miletos-go/internal/features/workflow-runtime/execution"
)

type publicExecutionResponse struct {
	ExecutionID    string `json:"executionId"`
	Mode           string `json:"mode"`
	Status         string `json:"status"`
	CorrelationID  string `json:"correlationId"`
	ScheduledRoots int    `json:"scheduledRoots"`
	Replayed       bool   `json:"replayed"`
}

type publicErrorResponse struct {
	Status        int    `json:"status"`
	Code          string `json:"code"`
	Message       string `json:"message"`
	RequestID     string `json:"requestId,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
}

func mapWebhookPayload(
	request *http.Request,
	body any,
	requestID string,
	correlationID string,
) map[string]any {
	return map[string]any{
		"method":        request.Method,
		"path":          "/hooks/{redacted}",
		"query":         request.URL.Query(),
		"headers":       safeHeaders(request.Header),
		"body":          body,
		"requestId":     requestID,
		"correlationId": correlationID,
	}
}

func mapPublicExecution(outcome execution.ExecutionOutcome) publicExecutionResponse {
	return publicExecutionResponse{
		ExecutionID: outcome.Execution.ID, Mode: outcome.Execution.Mode,
		Status: string(outcome.Execution.Status), CorrelationID: outcome.Execution.CorrelationID,
		ScheduledRoots: outcome.ScheduledEntryNodes, Replayed: outcome.Replayed,
	}
}

func mapPublicError(
	status int,
	code string,
	message string,
	requestID string,
	correlationID string,
) publicErrorResponse {
	return publicErrorResponse{
		Status: status, Code: code, Message: message,
		RequestID: requestID, CorrelationID: correlationID,
	}
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
