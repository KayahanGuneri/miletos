package postgres

import (
	"context"
	"errors"
	"fmt"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
	"strings"
	"time"
)

const nodeExecutionSelectColumns = `
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
`

var _ repository.NodeExecutionReader = (*Store)(nil)

func (store *Store) GetNodeExecution(ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID) (repository.NodeExecutionRecord, error) {
	if !store.IsValid() {
		return repository.NodeExecutionRecord{}, fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.NodeExecutionRecord{}, fmt.Errorf("get node execution context must not be nil")
	}
	row := store.pool.QueryRow(ctx,
		`SELECT `+nodeExecutionSelectColumns+` FROM workflow_runtime.node_executions WHERE company_id=$1 AND workflow_execution_id=$2 AND node_execution_id=$3`, companyID.String(),
		workflowExecutionID.String(), nodeExecutionID.String())
	record, err := scanNodeExecutionRecord(row)
	if err != nil {
		return repository.NodeExecutionRecord{}, mapPostgreSQLError("get", "node execution", err)
	}
	return record, nil
}

func (store *Store,
) ListNodeExecutions(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest) (repository.Page[repository.NodeExecutionRecord], error) {
	if !store.IsValid() {
		return repository.Page[repository.NodeExecutionRecord]{}, fmt.Errorf(
			"PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.Page[repository.NodeExecutionRecord]{},
			fmt.Errorf("list node executions context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.Page[repository.NodeExecutionRecord]{}, err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return repository.Page[repository.NodeExecutionRecord]{}, fmt.Errorf(
			"company ID must be valid: %w", err)
	}
	normalizedWorkflowExecutionID, err :=
		execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return repository.Page[repository.NodeExecutionRecord]{}, fmt.Errorf(
			"workflow execution ID must be valid: %w", err)
	}
	if !pageRequest.IsValid() {
		return repository.Page[repository.NodeExecutionRecord]{}, fmt.Errorf("node execution page request must be valid")
	}
	query, arguments, err := buildNodeExecutionListQuery(normalizedCompanyID, normalizedWorkflowExecutionID,
		pageRequest)
	if err != nil {
		return repository.Page[repository.NodeExecutionRecord]{}, err
	}
	if err := store.requireWorkflowExecutionScope(ctx, normalizedCompanyID,
		normalizedWorkflowExecutionID); err != nil {
		return repository.Page[repository.NodeExecutionRecord]{}, err
	}
	rows, err := store.pool.Query(
		ctx, query, arguments...,
	)
	if err != nil {
		return repository.Page[repository.NodeExecutionRecord]{},
			mapPostgreSQLError("list", "node executions",
				err)
	}
	defer rows.Close()
	records := make(
		[]repository.NodeExecutionRecord, 0, pageRequest.Limit()+1,
	)
	for rows.Next() {
		record, scanErr := scanNodeExecutionRecord(rows)
		if scanErr != nil {
			return repository.Page[repository.NodeExecutionRecord]{},
				mapPostgreSQLError("list", "node executions",
					scanErr)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return repository.Page[repository.NodeExecutionRecord]{},
			mapPostgreSQLError("list", "node executions",
				err)
	}
	var nextToken repository.PageToken
	if len(records) > pageRequest.Limit() {
		records = records[:pageRequest.Limit()]
		lastRecord := records[len(records)-1]
		nextToken, err = encodeTimestampCursor(
			cursorKindNodeExecutions, lastRecord.CreatedAt(), lastRecord.ID().String(),
		)
		if err != nil {
			return repository.Page[repository.NodeExecutionRecord]{},
				fmt.Errorf("encode node execution page token: %w", err)
		}
	}
	page, err := repository.NewPage(records,
		nextToken)
	if err != nil {
		return repository.Page[repository.NodeExecutionRecord]{}, fmt.Errorf("create node execution page: %w",
			err)
	}
	return page, nil
}
func buildNodeExecutionListQuery(companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest) (string, []any, error) {
	var builder strings.Builder
	builder.WriteString("SELECT\n")
	builder.WriteString(nodeExecutionSelectColumns)
	builder.WriteString("\nFROM node_executions\n")
	builder.WriteString("WHERE company_id = $1")
	builder.WriteString("\n  AND workflow_execution_id = $2")
	arguments := []any{
		companyID.String(), workflowExecutionID.String()}
	if token, exists := pageRequest.After(); exists {
		createdAt, nodeExecutionID, err := decodeTimestampCursor(
			cursorKindNodeExecutions, token)
		if err != nil {
			return "", nil, &repository.ValidationError{
				Field: "pageToken", Reason: "must contain a valid node execution cursor",
			}
		}
		arguments = append(arguments, createdAt,
			nodeExecutionID)
		createdAtPosition := len(arguments) - 1
		nodeExecutionIDPosition := len(arguments)
		builder.WriteString(fmt.Sprintf("\n  AND "+
			"(created_at, node_execution_id) "+"> ($%d, $%d)", createdAtPosition,
			nodeExecutionIDPosition))
	}
	builder.WriteString(
		"\nORDER BY created_at ASC, node_execution_id ASC")
	arguments = append(arguments, pageRequest.Limit()+1)
	builder.WriteString(
		fmt.Sprintf("\nLIMIT $%d", len(arguments)))
	return builder.String(), arguments, nil
}
func scanNodeExecutionRecord(row rowScanner) (repository.NodeExecutionRecord, error) {
	if row == nil {
		return repository.NodeExecutionRecord{}, fmt.Errorf(
			"node execution row must not be nil")
	}
	var (
		nodeExecutionID     string
		workflowExecutionID string
		companyID           string
		nodeID              string
		pluginType          string
		pluginVersion       string
		status              string
		attempt             int16
		retryMaxAttempts    pgtype.Int2
		retryInitialBackoff pgtype.Int8
		retryMaxBackoff     pgtype.Int8
		nextAttemptAt       pgtype.Timestamptz
		createdAt           time.Time
		readyAt             pgtype.Timestamptz
		queuedAt            pgtype.Timestamptz
		startedAt           pgtype.Timestamptz
		finishedAt          pgtype.Timestamptz
		updatedAt           time.Time
		inputSummary        []byte
		outputSummary       []byte
		failureSummary      []byte
		lockVersion         int64
	)
	if err := row.Scan(&nodeExecutionID, &workflowExecutionID,
		&companyID, &nodeID, &pluginType,
		&pluginVersion, &status, &attempt,
		&retryMaxAttempts, &retryInitialBackoff, &retryMaxBackoff,
		&nextAttemptAt,
		&createdAt, &readyAt, &queuedAt,
		&startedAt, &finishedAt, &updatedAt,
		&inputSummary, &outputSummary, &failureSummary,
		&lockVersion); err != nil {
		return repository.NodeExecutionRecord{}, err
	}
	retryPolicy, err := retryPolicyFromNullable(
		retryMaxAttempts, retryInitialBackoff, retryMaxBackoff,
	)
	if err != nil {
		return repository.NodeExecutionRecord{}, err
	}
	record, err := repository.NewNodeExecutionRecord(
		repository.NodeExecutionRecordParams{ID: execution.NodeExecutionID(nodeExecutionID), WorkflowExecutionID: execution.WorkflowExecutionID(workflowExecutionID),
			CompanyID: workflow.CompanyID(companyID), NodeID: workflow.NodeID(nodeID), PluginType: workflow.PluginType(pluginType), PluginVersion: workflow.PluginVersion(pluginVersion),
			Status: execution.NodeExecutionStatus(status), Attempt: attempt, RetryPolicy: retryPolicy,
			NextAttemptAt: nullableTimestampValue(nextAttemptAt), CreatedAt: createdAt,
			ReadyAt: nullableTimestampValue(readyAt), QueuedAt: nullableTimestampValue(queuedAt), StartedAt: nullableTimestampValue(startedAt),
			FinishedAt: nullableTimestampValue(finishedAt), UpdatedAt: updatedAt, InputSummary: inputSummary,
			OutputSummary: outputSummary, FailureSummary: failureSummary, LockVersion: lockVersion,
		})
	if err != nil {
		return repository.NodeExecutionRecord{}, fmt.Errorf("reconstruct node execution: %w",
			err)
	}
	return record, nil
}

const advanceWorkflowForNodeTransitionSQL = `
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
const updateNodeTransitionSQL = `
UPDATE workflow_runtime.node_executions
SET
	status = $7,
	attempt = $8,
	retry_max_attempts = $9,
	retry_initial_backoff_ns = $10,
	retry_max_backoff_ns = $11,
	next_attempt_at = $12,
	ready_at = $13,
	queued_at = $14,
	started_at = $15,
	finished_at = $16,
	updated_at = $17,
	input_summary = $18::jsonb,
	output_summary = $19::jsonb,
	failure_summary = $20::jsonb,
	lock_version = $21
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND node_execution_id = $3
  AND status = $4
  AND lock_version = $5
  AND attempt = $6
`

var _ repository.NodeTransitionStore = (*Store)(nil)
var _ repository.ExecutionLifecycleStore = (*Store)(nil)

func (store *Store,
) ApplyNodeTransition(ctx context.Context, command repository.NodeTransitionCommand,
) error {
	if !store.IsValid() {
		return fmt.Errorf(
			"PostgreSQL store must be valid")
	}
	if ctx == nil {
		return fmt.Errorf(
			"node transition context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !command.IsValid() {
		return fmt.Errorf("node transition command must be valid")
	}
	timeline := command.Timeline()
	nextSequenceNumber, err := nextSequenceAfterTimeline(command.ExpectedNextSequenceNumber(),
		len(timeline))
	if err != nil {
		return fmt.Errorf("calculate node transition sequence allocation: %w", err)
	}
	return store.withinTransaction(ctx, "apply node transition",
		func(tx pgx.Tx) error {
			if err := advanceWorkflowForNodeTransitionTransaction(ctx,
				tx, command, nextSequenceNumber,
			); err != nil {
				return err
			}
			if err := updateNodeTransitionTransaction(ctx,
				tx, command); err != nil {
				return err
			}
			if attemptMutation, exists := command.AttemptMutation(); exists {
				if err := applyNodeAttemptMutationTransaction(
					ctx, tx, attemptMutation,
				); err != nil {
					return err
				}
			}
			actualNextSequenceNumber, err := insertTimelineEntriesTransaction(ctx,
				tx, command.ExpectedNextSequenceNumber(), timeline,
			)
			if err != nil {
				return err
			}
			if actualNextSequenceNumber !=
				nextSequenceNumber {
				return fmt.Errorf("node transition timeline sequence allocation is inconsistent")
			}
			if err := insertExecutionErrorsTransaction(ctx, tx,
				command.Errors()); err != nil {
				return err
			}
			return nil
		})
}
func advanceWorkflowForNodeTransitionTransaction(ctx context.Context,
	tx pgx.Tx, command repository.NodeTransitionCommand, nextSequenceNumber repository.SequenceNumber,
) error {
	if tx == nil {
		return fmt.Errorf(
			"node transition workflow transaction must not be nil")
	}
	commandTag, err := tx.Exec(ctx,
		advanceWorkflowForNodeTransitionSQL, command.CompanyID().String(), command.WorkflowExecutionID().String(),
		command.ExpectedWorkflowStatus().String(), command.ExpectedWorkflowLockVersion(), command.ExpectedNextSequenceNumber().Int64(),
		nextSequenceNumber.Int64())
	if err != nil {
		return mapPostgreSQLError("update", "workflow execution for node transition",
			err)
	}
	if commandTag.RowsAffected() != 1 {
		return repository.NewStaleWriteError(
			"update", "workflow execution for node transition", errors.New(
				"workflow execution state no longer matches the expected status, lock version or sequence"))
	}
	return nil
}
func updateNodeTransitionTransaction(
	ctx context.Context, tx pgx.Tx, command repository.NodeTransitionCommand,
) error {
	if tx == nil {
		return fmt.Errorf(
			"node transition transaction must not be nil")
	}
	record := command.NodeExecution()
	retryMaxAttempts, retryInitialBackoff, retryMaxBackoff :=
		optionalRetryPolicyValues(record.RetryPolicy)
	commandTag, err := tx.Exec(ctx, updateNodeTransitionSQL,
		command.CompanyID().String(), command.WorkflowExecutionID().String(), command.NodeExecutionID().String(),
		command.ExpectedNodeStatus().String(), command.ExpectedNodeLockVersion(), command.ExpectedNodeAttempt(),
		record.Status().String(), record.Attempt(),
		retryMaxAttempts, retryInitialBackoff, retryMaxBackoff,
		optionalValue(record.NextAttemptAt), optionalValue(record.ReadyAt), optionalValue(record.QueuedAt), optionalValue(record.StartedAt), optionalValue(record.FinishedAt), record.UpdatedAt(),
		optionalStringValue(
			record.InputSummary), optionalStringValue(
			record.OutputSummary), optionalStringValue(
			record.FailureSummary), record.LockVersion(),
	)
	if err != nil {
		return mapPostgreSQLError(
			"update", "node execution", err,
		)
	}
	if commandTag.RowsAffected() != 1 {
		return repository.NewStaleWriteError("update",
			"node execution", errors.New("node execution state no longer matches the expected status, lock version or attempt"))
	}
	return nil
}

func optionalRetryPolicyValues(
	getter func() (execution.RetryPolicy, bool),
) (any, any, any) {
	policy, exists := getter()
	if !exists {
		return nil, nil, nil
	}
	return policy.MaxAttempts().Int16(),
		int64(policy.InitialBackoff()), int64(policy.MaxBackoff())
}

func retryPolicyFromNullable(
	maxAttempts pgtype.Int2,
	initialBackoff pgtype.Int8,
	maxBackoff pgtype.Int8,
) (execution.RetryPolicy, error) {
	if !maxAttempts.Valid && !initialBackoff.Valid && !maxBackoff.Valid {
		return execution.RetryPolicy{}, nil
	}
	if !maxAttempts.Valid || !initialBackoff.Valid || !maxBackoff.Valid {
		return execution.RetryPolicy{}, fmt.Errorf(
			"node execution retry policy columns must be all null or all non-null")
	}
	attempts, err := execution.NewAttemptNumber(maxAttempts.Int16)
	if err != nil {
		return execution.RetryPolicy{}, fmt.Errorf(
			"reconstruct node execution retry policy max attempts: %w", err)
	}
	policy, err := execution.NewRetryPolicy(
		attempts, time.Duration(initialBackoff.Int64), time.Duration(maxBackoff.Int64),
	)
	if err != nil {
		return execution.RetryPolicy{}, fmt.Errorf(
			"reconstruct node execution retry policy: %w", err)
	}
	return policy, nil
}
