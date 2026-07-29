//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"miletos-go/internal/model"
)

func TestNodeRepositorySuccessIntegration(t *testing.T) {
	pool := integrationDatabase(t)
	workflow := integrationWorkflow()
	_, execution := seedIntegrationExecution(t, pool, workflow, "SYNC")
	repository := NewExecutionRepository(pool)

	nodes, err := repository.ListAllNodes(context.Background(), workflow.CompanyID, execution.ID)
	if err != nil || len(nodes) != len(workflow.Nodes) {
		t.Fatalf("ListAllNodes() = (%#v, %v)", nodes, err)
	}
	queued, changed, err := repository.MarkNodeQueued(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil || !changed {
		t.Fatalf("MarkNodeQueued() = (%#v, %v, %v)", queued, changed, err)
	}
	if _, changed, err := repository.MarkNodeQueued(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	); err != nil || changed {
		t.Fatalf("duplicate MarkNodeQueued() changed=%v err=%v", changed, err)
	}
	job := model.NodeJob{
		CompanyID: workflow.CompanyID, WorkflowID: workflow.ID, ExecutionID: execution.ID,
		NodeID: "root", NodeExecutionID: queued.ID, Attempt: queued.Attempt,
		CorrelationID: "corr-1", Payload: map[string]any{"input": "payload"},
	}
	started, err := repository.MarkNodeRunning(context.Background(), job)
	if err != nil || !started {
		t.Fatalf("MarkNodeRunning() = (%v, %v)", started, err)
	}
	if err := repository.SaveNodeSuccess(
		context.Background(), job, map[string]any{"result": "success"},
	); err != nil {
		t.Fatalf("SaveNodeSuccess() error = %v", err)
	}
	succeeded, err := repository.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil {
		t.Fatalf("FindNode() error = %v", err)
	}
	if succeeded.Status != model.NodeSucceeded ||
		succeeded.Input["input"] != "payload" ||
		succeeded.Output["result"] != "success" {
		t.Fatalf("succeeded node = %#v", succeeded)
	}
}

func TestNodeRepositoryRetryAndPaginationIntegration(t *testing.T) {
	pool := integrationDatabase(t)
	workflow := integrationWorkflow()
	_, execution := seedIntegrationExecution(t, pool, workflow, "ASYNC")
	repository := NewExecutionRepository(pool)
	page, err := repository.ListNodes(
		context.Background(), workflow.CompanyID, execution.ID, "", 1,
	)
	if err != nil || len(page.Items) != 1 || !page.HasNext {
		t.Fatalf("ListNodes() = (%#v, %v)", page, err)
	}
	nextPage, err := repository.ListNodes(
		context.Background(), workflow.CompanyID, execution.ID, page.Next, 1,
	)
	if err != nil || len(nextPage.Items) != 1 {
		t.Fatalf("second ListNodes() = (%#v, %v)", nextPage, err)
	}

	queued, _, err := repository.MarkNodeQueued(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil {
		t.Fatalf("MarkNodeQueued() error = %v", err)
	}
	job := model.NodeJob{
		CompanyID: workflow.CompanyID, WorkflowID: workflow.ID, ExecutionID: execution.ID,
		NodeID: "root", NodeExecutionID: queued.ID, Attempt: queued.Attempt,
	}
	if started, err := repository.MarkNodeRunning(context.Background(), job); err != nil || !started {
		t.Fatalf("MarkNodeRunning() = (%v, %v)", started, err)
	}
	running, err := repository.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil || running.StartedAt == nil {
		t.Fatalf("running FindNode() = (%#v, %v)", running, err)
	}
	failedAt := *running.StartedAt
	nextAttempt := failedAt.Add(time.Second)
	if err := repository.SaveNodeFailure(
		context.Background(), job,
		map[string]any{"category": "EXECUTION", "code": "FAILED", "message": "failed"},
		true, failedAt, nextAttempt, 3, time.Second,
	); err != nil {
		t.Fatalf("SaveNodeFailure() error = %v", err)
	}
	retrying, err := repository.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil || retrying.Status != model.NodeRetryPending {
		t.Fatalf("retrying node = (%#v, %v)", retrying, err)
	}
	if !retrying.UpdatedAt.Equal(failedAt) {
		t.Fatalf("retrying node updated_at = %v, want %v", retrying.UpdatedAt, failedAt)
	}
	var attemptFinishedAt, attemptUpdatedAt, attemptNextAt, errorCreatedAt time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT finished_at, updated_at, next_attempt_at
		FROM workflow_runtime.node_execution_attempts
		WHERE workflow_execution_id = $1 AND node_execution_id = $2 AND attempt = 1`,
		execution.ID, job.NodeExecutionID,
	).Scan(&attemptFinishedAt, &attemptUpdatedAt, &attemptNextAt); err != nil {
		t.Fatalf("load failed retry attempt timestamps: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT created_at
		FROM workflow_runtime.execution_errors
		WHERE workflow_execution_id = $1 AND node_execution_id = $2
		ORDER BY created_at LIMIT 1`,
		execution.ID, job.NodeExecutionID,
	).Scan(&errorCreatedAt); err != nil {
		t.Fatalf("load execution error timestamp: %v", err)
	}
	if !attemptFinishedAt.Equal(failedAt) ||
		!attemptUpdatedAt.Equal(failedAt) ||
		!errorCreatedAt.Equal(failedAt) ||
		!attemptNextAt.Equal(nextAttempt) {
		t.Fatalf(
			"failure timestamps = node:%v attempt-finished:%v attempt-updated:%v error:%v next:%v",
			retrying.UpdatedAt, attemptFinishedAt, attemptUpdatedAt, errorCreatedAt, attemptNextAt,
		)
	}
	retryJob, err := repository.PrepareRetry(context.Background(), job)
	if err != nil {
		t.Fatalf("PrepareRetry() error = %v", err)
	}
	if retryJob.Attempt != 2 {
		t.Fatalf("retry attempt = %d", retryJob.Attempt)
	}
	if started, err := repository.MarkNodeRunning(context.Background(), retryJob); err != nil || !started {
		t.Fatalf("retry MarkNodeRunning() = (%v, %v)", started, err)
	}
	secondRunning, err := repository.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil || secondRunning.StartedAt == nil {
		t.Fatalf("second running FindNode() = (%#v, %v)", secondRunning, err)
	}
	if err := repository.SaveNodeFailure(
		context.Background(), retryJob,
		map[string]any{"category": "EXECUTION", "code": "FAILED", "message": "terminal"},
		false, secondRunning.UpdatedAt, time.Time{}, 3, time.Second,
	); err != nil {
		t.Fatalf("terminal SaveNodeFailure() error = %v", err)
	}
	failed, err := repository.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil || failed.Status != model.NodeFailed || failed.Attempt != 2 {
		t.Fatalf("failed node = (%#v, %v)", failed, err)
	}
}
