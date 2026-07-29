package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"miletos-go/internal/middleware"
)

func TestCompanyContextMakesTrustedHeaderAvailable(t *testing.T) {
	handler := middleware.CompanyContext(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if got := middleware.CompanyID(request.Context()); got != "company-1" {
				t.Errorf("CompanyID() = %q", got)
			}
			writer.WriteHeader(http.StatusNoContent)
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(middleware.HeaderCompanyID, "  company-1  ")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestCompanyContextRejectsInvalidHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
	}{
		{name: "missing"},
		{name: "empty", header: ""},
		{name: "whitespace", header: " \t "},
		{name: "too long", header: string(make([]byte, 256))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			handler := middleware.CompanyContext(http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { called = true },
			))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.name == "too long" {
				request.Header.Set(middleware.HeaderCompanyID, test.header)
			} else if test.header != "" {
				request.Header.Set(middleware.HeaderCompanyID, test.header)
			}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest || called {
				t.Fatalf("status=%d called=%v", response.Code, called)
			}
			var document map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if document["code"] != "INVALID_COMPANY_CONTEXT" {
				t.Fatalf("error response = %#v", document)
			}
		})
	}
}

func TestRequestContextPropagatesAndReturnsIdentifiers(t *testing.T) {
	handler := middleware.RequestContext(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if middleware.RequestID(request.Context()) != "req-1" {
				t.Errorf("RequestID() = %q", middleware.RequestID(request.Context()))
			}
			if middleware.CorrelationID(request.Context()) != "corr-1" {
				t.Errorf("CorrelationID() = %q", middleware.CorrelationID(request.Context()))
			}
			writer.WriteHeader(http.StatusNoContent)
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(middleware.HeaderRequestID, "req-1")
	request.Header.Set(middleware.HeaderCorrelationID, "corr-1")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Header().Get(middleware.HeaderRequestID) != "req-1" ||
		response.Header().Get(middleware.HeaderCorrelationID) != "corr-1" {
		t.Fatalf("response headers = %#v", response.Header())
	}
}

func TestRequestContextGeneratesMissingIdentifiers(t *testing.T) {
	handler := middleware.RequestContext(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {},
	))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Header().Get(middleware.HeaderRequestID) == "" ||
		response.Header().Get(middleware.HeaderCorrelationID) == "" {
		t.Fatalf("response headers = %#v", response.Header())
	}
}
