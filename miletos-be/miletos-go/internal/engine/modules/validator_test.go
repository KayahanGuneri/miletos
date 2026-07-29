package modules_test

import (
	"strings"
	"testing"

	"miletos-go/internal/engine/modules"
	"miletos-go/internal/model"
)

func validWorkflow() model.Workflow {
	return model.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Workflow", Revision: 1,
		Nodes: []model.Node{{
			ID: "node-1", Type: "test.node",
			Configuration: map[string]any{"dynamic": []any{"value", float64(2)}},
		}},
		Metadata: map[string]any{"owner": "test"},
	}
}

func TestValidateWorkflowRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.Workflow)
		want   string
	}{
		{name: "workflow ID", mutate: func(w *model.Workflow) { w.ID = " " }, want: "workflow id"},
		{name: "company ID", mutate: func(w *model.Workflow) { w.CompanyID = "" }, want: "company id"},
		{name: "workflow name", mutate: func(w *model.Workflow) { w.Name = "" }, want: "workflow name"},
		{name: "revision", mutate: func(w *model.Workflow) { w.Revision = 0 }, want: "revision"},
		{name: "empty nodes", mutate: func(w *model.Workflow) { w.Nodes = nil }, want: "at least one node"},
		{name: "node ID", mutate: func(w *model.Workflow) { w.Nodes[0].ID = "" }, want: "node id"},
		{name: "node type", mutate: func(w *model.Workflow) { w.Nodes[0].Type = "" }, want: "type"},
		{
			name: "duplicate node",
			mutate: func(w *model.Workflow) {
				w.Nodes = append(w.Nodes, w.Nodes[0])
			},
			want: "duplicated",
		},
		{
			name: "unsupported type",
			mutate: func(w *model.Workflow) {
				w.Nodes[0].Type = "unsupported"
			},
			want: "undefined",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := validWorkflow()
			test.mutate(&workflow)
			err := modules.ValidateWorkflow(workflow, func(nodeType string) bool {
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
		edge model.Edge
		want string
	}{
		{name: "missing edge ID", edge: graphEdge("", "node-1", "node-2"), want: "edge id"},
		{name: "missing source", edge: graphEdge("edge-1", "missing", "node-2"), want: "source"},
		{name: "missing target", edge: graphEdge("edge-1", "node-1", "missing"), want: "target"},
		{name: "self loop", edge: graphEdge("edge-1", "node-1", "node-1"), want: "itself"},
		{
			name: "missing source port",
			edge: model.Edge{
				ID: "edge-1", SourceNodeID: "node-1", TargetNodeID: "node-2",
				TargetInputPort: "input",
			},
			want: "ports",
		},
		{
			name: "missing target port",
			edge: model.Edge{
				ID: "edge-1", SourceNodeID: "node-1", TargetNodeID: "node-2",
				SourceOutputPort: "output",
			},
			want: "ports",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workflow := validWorkflow()
			workflow.Nodes = append(workflow.Nodes, model.Node{ID: "node-2", Type: "test.node"})
			workflow.Edges = []model.Edge{test.edge}
			err := modules.ValidateWorkflow(workflow, func(string) bool { return true })
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("ValidateWorkflow() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateWorkflowRejectsDuplicateEdgeAndCycle(t *testing.T) {
	workflow := validWorkflow()
	workflow.Nodes = append(workflow.Nodes, model.Node{ID: "node-2", Type: "test.node"})
	edge := graphEdge("edge-1", "node-1", "node-2")
	workflow.Edges = []model.Edge{edge, edge}
	if err := modules.ValidateWorkflow(workflow, func(string) bool { return true }); err == nil ||
		!strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate edge error = %v", err)
	}

	workflow.Edges = []model.Edge{
		graphEdge("edge-1", "node-1", "node-2"),
		graphEdge("edge-2", "node-2", "node-1"),
	}
	if err := modules.ValidateWorkflow(workflow, func(string) bool { return true }); err == nil ||
		!strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestValidateWorkflowAcceptsMinimalAndMultiNodeWorkflows(t *testing.T) {
	minimal := validWorkflow()
	if err := modules.ValidateWorkflow(minimal, func(string) bool { return true }); err != nil {
		t.Fatalf("minimal workflow error = %v", err)
	}

	multi := validWorkflow()
	multi.Nodes = append(multi.Nodes, model.Node{ID: "node-2", Type: "test.node"})
	multi.Edges = []model.Edge{graphEdge("edge-1", "node-1", "node-2")}
	if err := modules.ValidateWorkflow(multi, func(string) bool { return true }); err != nil {
		t.Fatalf("multi-node workflow error = %v", err)
	}
}

func TestValidateExecutionRequestLimits(t *testing.T) {
	workflow := validWorkflow()
	workflow.Nodes = make([]model.Node, 1001)
	if err := modules.ValidateExecutionRequest(workflow); err == nil {
		t.Fatal("node limit error = nil")
	}
	workflow.Nodes = nil
	workflow.Edges = make([]model.Edge, 5001)
	if err := modules.ValidateExecutionRequest(workflow); err == nil {
		t.Fatal("edge limit error = nil")
	}
}
