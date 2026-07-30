package execution

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
	request.Header.Set("Idempotency-Key", "malformed-json-test")
	response := httptest.NewRecorder()

	testExecutionController().ExecuteSync(response, request)

	assertAPIError(t, response, http.StatusBadRequest, "INVALID_EXECUTION_REQUEST")
}

func TestExecuteAsyncRequiresIdempotencyKeyBeforeDecoding(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/executions/async", bytes.NewBufferString("{}"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	testExecutionController().ExecuteAsync(response, request)

	assertAPIError(t, response, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY")
}

func TestListRejectsInvalidQueries(t *testing.T) {
	tests := []string{
		"/api/v1/executions?limit=0",
		"/api/v1/executions?limit=101",
		"/api/v1/executions?status=UNKNOWN",
		"/api/v1/executions?status=",
		"/api/v1/executions?workflowId=",
	}
	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			response := httptest.NewRecorder()
			testExecutionController().List(response, httptest.NewRequest(http.MethodGet, target, nil))
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
