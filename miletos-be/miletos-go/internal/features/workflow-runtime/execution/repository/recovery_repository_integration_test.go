//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"

	model "miletos-go/internal/features/workflow-runtime/execution/model"
)

func TestRecoveryRepositoryIntegration(t *testing.T) {
	pool := integrationDatabase(t)
	workflow := integrationWorkflow()
	_, source := seedIntegrationExecution(t, pool, workflow, "ASYNC")
	_, recovery := seedIntegrationExecution(t, pool, workflow, "ASYNC")
	repository := NewExecutionRepository(integrationDatabaseClient(t))

	queued, _, err := repository.MarkNodeQueued(
		context.Background(), workflow.CompanyID, source.ID, "root",
	)
	if err != nil {
		t.Fatalf("MarkNodeQueued() error = %v", err)
	}
	job := model.NodeJob{
		CompanyID: workflow.CompanyID, WorkflowID: workflow.ID, ExecutionID: source.ID,
		NodeID: "root", NodeExecutionID: queued.ID, Attempt: queued.Attempt,
	}
	if started, err := repository.MarkNodeRunning(context.Background(), job); err != nil || !started {
		t.Fatalf("MarkNodeRunning() = (%v, %v)", started, err)
	}
	if err := repository.SaveNodeSuccess(
		context.Background(), job, map[string]any{"value": "preserved"}, model.NodeRoutingOutcome{},
	); err != nil {
		t.Fatalf("SaveNodeSuccess() error = %v", err)
	}
	if err := repository.CopySuccessfulNodes(
		context.Background(), source.ID, recovery.ID, []string{"root"},
	); err != nil {
		t.Fatalf("CopySuccessfulNodes() error = %v", err)
	}
	preserved, err := repository.FindNode(
		context.Background(), workflow.CompanyID, recovery.ID, "root",
	)
	if err != nil || preserved.Status != model.NodeSucceeded ||
		preserved.Output["value"] != "preserved" {
		t.Fatalf("preserved node = (%#v, %v)", preserved, err)
	}
	original, err := repository.FindNode(
		context.Background(), workflow.CompanyID, source.ID, "root",
	)
	if err != nil || original.Status != model.NodeSucceeded {
		t.Fatalf("original node = (%#v, %v)", original, err)
	}

	fingerprint := strings.Repeat("a", 64)
	if err := repository.SaveRecoveryRequest(
		context.Background(), workflow.CompanyID, "recovery-key", fingerprint,
		source.ID, recovery.ID, 1, 1, 1,
	); err != nil {
		t.Fatalf("SaveRecoveryRequest() error = %v", err)
	}
	sourceID, recoveryID, storedFingerprint, preservedCount, scheduled, reset, _, found, err :=
		repository.FindRecovery(context.Background(), workflow.CompanyID, "recovery-key")
	if err != nil || !found || sourceID != source.ID || recoveryID != recovery.ID ||
		storedFingerprint != fingerprint || preservedCount != 1 || scheduled != 1 || reset != 1 {
		t.Fatalf("FindRecovery() = (%q, %q, %q, %d, %d, %d, %v, %v)",
			sourceID, recoveryID, storedFingerprint, preservedCount, scheduled, reset, found, err)
	}
	if _, _, _, _, _, _, _, found, err := repository.FindRecovery(
		context.Background(), workflow.CompanyID, "missing",
	); err != nil || found {
		t.Fatalf("missing FindRecovery() found=%v err=%v", found, err)
	}
	if err := repository.SaveRecoveryRequest(
		context.Background(), workflow.CompanyID, "recovery-key", strings.Repeat("b", 64),
		source.ID, recovery.ID, 1, 1, 1,
	); err == nil {
		t.Fatal("conflicting recovery key error = nil")
	}
}
