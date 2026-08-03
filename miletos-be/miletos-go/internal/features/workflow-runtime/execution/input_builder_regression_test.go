package execution_test

import (
	"reflect"
	"testing"

	execution "miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

func TestJoinInputPreservesImmutableEdgeOrderAndEveryInput(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	definition := workflow.Workflow{
		Nodes: []workflow.WorkflowNode{
			{ID: "left", Type: "core.pass-through", Version: "v1"},
			{ID: "right", Type: "core.pass-through", Version: "v1"},
			{ID: "join", Type: "core.join", Version: "v1"},
		},
		Edges: []workflow.Edge{
			{
				ID: "right-first", SourceNodeID: "right", TargetNodeID: "join",
				SourceOutputPort: "output", TargetInputPort: "input",
			},
			{
				ID: "left-second", SourceNodeID: "left", TargetNodeID: "join",
				SourceOutputPort: "output", TargetInputPort: "input",
			},
		},
	}
	input := execution.BuildNodeInput(
		definition,
		"join",
		map[string]any{
			"left":  map[string]any{"value": "left-output"},
			"right": map[string]any{"value": "right-output"},
		},
		registry,
	)
	mapped, ok := input.(map[string]any)
	if !ok {
		t.Fatalf("join input type = %T", input)
	}
	items, ok := mapped["inputs"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("join inputs = %#v, want two entries", mapped["inputs"])
	}
	want := []map[string]any{
		{
			"edgeId": "right-first", "sourceNodeId": "right",
			"sourceOutputPort": "output", "targetInputPort": "input",
			"value": "right-output",
		},
		{
			"edgeId": "left-second", "sourceNodeId": "left",
			"sourceOutputPort": "output", "targetInputPort": "input",
			"value": "left-output",
		},
	}
	for index, expected := range want {
		if !reflect.DeepEqual(items[index], expected) {
			t.Fatalf("input[%d] = %#v, want %#v", index, items[index], expected)
		}
	}
}
