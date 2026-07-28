package postgres

import (
	"context"
	"errors"
	"fmt"
	pgx "github.com/jackc/pgx/v5"
	repository "miletos-go/internal/ports/persistence"
)

const insertDefinitionSnapshotTransactionSQL = `
INSERT INTO workflow_runtime.workflow_definition_snapshots (
	snapshot_id,
	company_id,
	workflow_id,
	workflow_revision,
	workflow_name,
	definition_json,
	created_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6::jsonb,
	$7
)
`
const insertWorkflowExecutionTransactionSQL = `
INSERT INTO workflow_runtime.workflow_executions (
	workflow_execution_id,
	company_id,
	workflow_id,
	workflow_revision,
	snapshot_id,
	mode,
	correlation_id,
	status,
	created_at,
	validating_at,
	queued_at,
	started_at,
	finished_at,
	updated_at,
	terminal_outputs,
	failure_summary,
	is_stalled,
	next_sequence_number,
	lock_version
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8,
	$9,
	$10,
	$11,
	$12,
	$13,
	$14,
	$15::jsonb,
	$16::jsonb,
	$17,
	$18,
	$19
)
`

type createExecutionStore interface {
	CreateExecution(ctx context.Context, command repository.CreateExecutionCommand,
	) error
}

var _ createExecutionStore = (*Store)(nil)

func (
	store *Store) CreateExecution(ctx context.Context,
	command repository.CreateExecutionCommand) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("create execution context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !command.IsValid() {
		return fmt.Errorf("create execution command must be valid")
	}
	timeline := command.Timeline()
	nextSequenceNumber, err := nextSequenceAfterTimeline(
		command.WorkflowExecution().NextSequenceNumber(), len(timeline))
	if err != nil {
		return fmt.Errorf("calculate create execution sequence allocation: %w",
			err)
	}
	return store.withinTransaction(ctx,
		"create execution", func(tx pgx.Tx) error {
			if err := insertDefinitionSnapshotTransaction(
				ctx, tx, command.Snapshot(),
			); err != nil {
				return err
			}
			if err := insertWorkflowExecutionTransaction(ctx,
				tx, command.WorkflowExecution(), nextSequenceNumber,
			); err != nil {
				return err
			}
			actualNextSequenceNumber, err := insertTimelineEntriesTransaction(
				ctx, tx, command.WorkflowExecution().
					NextSequenceNumber(), timeline)
			if err != nil {
				return err
			}
			if actualNextSequenceNumber != nextSequenceNumber {
				return fmt.Errorf("create execution timeline sequence allocation is inconsistent")
			}
			return nil
		})
}

func insertDefinitionSnapshotTransaction(ctx context.Context,
	tx pgx.Tx, snapshot repository.DefinitionSnapshot) error {
	if tx == nil {
		return fmt.Errorf("create execution transaction must not be nil")
	}
	_, err := tx.Exec(ctx, insertDefinitionSnapshotTransactionSQL,
		snapshot.ID().String(), snapshot.CompanyID().String(), snapshot.WorkflowID().String(),
		int64(snapshot.WorkflowRevision()), snapshot.WorkflowName(), snapshot.DefinitionJSON().String(),
		snapshot.CreatedAt())
	if err != nil {
		return mapPostgreSQLError("create", "definition snapshot",
			err)
	}
	return nil
}
func insertWorkflowExecutionTransaction(ctx context.Context,
	tx pgx.Tx, record repository.WorkflowExecutionRecord, nextSequenceNumber repository.SequenceNumber,
) error {
	if tx == nil {
		return fmt.Errorf(
			"create execution transaction must not be nil")
	}
	_, err := tx.Exec(ctx,
		insertWorkflowExecutionTransactionSQL, record.ID().String(), record.CompanyID().String(),
		record.WorkflowID().String(), int64(record.WorkflowRevision()), record.SnapshotID().String(),
		record.Mode().String(), optionalValue(record.CorrelationID), record.Status().String(), record.CreatedAt(),
		optionalValue(record.ValidatingAt),
		optionalValue(record.QueuedAt),
		optionalValue(record.StartedAt),
		optionalValue(record.FinishedAt),
		record.UpdatedAt(), record.TerminalOutputs().String(), optionalStringValue(
			record.FailureSummary), record.IsStalled(),
		nextSequenceNumber.Int64(), record.LockVersion())
	if err != nil {
		return mapPostgreSQLError("create",
			"workflow execution", err)
	}
	return nil
}
func optionalValue[T any](getter func() (T, bool)) any {
	value, exists := getter()
	if !exists {
		return nil
	}
	return value
}
func optionalStringValue[T interface{ String() string }](getter func() (T, bool)) any {
	value, exists := getter()
	if !exists {
		return nil
	}
	return value.String()
}

