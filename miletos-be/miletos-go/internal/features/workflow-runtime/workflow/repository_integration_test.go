//go:build integration

package workflow_test

import (
	"context"
	"reflect"
	"testing"

	model "miletos-go/internal/features/workflow-runtime/execution/model"
	executionrepository "miletos-go/internal/features/workflow-runtime/execution/repository"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
)

func TestWorkflowRepositoryIntegration(t *testing.T) {
	pool := integrationDatabase(t)
	_ = pool
	workflow := integrationWorkflow()
	client := workflowIntegrationDatabaseClient(t)
	repository := workflowfeature.NewWorkflowRepository(client)

	first, err := repository.Save(context.Background(), workflow)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	second, err := repository.Save(context.Background(), workflow)
	if err != nil {
		t.Fatalf("duplicate Save() error = %v", err)
	}
	if first.ID == second.ID {
		t.Fatal("duplicate snapshots reused an ID")
	}

	execution, err := executionrepository.NewExecutionRepository(client).Create(
		context.Background(), workflow, first.ID, "SYNC",
		model.ExecutionOriginManualDirect, "corr-1", "", "",
	)
	if err != nil {
		t.Fatalf("Create() execution error = %v", err)
	}
	found, err := repository.FindByExecutionID(
		context.Background(), workflow.CompanyID, execution.ID,
	)
	if err != nil {
		t.Fatalf("FindByExecutionID() error = %v", err)
	}
	if found.Workflow.ID != workflow.ID ||
		!reflect.DeepEqual(found.Workflow.Nodes[0].Configuration, workflow.Nodes[0].Configuration) ||
		!reflect.DeepEqual(found.Workflow.Metadata, workflow.Metadata) {
		t.Fatalf("workflow round trip = %#v", found.Workflow)
	}
	if _, err := repository.FindByExecutionID(
		context.Background(), "another-company", execution.ID,
	); err != workflowfeature.ErrNotFound {
		t.Fatalf("company-isolated lookup error = %v, want ErrNotFound", err)
	}
}
