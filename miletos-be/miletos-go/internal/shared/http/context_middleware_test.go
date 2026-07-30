package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"miletos-go/internal/shared/requestcontext"
)

func TestCompanyContextMakesTrustedHeaderAvailable(t *testing.T) {
	handler := companyContext(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if got := requestcontext.CompanyID(request.Context()); got != "company-1" {
				t.Errorf("CompanyID() = %q", got)
			}
			writer.WriteHeader(http.StatusNoContent)
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(requestcontext.HeaderCompanyID, "  company-1  ")
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
			handler := companyContext(http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { called = true },
			))
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.name == "too long" {
				request.Header.Set(requestcontext.HeaderCompanyID, test.header)
			} else if test.header != "" {
				request.Header.Set(requestcontext.HeaderCompanyID, test.header)
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
	handler := requestContext(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			if requestcontext.RequestID(request.Context()) != "req-1" {
				t.Errorf("RequestID() = %q", requestcontext.RequestID(request.Context()))
			}
			if requestcontext.CorrelationID(request.Context()) != "corr-1" {
				t.Errorf("CorrelationID() = %q", requestcontext.CorrelationID(request.Context()))
			}
			writer.WriteHeader(http.StatusNoContent)
		},
	))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(requestcontext.HeaderRequestID, "req-1")
	request.Header.Set(requestcontext.HeaderCorrelationID, "corr-1")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Header().Get(requestcontext.HeaderRequestID) != "req-1" ||
		response.Header().Get(requestcontext.HeaderCorrelationID) != "corr-1" {
		t.Fatalf("response headers = %#v", response.Header())
	}
}

func TestRequestContextGeneratesMissingIdentifiers(t *testing.T) {
	handler := requestContext(http.HandlerFunc(
		func(http.ResponseWriter, *http.Request) {},
	))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Header().Get(requestcontext.HeaderRequestID) == "" ||
		response.Header().Get(requestcontext.HeaderCorrelationID) == "" {
		t.Fatalf("response headers = %#v", response.Header())
	}
}
