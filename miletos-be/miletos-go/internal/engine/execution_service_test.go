package engine_test

import (
	"context"
	"errors"
	"testing"

	"miletos-go/internal/engine"
	"miletos-go/internal/model"
)

func TestExecutionServiceRejectsAsyncWhenUnavailable(t *testing.T) {
	service := engine.NewExecutionService(nil, nil, nil, nil, false)

	_, err := service.ExecuteAsync(
		context.Background(), model.Workflow{}, nil, "corr-1", "key-1", "fingerprint",
	)

	if !errors.Is(err, engine.ErrAsyncUnavailable) {
		t.Fatalf("ExecuteAsync() error = %v, want ErrAsyncUnavailable", err)
	}
}

func TestExecutionServiceRejectsInvalidSyncWorkflowBeforePersistence(t *testing.T) {
	workflowService := engine.NewWorkflowService(nil, engine.NewNodeRegistry())
	service := engine.NewExecutionService(workflowService, nil, nil, nil, false)

	_, err := service.ExecuteSync(context.Background(), model.Workflow{}, nil, "corr-1")

	if !errors.Is(err, engine.ErrInvalidWorkflow) {
		t.Fatalf("ExecuteSync() error = %v, want ErrInvalidWorkflow", err)
	}
}
