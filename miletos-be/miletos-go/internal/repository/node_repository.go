package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/model"

	"github.com/jackc/pgx/v5"
)

func (repository *ExecutionRepository) FindNode(
	ctx context.Context,
	companyID string,
	executionID string,
	nodeID string,
) (model.NodeExecution, error) {
	node, err := scanNode(repository.db.QueryRow(ctx, nodeSelect+`
		WHERE company_id = $1 AND workflow_execution_id = $2 AND node_id = $3`,
		companyID, executionID, nodeID,
	))
	return node, mapNoRows(err)
}

func (repository *ExecutionRepository) ListAllNodes(
	ctx context.Context,
	companyID string,
	executionID string,
) ([]model.NodeExecution, error) {
	rows, err := repository.db.Query(ctx, nodeSelect+`
		WHERE company_id = $1 AND workflow_execution_id = $2
		ORDER BY created_at, node_id`,
		companyID, executionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nodes := make([]model.NodeExecution, 0)
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (repository *ExecutionRepository) ListNodes(
	ctx context.Context,
	companyID string,
	executionID string,
	after string,
	limit int,
) (model.Page[model.NodeExecution], error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	arguments := []any{companyID, executionID}
	query := nodeSelect + ` WHERE company_id = $1 AND workflow_execution_id = $2`
	if after != "" {
		arguments = append(arguments, after)
		query += fmt.Sprintf(" AND node_execution_id > $%d", len(arguments))
	}
	arguments = append(arguments, limit+1)
	query += fmt.Sprintf(" ORDER BY node_execution_id LIMIT $%d", len(arguments))
	rows, err := repository.db.Query(ctx, query, arguments...)
	if err != nil {
		return model.Page[model.NodeExecution]{}, err
	}
	defer rows.Close()
	items := make([]model.NodeExecution, 0, limit+1)
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return model.Page[model.NodeExecution]{}, err
		}
		items = append(items, node)
	}
	page := model.Page[model.NodeExecution]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = page.Items[len(page.Items)-1].ID
	}
	return page, rows.Err()
}

func (repository *ExecutionRepository) MarkNodeQueued(
	ctx context.Context,
	companyID string,
	executionID string,
	nodeID string,
) (model.NodeExecution, bool, error) {
	now := time.Now().UTC()
	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return model.NodeExecution{}, false, fmt.Errorf("begin node queue transition: %w", err)
	}
	defer transaction.Rollback(ctx)
	var nodeExecutionID string
	err = transaction.QueryRow(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'QUEUED', ready_at = COALESCE(ready_at, $4), queued_at = $4,
			next_attempt_at = NULL, updated_at = $4, lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2 AND node_id = $3
		  AND status IN ('PENDING', 'RETRY_PENDING')
		RETURNING node_execution_id`,
		companyID, executionID, nodeID, now,
	).Scan(&nodeExecutionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.NodeExecution{}, false, nil
	}
	if err != nil {
		return model.NodeExecution{}, false, fmt.Errorf("mark node queued: %w", err)
	}
	node, err := scanNode(transaction.QueryRow(ctx, nodeSelect+`
		WHERE company_id = $1 AND workflow_execution_id = $2 AND node_id = $3`,
		companyID, executionID, nodeID,
	))
	if err != nil {
		return model.NodeExecution{}, false, err
	}
	if err := recordEvent(
		ctx, transaction, executionID, node.ID, "NODE_QUEUED", "",
		model.NodeQueued, "Node queued", map[string]any{},
	); err != nil {
		return model.NodeExecution{}, false, fmt.Errorf("record node queued event: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return model.NodeExecution{}, false, fmt.Errorf("commit node queue transition: %w", err)
	}
	return node, true, nil
}

func (repository *ExecutionRepository) MarkNodeRunning(ctx context.Context, job model.NodeJob) (bool, error) {
	now := time.Now().UTC()
	input, err := encodeJSON(objectSummary(job.Payload))
	if err != nil {
		return false, err
	}
	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin node start transaction: %w", err)
	}
	defer transaction.Rollback(ctx)
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'RUNNING', attempt = $5, started_at = COALESCE(started_at, $6),
			finished_at = NULL, output_summary = NULL, failure_summary = NULL,
			input_summary = $7, updated_at = $6, lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND node_execution_id = $3 AND node_id = $4
		  AND status = 'QUEUED' AND attempt = $5`,
		job.CompanyID, job.ExecutionID, job.NodeExecutionID, job.NodeID, job.Attempt, now, input,
	)
	if err != nil {
		return false, fmt.Errorf("mark node running: %w", err)
	}
	if command.RowsAffected() == 0 {
		return false, nil
	}
	command, err = transaction.Exec(ctx, `
		INSERT INTO workflow_runtime.node_execution_attempts (
			company_id, workflow_execution_id, node_execution_id, attempt,
			attempt_status, started_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 'RUNNING', $5, $5, $5)`,
		job.CompanyID, job.ExecutionID, job.NodeExecutionID, job.Attempt, now,
	)
	if err != nil {
		return false, fmt.Errorf("create node attempt: %w", err)
	}
	if command.RowsAffected() != 1 {
		return false, fmt.Errorf("%w: node attempt was not created", ErrStateTransition)
	}
	if err := recordEvent(
		ctx, transaction, job.ExecutionID, job.NodeExecutionID, "NODE_STARTED",
		model.NodeQueued, model.NodeRunning, "Node started", map[string]any{},
	); err != nil {
		return false, fmt.Errorf("record node start event: %w", err)
	}
	if err := recordLog(
		ctx, transaction, job.ExecutionID, job.NodeExecutionID,
		"INFO", "Node execution started",
		map[string]any{"nodeId": job.NodeID, "attempt": job.Attempt},
	); err != nil {
		return false, fmt.Errorf("record node start log: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit node start: %w", err)
	}
	return true, nil
}

