package workflow_test

import (
	"context"
	"errors"
	"testing"

	"miletos-go/internal/features/workflowruntime/plugin"
	workflow "miletos-go/internal/features/workflowruntime/workflow"
)

func TestCreateWorkflowRejectsInvalidWorkflowBeforePersistence(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	service := workflow.NewWorkflowService(nil, registry)
	definition := workflow.Workflow{
		CompanyID: "company-1", Name: "Invalid", Revision: 1,
		Nodes: []workflow.WorkflowNode{{ID: "node-1", Type: "unknown"}},
	}

	_, err := service.CreateWorkflow(context.Background(), definition)

	if !errors.Is(err, workflow.ErrInvalidWorkflow) {
		t.Fatalf("CreateWorkflow() error = %v, want ErrInvalidWorkflow", err)
	}
}

func TestCreateWorkflowRejectsUndefinedNodeBeforePersistence(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	service := workflow.NewWorkflowService(nil, registry)
	definition := workflow.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Invalid", Revision: 1,
		Nodes: []workflow.WorkflowNode{{
			ID: "node-1", Type: "unknown",
			Configuration: map[string]any{"preserved": true},
		}},
		Metadata: map[string]any{"preserved": true},
	}

	_, err := service.CreateWorkflow(context.Background(), definition)

	if !errors.Is(err, workflow.ErrInvalidWorkflow) {
		t.Fatalf("CreateWorkflow() error = %v, want ErrInvalidWorkflow", err)
	}
}
