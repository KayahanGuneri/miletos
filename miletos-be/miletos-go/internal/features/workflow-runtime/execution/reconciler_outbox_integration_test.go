//go:build integration

package execution_test

import (
	"context"
	"testing"
	"time"

	executionfeature "miletos-go/internal/features/workflow-runtime/execution"
	repository "miletos-go/internal/features/workflow-runtime/execution/repository"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
)

func TestReconcilerRepairsStaleQueuedNodeWithDurableOutboxCommandIntegration(
	t *testing.T,
) {
	ctx := context.Background()
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()

	execution := seedEngineExecution(
		t,
		pool,
		workflow,
		"ASYNC",
	)

	client := engineDatabaseClient(t)
	workflows := workflowfeature.NewWorkflowRepository(client)
	executions := repository.NewExecutionRepository(client)
	outbox := repository.NewOutboxRepository(client)
	nodeQueue := &recordingQueue{}

	const commandTopic = "miletos.workflow.node.commands.v1"

	scheduler := newEngineScheduler(
		t,
		workflows,
		executions,
		nodeQueue,
		commandTopic,
	)

	scheduled, err := scheduler.Activate(
		ctx,
		execution,
		nil,
	)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if scheduled != 1 {
		t.Fatalf(
			"Activate() scheduled = %d, want 1",
			scheduled,
		)
	}
	if jobs := nodeQueue.snapshot(); len(jobs) != 0 {
		t.Fatalf(
			"scheduler published directly before reconciliation: %#v",
			jobs,
		)
	}

	if commandCount := countNodeCommandOutbox(
		t,
		pool,
		workflow.CompanyID,
		execution.ID,
	); commandCount != 1 {
		t.Fatalf(
			"initial durable command count = %d, want 1",
			commandCount,
		)
	}

	deleted, err := pool.Exec(
		ctx,
		`
			DELETE FROM workflow_runtime.outbox_messages
			WHERE company_id = $1
			  AND workflow_execution_id = $2
			  AND node_id = $3`,
		workflow.CompanyID,
		execution.ID,
		"root",
	)
	if err != nil {
		t.Fatalf(
			"delete initial node command: %v",
			err,
		)
	}
	if deleted.RowsAffected() != 1 {
		t.Fatalf(
			"deleted node commands = %d, want 1",
			deleted.RowsAffected(),
		)
	}

	reconciler, err := executionfeature.NewReconciler(
		executions,
		workflows,
		scheduler,
		nil,
		outbox,
		commandTopic,
		time.Second,
		time.Nanosecond,
		time.Second,
		time.Second,
	)
	if err != nil {
		t.Fatalf(
			"NewReconciler() error = %v",
			err,
		)
	}

	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatalf(
			"RunOnce() error = %v",
			err,
		)
	}

	if jobs := nodeQueue.snapshot(); len(jobs) != 0 {
		t.Fatalf(
			"reconciler published directly to queue: %#v",
			jobs,
		)
	}

	if commandCount := countNodeCommandOutbox(
		t,
		pool,
		workflow.CompanyID,
		execution.ID,
	); commandCount != 1 {
		t.Fatalf(
			"repaired durable command count = %d, want 1",
			commandCount,
		)
	}

	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatalf(
			"replayed RunOnce() error = %v",
			err,
		)
	}

	if jobs := nodeQueue.snapshot(); len(jobs) != 0 {
		t.Fatalf(
			"replayed reconciliation published directly: %#v",
			jobs,
		)
	}

	if commandCount := countNodeCommandOutbox(
		t,
		pool,
		workflow.CompanyID,
		execution.ID,
	); commandCount != 1 {
		t.Fatalf(
			"replayed durable command count = %d, want 1",
			commandCount,
		)
	}
}