const advanceWorkflowForNodeCreationSQL = `
UPDATE workflow_runtime.workflow_executions
SET
	next_sequence_number = $6,
	lock_version = lock_version + 1
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND status = $3
  AND lock_version = $4
  AND next_sequence_number = $5
`
const insertNodeExecutionTransactionSQL = `
INSERT INTO workflow_runtime.node_executions (
	node_execution_id,
	workflow_execution_id,
	company_id,
	node_id,
	plugin_type,
	plugin_version,
	status,
	attempt,
	retry_max_attempts,
	retry_initial_backoff_ns,
	retry_max_backoff_ns,
	next_attempt_at,
	created_at,
	ready_at,
	queued_at,
	started_at,
	finished_at,
	updated_at,
	input_summary,
	output_summary,
	failure_summary,
	lock_version
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6,
	$7,
	$8,
	$9,
	$10,
	$11,
	$12,
	$13,
	$14,
	$15,
	$16,
	$17,
	$18,
	$19::jsonb,
	$20::jsonb,
	$21::jsonb,
	$22
)
`

var _ repository.ExecutionCreationStore = (*Store)(nil)

func (store *Store) CreateNodeExecutions(
	ctx context.Context, command repository.CreateNodeExecutionsCommand) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("create node executions context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !command.IsValid() {
		return fmt.Errorf(
			"create node executions command must be valid")
	}
	nodeExecutions := command.NodeExecutions()
	timeline := command.Timeline()
	nextSequenceNumber, err := nextSequenceAfterTimeline(command.ExpectedNextSequenceNumber(),
		len(timeline))
	if err != nil {
		return fmt.Errorf("calculate node creation sequence allocation: %w", err)
	}
	return store.withinTransaction(ctx, "create node executions",
		func(tx pgx.Tx) error {
			if err := advanceWorkflowForNodeCreationTransaction(ctx,
				tx, command, nextSequenceNumber,
			); err != nil {
				return err
			}
			for _, nodeExecution := range nodeExecutions {
				if err := insertNodeExecutionTransaction(
					ctx, tx, nodeExecution,
				); err != nil {
					return err
				}
			}
			actualNextSequenceNumber, err :=
				insertTimelineEntriesTransaction(ctx, tx,
					command.ExpectedNextSequenceNumber(), timeline)
			if err != nil {
				return err
			}
			if actualNextSequenceNumber != nextSequenceNumber {
				return fmt.Errorf("node creation timeline sequence allocation is inconsistent")
			}
			return nil
		})
}
func advanceWorkflowForNodeCreationTransaction(ctx context.Context,
	tx pgx.Tx, command repository.CreateNodeExecutionsCommand, nextSequenceNumber repository.SequenceNumber,
) error {
	if tx == nil {
		return fmt.Errorf(
			"create node executions transaction must not be nil")
	}
	commandTag, err := tx.Exec(ctx,
		advanceWorkflowForNodeCreationSQL, command.CompanyID().String(), command.WorkflowExecutionID().String(),
		command.ExpectedWorkflowStatus().String(), command.ExpectedWorkflowLockVersion(), command.ExpectedNextSequenceNumber().Int64(),
		nextSequenceNumber.Int64())
	if err != nil {
		return mapPostgreSQLError("create", "node executions",
			err)
	}
	if commandTag.RowsAffected() != 1 {
		return repository.NewStaleWriteError(
			"create", "node executions", errors.New(
				"workflow execution state no longer matches the expected status, lock version or sequence"))
	}
	return nil
}
func insertNodeExecutionTransaction(
	ctx context.Context, tx pgx.Tx, record repository.NodeExecutionRecord,
) error {
	if tx == nil {
		return fmt.Errorf(
			"node execution transaction must not be nil")
	}
	retryMaxAttempts, retryInitialBackoff, retryMaxBackoff :=
		optionalRetryPolicyValues(record.RetryPolicy)
	_, err := tx.Exec(ctx,
		insertNodeExecutionTransactionSQL, record.ID().String(), record.WorkflowExecutionID().String(),
		record.CompanyID().String(), record.NodeID().String(), record.PluginType().String(),
		record.PluginVersion().String(), record.Status().String(), record.Attempt(),
		retryMaxAttempts, retryInitialBackoff, retryMaxBackoff, optionalValue(record.NextAttemptAt),
		record.CreatedAt(), optionalValue(record.ReadyAt), optionalValue(record.QueuedAt), optionalValue(record.StartedAt), optionalValue(record.FinishedAt), record.UpdatedAt(),
		optionalStringValue(
			record.InputSummary), optionalStringValue(
			record.OutputSummary), optionalStringValue(
			record.FailureSummary), record.LockVersion(),
	)
	if err != nil {
		return mapPostgreSQLError(
			"create", "node execution", err,
		)
	}
	return nil
}
