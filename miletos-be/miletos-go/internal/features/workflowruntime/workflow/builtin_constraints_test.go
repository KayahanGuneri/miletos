package workflow_test

import (
	"errors"
	"testing"

	"miletos-go/internal/features/workflowruntime/plugin"
	workflow "miletos-go/internal/features/workflowruntime/workflow"
)

func TestBuiltinJoinAndPassThroughInputConstraints(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	tests := []struct {
		name       string
		targetType string
		edges      []workflow.Edge
		wantCode   string
	}{
		{
			name: "join requires two incoming edges", targetType: "core.join",
			edges:    []workflow.Edge{builtinEdge("edge-1", "root-1", "target")},
			wantCode: "INPUT_EDGE_COUNT_BELOW_MINIMUM",
		},
		{
			name:       "pass-through permits at most one incoming edge",
			targetType: "core.pass-through",
			edges: []workflow.Edge{
				builtinEdge("edge-1", "root-1", "target"),
				builtinEdge("edge-2", "root-2", "target"),
			},
			wantCode: "INPUT_EDGE_COUNT_ABOVE_MAXIMUM",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := workflow.Workflow{
				ID: "workflow-1", CompanyID: "company-1", Name: "Workflow", Revision: 1,
				Nodes: []workflow.WorkflowNode{
					{
						ID: "root-1", Type: "core.static-input", Version: "v1",
						Configuration: map[string]any{"value": "one"},
					},
					{
						ID: "root-2", Type: "core.static-input", Version: "v1",
						Configuration: map[string]any{"value": "two"},
					},
					{ID: "target", Type: test.targetType, Version: "v1"},
				},
				Edges: test.edges,
			}
			err := workflow.ValidateWorkflowDefinition(
				definition,
				registry.Definition,
				registry.HasType,
				registry.ValidateConfiguration,
			)
			var validation *workflow.WorkflowDefinitionValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("validation error = %v", err)
			}
			found := false
			for _, issue := range validation.Issues {
				if issue.Code == test.wantCode && issue.NodeID == "target" {
					found = true
				}
			}
			if !found {
				t.Fatalf("issues = %#v, want %s for target", validation.Issues, test.wantCode)
			}
		})
	}
}

func TestHTTPTriggerIsAcceptedAsTriggerRoot(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	definition := workflow.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Workflow", Revision: 1,
		Nodes: []workflow.WorkflowNode{
			{
				ID: "trigger", Type: "core.http-trigger", Version: "v1",
				Configuration: map[string]any{"method": "POST"},
			},
			{ID: "terminal", Type: "core.terminal", Version: "v1"},
		},
		Edges: []workflow.Edge{builtinEdge("edge-1", "trigger", "terminal")},
	}
	if err := workflow.ValidateWorkflowDefinition(
		definition,
		registry.Definition,
		registry.HasType,
		registry.ValidateConfiguration,
	); err != nil {
		t.Fatalf("HTTP trigger root validation error = %v", err)
	}
}

func builtinEdge(id, source, target string) workflow.Edge {
	return workflow.Edge{
		ID: id, SourceNodeID: source, TargetNodeID: target,
		SourceOutputPort: "output", TargetInputPort: "input",
	}
}
