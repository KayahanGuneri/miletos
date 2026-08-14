package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/persistence"

	"gorm.io/gorm"
)

func (repository *ExecutionRepository) FindNode(
	ctx context.Context,
	companyID string,
	executionID string,
	nodeID string,
) (model.NodeExecution, error) {
	var record persistence.NodeExecutionRecord
	err := repository.dbClient.DB(ctx).Table("workflow_runtime.node_executions").
		Where("company_id = ? AND workflow_execution_id = ? AND node_id = ?", companyID, executionID, nodeID).
		Take(&record).Error
	if err != nil {
		return model.NodeExecution{}, mapNoRows(err)
	}
	return persistence.ToNodeExecution(record), nil
}

func (repository *ExecutionRepository) ListAllNodes(
	ctx context.Context,
	companyID string,
	executionID string,
) ([]model.NodeExecution, error) {
	var records []persistence.NodeExecutionRecord
	err := repository.dbClient.DB(ctx).Table("workflow_runtime.node_executions").
		Where("company_id = ? AND workflow_execution_id = ?", companyID, executionID).
		Order("created_at, node_id").Find(&records).Error
	if err != nil {
		return nil, err
	}
	return persistence.ToNodeExecutions(records), nil
}

func (repository *ExecutionRepository) ListNodes(
	ctx context.Context,
	companyID string,
	executionID string,
	after string,
	limit int,
) (model.Page[model.NodeExecution], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := repository.dbClient.DB(ctx).Table("workflow_runtime.node_executions").
		Where("company_id = ? AND workflow_execution_id = ?", companyID, executionID)
	if after != "" {
		query = query.Where("node_execution_id > ?", after)
	}
	var records []persistence.NodeExecutionRecord
	if err := query.Order("node_execution_id").Limit(limit + 1).Find(&records).Error; err != nil {
		return model.Page[model.NodeExecution]{}, err
	}
	items := persistence.ToNodeExecutions(records)
	page := model.Page[model.NodeExecution]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}

