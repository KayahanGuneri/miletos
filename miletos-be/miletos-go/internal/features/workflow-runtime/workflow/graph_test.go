package workflow_test

import (
	"reflect"
	"testing"

	workflow "miletos-go/internal/features/workflow-runtime/workflow"
)

func graphWorkflow(nodeIDs []string, edges ...workflow.Edge) workflow.Workflow {
	nodes := make([]workflow.WorkflowNode, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		nodes = append(nodes, workflow.WorkflowNode{ID: id, Type: "test.node"})
	}
	return workflow.Workflow{Nodes: nodes, Edges: edges}
}

func graphEdge(id, source, target string) workflow.Edge {
	return workflow.Edge{
		ID: id, SourceNodeID: source, TargetNodeID: target,
		SourceOutputPort: "output", TargetInputPort: "input",
	}
}

func TestTopologicalOrder(t *testing.T) {
	tests := []struct {
		name     string
		workflow workflow.Workflow
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
			got, err := workflow.TopologicalOrder(test.workflow)
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
		workflow workflow.Workflow
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
			if _, err := workflow.TopologicalOrder(test.workflow); err == nil {
				t.Fatal("TopologicalOrder() error = nil")
			}
		})
	}
}

func TestFindCyclePathIsCanonicalAndDeterministic(t *testing.T) {
	definition := graphWorkflow(
		[]string{"z-isolated", "cycle-c", "cycle-a", "cycle-b", "a-isolated"},
		graphEdge("e3", "cycle-c", "cycle-a"),
		graphEdge("e1", "cycle-a", "cycle-b"),
		graphEdge("e2", "cycle-b", "cycle-c"),
	)
	want := []string{"cycle-a", "cycle-b", "cycle-c", "cycle-a"}
	for attempt := 0; attempt < 20; attempt++ {
		if got := workflow.FindCyclePath(definition); !reflect.DeepEqual(got, want) {
			t.Fatalf("FindCyclePath() attempt %d = %v, want %v", attempt, got, want)
		}
	}
}

func TestFindCyclePathSupportsSelfLoopsAndDisconnectedComponents(t *testing.T) {
	t.Run("self loop", func(t *testing.T) {
		definition := graphWorkflow(
			[]string{"other", "self"},
			graphEdge("self-edge", "self", "self"),
		)
		if got := workflow.FindCyclePath(definition); !reflect.DeepEqual(got, []string{"self", "self"}) {
			t.Fatalf("FindCyclePath() = %v", got)
		}
	})
	t.Run("cycle in later component", func(t *testing.T) {
		definition := graphWorkflow(
			[]string{"a-root", "a-leaf", "z-left", "z-right"},
			graphEdge("tree", "a-root", "a-leaf"),
			graphEdge("cycle-1", "z-left", "z-right"),
			graphEdge("cycle-2", "z-right", "z-left"),
		)
		if got := workflow.FindCyclePath(definition); !reflect.DeepEqual(
			got, []string{"z-left", "z-right", "z-left"},
		) {
			t.Fatalf("FindCyclePath() = %v", got)
		}
	})
}

func TestPredecessorsAndDownstream(t *testing.T) {
	definition := graphWorkflow(
		[]string{"root", "left", "right", "join", "independent"},
		graphEdge("e1", "root", "right"),
		graphEdge("e2", "root", "left"),
		graphEdge("e3", "right", "join"),
		graphEdge("e4", "left", "join"),
	)

	if got := workflow.Predecessors(definition, "join"); !reflect.DeepEqual(got, []string{"left", "right"}) {
		t.Fatalf("Predecessors() = %v", got)
	}
	downstream := workflow.Downstream(definition, []string{"left"})
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
