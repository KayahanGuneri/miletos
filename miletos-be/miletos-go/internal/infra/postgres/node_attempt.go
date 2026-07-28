package postgres

import (
	"context"
	"errors"
	"fmt"

	pgx "github.com/jackc/pgx/v5"
	"miletos-go/internal/features/execution"
	repository "miletos-go/internal/ports/persistence"
)

const insertNodeAttemptSQL = `
INSERT INTO workflow_runtime.node_execution_attempts (
	company_id,
	workflow_execution_id,
	node_execution_id,
	attempt,
	attempt_status,
	started_at,
	finished_at,
	output_summary,
	failure_summary,
	retry_decision_kind,
	retry_decision_reason,
	retry_backoff_ns,
	next_attempt_at,
	created_at,
	updated_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8::jsonb,
	$9::jsonb,
	$10,
	$11,
	$12,
	$13,
	$14,
	$15
)
`

const completeNodeAttemptSQL = `
UPDATE workflow_runtime.node_execution_attempts
SET
	attempt_status = $5,
	finished_at = $6,
	output_summary = $7::jsonb,
	failure_summary = $8::jsonb,
	retry_decision_kind = $9,
	retry_decision_reason = $10,
	retry_backoff_ns = $11,
	next_attempt_at = $12,
	updated_at = $6
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND node_execution_id = $3
  AND attempt = $4
  AND attempt_status = $13
  AND finished_at IS NULL
`

func applyNodeAttemptMutationTransaction(
	ctx context.Context,
	tx pgx.Tx,
	mutation repository.NodeAttemptMutation,
) error {
	if tx == nil {
		return fmt.Errorf("node attempt transaction must not be nil")
	}
	switch mutation.Kind() {
	case repository.NodeAttemptMutationStart:
		record, exists := mutation.Start()
		if !exists {
			return fmt.Errorf("node attempt START mutation must contain a record")
		}
		return insertNodeAttemptTransaction(ctx, tx, record)
	case repository.NodeAttemptMutationComplete:
		completion, exists := mutation.Completion()
		if !exists {
			return fmt.Errorf("node attempt COMPLETE mutation must contain completion data")
		}
		return completeNodeAttemptTransaction(ctx, tx, completion)
	default:
		return fmt.Errorf("node attempt mutation kind must be supported")
	}
}

func insertNodeAttemptTransaction(
	ctx context.Context,
	tx pgx.Tx,
	record repository.NodeExecutionAttemptRecord,
) error {
	if !record.IsValid() ||
		record.Status() != execution.NodeExecutionStatusRunning {
		return fmt.Errorf("node attempt START record must be valid and RUNNING")
	}
	retryKind, retryReason, retryBackoff :=
		optionalAttemptRetryDecisionValues(record.RetryDecision)
	_, err := tx.Exec(
		ctx,
		insertNodeAttemptSQL,
		record.CompanyID().String(),
		record.WorkflowExecutionID().String(),
		record.NodeExecutionID().String(),
		record.Attempt().Int16(),
		record.Status().String(),
		record.StartedAt(),
		optionalValue(record.FinishedAt),
		optionalStringValue(record.OutputSummary),
		optionalStringValue(record.FailureSummary),
		retryKind,
		retryReason,
		retryBackoff,
		optionalValue(record.NextAttemptAt),
		record.CreatedAt(),
		record.UpdatedAt(),
	)
	if err != nil {
		return mapPostgreSQLError("create", "node execution attempt", err)
	}
	return nil
}

func completeNodeAttemptTransaction(
	ctx context.Context,
	tx pgx.Tx,
	completion repository.NodeAttemptCompletion,
) error {
	if !completion.IsValid() {
		return fmt.Errorf("node attempt completion must be valid")
	}
	retryKind, retryReason, retryBackoff :=
		optionalAttemptRetryDecisionValues(completion.RetryDecision)
	commandTag, err := tx.Exec(
		ctx,
		completeNodeAttemptSQL,
		completion.CompanyID().String(),
		completion.WorkflowExecutionID().String(),
		completion.NodeExecutionID().String(),
		completion.ExpectedAttempt().Int16(),
		completion.Status().String(),
		completion.FinishedAt(),
		optionalStringValue(completion.OutputSummary),
		optionalStringValue(completion.FailureSummary),
		retryKind,
		retryReason,
		retryBackoff,
		optionalValue(completion.NextAttemptAt),
		execution.NodeExecutionStatusRunning.String(),
	)
	if err != nil {
		return mapPostgreSQLError("update", "node execution attempt", err)
	}
	if commandTag.RowsAffected() != 1 {
		return repository.NewStaleWriteError(
			"update",
			"node execution attempt",
			errors.New("node execution attempt is no longer RUNNING or does not match the expected identity and attempt"),
		)
	}
	return nil
}

func optionalAttemptRetryDecisionValues(
	getter func() (execution.RetryDecision, bool),
) (any, any, any) {
	decision, exists := getter()
	if !exists {
		return nil, nil, nil
	}
	var backoff any
	if value, exists := decision.Backoff(); exists {
		backoff = int64(value)
	}
	return decision.Kind().String(), decision.Reason().String(), backoff
}