func (repository *ExecutionRepository) SaveNodeSuccess(
	ctx context.Context,
	job model.NodeJob,
	output any,
) error {
	now := time.Now().UTC()
	encoded, err := encodeJSON(objectSummary(output))
	if err != nil {
		return err
	}
	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin node success transaction: %w", err)
	}
	defer transaction.Rollback(ctx)
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'SUCCEEDED', output_summary = $5, failure_summary = NULL,
			finished_at = $6, updated_at = $6, next_attempt_at = NULL,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND node_execution_id = $3 AND attempt = $4 AND status = 'RUNNING'`,
		job.CompanyID, job.ExecutionID, job.NodeExecutionID, job.Attempt, encoded, now,
	)
	if err != nil {
		return fmt.Errorf("save node result: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: node success requires one RUNNING node", ErrStateTransition)
	}
	command, err = transaction.Exec(ctx, `
		UPDATE workflow_runtime.node_execution_attempts
		SET attempt_status = 'SUCCEEDED', output_summary = $5, finished_at = $6, updated_at = $6
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND node_execution_id = $3 AND attempt = $4 AND attempt_status = 'RUNNING'`,
		job.CompanyID, job.ExecutionID, job.NodeExecutionID, job.Attempt, encoded, now,
	)
	if err != nil {
		return fmt.Errorf("complete node attempt: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: node success requires one RUNNING attempt", ErrStateTransition)
	}
	if err := recordEvent(
		ctx, transaction, job.ExecutionID, job.NodeExecutionID, "NODE_SUCCEEDED",
		model.NodeRunning, model.NodeSucceeded, "Node succeeded", map[string]any{},
	); err != nil {
		return fmt.Errorf("record node success event: %w", err)
	}
	if err := recordLog(
		ctx, transaction, job.ExecutionID, job.NodeExecutionID,
		"INFO", "Node execution succeeded",
		map[string]any{"nodeId": job.NodeID, "attempt": job.Attempt},
	); err != nil {
		return fmt.Errorf("record node success log: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit node success: %w", err)
	}
	return nil
}

func (repository *ExecutionRepository) SaveNodeFailure(
	ctx context.Context,
	job model.NodeJob,
	failure map[string]any,
	retry bool,
	failedAt time.Time,
	nextAttempt time.Time,
	maximumAttempts int,
	retryDelay time.Duration,
) error {
	encoded, err := encodeJSON(failure)
	if err != nil {
		return err
	}

	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin node failure transaction: %w", err)
	}
	defer transaction.Rollback(ctx)

	status := model.NodeFailed
	eventType := "NODE_FAILED"

	// Final failure:
	// finished_at = failedAt
	// next_attempt_at = NULL
	var finishedAt any = failedAt
	var scheduledAt any

	// Retriable failure:
	// finished_at = NULL
	// next_attempt_at = nextAttempt
	if retry {
		status = model.NodeRetryPending
		eventType = "NODE_RETRY_PENDING"
		finishedAt = nil
		scheduledAt = nextAttempt
	}

	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = $5::text,
			output_summary = NULL,
			failure_summary = $6,
			finished_at = $7::timestamptz,
			next_attempt_at = $8::timestamptz,
			updated_at = $9::timestamptz,
			lock_version = lock_version + 1,
			retry_max_attempts = CASE
				WHEN $5::text = 'RETRY_PENDING'
					THEN $10::integer
				ELSE retry_max_attempts
			END,
			retry_initial_backoff_ns = CASE
				WHEN $5::text = 'RETRY_PENDING'
					THEN $11::bigint
				ELSE retry_initial_backoff_ns
			END,
			retry_max_backoff_ns = CASE
				WHEN $5::text = 'RETRY_PENDING'
					THEN $11::bigint
				ELSE retry_max_backoff_ns
			END
		WHERE company_id = $1
		  AND workflow_execution_id = $2
		  AND node_execution_id = $3
		  AND attempt = $4
		  AND status = 'RUNNING'`,
		job.CompanyID,
		job.ExecutionID,
		job.NodeExecutionID,
		job.Attempt,
		status,
		encoded,
		finishedAt,
		scheduledAt,
		failedAt,
		maximumAttempts,
		retryDelay.Nanoseconds(),
	)
	if err != nil {
		return fmt.Errorf("save node failure: %w", err)
	}

	if command.RowsAffected() != 1 {
		return fmt.Errorf(
			"%w: node failure requires one RUNNING node",
			ErrStateTransition,
		)
	}

	decisionKind := "EXHAUSTED"
	decisionReason := "MAX_ATTEMPTS_REACHED"

	var backoff any

	if retry {
		decisionKind = "RETRY"
		decisionReason = "RETRY_SCHEDULED"
		backoff = nextAttempt.Sub(failedAt).Nanoseconds()
	} else if configuredKind, ok :=
		failure["retryDecisionKind"].(string); ok &&
		configuredKind == "DO_NOT_RETRY" {

		decisionKind = configuredKind

		if configuredReason, ok :=
			failure["retryDecisionReason"].(string); ok {

			decisionReason = configuredReason
		} else {
			decisionReason = "FAILURE_NOT_RETRYABLE"
		}
	}

	command, err = transaction.Exec(ctx, `
		UPDATE workflow_runtime.node_execution_attempts
		SET attempt_status = 'FAILED',
			failure_summary = $5,
			finished_at = $6::timestamptz,
			updated_at = $6::timestamptz,
			retry_decision_kind = $7,
			retry_decision_reason = $8,
			retry_backoff_ns = $9::bigint,
			next_attempt_at = $10::timestamptz
		WHERE company_id = $1
		  AND workflow_execution_id = $2
		  AND node_execution_id = $3
		  AND attempt = $4
		  AND attempt_status = 'RUNNING'`,
		job.CompanyID,
		job.ExecutionID,
		job.NodeExecutionID,
		job.Attempt,
		encoded,
		failedAt,
		decisionKind,
		decisionReason,
		backoff,
		scheduledAt,
	)
	if err != nil {
		return fmt.Errorf("complete failed node attempt: %w", err)
	}

	if command.RowsAffected() != 1 {
		return fmt.Errorf(
			"%w: node failure requires one RUNNING attempt",
			ErrStateTransition,
		)
	}

	if err := recordError(
		ctx,
		transaction,
		job,
		failure,
		retry,
		failedAt,
	); err != nil {
		return fmt.Errorf("record node execution error: %w", err)
	}

	if err := recordEvent(
		ctx,
		transaction,
		job.ExecutionID,
		job.NodeExecutionID,
		eventType,
		model.NodeRunning,
		status,
		"Node execution failed",
		map[string]any{
			"nodeId":         job.NodeID,
			"attempt":        job.Attempt,
			"retryScheduled": retry,
		},
	); err != nil {
		return fmt.Errorf("record node failure event: %w", err)
	}

	logLevel := "ERROR"
	if retry {
		logLevel = "WARN"
	}

	if err := recordLog(
		ctx,
		transaction,
		job.ExecutionID,
		job.NodeExecutionID,
		logLevel,
		"Node execution failed",
		map[string]any{
			"nodeId":         job.NodeID,
			"attempt":        job.Attempt,
			"retryScheduled": retry,
		},
	); err != nil {
		return fmt.Errorf("record node failure log: %w", err)
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit node failure: %w", err)
	}

	return nil
}
func (repository *ExecutionRepository) ResetNodeQueue(
	ctx context.Context,
	companyID string,
	executionID string,
	nodeID string,
) error {
	command, err := repository.db.Exec(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'PENDING', ready_at = NULL, queued_at = NULL, updated_at = $4,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2 AND node_id = $3
		  AND status = 'QUEUED'`,
		companyID, executionID, nodeID, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: queued node could not be reset", ErrStateTransition)
	}
	return nil
}

func (repository *ExecutionRepository) MarkNodeSkipped(
	ctx context.Context,
	companyID string,
	executionID string,
	nodeID string,
) error {
	now := time.Now().UTC()
	transaction, err := repository.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin node skip transition: %w", err)
	}
	defer transaction.Rollback(ctx)
	var nodeExecutionID string
	err = transaction.QueryRow(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'SKIPPED', finished_at = $4, updated_at = $4,
			output_summary = NULL, failure_summary = NULL, lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2 AND node_id = $3
		  AND status = 'PENDING'
		RETURNING node_execution_id`,
		companyID, executionID, nodeID, now,
	).Scan(&nodeExecutionID)
	if errors.Is(err, pgx.ErrNoRows) {
		state, findErr := repository.FindNode(ctx, companyID, executionID, nodeID)
		if findErr != nil {
			return findErr
		}
		if state.Status == model.NodeSkipped {
			return nil
		}
		return fmt.Errorf(
			"%w: node %s cannot be skipped from status %s",
			ErrStateTransition, nodeID, state.Status,
		)
	}
	if err != nil {
		return err
	}
	if err := recordEvent(
		ctx, transaction, executionID, nodeExecutionID, "NODE_SKIPPED",
		model.NodePending, model.NodeSkipped, "Node skipped because a dependency failed",
		map[string]any{},
	); err != nil {
		return fmt.Errorf("record node skipped event: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit node skip transition: %w", err)
	}
	return nil
}