func (repository *ExecutionRepository) MarkNodeQueued(
	ctx context.Context,
	companyID string,
	executionID string,
	nodeID string,
	payload ...any,
) (model.NodeExecution, bool, error) {
	now := time.Now().UTC()
	var encodedInput any
	if len(payload) > 0 {
		encoded, encodeErr := encodeJSON(objectSummary(payload[0]))
		if encodeErr != nil {
			return model.NodeExecution{}, false, encodeErr
		}
		encodedInput = string(encoded)
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return model.NodeExecution{}, false, fmt.Errorf("begin node queue transition: %w", err)
	}
	defer transaction.Rollback(ctx)
	var nodeExecutionID string
	// Raw SQL needs UPDATE RETURNING so the guarded queue transition yields its row identity atomically.
	err = transaction.QueryRow(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'QUEUED', ready_at = COALESCE(ready_at, $4), queued_at = $4,
			next_attempt_at = NULL,
			input_summary = COALESCE(CAST($5 AS jsonb), input_summary),
			updated_at = $4, lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2 AND node_id = $3
		  AND status IN ('PENDING', 'RETRY_PENDING')
		RETURNING node_execution_id`,
		companyID, executionID, nodeID, now, encodedInput,
	).Scan(&nodeExecutionID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.NodeExecution{}, false, nil
	}
	if err != nil {
		return model.NodeExecution{}, false, fmt.Errorf("mark node queued: %w", err)
	}
	var record persistence.NodeExecutionRecord
	if err := transaction.DB(ctx).Table("workflow_runtime.node_executions").
		Where("company_id = ? AND workflow_execution_id = ? AND node_id = ?", companyID, executionID, nodeID).
		Take(&record).Error; err != nil {
		return model.NodeExecution{}, false, err
	}
	node := persistence.ToNodeExecution(record)
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

func (repository *ExecutionRepository) QueueNodeCommand(
	ctx context.Context,
	execution model.Execution,
	nodeID string,
	payload any,
	destination string,
) (model.NodeExecution, bool, error) {
	now := time.Now().UTC()

	encodedInput, err := encodeJSON(
		objectSummary(payload),
	)
	if err != nil {
		return model.NodeExecution{}, false, err
	}

	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return model.NodeExecution{}, false, fmt.Errorf(
			"begin atomic node command queue transaction: %w",
			err,
		)
	}
	defer transaction.Rollback(ctx)

	var nodeExecutionID string

	// The node transition and its durable command are committed atomically.
	err = transaction.QueryRow(ctx, `
UPDATE workflow_runtime.node_executions
SET status = 'QUEUED',
ready_at = COALESCE(ready_at, $4),
queued_at = $4,
next_attempt_at = NULL,
input_summary = $5,
updated_at = $4,
lock_version = lock_version + 1
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND node_id = $3
  AND status IN ('PENDING', 'RETRY_PENDING')
RETURNING node_execution_id`,
		execution.CompanyID,
		execution.ID,
		nodeID,
		now,
		string(encodedInput),
	).Scan(&nodeExecutionID)

	if errors.Is(err, sql.ErrNoRows) {
		return model.NodeExecution{}, false, nil
	}

	if err != nil {
		return model.NodeExecution{}, false, fmt.Errorf(
			"mark node queued for durable command: %w",
			err,
		)
	}

	var record persistence.NodeExecutionRecord

	if err := transaction.DB(ctx).
		Table("workflow_runtime.node_executions").
		Where(
			"company_id = ? AND workflow_execution_id = ? AND node_execution_id = ?",
			execution.CompanyID,
			execution.ID,
			nodeExecutionID,
		).
		Take(&record).
		Error; err != nil {
		return model.NodeExecution{}, false, fmt.Errorf(
			"load queued node for durable command: %w",
			err,
		)
	}

	node := persistence.ToNodeExecution(record)

	job := model.NodeJob{
		CompanyID:       execution.CompanyID,
		WorkflowID:      execution.WorkflowID,
		ExecutionID:     execution.ID,
		NodeID:          node.NodeID,
		NodeExecutionID: node.ID,
		Attempt:         node.Attempt,
		CorrelationID:   execution.CorrelationID,
		Origin:          execution.Origin,
		Payload:         payload,
	}

	if err := recordEvent(
		ctx,
		transaction,
		execution.ID,
		node.ID,
		"NODE_QUEUED",
		"",
		model.NodeQueued,
		"Node queued",
		map[string]any{},
	); err != nil {
		return model.NodeExecution{}, false, fmt.Errorf(
			"record durable node queued event: %w",
			err,
		)
	}

	if err := persistNodeCommand(
		ctx,
		transaction,
		job,
		node.Attempt,
		nodeCommandOperationKey(job),
		destination,
		now,
	); err != nil {
		return model.NodeExecution{}, false, fmt.Errorf(
			"persist queued node command: %w",
			err,
		)
	}

	if err := transaction.Commit(ctx); err != nil {
		return model.NodeExecution{}, false, fmt.Errorf(
			"commit atomic node command queue transaction: %w",
			err,
		)
	}

	return node, true, nil
}
func (repository *ExecutionRepository) MarkNodeRunning(ctx context.Context, job model.NodeJob) (bool, error) {
	now := time.Now().UTC()
	input, err := encodeJSON(objectSummary(job.Payload))
	if err != nil {
		return false, err
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin node start transaction: %w", err)
	}
	defer transaction.Rollback(ctx)
	// Raw SQL performs the queued-to-running compare-and-set in the attempt transaction.
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
	attemptRecord := map[string]any{
		"company_id": job.CompanyID, "workflow_execution_id": job.ExecutionID,
		"node_execution_id": job.NodeExecutionID, "attempt": job.Attempt,
		"attempt_status": "RUNNING", "started_at": now,
		"created_at": now, "updated_at": now,
	}
	attemptResult := transaction.DB(ctx).
		Table("workflow_runtime.node_execution_attempts").Create(attemptRecord)
	if attemptResult.Error != nil {
		return false, fmt.Errorf("create node attempt: %w", attemptResult.Error)
	}
	if attemptResult.RowsAffected != 1 {
		return false, fmt.Errorf("%w: node attempt was not created", ErrStateTransition)
	}
	if err := transaction.DB(ctx).Table("workflow_runtime.workflow_executions").
		Where("company_id = ? AND workflow_execution_id = ?", job.CompanyID, job.ExecutionID).
		Where("status IN ?", []model.ExecutionStatus{
			model.ExecutionCreated, model.ExecutionQueued, model.ExecutionRunning,
		}).
		Updates(map[string]any{
			"updated_at": now, "is_stalled": false,
			"lock_version": gorm.Expr("lock_version + 1"),
		}).Error; err != nil {
		return false, fmt.Errorf("update execution start progress: %w", err)
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
	routing model.NodeRoutingOutcome,
) error {
	now := time.Now().UTC()
	outputSummary := objectSummary(output)
	var persistedOutput any = outputSummary
	if routing.Explicit {
		persistedOutput = model.PersistedRoutedOutput{
			Format:       model.PersistedRoutedOutputFormat,
			Output:       outputSummary,
			EdgePayloads: routing.EdgePayloads,
		}
	}
	encoded, err := encodeJSON(persistedOutput)
	if err != nil {
		return err
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin node success transaction: %w", err)
	}
	defer transaction.Rollback(ctx)
	// Raw SQL guards success persistence by node execution id, attempt, and running state.
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
	// Raw SQL completes only the exact running attempt in the node-success transaction.
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
	if err := transaction.DB(ctx).Table("workflow_runtime.workflow_executions").
		Where("company_id = ? AND workflow_execution_id = ?", job.CompanyID, job.ExecutionID).
		Where("status IN ?", []model.ExecutionStatus{
			model.ExecutionCreated, model.ExecutionQueued, model.ExecutionRunning,
		}).
		Updates(map[string]any{
			"updated_at": now, "is_stalled": false,
			"lock_version": gorm.Expr("lock_version + 1"),
		}).Error; err != nil {
		return fmt.Errorf("update execution success progress: %w", err)
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
	retryDestination string,
) error {
	failure["executionId"] = job.ExecutionID
	failure["nodeId"] = job.NodeID
	failure["nodeExecutionId"] = job.NodeExecutionID
	failure["correlationId"] = job.CorrelationID
	if retry {
		failure["nextAttemptAt"] = nextAttempt.UTC().Format(time.RFC3339Nano)
	}
	encoded, err := encodeJSON(failure)
	if err != nil {
		return err
	}

	transaction, err := repository.dbClient.Begin(ctx)
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

	// Raw SQL persists failure and retry scheduling as one guarded state transition.
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

	// Raw SQL synchronizes the parent execution state with the node failure transaction.
	command, err = transaction.Exec(ctx, `
		UPDATE workflow_runtime.workflow_executions
		SET updated_at = $3::timestamptz,
			is_stalled = FALSE,
			lock_version = lock_version + 1
		WHERE company_id = $1
		  AND workflow_execution_id = $2
		  AND status IN ('CREATED', 'QUEUED', 'RUNNING')`,
		job.CompanyID,
		job.ExecutionID,
		failedAt,
	)
	if err != nil {
		return fmt.Errorf("update execution failure progress: %w", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf(
			"%w: node failure requires one active execution",
			ErrStateTransition,
		)
	}

	decisionKind := model.RetryKindExhausted
	decisionReason := model.RetryReasonMaxAttempts

	var backoff any

	if retry {
		decisionKind = model.RetryKindRetry
		decisionReason = model.RetryReasonScheduled
		backoff = nextAttempt.Sub(failedAt).Nanoseconds()
	} else if configuredKind := retryDecisionKindValue(
		failure["retryDecisionKind"],
	); configuredKind == model.RetryKindDoNotRetry {

		decisionKind = configuredKind

		if configuredReason := retryDecisionReasonValue(
			failure["retryDecisionReason"],
		); configuredReason != "" {

			decisionReason = configuredReason
		} else {
			decisionReason = model.RetryReasonNotRetryable
		}
	}

	// Raw SQL records retry metadata only for the exact failed attempt.
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

	classifiedRetryable, _ := failure["retryable"].(bool)
	if err := recordError(
		ctx,
		transaction,
		job,
		failure,
		classifiedRetryable,
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

	if retry && retryDestination != "" {
		if err := persistRetryCommand(
			ctx, transaction, job, retryDestination, nextAttempt,
		); err != nil {
			return err
		}
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit node failure: %w", err)
	}

	return nil
}

func retryDecisionKindValue(value any) model.RetryDecisionKind {
	var raw string
	switch typed := value.(type) {
	case model.RetryDecisionKind:
		raw = string(typed)
	case string:
		raw = typed
	default:
		return ""
	}
	kind := model.RetryDecisionKind(strings.TrimSpace(raw))
	switch kind {
	case model.RetryKindRetry, model.RetryKindExhausted, model.RetryKindDoNotRetry:
		return kind
	default:
		return ""
	}
}

func retryDecisionReasonValue(value any) model.RetryDecisionReason {
	var raw string
	switch typed := value.(type) {
	case model.RetryDecisionReason:
		raw = string(typed)
	case string:
		raw = typed
	default:
		return ""
	}
	reason := model.RetryDecisionReason(strings.TrimSpace(raw))
	switch reason {
	case model.RetryReasonScheduled,
		model.RetryReasonMaxAttempts,
		model.RetryReasonNotRetryable,
		model.RetryReasonCategory,
		model.RetryReasonDeadline:
		return reason
	default:
		return ""
	}
}
func (repository *ExecutionRepository) MarkNodeSkipped(
	ctx context.Context,
	companyID string,
	executionID string,
	nodeID string,
	skipReason string,
) error {
	now := time.Now().UTC()
	reason := strings.TrimSpace(skipReason)
	if reason == "" {
		reason = string(model.SkipReasonDependencyFailed)
	}
	switch reason {
	case string(model.SkipReasonOutOfTriggerScope),
		string(model.SkipReasonDependencyFailed),
		string(model.SkipReasonNoActiveRoute):
	default:
		return fmt.Errorf("invalid node skip reason %q", reason)
	}
	failureSummary, err := encodeJSON(map[string]any{"skipReason": reason})
	if err != nil {
		return err
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin node skip transition: %w", err)
	}
	defer transaction.Rollback(ctx)
	var nodeExecutionID string
	// Raw SQL needs UPDATE RETURNING to couple the guarded skip transition to its event.
	err = transaction.QueryRow(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'SKIPPED', finished_at = $4, updated_at = $4,
			output_summary = NULL, failure_summary = $5, lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2 AND node_id = $3
		  AND status = 'PENDING'
		RETURNING node_execution_id`,
		companyID, executionID, nodeID, now, failureSummary,
	).Scan(&nodeExecutionID)
	if errors.Is(err, sql.ErrNoRows) {
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
		model.NodePending, model.NodeSkipped, skipReasonMessage(reason),
		map[string]any{"skipReason": reason},
	); err != nil {
		return fmt.Errorf("record node skipped event: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit node skip transition: %w", err)
	}
	return nil
}

