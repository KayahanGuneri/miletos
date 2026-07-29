package modules_test

import (
	"reflect"
	"testing"

	"miletos-go/internal/engine/modules"
	"miletos-go/internal/model"
)

func graphWorkflow(nodeIDs []string, edges ...model.Edge) model.Workflow {
	nodes := make([]model.Node, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		nodes = append(nodes, model.Node{ID: id, Type: "test.node"})
	}
	return model.Workflow{Nodes: nodes, Edges: edges}
}

func graphEdge(id, source, target string) model.Edge {
	return model.Edge{
		ID: id, SourceNodeID: source, TargetNodeID: target,
		SourceOutputPort: "output", TargetInputPort: "input",
	}
}

func TestTopologicalOrder(t *testing.T) {
	tests := []struct {
		name     string
		workflow model.Workflow
		want     []string
	}{
		{name: "single", workflow: graphWorkflow([]string{"only"}), want: []string{"only"}},
		{
			name: "linear",
			workflow: graphWorkflow(
				[]string{"third", "first", "second"},
				graphEdge("e1", "first", "second"),
				graphEdge("e2", "second", "third"),
			),
			want: []string{"first", "second", "third"},
		},
		{
			name:     "multiple roots are deterministic",
			workflow: graphWorkflow([]string{"z", "a", "m"}),
			want:     []string{"a", "m", "z"},
		},
		{
			name: "fan out",
			workflow: graphWorkflow(
				[]string{"root", "right", "left"},
				graphEdge("e1", "root", "right"),
				graphEdge("e2", "root", "left"),
			),
			want: []string{"root", "left", "right"},
		},
		{
			name: "fan in",
			workflow: graphWorkflow(
				[]string{"join", "right", "left"},
				graphEdge("e1", "right", "join"),
				graphEdge("e2", "left", "join"),
			),
			want: []string{"left", "right", "join"},
		},
		{
			name:     "disconnected nodes remain schedulable",
			workflow: graphWorkflow([]string{"connected", "isolated"}),
			want:     []string{"connected", "isolated"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := modules.TopologicalOrder(test.workflow)
			if err != nil {
				t.Fatalf("TopologicalOrder() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("TopologicalOrder() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTopologicalOrderRejectsInvalidGraphShapes(t *testing.T) {
	tests := []struct {
		name     string
		workflow model.Workflow
	}{
		{
			name: "self loop",
			workflow: graphWorkflow(
				[]string{"node"},
				graphEdge("e1", "node", "node"),
			),
		},
		{
			name: "direct cycle",
			workflow: graphWorkflow(
				[]string{"a", "b"},
				graphEdge("e1", "a", "b"),
				graphEdge("e2", "b", "a"),
			),
		},
		{
			name: "indirect cycle",
			workflow: graphWorkflow(
				[]string{"a", "b", "c"},
				graphEdge("e1", "a", "b"),
				graphEdge("e2", "b", "c"),
				graphEdge("e3", "c", "a"),
			),
		},
		{
			name:     "duplicate node IDs",
			workflow: graphWorkflow([]string{"duplicate", "duplicate"}),
		},
		{
			name: "missing source",
			workflow: graphWorkflow(
				[]string{"target"},
				graphEdge("e1", "missing", "target"),
			),
		},
		{
			name: "missing target",
			workflow: graphWorkflow(
				[]string{"source"},
				graphEdge("e1", "source", "missing"),
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := modules.TopologicalOrder(test.workflow); err == nil {
				t.Fatal("TopologicalOrder() error = nil")
			}
		})
	}
}

func TestPredecessorsAndDownstream(t *testing.T) {
	workflow := graphWorkflow(
		[]string{"root", "left", "right", "join", "independent"},
		graphEdge("e1", "root", "right"),
		graphEdge("e2", "root", "left"),
		graphEdge("e3", "right", "join"),
		graphEdge("e4", "left", "join"),
	)

	if got := modules.Predecessors(workflow, "join"); !reflect.DeepEqual(got, []string{"left", "right"}) {
		t.Fatalf("Predecessors() = %v", got)
	}
	downstream := modules.Downstream(workflow, []string{"left"})
	for _, nodeID := range []string{"left", "join"} {
		if !downstream[nodeID] {
			t.Errorf("Downstream() does not include %q", nodeID)
		}
	}
	for _, nodeID := range []string{"root", "right", "independent"} {
		if downstream[nodeID] {
			t.Errorf("Downstream() unexpectedly includes %q", nodeID)
		}
	}
}
