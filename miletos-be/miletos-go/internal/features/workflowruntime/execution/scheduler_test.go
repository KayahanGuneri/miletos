package execution_test

import (
	"testing"

	execution "miletos-go/internal/features/workflowruntime/execution"
	"miletos-go/internal/features/workflowruntime/workflow"
)

func TestSchedulerBlockedDecision(t *testing.T) {
	definition := workflow.Workflow{
		Nodes: []workflow.WorkflowNode{{ID: "root"}, {ID: "dependent"}, {ID: "independent"}},
		Edges: []workflow.Edge{{
			ID: "edge-1", SourceNodeID: "root", TargetNodeID: "dependent",
			SourceOutputPort: "output", TargetInputPort: "input",
		}},
	}
	scheduler := execution.NewScheduler(nil, nil, nil, "")

	if !scheduler.Blocked(definition, "dependent", map[string]bool{"root": true}) {
		t.Fatal("dependent node was not blocked")
	}
	if scheduler.Blocked(definition, "independent", map[string]bool{"root": true}) {
		t.Fatal("independent branch was blocked")
	}
	if scheduler.Blocked(definition, "dependent", map[string]bool{}) {
		t.Fatal("dependent node blocked without failed predecessor")
	}
}
