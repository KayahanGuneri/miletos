package engine_test

import (
	"context"
	"errors"
	"testing"

	"miletos-go/internal/engine"
	"miletos-go/internal/model"
)

func TestCreateWorkflowRejectsInvalidWorkflowBeforePersistence(t *testing.T) {
	registry := engine.NewNodeRegistry()
	service := engine.NewWorkflowService(nil, registry)
	workflow := model.Workflow{
		CompanyID: "company-1", Name: "Invalid", Revision: 1,
		Nodes: []model.Node{{ID: "node-1", Type: "unknown"}},
	}

	_, err := service.CreateWorkflow(context.Background(), workflow)

	if !errors.Is(err, engine.ErrInvalidWorkflow) {
		t.Fatalf("CreateWorkflow() error = %v, want ErrInvalidWorkflow", err)
	}
}

func TestCreateWorkflowRejectsUndefinedNodeBeforePersistence(t *testing.T) {
	registry := engine.NewNodeRegistry()
	service := engine.NewWorkflowService(nil, registry)
	workflow := model.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Invalid", Revision: 1,
		Nodes: []model.Node{{
			ID: "node-1", Type: "unknown",
			Configuration: map[string]any{"preserved": true},
		}},
		Metadata: map[string]any{"preserved": true},
	}

	_, err := service.CreateWorkflow(context.Background(), workflow)

	if !errors.Is(err, engine.ErrInvalidWorkflow) {
		t.Fatalf("CreateWorkflow() error = %v, want ErrInvalidWorkflow", err)
	}
}
