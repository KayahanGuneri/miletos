package execution

import (
	"testing"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
)

func TestMapGRPCNodeExecutionPreservesGenericNodeData(t *testing.T) {
	now := time.Now().UTC()
	mapped := mapGRPCNodeExecution(model.NodeExecution{
		ID: "node-exec", ExecutionID: "execution", NodeID: "node",
		Type: "plugin.example", Version: "v1", Status: model.NodeSucceeded,
		Attempt: 1, CreatedAt: now, UpdatedAt: now,
		Configuration: map[string]any{"setting": "configured"},
		Input:         map[string]any{"request": "value"},
		Output:        map[string]any{"result": "value"},
		Failure:       map[string]any{"code": "EXAMPLE"},
	})
	if mapped.GetConfiguration().AsMap()["setting"] != "configured" {
		t.Fatalf("configuration was not preserved: %v", mapped.GetConfiguration())
	}
	if mapped.GetInputSummary().AsMap()["request"] != "value" ||
		mapped.GetOutputSummary().AsMap()["result"] != "value" ||
		mapped.GetFailureSummary().AsMap()["code"] != "EXAMPLE" {
		t.Fatalf("generic node data was not preserved: %v", mapped)
	}
}
