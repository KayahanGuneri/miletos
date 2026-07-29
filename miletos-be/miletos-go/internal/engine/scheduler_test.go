package engine_test

import (
	"testing"

	"miletos-go/internal/engine"
	"miletos-go/internal/model"
)

func TestSchedulerBlockedDecision(t *testing.T) {
	workflow := model.Workflow{
		Nodes: []model.Node{{ID: "root"}, {ID: "dependent"}, {ID: "independent"}},
		Edges: []model.Edge{{
			ID: "edge-1", SourceNodeID: "root", TargetNodeID: "dependent",
			SourceOutputPort: "output", TargetInputPort: "input",
		}},
	}
	scheduler := engine.NewScheduler(nil, nil, nil, "")

	if !scheduler.Blocked(workflow, "dependent", map[string]bool{"root": true}) {
		t.Fatal("dependent node was not blocked")
	}
	if scheduler.Blocked(workflow, "independent", map[string]bool{"root": true}) {
		t.Fatal("independent branch was blocked")
	}
	if scheduler.Blocked(workflow, "dependent", map[string]bool{}) {
		t.Fatal("dependent node blocked without failed predecessor")
	}
}
