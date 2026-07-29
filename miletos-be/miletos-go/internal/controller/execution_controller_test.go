package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testExecutionController() *ExecutionController {
	return NewExecutionController(nil, nil, nil, nil)
}

func TestExecuteSyncRejectsMalformedJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/executions/sync", bytes.NewBufferString("{invalid"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	testExecutionController().ExecuteSync(response, request)

	assertAPIError(t, response, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST")
}

func TestExecuteSyncRejectsUnsupportedContentType(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/executions/sync", bytes.NewBufferString("{}"))
	request.Header.Set("Content-Type", "text/plain")
	response := httptest.NewRecorder()

	testExecutionController().ExecuteSync(response, request)

	assertAPIError(t, response, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE")
}

func TestExecuteAsyncRequiresIdempotencyKeyBeforeDecoding(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/executions/async", bytes.NewBufferString("{}"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	testExecutionController().ExecuteAsync(response, request)

	assertAPIError(t, response, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY")
}

func TestRecoverRejectsBodyAndQuery(t *testing.T) {
	tests := []string{
		"/api/v1/executions/exec-1/recover?unexpected=true",
		"/api/v1/executions/exec-1/recover",
	}
	for index, target := range tests {
		t.Run(target, func(t *testing.T) {
			var body *bytes.Reader
			if index == 1 {
				body = bytes.NewReader([]byte("{}"))
			} else {
				body = bytes.NewReader(nil)
			}
			request := httptest.NewRequest(http.MethodPost, target, body)
			request.Header.Set("Idempotency-Key", "key-1")
			response := httptest.NewRecorder()

			testExecutionController().Recover(response, request)

			assertAPIError(t, response, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST")
		})
	}
}

func TestListRejectsInvalidQueries(t *testing.T) {
	tests := []string{
		"/api/v1/executions?unknown=value",
		"/api/v1/executions?limit=0",
		"/api/v1/executions?limit=101",
		"/api/v1/executions?status=UNKNOWN",
		"/api/v1/executions?status=",
		"/api/v1/executions?workflowId=",
		"/api/v1/executions?limit=10&limit=20",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			response := httptest.NewRecorder()
			testExecutionController().List(response, httptest.NewRequest(http.MethodGet, target, nil))
			assertAPIError(t, response, http.StatusBadRequest, "INVALID_EXECUTION_QUERY")
		})
	}
}

func TestDetailEndpointsRejectQueries(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(http.ResponseWriter, *http.Request)
	}{
		{name: "detail", invoke: testExecutionController().Get},
		{name: "definition", invoke: testExecutionController().GetDefinition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/executions/exec-1?unexpected=true", nil)
			response := httptest.NewRecorder()

			test.invoke(response, request)

			assertAPIError(t, response, http.StatusBadRequest, "INVALID_EXECUTION_QUERY")
		})
	}
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, status, response.Body.String())
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q", response.Header().Get("Content-Type"))
	}
	var envelope apiErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if envelope.Status != status || envelope.Code != code || envelope.Timestamp == "" {
		t.Fatalf("error envelope = %#v", envelope)
	}
}
