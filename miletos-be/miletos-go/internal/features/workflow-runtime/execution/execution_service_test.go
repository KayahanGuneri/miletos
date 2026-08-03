package execution_test

import (
	"context"
	"errors"
	"testing"

	execution "miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

func TestExecutionServiceRejectsAsyncWhenUnavailable(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	workflowService := workflow.NewWorkflowService(nil, registry)
	scheduler := execution.NewScheduler(nil, nil, nil, "", registry)
	service := execution.NewExecutionService(workflowService, nil, scheduler, nil, false)
	definition := workflow.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Workflow", Revision: 1,
		Nodes: []workflow.WorkflowNode{{
			ID: "root", Type: "core.static-input", Version: "v1",
			Configuration: map[string]any{"value": "payload"},
		}},
	}

	_, err := service.ExecuteAsync(
		context.Background(), definition, nil, "corr-1", "key-1", "fingerprint",
	)

	if !errors.Is(err, execution.ErrAsyncUnavailable) {
		t.Fatalf("ExecuteAsync() error = %v, want ErrAsyncUnavailable", err)
	}
}

func TestExecutionServiceRejectsInvalidSyncWorkflowBeforePersistence(t *testing.T) {
	workflowService := workflow.NewWorkflowService(nil, plugin.NewNodeRegistry())
	service := execution.NewExecutionService(workflowService, nil, nil, nil, false)

	_, err := service.ExecuteSync(
		context.Background(), workflow.Workflow{}, nil, "corr-1", "key-1", "fingerprint",
	)

	if !errors.Is(err, workflow.ErrInvalidWorkflow) {
		t.Fatalf("ExecuteSync() error = %v, want ErrInvalidWorkflow", err)
	}
}

func TestExecutionServiceRejectsStartInputForStaticEntryNodeBeforePersistence(t *testing.T) {
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	workflowService := workflow.NewWorkflowService(nil, registry)
	scheduler := execution.NewScheduler(nil, nil, nil, "", registry)
	service := execution.NewExecutionService(workflowService, nil, scheduler, nil, false)
	definition := workflow.Workflow{
		ID: "workflow-1", CompanyID: "company-1", Name: "Workflow", Revision: 1,
		Nodes: []workflow.WorkflowNode{{
			ID: "root", Type: "core.static-input", Version: "v1",
			Configuration: map[string]any{"value": "configured"},
		}},
	}

	_, err := service.ExecuteSync(
		context.Background(), definition, map[string]any{"spoof": true},
		"corr-1", "key-1", "fingerprint",
	)

	if !errors.Is(err, execution.ErrStartInputNotAccepted) {
		t.Fatalf("ExecuteSync() error = %v, want ErrStartInputNotAccepted", err)
	}
}
