package httptrigger

import (
	"net/http"
	"net/url"
	"sort"
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

// fingerprintExcludedHeaders are hop-by-hop / transport / client-control values
// that must not participate in logical request identity. safeHeaders still keeps
// many of these for the workflow payload; only the idempotency fingerprint
// strips them.
var fingerprintExcludedHeaders = map[string]bool{
	"connection":         true,
	"keep-alive":         true,
	"proxy-authenticate": true,
	"te":                 true,
	"trailers":           true,
	"transfer-encoding":  true,
	"upgrade":            true,
	"content-length":     true,
	"host":               true,
	"accept":             true,
	"accept-encoding":    true,
	"user-agent":         true,
	"date":               true,
	"expect":             true,
	"forwarded":          true,
	"via":                true,
	"origin":             true,
	"referer":            true,
	"cache-control":      true,
	"pragma":             true,
	"priority":           true,
	"postman-token":      true,
}

func fingerprintHeaders(headers map[string][]string) map[string][]string {
	if headers == nil {
		return map[string][]string{}
	}
	result := make(map[string][]string, len(headers))
	for name, values := range headers {
		normalized := strings.ToLower(name)
		if fingerprintExcludedHeaders[normalized] ||
			strings.HasPrefix(normalized, "x-forwarded-") ||
			strings.HasPrefix(normalized, "sec-fetch-") ||
			strings.HasPrefix(normalized, "sec-ch-") {
			continue
		}
		sorted := append([]string(nil), values...)
		sort.Strings(sorted)
		result[http.CanonicalHeaderKey(name)] = sorted
	}
	return result
}

func fingerprintQuery(query any) map[string][]string {
	values, ok := query.(url.Values)
	if !ok {
		if typed, typedOK := query.(map[string][]string); typedOK {
			values = url.Values(typed)
		} else {
			return map[string][]string{}
		}
	}
	result := make(map[string][]string, len(values))
	for key, items := range values {
		sorted := append([]string(nil), items...)
		sort.Strings(sorted)
		result[key] = sorted
	}
	return result
}
