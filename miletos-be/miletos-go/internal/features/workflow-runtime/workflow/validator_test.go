package workflow_test

import (
	"strings"
	"testing"

	workflow "miletos-go/internal/features/workflow-runtime/workflow"
)

func validWorkflow() workflow.Workflow {
	return workflow.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Workflow", Revision: 1,
		Nodes: []workflow.WorkflowNode{{
			ID: "node-1", Type: "test.node",
			Configuration: map[string]any{"dynamic": []any{"value", float64(2)}},
		}},
		Metadata: map[string]any{"owner": "test"},
	}
}

func TestValidateWorkflowRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*workflow.Workflow)
		want   string
	}{
		{name: "workflow ID", mutate: func(w *workflow.Workflow) { w.ID = " " }, want: "workflow id"},
		{name: "company ID", mutate: func(w *workflow.Workflow) { w.CompanyID = "" }, want: "company id"},
		{name: "workflow name", mutate: func(w *workflow.Workflow) { w.Name = "" }, want: "workflow name"},
		{name: "revision", mutate: func(w *workflow.Workflow) { w.Revision = 0 }, want: "revision"},
		{name: "empty nodes", mutate: func(w *workflow.Workflow) { w.Nodes = nil }, want: "at least one node"},
		{name: "node ID", mutate: func(w *workflow.Workflow) { w.Nodes[0].ID = "" }, want: "node id"},
		{name: "node type", mutate: func(w *workflow.Workflow) { w.Nodes[0].Type = "" }, want: "type"},
		{
			name: "duplicate node",
			mutate: func(w *workflow.Workflow) {
				w.Nodes = append(w.Nodes, w.Nodes[0])
			},
			want: "duplicated",
		},
		{
			name: "unsupported type",
			mutate: func(w *workflow.Workflow) {
				w.Nodes[0].Type = "unsupported"
			},
			want: "undefined",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validWorkflow()
			test.mutate(&definition)
			err := workflow.ValidateWorkflow(definition, func(nodeType string) bool {
				return nodeType == "test.node"
			})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("ValidateWorkflow() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateWorkflowRejectsInvalidEdges(t *testing.T) {
	tests := []struct {
		name string
		edge workflow.Edge
		want string
	}{
		{name: "missing edge ID", edge: graphEdge("", "node-1", "node-2"), want: "edge id"},
		{name: "missing source", edge: graphEdge("edge-1", "missing", "node-2"), want: "source"},
		{name: "missing target", edge: graphEdge("edge-1", "node-1", "missing"), want: "target"},
		{name: "self loop", edge: graphEdge("edge-1", "node-1", "node-1"), want: "itself"},
		{
			name: "missing source port",
			edge: workflow.Edge{
				ID: "edge-1", SourceNodeID: "node-1", TargetNodeID: "node-2",
				TargetInputPort: "input",
			},
			want: "ports",
		},
		{
			name: "missing target port",
			edge: workflow.Edge{
				ID: "edge-1", SourceNodeID: "node-1", TargetNodeID: "node-2",
				SourceOutputPort: "output",
			},
			want: "ports",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validWorkflow()
			definition.Nodes = append(
				definition.Nodes,
				workflow.WorkflowNode{ID: "node-2", Type: "test.node"},
			)
			definition.Edges = []workflow.Edge{test.edge}
			err := workflow.ValidateWorkflow(definition, func(string) bool { return true })
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("ValidateWorkflow() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateWorkflowRejectsDuplicateEdgeAndCycle(t *testing.T) {
	definition := validWorkflow()
	definition.Nodes = append(
		definition.Nodes,
		workflow.WorkflowNode{ID: "node-2", Type: "test.node"},
	)
	edge := graphEdge("edge-1", "node-1", "node-2")
	definition.Edges = []workflow.Edge{edge, edge}
	if err := workflow.ValidateWorkflow(definition, func(string) bool { return true }); err == nil ||
		!strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate edge error = %v", err)
	}

	definition.Edges = []workflow.Edge{
		graphEdge("edge-1", "node-1", "node-2"),
		graphEdge("edge-2", "node-2", "node-1"),
	}
	if err := workflow.ValidateWorkflow(definition, func(string) bool { return true }); err == nil ||
		!strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestValidateWorkflowAcceptsMinimalAndMultiNodeWorkflows(t *testing.T) {
	minimal := validWorkflow()
	if err := workflow.ValidateWorkflow(minimal, func(string) bool { return true }); err != nil {
		t.Fatalf("minimal workflow error = %v", err)
	}

	multi := validWorkflow()
	multi.Nodes = append(multi.Nodes, workflow.WorkflowNode{ID: "node-2", Type: "test.node"})
	multi.Edges = []workflow.Edge{graphEdge("edge-1", "node-1", "node-2")}
	if err := workflow.ValidateWorkflow(multi, func(string) bool { return true }); err != nil {
		t.Fatalf("multi-node workflow error = %v", err)
	}
}

func TestValidateExecutionRequestLimits(t *testing.T) {
	definition := validWorkflow()
	definition.Nodes = make([]workflow.WorkflowNode, 1001)
	if err := workflow.ValidateExecutionRequest(definition); err == nil {
		t.Fatal("node limit error = nil")
	}
	definition.Nodes = nil
	definition.Edges = make([]workflow.Edge, 5001)
	if err := workflow.ValidateExecutionRequest(definition); err == nil {
		t.Fatal("edge limit error = nil")
	}
}
