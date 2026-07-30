package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"miletos-go/internal/shared/requestcontext"
)

type InternalTokenValidator struct {
	tokenDigest [sha256.Size]byte
}

type Authentication struct {
	validator InternalTokenValidator
}

func NewAuthentication(token string) Authentication {
	return Authentication{validator: NewInternalTokenValidator(token)}
}

func (authentication Authentication) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !authentication.validator.ValidateAuthorization(
			request.Header.Get("Authorization"),
		) {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"timestamp":     time.Now().UTC().Format(time.RFC3339Nano),
				"status":        http.StatusUnauthorized,
				"code":          "UNAUTHORIZED",
				"message":       "Valid internal service authentication is required.",
				"requestId":     requestcontext.RequestID(request.Context()),
				"correlationId": requestcontext.CorrelationID(request.Context()),
			})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func NewInternalTokenValidator(token string) InternalTokenValidator {
	return InternalTokenValidator{
		tokenDigest: sha256.Sum256([]byte(strings.TrimSpace(token))),
	}
}

func (validator InternalTokenValidator) ValidateAuthorization(value string) bool {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	actual := sha256.Sum256([]byte(parts[1]))
	return subtle.ConstantTimeCompare(validator.tokenDigest[:], actual[:]) == 1
}
