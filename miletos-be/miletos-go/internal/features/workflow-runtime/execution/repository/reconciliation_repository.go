package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/persistence"
)

type ReconciliationThresholds struct {
	QueuedBefore       time.Time
	RunningBefore      time.Time
	RetryPendingBefore time.Time
}

func (repository *ExecutionRepository) FindStaleNodes(
	ctx context.Context,
	thresholds ReconciliationThresholds,
	limit int,
) ([]model.NodeExecution, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}

	_, err := repository.dbClient.Exec(ctx, `
		UPDATE workflow_runtime.workflow_executions execution
		SET is_stalled = EXISTS (
			SELECT 1
			FROM workflow_runtime.node_executions node
			WHERE node.company_id = execution.company_id
			  AND node.workflow_execution_id = execution.workflow_execution_id
			  AND (
				(node.status = 'QUEUED' AND node.queued_at < $1)
				OR (node.status = 'RUNNING' AND node.updated_at < $2)
				OR (
					node.status = 'RETRY_PENDING'
					AND node.next_attempt_at < $3
				)
			  )
		)
		WHERE execution.status IN ('CREATED', 'QUEUED', 'RUNNING')
		  AND execution.is_stalled IS DISTINCT FROM EXISTS (
			SELECT 1
			FROM workflow_runtime.node_executions node
			WHERE node.company_id = execution.company_id
			  AND node.workflow_execution_id = execution.workflow_execution_id
			  AND (
				(node.status = 'QUEUED' AND node.queued_at < $1)
				OR (node.status = 'RUNNING' AND node.updated_at < $2)
				OR (
					node.status = 'RETRY_PENDING'
					AND node.next_attempt_at < $3
				)
			  )
		  )`,
		thresholds.QueuedBefore,
		thresholds.RunningBefore,
		thresholds.RetryPendingBefore,
	)
	if err != nil {
		return nil, fmt.Errorf("refresh stalled execution state: %w", err)
	}
	var records []persistence.NodeExecutionRecord
	err = repository.dbClient.DB(ctx).Table("workflow_runtime.node_executions").
		Where("status IN ?", []model.NodeStatus{
			model.NodeQueued, model.NodeRunning, model.NodeRetryPending,
		}).
		Where(`
			(status = ? AND queued_at < ?)
			OR (status = ? AND updated_at < ?)
			OR (status = ? AND next_attempt_at <= CURRENT_TIMESTAMP)`,
			model.NodeQueued, thresholds.QueuedBefore,
			model.NodeRunning, thresholds.RunningBefore,
			model.NodeRetryPending,
		).
		Order("updated_at, company_id, workflow_execution_id, node_id").
		Limit(limit).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("list stale node executions: %w", err)
	}
	return persistence.ToNodeExecutions(records), nil
}

func (repository *ExecutionRepository) RepairQueuedNodeCommand(
	ctx context.Context,
	node model.NodeExecution,
	job model.NodeJob,
	destination string,
	availableAt time.Time,
) (bool, error) {
	if node.Status != model.NodeQueued {
		return false, ErrStateTransition
	}

	if node.CompanyID != job.CompanyID ||
		node.ExecutionID != job.ExecutionID ||
		node.ID != job.NodeExecutionID ||
		node.NodeID != job.NodeID ||
		node.Attempt != job.Attempt {
		return false, fmt.Errorf("queued node command identity mismatch")
	}

	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin queued node command repair: %w", err)
	}
	defer transaction.Rollback(ctx)

	now := time.Now().UTC()

	result, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.node_executions
		SET updated_at = $6,
			lock_version = lock_version + 1
		WHERE company_id = $1
		  AND workflow_execution_id = $2
		  AND node_execution_id = $3
		  AND status = 'QUEUED'
		  AND attempt = $4
		  AND lock_version = $5`,
		node.CompanyID,
		node.ExecutionID,
		node.ID,
		node.Attempt,
		node.LockVersion,
		now,
	)
	if err != nil {
		return false, fmt.Errorf("reserve queued node command repair: %w", err)
	}
	if result.RowsAffected() != 1 {
		return false, nil
	}

	operationKey := nodeCommandOperationKey(job)

	if err := persistNodeCommand(
		ctx,
		transaction,
		job,
		job.Attempt,
		operationKey,
		destination,
		availableAt,
	); err != nil {
		return false, err
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return false, fmt.Errorf("encode repaired node command: %w", err)
	}

	_, err = transaction.Exec(ctx, `
		UPDATE workflow_runtime.outbox_messages
		SET destination = $4,
			message_key = $5,
			encoded_payload = $6,
			publication_state = 'PENDING',
			available_at = $7,
			claimed_at = NULL,
			claim_owner = NULL,
			published_at = NULL,
			lock_version = lock_version + 1
		WHERE company_id = $1
		  AND workflow_execution_id = $2
		  AND operation_key = $3
		  AND operation_kind = 'NODE_COMMAND'
		  AND publication_state = 'PUBLISHED'`,
		job.CompanyID,
		job.ExecutionID,
		operationKey,
		destination,
		job.NodeExecutionID,
		payload,
		availableAt,
	)
	if err != nil {
		return false, fmt.Errorf(
			"rearm queued node command outbox message: %w",
			err,
		)
	}

	if err := transaction.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit queued node command repair: %w", err)
	}

	return true, nil
}
