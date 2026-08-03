// Package response provides shared HTTP response encoding helpers.
package response

import (
	"encoding/json"
	"net/http"
	"time"

	"miletos-go/internal/shared/requestcontext"
)

func WriteJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func WriteAPIError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code string,
	message string,
) {
	WriteJSON(writer, status, map[string]any{
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
		"status":        status,
		"code":          code,
		"message":       message,
		"path":          request.URL.Path,
		"requestId":     requestcontext.RequestID(request.Context()),
		"correlationId": requestcontext.CorrelationID(request.Context()),
	})
}
