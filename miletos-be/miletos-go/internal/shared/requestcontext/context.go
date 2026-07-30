package requestcontext

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
)

const (
	HeaderRequestID      = "x-request-id"
	HeaderCorrelationID  = "x-correlation-id"
	HeaderCompanyID      = "x-miletos-company-id"
	HeaderIdempotencyKey = "idempotency-key"
)

type contextKey uint8

const (
	companyKey contextKey = iota
	correlationKey
	requestKey
	idempotencyKey
)

type Values struct {
	CompanyID      string
	CorrelationID  string
	RequestID      string
	IdempotencyKey string
}

func WithValues(ctx context.Context, values Values) context.Context {
	ctx = context.WithValue(ctx, companyKey, values.CompanyID)
	ctx = context.WithValue(ctx, correlationKey, values.CorrelationID)
	ctx = context.WithValue(ctx, requestKey, values.RequestID)
	return context.WithValue(ctx, idempotencyKey, values.IdempotencyKey)
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

func IdempotencyKey(ctx context.Context) string {
	value, _ := ctx.Value(idempotencyKey).(string)
	return value
}

func NormalizeID(value, prefix string) string {
	value = strings.TrimSpace(value)
	if value != "" && len(value) <= 255 {
		return value
	}
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return prefix + "unavailable"
	}
	return prefix + hex.EncodeToString(random)
}
