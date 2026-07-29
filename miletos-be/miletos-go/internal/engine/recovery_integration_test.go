//go:build integration

package engine_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"miletos-go/internal/engine"
	"miletos-go/internal/model"
	"miletos-go/internal/repository"
)

func TestRecoveryPreservesSuccessfulNodesAndSchedulesResetNodesIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()
	source := seedEngineExecution(t, pool, workflow, "ASYNC")
	workflows := repository.NewWorkflowRepository(pool)
	executions := repository.NewExecutionRepository(pool)
	runNodeSuccess(t, executions, workflow, source, "root", "preserved-output")
	terminal, _, err := executions.MarkNodeQueued(
		context.Background(), workflow.CompanyID, source.ID, "terminal",
	)
	if err != nil {
		t.Fatalf("terminal MarkNodeQueued() error = %v", err)
	}
	terminalJob := model.NodeJob{
		CompanyID: workflow.CompanyID, WorkflowID: workflow.ID, ExecutionID: source.ID,
		NodeID: "terminal", NodeExecutionID: terminal.ID, Attempt: terminal.Attempt,
	}
	if started, err := executions.MarkNodeRunning(context.Background(), terminalJob); err != nil || !started {
		t.Fatalf("terminal MarkNodeRunning() = (%v, %v)", started, err)
	}
	failedAt := nodeStartedAt(t, pool, terminalJob.NodeExecutionID)
	if err := executions.SaveNodeFailure(
		context.Background(), terminalJob,
		map[string]any{"category": "EXECUTION", "code": "FAILED", "message": "failed"},
		false, failedAt, time.Time{}, 1, time.Second,
	); err != nil {
		t.Fatalf("terminal SaveNodeFailure() error = %v", err)
	}
	if err := executions.Finalize(
		context.Background(), source.ID, model.ExecutionFailed, map[string]any{},
		map[string]any{"message": "failed"},
	); err != nil {
		t.Fatalf("source Finalize() error = %v", err)
	}

	registry := engine.NewNodeRegistry()
	if err := engine.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	nodeQueue := &recordingQueue{}
	scheduler := engine.NewScheduler(workflows, executions, nodeQueue, "commands")
	recovery := engine.NewRecoveryService(
		workflows, executions, engine.NewWorkflowService(workflows, registry), scheduler,
	)
	fingerprint := strings.Repeat("a", 64)

	outcome, err := recovery.Recover(
		context.Background(), workflow.CompanyID, source.ID, "recovery-key", fingerprint,
	)

	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if outcome.RecoveryExecutionID == source.ID || outcome.PreservedNodeCount != 1 ||
		outcome.ResetNodeCount != 1 || outcome.ScheduledNodeCount != 1 {
		t.Fatalf("recovery outcome = %#v", outcome)
	}
	preserved, err := executions.FindNode(
		context.Background(), workflow.CompanyID, outcome.RecoveryExecutionID, "root",
	)
	if err != nil || preserved.Status != model.NodeSucceeded ||
		preserved.Output["value"] != "preserved-output" {
		t.Fatalf("preserved node = (%#v, %v)", preserved, err)
	}
	reset, err := executions.FindNode(
		context.Background(), workflow.CompanyID, outcome.RecoveryExecutionID, "terminal",
	)
	if err != nil || reset.Status != model.NodeQueued {
		t.Fatalf("reset node = (%#v, %v)", reset, err)
	}
	original, err := executions.FindByID(context.Background(), workflow.CompanyID, source.ID)
	if err != nil || original.Status != model.ExecutionFailed {
		t.Fatalf("original execution = (%#v, %v)", original, err)
	}
	replayed, err := recovery.Recover(
		context.Background(), workflow.CompanyID, source.ID, "recovery-key", fingerprint,
	)
	if err != nil || !replayed.Replayed ||
		replayed.RecoveryExecutionID != outcome.RecoveryExecutionID {
		t.Fatalf("replayed recovery = (%#v, %v)", replayed, err)
	}
	if _, err := recovery.Recover(
		context.Background(), workflow.CompanyID, "different-source",
		"recovery-key", strings.Repeat("b", 64),
	); !errors.Is(err, repository.ErrIdempotencyConflict) {
		t.Fatalf("conflicting recovery error = %v", err)
	}
}

func TestRecoveryRejectsMissingAndNonFailedSourcesIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()
	source := seedEngineExecution(t, pool, workflow, "ASYNC")
	workflows := repository.NewWorkflowRepository(pool)
	executions := repository.NewExecutionRepository(pool)
	registry := engine.NewNodeRegistry()
	if err := engine.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	scheduler := engine.NewScheduler(workflows, executions, &recordingQueue{}, "commands")
	recovery := engine.NewRecoveryService(
		workflows, executions, engine.NewWorkflowService(workflows, registry), scheduler,
	)

	if _, err := recovery.Recover(
		context.Background(), workflow.CompanyID, "missing", "key-1", strings.Repeat("a", 64),
	); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("missing source error = %v", err)
	}
	if _, err := recovery.Recover(
		context.Background(), workflow.CompanyID, source.ID, "key-2", strings.Repeat("b", 64),
	); !errors.Is(err, engine.ErrRecoveryUnsupported) {
		t.Fatalf("non-failed source error = %v", err)
	}
}
