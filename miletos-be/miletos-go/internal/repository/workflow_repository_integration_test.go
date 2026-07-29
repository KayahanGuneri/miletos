//go:build integration

package repository

import (
	"context"
	"reflect"
	"testing"
)

func TestWorkflowRepositoryIntegration(t *testing.T) {
	pool := integrationDatabase(t)
	workflow := integrationWorkflow()
	repository := NewWorkflowRepository(pool)

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

	execution, err := NewExecutionRepository(pool).Create(
		context.Background(), workflow, first.ID, "SYNC", "corr-1", "", "",
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
	); err != ErrNotFound {
		t.Fatalf("company-isolated lookup error = %v, want ErrNotFound", err)
	}
}
