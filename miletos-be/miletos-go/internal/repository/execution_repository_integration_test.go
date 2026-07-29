//go:build integration

package repository

import (
	"context"
	"testing"

	"miletos-go/internal/model"
)

func TestExecutionRepositoryIntegration(t *testing.T) {
	pool := integrationDatabase(t)
	workflow := integrationWorkflow()
	_, first := seedIntegrationExecution(t, pool, workflow, "ASYNC")
	repository := NewExecutionRepository(pool)

	found, err := repository.FindByID(context.Background(), workflow.CompanyID, first.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if found.WorkflowID != workflow.ID || found.Status != model.ExecutionCreated {
		t.Fatalf("execution = %#v", found)
	}
	if _, err := repository.FindByID(
		context.Background(), "another-company", first.ID,
	); err != ErrNotFound {
		t.Fatalf("company-isolated FindByID() error = %v", err)
	}
	if _, err := repository.FindByID(
		context.Background(), workflow.CompanyID, "missing",
	); err != ErrNotFound {
		t.Fatalf("missing FindByID() error = %v", err)
	}

	_, second := seedIntegrationExecution(t, pool, workflow, "SYNC")
	page, err := repository.List(
		context.Background(), workflow.CompanyID, workflow.ID, model.ExecutionCreated, "", 1,
	)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Items) != 1 || !page.HasNext || page.Next == "" {
		t.Fatalf("first page = %#v", page)
	}
	next, err := repository.List(
		context.Background(), workflow.CompanyID, workflow.ID,
		model.ExecutionCreated, page.Next, 1,
	)
	if err != nil {
		t.Fatalf("second List() error = %v", err)
	}
	if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatalf("second page = %#v", next)
	}

	if err := repository.MarkExecutionQueued(context.Background(), second.ID); err != nil {
		t.Fatalf("MarkExecutionQueued() error = %v", err)
	}
	if err := repository.MarkExecutionRunning(context.Background(), second.ID); err != nil {
		t.Fatalf("MarkExecutionRunning() error = %v", err)
	}
	if err := repository.Finalize(
		context.Background(), second.ID, model.ExecutionSucceeded,
		map[string]any{"terminal": "output"}, nil,
	); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	completed, err := repository.FindByID(context.Background(), workflow.CompanyID, second.ID)
	if err != nil {
		t.Fatalf("completed FindByID() error = %v", err)
	}
	if completed.Status != model.ExecutionSucceeded ||
		completed.TerminalOutputs["terminal"] != "output" ||
		completed.FinishedAt == nil {
		t.Fatalf("completed execution = %#v", completed)
	}
}

func TestExecutionRepositoryIdempotencyIntegration(t *testing.T) {
	pool := integrationDatabase(t)
	workflow := integrationWorkflow()
	workflowRepository := NewWorkflowRepository(pool)
	repository := NewExecutionRepository(pool)
	snapshot, err := workflowRepository.Save(context.Background(), workflow)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	created, err := repository.Create(
		context.Background(), workflow, snapshot.ID, "ASYNC", "corr-1", "key-1", "fingerprint-1",
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	found, replayed, err := repository.FindIdempotent(
		context.Background(), workflow.CompanyID, "key-1", "fingerprint-1",
	)
	if err != nil || !replayed || found.ID != created.ID {
		t.Fatalf("FindIdempotent() = (%#v, %v, %v)", found, replayed, err)
	}
	if _, _, err := repository.FindIdempotent(
		context.Background(), workflow.CompanyID, "key-1", "different",
	); err != ErrIdempotencyConflict {
		t.Fatalf("conflicting FindIdempotent() error = %v", err)
	}
}