func (repository *ExecutionRepository) PrepareRetry(ctx context.Context, job model.NodeJob) (model.NodeJob, error) {
	now := time.Now().UTC()
	nextAttempt := job.Attempt + 1
	err := repository.db.QueryRow(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'QUEUED', attempt = $5, queued_at = $6, next_attempt_at = NULL,
			failure_summary = NULL, updated_at = $6, lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND node_execution_id = $3 AND attempt = $4 AND status = 'RETRY_PENDING'
		RETURNING node_execution_id`,
		job.CompanyID, job.ExecutionID, job.NodeExecutionID, job.Attempt, nextAttempt, now,
	).Scan(&job.NodeExecutionID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return model.NodeJob{}, err
		}
		state, findErr := repository.FindNode(
			ctx, job.CompanyID, job.ExecutionID, job.NodeID,
		)
		if findErr != nil {
			return model.NodeJob{}, findErr
		}
		if state.Attempt >= nextAttempt {
			job.NodeExecutionID = state.ID
			job.Attempt = state.Attempt
			return job, nil
		}
		return model.NodeJob{}, fmt.Errorf(
			"%w: retry could not be prepared from status %s",
			ErrStateTransition, state.Status,
		)
	}
	job.Attempt = nextAttempt
	return job, nil
}

const nodeSelect = `
	SELECT node_execution_id, workflow_execution_id, company_id, node_id,
		plugin_type, plugin_version, status, attempt, created_at,
		ready_at, queued_at, started_at, finished_at, next_attempt_at, updated_at,
		input_summary, output_summary, failure_summary
	FROM workflow_runtime.node_executions`

func scanNode(row rowScanner) (model.NodeExecution, error) {
	var node model.NodeExecution
	var readyAt, queuedAt, startedAt, finishedAt, nextAttemptAt sql.NullTime
	var input, output, failure []byte
	err := row.Scan(
		&node.ID, &node.ExecutionID, &node.CompanyID, &node.NodeID,
		&node.Type, &node.Version, &node.Status, &node.Attempt, &node.CreatedAt,
		&readyAt, &queuedAt, &startedAt, &finishedAt, &nextAttemptAt, &node.UpdatedAt,
		&input, &output, &failure,
	)
	if err != nil {
		return model.NodeExecution{}, err
	}
	node.ReadyAt = optionalTime(readyAt)
	node.QueuedAt = optionalTime(queuedAt)
	node.StartedAt = optionalTime(startedAt)
	node.FinishedAt = optionalTime(finishedAt)
	node.NextAttemptAt = optionalTime(nextAttemptAt)
	node.Input = decodeObject(input)
	node.Output = decodeObject(output)
	node.Failure = decodeObject(failure)
	return node, nil
}

func objectSummary(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return map[string]any{"value": value}
}
