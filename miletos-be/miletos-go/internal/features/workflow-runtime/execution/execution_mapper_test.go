package execution

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	model "miletos-go/internal/features/workflow-runtime/execution/model"
	repository "miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/shared/requestcontext"
)

func TestMapWorkflowRequestPreservesContractData(t *testing.T) {
	request := workflowRequest{
		ID: "workflow-1", Name: "Workflow", Revision: 3,
		Metadata: map[string]any{"owner": "test"},
		Nodes: []nodeRequest{{
			ID: "node-1", PluginType: "custom.node", PluginVersion: "v2",
			Configuration: map[string]any{"value": "configured"},
			Position:      &workflow.NodePosition{X: 10, Y: 20},
		}},
		Edges: []edgeRequest{{
			ID: "edge-1", SourceNodeID: "node-1", SourceOutputPort: "output",
			TargetNodeID: "node-2", TargetInputPort: "input",
		}},
	}

	workflow := mapWorkflowRequest(request, "company-1")

	if workflow.ID != request.ID || workflow.CompanyID != "company-1" ||
		workflow.Revision != request.Revision {
		t.Fatalf("workflow identity = %#v", workflow)
	}
	if !reflect.DeepEqual(workflow.Metadata, request.Metadata) ||
		!reflect.DeepEqual(workflow.Nodes[0].Configuration, request.Nodes[0].Configuration) {
		t.Fatalf("dynamic values were not preserved: %#v", workflow)
	}
	if workflow.Nodes[0].Type != "custom.node" || workflow.Edges[0].TargetInputPort != "input" {
		t.Fatalf("mapped workflow = %#v", workflow)
	}
}

func TestExecutionResponseMappingAndJSONNames(t *testing.T) {
	started := time.Unix(2, 0).UTC()
	execution := model.Execution{
		ID: "exec-1", WorkflowID: "workflow-1", WorkflowRevision: 4,
		Mode: "ASYNC", Status: model.ExecutionRunning, CorrelationID: "corr-1",
		CreatedAt: time.Unix(1, 0).UTC(), StartedAt: &started, UpdatedAt: started,
	}

	response := mapExecutionOutcome(ExecutionOutcome{
		Execution: execution, ScheduledEntryNodes: 2,
	})
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{
		"executionId", "workflowId", "workflowRevision", "mode",
		"status", "correlationId", "createdAt", "startedAt", "scheduledRoots",
	} {
		if _, exists := document[key]; !exists {
			t.Errorf("property %q is absent", key)
		}
	}
	if _, exists := document["finishedAt"]; exists {
		t.Fatal("optional finishedAt was serialized")
	}
}

func TestMapExecutionPage(t *testing.T) {
	now := time.Unix(1, 0).UTC()
	page := model.Page[model.Execution]{
		Items: []model.Execution{{
			ID: "exec-1", WorkflowID: "workflow-1", Status: model.ExecutionSucceeded,
			CreatedAt: now, UpdatedAt: now,
		}},
		Next: "exec-next", HasNext: true,
	}

	mapped := mapExecutionPage(page)

	if mapped.Next != "exec-next" || !mapped.HasNext {
		t.Fatalf("mapped page = %#v", mapped)
	}
	if len(mapped.Items) != 1 || mapped.Items[0].ExecutionID != "exec-1" {
		t.Fatalf("mapped items = %#v", mapped.Items)
	}
}

func TestWriteExecutionErrorMappings(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "not found", err: repository.ErrNotFound, status: http.StatusNotFound, code: "NOT_FOUND"},
		{
			name: "idempotency", err: repository.ErrIdempotencyConflict,
			status: http.StatusConflict, code: "IDEMPOTENCY_KEY_REUSED",
		},
		{
			name: "idempotency in progress", err: ErrIdempotencyRequestInProgress,
			status: http.StatusConflict, code: "IDEMPOTENCY_REQUEST_IN_PROGRESS",
		},
		{
			name: "timeout", err: context.DeadlineExceeded,
			status: http.StatusGatewayTimeout, code: "REQUEST_TIMEOUT",
		},
		{
			name: "async unavailable", err: ErrAsyncUnavailable,
			status: http.StatusServiceUnavailable, code: "EXECUTION_UNAVAILABLE",
		},
		{
			name: "recovery", err: ErrRecoveryUnsupported,
			status: http.StatusConflict, code: "RECOVERY_NOT_SUPPORTED",
		},
		{
			name: "validation", err: workflow.ErrInvalidWorkflow,
			status: http.StatusUnprocessableEntity, code: "WORKFLOW_VALIDATION_FAILED",
		},
		{
			name: "internal", err: errors.New("database detail"),
			status: http.StatusInternalServerError, code: "INTERNAL_SERVER_ERROR",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request = request.WithContext(context.WithValue(request.Context(), struct{}{}, "ignored"))
			response := httptest.NewRecorder()

			writeExecutionError(response, request, test.err)

			assertAPIError(t, response, test.status, test.code)
			if test.code == "INTERNAL_SERVER_ERROR" &&
				strings.Contains(response.Body.String(), "database detail") {
				t.Fatal("internal error detail was exposed")
			}
		})
	}
}

func TestWriteAPIErrorIncludesRequestContextAndOmitsEmptyDetails(t *testing.T) {
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		WriteAPIError(writer, request, http.StatusBadRequest, "BAD_REQUEST", "bad request")
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request = request.WithContext(requestcontext.WithValues(
		request.Context(),
		requestcontext.Values{RequestID: "req-test", CorrelationID: "corr-test"},
	))
	response := httptest.NewRecorder()

	next.ServeHTTP(response, request)

	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if document["requestId"] != "req-test" || document["correlationId"] != "corr-test" {
		t.Fatalf("error document = %#v", document)
	}
	if _, exists := document["details"]; exists {
		t.Fatal("empty optional details were serialized")
	}
}
