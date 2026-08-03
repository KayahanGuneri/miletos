package http

import (
	"encoding/json"
	nethttp "net/http"
	"strings"
	"time"

	"miletos-go/internal/shared/requestcontext"
)

func requestContext(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(
		writer nethttp.ResponseWriter,
		request *nethttp.Request,
	) {
		requestID := request.Header.Get("X-Request-ID")
		correlationID := request.Header.Get("X-Correlation-ID")
		if strings.HasPrefix(request.URL.Path, "/hooks/") {
			requestID = ""
			correlationID = ""
		}
		values := requestcontext.Values{
			RequestID: requestcontext.NormalizeID(
				requestID, "req_",
			),
			CorrelationID: requestcontext.NormalizeID(
				correlationID, "corr_",
			),
		}
		writer.Header().Set("X-Request-ID", values.RequestID)
		writer.Header().Set("X-Correlation-ID", values.CorrelationID)
		next.ServeHTTP(
			writer,
			request.WithContext(requestcontext.WithValues(request.Context(), values)),
		)
	})
}

func companyContext(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(
		writer nethttp.ResponseWriter,
		request *nethttp.Request,
	) {
		companyID := strings.TrimSpace(request.Header.Get("X-Miletos-Company-ID"))
		if companyID == "" || len(companyID) > 255 {
			writeContextError(
				writer,
				request,
				nethttp.StatusBadRequest,
				"INVALID_COMPANY_CONTEXT",
				"Trusted company context is invalid.",
			)
			return
		}
		values := requestcontext.Values{
			CompanyID:     companyID,
			RequestID:     requestcontext.RequestID(request.Context()),
			CorrelationID: requestcontext.CorrelationID(request.Context()),
			IdempotencyKey: strings.TrimSpace(
				request.Header.Get("Idempotency-Key"),
			),
		}
		next.ServeHTTP(
			writer,
			request.WithContext(requestcontext.WithValues(request.Context(), values)),
		)
	})
}

func writeContextError(
	writer nethttp.ResponseWriter,
	request *nethttp.Request,
	status int,
	code string,
	message string,
) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
		"status":        status,
		"code":          code,
		"message":       message,
		"requestId":     requestcontext.RequestID(request.Context()),
		"correlationId": requestcontext.CorrelationID(request.Context()),
	})
}
