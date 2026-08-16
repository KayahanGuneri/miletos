package workflow_test

import (
	"errors"
	"testing"

	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

func TestOutputDestinationTopology(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	if err := plugin.RegisterOutputDestinationNodes(registry, plugin.OutputNodeRuntime{
		OutputDirectory: t.TempDir(),
	}); err != nil {
		t.Fatalf("RegisterOutputDestinationNodes() error = %v", err)
	}

	valid := outputWorkflow([]workflow.Edge{outputEdge("edge-1", "source-1", "sink")})
	if err := validateWithRegistry(valid, registry); err != nil {
		t.Fatalf("processing -> output validation error = %v", err)
	}

	withDownstream := valid
	withDownstream.Nodes = append(withDownstream.Nodes, workflow.WorkflowNode{
		ID: "downstream", Type: "core.output", Version: "v1",
	})
	withDownstream.Edges = append(withDownstream.Edges, outputEdge("edge-2", "sink", "downstream"))
	assertWorkflowIssue(t, validateWithRegistry(withDownstream, registry), "UNKNOWN_OUTPUT_PORT", "sink")

	twoInputs := outputWorkflow([]workflow.Edge{
		outputEdge("edge-1", "source-1", "sink"),
		outputEdge("edge-2", "source-2", "sink"),
	})
	assertWorkflowIssue(t, validateWithRegistry(twoInputs, registry), "INPUT_EDGE_COUNT_ABOVE_MAXIMUM", "sink")
}

func outputWorkflow(edges []workflow.Edge) workflow.Workflow {
	return workflow.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Output workflow", Revision: 1,
		Nodes: []workflow.WorkflowNode{
			{ID: "source-1", Type: "core.static-input", Version: "v1", Configuration: map[string]any{"value": "one"}},
			{ID: "source-2", Type: "core.static-input", Version: "v1", Configuration: map[string]any{"value": "two"}},
			{ID: "sink", Type: "core.csv-output", Version: "v1", Configuration: map[string]any{"fileName": "result.csv"}},
		},
		Edges: edges,
	}
}

func outputEdge(id, source, target string) workflow.Edge {
	return workflow.Edge{
		ID: id, SourceNodeID: source, TargetNodeID: target,
		SourceOutputPort: "output", TargetInputPort: "input",
	}
}

func validateWithRegistry(definition workflow.Workflow, registry *plugin.NodeRegistry) error {
	return workflow.ValidateWorkflowDefinition(
		definition, registry.Get, registry.ValidateConfiguration, registry.ConnectionRestricted,
	)
}

func assertWorkflowIssue(t *testing.T, err error, code, nodeID string) {
	t.Helper()
	var validation *workflow.WorkflowDefinitionValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("validation error = %v", err)
	}
	for _, issue := range validation.Issues {
		if issue.Code == code && issue.NodeID == nodeID {
			return
		}
	}
	t.Fatalf("issues = %#v, want %s for %s", validation.Issues, code, nodeID)
}
