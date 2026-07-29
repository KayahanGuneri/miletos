package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

type Authentication struct {
	tokenDigest [sha256.Size]byte
}

func NewAuthentication(token string) Authentication {
	return Authentication{tokenDigest: sha256.Sum256([]byte(strings.TrimSpace(token)))}
}

func (authentication Authentication) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		parts := strings.Fields(request.Header.Get("Authorization"))
		valid := len(parts) == 2 && strings.EqualFold(parts[0], "Bearer")
		if valid {
			actual := sha256.Sum256([]byte(parts[1]))
			valid = subtle.ConstantTimeCompare(authentication.tokenDigest[:], actual[:]) == 1
		}
		if !valid {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			writeError(writer, request, http.StatusUnauthorized, "UNAUTHORIZED",
				"Valid internal service authentication is required.")
			return
		}
		next.ServeHTTP(writer, request)
	})
}
