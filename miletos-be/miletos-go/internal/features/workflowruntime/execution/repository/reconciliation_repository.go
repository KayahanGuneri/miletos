package repository

import (
	"context"
	"fmt"
	"time"

	"miletos-go/internal/features/workflowruntime/execution/model"
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
	rows, err := repository.dbClient.Query(ctx, nodeSelect+`
		WHERE status IN ('QUEUED', 'RUNNING', 'RETRY_PENDING')
		  AND (
			(status = 'QUEUED' AND queued_at < $1)
			OR (status = 'RUNNING' AND updated_at < $2)
			OR (
				status = 'RETRY_PENDING'
				AND next_attempt_at <= CURRENT_TIMESTAMP
			)
		  )
		ORDER BY updated_at, company_id, workflow_execution_id, node_id
		LIMIT $4`,
		thresholds.QueuedBefore,
		thresholds.RunningBefore,
		thresholds.RetryPendingBefore,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list stale node executions: %w", err)
	}
	defer rows.Close()
	result := make([]model.NodeExecution, 0)
	for rows.Next() {
		node, scanErr := scanNode(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, node)
	}
	return result, rows.Err()
}

func (repository *ExecutionRepository) ReserveQueuedRepublish(
	ctx context.Context,
	node model.NodeExecution,
) (bool, error) {
	result, err := repository.dbClient.Exec(ctx, `
		UPDATE workflow_runtime.node_executions
		SET updated_at = $6, lock_version = lock_version + 1
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
		time.Now().UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("reserve queued node republish: %w", err)
	}
	return result.RowsAffected() == 1, nil
}
