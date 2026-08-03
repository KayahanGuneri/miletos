package security_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	security "miletos-go/internal/shared/security"
)

func TestAuthenticationAcceptsValidBearerToken(t *testing.T) {
	const token = "internal-service-token-value-32-bytes"
	var calls atomic.Int32
	handler := authentication(t, token).Handle(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			writer.WriteHeader(http.StatusNoContent)
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || calls.Load() != 1 {
		t.Fatalf("status=%d calls=%d", response.Code, calls.Load())
	}
}

func TestAuthenticationRejectsInvalidHeaders(t *testing.T) {
	const token = "internal-service-token-value-32-bytes"
	tests := []struct {
		name   string
		header string
	}{
		{name: "missing"},
		{name: "invalid scheme", header: "Basic " + token},
		{name: "empty token", header: "Bearer"},
		{name: "incorrect token", header: "Bearer incorrect"},
		{name: "malformed", header: "Bearer one two"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			handler := authentication(t, token).Handle(http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { calls.Add(1) },
			))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized || calls.Load() != 0 {
				t.Fatalf("status=%d calls=%d", response.Code, calls.Load())
			}
			if response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("WWW-Authenticate = %q", response.Header().Get("WWW-Authenticate"))
			}
			var document map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if document["code"] != "UNAUTHORIZED" {
				t.Fatalf("error response = %#v", document)
			}
			if strings.Contains(response.Body.String(), token) {
				t.Fatal("secret token was exposed")
			}
		})
	}
}

func TestNewInternalTokenValidatorRejectsWeakTokens(t *testing.T) {
	for _, token := range []string{
		"", "too-short", "internal service token with whitespace 32",
	} {
		validator, err := security.NewInternalTokenValidator(token)
		if err == nil || validator.ValidateAuthorization("Bearer "+token) {
			t.Fatalf("NewInternalTokenValidator(%q) = (%#v, %v)", token, validator, err)
		}
	}
}

func authentication(t *testing.T, token string) security.Authentication {
	t.Helper()
	validator, err := security.NewInternalTokenValidator(token)
	if err != nil {
		t.Fatalf("NewInternalTokenValidator() error = %v", err)
	}
	return security.NewAuthentication(validator)
}
