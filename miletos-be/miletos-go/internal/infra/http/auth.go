package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"miletos-go/internal/infra/http/transport"
)

const minimumInternalServiceTokenBytes = 32

type InternalAuthenticator struct {
	expectedDigest [sha256.Size]byte
	initialized    bool
}

func NewInternalAuthenticator(token string) (InternalAuthenticator, error) {
	normalized := strings.TrimSpace(token)
	if len(normalized) < minimumInternalServiceTokenBytes {
		return InternalAuthenticator{}, fmt.Errorf(
			"internal service token must contain at least %d bytes",
			minimumInternalServiceTokenBytes,
		)
	}
	return InternalAuthenticator{
		expectedDigest: sha256.Sum256([]byte(normalized)),
		initialized:    true,
	}, nil
}

func (authenticator InternalAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !authenticator.initialized {
			writeAPIError(
				writer,
				request,
				newAPIError(
					http.StatusInternalServerError,
					errorCodeInternalServerError,
					"Internal authentication is unavailable.",
				),
			)
			return
		}
		token, valid := bearerToken(request.Header.Get("Authorization"))
		if !valid || !authenticator.matches(token) {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			writeAPIError(
				writer,
				request,
				newAPIError(
					http.StatusUnauthorized,
					errorCodeUnauthorized,
					"Valid internal service authentication is required.",
				),
			)
			return
		}
		ctx := transport.WithInternalAuthentication(request.Context())
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func (authenticator InternalAuthenticator) matches(token string) bool {
	actualDigest := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(
		authenticator.expectedDigest[:],
		actualDigest[:],
	) == 1
}

func bearerToken(authorization string) (string, bool) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", false
	}
	return token, true
}