func skipReasonMessage(reason string) string {
	switch reason {
	case string(model.SkipReasonOutOfTriggerScope):
		return "Node skipped because it is outside the selected trigger scope"
	case string(model.SkipReasonNoActiveRoute):
		return "Node skipped because no incoming route was selected"
	default:
		return "Node skipped because a dependency failed"
	}
}

func (repository *ExecutionRepository) PrepareRetry(ctx context.Context, job model.NodeJob) (model.NodeJob, error) {
	now := time.Now().UTC()
	nextAttempt := job.Attempt + 1
	// Raw SQL needs UPDATE RETURNING for the attempt-and-status guarded retry handoff.
	err := repository.dbClient.QueryRow(ctx, `
		UPDATE workflow_runtime.node_executions
		SET status = 'QUEUED', attempt = $5, queued_at = $6, next_attempt_at = NULL,
			failure_summary = NULL, updated_at = $6, lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND node_execution_id = $3 AND attempt = $4 AND status = 'RETRY_PENDING'
		RETURNING node_execution_id`,
		job.CompanyID, job.ExecutionID, job.NodeExecutionID, job.Attempt, nextAttempt, now,
	).Scan(&job.NodeExecutionID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
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

func objectSummary(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if object, ok := value.(map[string]any); ok {
		if object == nil {
			return map[string]any{}
		}
		return object
	}
	return map[string]any{"value": value}
}
