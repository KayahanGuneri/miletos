package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	companyKey contextKey = iota
	correlationKey
	requestKey
)

type errorResponse struct {
	Timestamp     string `json:"timestamp"`
	Status        int    `json:"status"`
	Code          string `json:"code"`
	Message       string `json:"message"`
	RequestID     string `json:"requestId,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
}

func CompanyContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		companyID := strings.TrimSpace(request.Header.Get(HeaderCompanyID))
		if companyID == "" || len(companyID) > 255 {
			writeError(writer, request, http.StatusBadRequest, "INVALID_COMPANY_CONTEXT",
				"Trusted company context is invalid.")
			return
		}
		ctx := context.WithValue(request.Context(), companyKey, companyID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func RequestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := normalizedID(request.Header.Get(HeaderRequestID), "req_")
		correlationID := normalizedID(request.Header.Get(HeaderCorrelationID), "corr_")
		writer.Header().Set(HeaderRequestID, requestID)
		writer.Header().Set(HeaderCorrelationID, correlationID)
		ctx := context.WithValue(request.Context(), requestKey, requestID)
		ctx = context.WithValue(ctx, correlationKey, correlationID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func CompanyID(ctx context.Context) string {
	value, _ := ctx.Value(companyKey).(string)
	return value
}

func CorrelationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationKey).(string)
	return value
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestKey).(string)
	return value
}

func normalizedID(value, prefix string) string {
	value = strings.TrimSpace(value)
	if value != "" && len(value) <= 255 {
		return value
	}
	random := make([]byte, 12)
	_, _ = rand.Read(random)
	return prefix + hex.EncodeToString(random)
}

func writeError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code string,
	message string,
) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(errorResponse{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Status:    status, Code: code, Message: message,
		RequestID:     RequestID(request.Context()),
		CorrelationID: CorrelationID(request.Context()),
	})
}
