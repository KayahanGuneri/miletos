package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"miletos-go/internal/shared/requestcontext"
)

type InternalTokenValidator struct {
	tokenDigest [sha256.Size]byte
}

type Authentication struct {
	validator InternalTokenValidator
}

func NewAuthentication(validator InternalTokenValidator) Authentication {
	return Authentication{validator: validator}
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

func NewInternalTokenValidator(token string) (InternalTokenValidator, error) {
	if len([]byte(token)) < 32 || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
		return InternalTokenValidator{}, fmt.Errorf(
			"internal service token must contain at least 32 bytes and no whitespace",
		)
	}
	return InternalTokenValidator{tokenDigest: sha256.Sum256([]byte(token))}, nil
}

func (validator InternalTokenValidator) ValidateAuthorization(value string) bool {
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	actual := sha256.Sum256([]byte(parts[1]))
	return subtle.ConstantTimeCompare(validator.tokenDigest[:], actual[:]) == 1
}
