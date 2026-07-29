package model_test

import (
	"encoding/json"
	"testing"
	"time"

	"miletos-go/internal/model"
)

func TestExecutionJSONContract(t *testing.T) {
	execution := model.Execution{
		ID: "exec-1", WorkflowID: "workflow-1", WorkflowRevision: 2,
		Mode: "SYNC", Status: model.ExecutionSucceeded, CorrelationID: "corr-1",
		CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(),
	}

	encoded, err := json.Marshal(execution)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	for _, key := range []string{
		"executionId", "workflowId", "workflowRevision", "mode",
		"status", "correlationId", "createdAt", "updatedAt", "isStalled",
	} {
		if _, exists := document[key]; !exists {
			t.Errorf("JSON property %q is absent", key)
		}
	}
	if document["status"] != model.ExecutionSucceeded {
		t.Fatalf("status = %v", document["status"])
	}
	if _, exists := document["CompanyID"]; exists {
		t.Fatal("internal company field was serialized")
	}
}

func TestNodeExecutionJSONContract(t *testing.T) {
	node := model.NodeExecution{
		ID: "node-exec-1", ExecutionID: "exec-1", NodeID: "node-1",
		Type: "core.pass-through", Version: "v1", Status: model.NodeRetryPending,
		Attempt: 2, CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(),
	}

	encoded, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	for _, key := range []string{
		"nodeExecutionId", "workflowExecutionId", "nodeId",
		"pluginType", "pluginVersion", "status", "attempt",
	} {
		if _, exists := document[key]; !exists {
			t.Errorf("JSON property %q is absent", key)
		}
	}
	if document["status"] != model.NodeRetryPending {
		t.Fatalf("status = %v", document["status"])
	}
}

func TestNodeJobJSONRoundTrip(t *testing.T) {
	job := model.NodeJob{
		CompanyID: "company-1", WorkflowID: "workflow-1", ExecutionID: "exec-1",
		NodeID: "node-1", NodeExecutionID: "node-exec-1", Attempt: 3,
		CorrelationID: "corr-1", Payload: map[string]any{"value": "payload"},
	}

	encoded, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded model.NodeJob
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.CompanyID != job.CompanyID || decoded.CorrelationID != job.CorrelationID {
		t.Fatalf("decoded job = %#v", decoded)
	}
	if decoded.Attempt != 3 {
		t.Fatalf("Attempt = %d", decoded.Attempt)
	}
}

func TestContractStatusStrings(t *testing.T) {
	statuses := map[string]string{
		"VALIDATING":    model.ExecutionValidating,
		"REJECTED":      model.ExecutionRejected,
		"RUNNING":       model.ExecutionRunning,
		"SUCCEEDED":     model.ExecutionSucceeded,
		"FAILED":        model.ExecutionFailed,
		"CANCELLED":     model.ExecutionCancelled,
		"READY":         model.NodeReady,
		"RETRY_PENDING": model.NodeRetryPending,
		"SKIPPED":       model.NodeSkipped,
	}
	for want, got := range statuses {
		if got != want {
			t.Errorf("status = %q, want %q", got, want)
		}
	}
}
