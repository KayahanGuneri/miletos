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

const workflowExecutionSelectColumns = `
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
`
const getWorkflowExecutionSQL = `
SELECT
` +
	workflowExecutionSelectColumns + `
FROM workflow_executions
WHERE company_id = $1
  AND workflow_execution_id = $2
`

var _ repository.WorkflowExecutionReader = (*Store)(nil)

func (
	store *Store) GetWorkflowExecution(ctx context.Context,
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID) (repository.WorkflowExecutionRecord, error) {
	if !store.IsValid() {
		return repository.WorkflowExecutionRecord{}, fmt.Errorf(
			"PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.WorkflowExecutionRecord{},
			fmt.Errorf("get workflow execution context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.WorkflowExecutionRecord{}, err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(
		companyID.String())
	if err != nil {
		return repository.WorkflowExecutionRecord{}, fmt.Errorf("company ID must be valid: %w",
			err)
	}
	normalizedExecutionID, err := execution.NewWorkflowExecutionID(
		workflowExecutionID.String())
	if err != nil {
		return repository.WorkflowExecutionRecord{}, fmt.Errorf("workflow execution ID must be valid: %w",
			err)
	}
	record, err := scanWorkflowExecutionRecord(store.pool.QueryRow(
		ctx, getWorkflowExecutionSQL, normalizedCompanyID.String(),
		normalizedExecutionID.String()))
	if err != nil {
		return repository.WorkflowExecutionRecord{}, mapPostgreSQLError(
			"get", "workflow execution", err,
		)
	}
	return record, nil
}
func (store *Store) ListWorkflowExecutions(
	ctx context.Context, companyID workflow.CompanyID, filter repository.WorkflowExecutionFilter,
	pageRequest repository.PageRequest) (repository.Page[repository.WorkflowExecutionRecord],
	error) {
	if !store.IsValid() {
		return repository.Page[repository.WorkflowExecutionRecord]{}, fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.Page[repository.WorkflowExecutionRecord]{}, fmt.Errorf(
			"list workflow executions context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.Page[repository.WorkflowExecutionRecord]{},
			err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return repository.Page[repository.WorkflowExecutionRecord]{},
			fmt.Errorf("company ID must be valid: %w", err)
	}
	if !filter.IsValid() {
		return repository.Page[repository.WorkflowExecutionRecord]{}, fmt.Errorf(
			"workflow execution filter must be valid")
	}
	if !pageRequest.IsValid() {
		return repository.Page[repository.WorkflowExecutionRecord]{},
			fmt.Errorf("workflow execution page request must be valid")
	}
	query, arguments, err :=
		buildWorkflowExecutionListQuery(normalizedCompanyID, filter,
			pageRequest)
	if err != nil {
		return repository.Page[repository.WorkflowExecutionRecord]{}, err
	}
	rows, err := store.pool.Query(ctx,
		query, arguments...)
	if err != nil {
		return repository.Page[repository.WorkflowExecutionRecord]{}, mapPostgreSQLError(
			"list", "workflow executions", err,
		)
	}
	defer rows.Close()
	records := make([]repository.WorkflowExecutionRecord,
		0, pageRequest.Limit()+1)
	for rows.Next() {
		record, scanErr :=
			scanWorkflowExecutionRecord(rows)
		if scanErr != nil {
			return repository.Page[repository.WorkflowExecutionRecord]{},
				mapPostgreSQLError("list", "workflow executions",
					scanErr)
		}
		records = append(records,
			record)
	}
	if err := rows.Err(); err != nil {
		return repository.Page[repository.WorkflowExecutionRecord]{},
			mapPostgreSQLError("list", "workflow executions",
				err)
	}
	var nextToken repository.PageToken
	if len(records) > pageRequest.Limit() {
		records = records[:pageRequest.Limit()]
		lastRecord := records[len(records)-1]
		nextToken, err = encodeTimestampCursor(
			cursorKindWorkflowExecutions, lastRecord.CreatedAt(), lastRecord.ID().String(),
		)
		if err != nil {
			return repository.Page[repository.WorkflowExecutionRecord]{},
				fmt.Errorf("encode workflow execution page token: %w", err)
		}
	}
	page, err := repository.NewPage(records,
		nextToken)
	if err != nil {
		return repository.Page[repository.WorkflowExecutionRecord]{}, fmt.Errorf("create workflow execution page: %w",
			err)
	}
	return page, nil
}
func buildWorkflowExecutionListQuery(companyID workflow.CompanyID,
	filter repository.WorkflowExecutionFilter, pageRequest repository.PageRequest) (
	string, []any, error,
) {
	var builder strings.Builder
	builder.WriteString("SELECT\n")
	builder.WriteString(workflowExecutionSelectColumns)
	builder.WriteString(
		"\nFROM workflow_executions\n")
	builder.WriteString("WHERE company_id = $1")
	arguments := []any{companyID.String()}
	if workflowID, exists :=
		filter.WorkflowID(); exists {
		arguments = append(arguments,
			workflowID.String())
		builder.WriteString(fmt.Sprintf("\n  AND workflow_id = $%d",
			len(arguments)))
	}
	if status, exists :=
		filter.Status(); exists {
		arguments = append(arguments,
			status.String())
		builder.WriteString(fmt.Sprintf("\n  AND status = $%d",
			len(arguments)))
	}
	if token, exists :=
		pageRequest.After(); exists {
		createdAt, executionID, err := decodeTimestampCursor(
			cursorKindWorkflowExecutions, token)
		if err != nil {
			return "", nil, &repository.ValidationError{
				Field: "pageToken", Reason: "must contain a valid workflow execution cursor",
			}
		}
		arguments = append(arguments, createdAt,
			executionID)
		createdAtPosition := len(arguments) - 1
		executionIDPosition := len(arguments)
		builder.WriteString(fmt.Sprintf("\n  AND "+
			"(created_at, workflow_execution_id) "+"< ($%d, $%d)", createdAtPosition,
			executionIDPosition))
	}
	builder.WriteString(
		"\nORDER BY " + "created_at DESC, " + "workflow_execution_id DESC",
	)
	arguments = append(
		arguments, pageRequest.Limit()+1)
	builder.WriteString(fmt.Sprintf(
		"\nLIMIT $%d", len(arguments)),
	)
	return builder.String(),
		arguments, nil
}
func scanWorkflowExecutionRecord(row rowScanner,
) (repository.WorkflowExecutionRecord, error) {
	if row == nil {
		return repository.WorkflowExecutionRecord{},
			fmt.Errorf("workflow execution row must not be nil")
	}
	var (
		workflowExecutionID string
		companyID           string
		workflowID          string
		workflowRevision    int64
		snapshotID          string
		mode                string
		correlationID       pgtype.Text
		status              string
		createdAt           time.Time
		validatingAt        pgtype.Timestamptz
		queuedAt            pgtype.Timestamptz
		startedAt           pgtype.Timestamptz
		finishedAt          pgtype.Timestamptz
		updatedAt           time.Time
		terminalOutputs     []byte
		failureSummary      []byte
		isStalled           bool
		nextSequenceNumber  int64
		lockVersion         int64
	)
	if err := row.Scan(&workflowExecutionID, &companyID,
		&workflowID, &workflowRevision, &snapshotID,
		&mode, &correlationID, &status,
		&createdAt, &validatingAt, &queuedAt,
		&startedAt, &finishedAt, &updatedAt,
		&terminalOutputs, &failureSummary, &isStalled,
		&nextSequenceNumber, &lockVersion); err != nil {
		return repository.WorkflowExecutionRecord{}, err
	}
	if workflowRevision <= 0 {
		return repository.WorkflowExecutionRecord{},
			fmt.Errorf("workflow execution revision is invalid")
	}
	sequenceNumber, err :=
		repository.NewSequenceNumber(nextSequenceNumber)
	if err != nil {
		return repository.WorkflowExecutionRecord{}, fmt.Errorf(
			"workflow execution sequence is invalid: %w", err)
	}
	record, err :=
		repository.NewWorkflowExecutionRecord(repository.WorkflowExecutionRecordParams{ID: execution.WorkflowExecutionID(
			workflowExecutionID), CompanyID: workflow.CompanyID(
			companyID), WorkflowID: workflow.WorkflowID(
			workflowID), WorkflowRevision: uint64(workflowRevision),
			SnapshotID:    repository.DefinitionSnapshotID(snapshotID),
			Mode:          execution.ExecutionMode(mode),
			CorrelationID: nullableTextValue(correlationID),
			Status:        execution.WorkflowExecutionStatus(status),
			CreatedAt:     createdAt, ValidatingAt: nullableTimestampValue(validatingAt), QueuedAt: nullableTimestampValue(queuedAt), StartedAt: nullableTimestampValue(startedAt),
			FinishedAt: nullableTimestampValue(finishedAt), UpdatedAt: updatedAt, TerminalOutputs: terminalOutputs,
			FailureSummary: failureSummary, IsStalled: isStalled, NextSequenceNumber: sequenceNumber,
			LockVersion: lockVersion})
	if err != nil {
		return repository.WorkflowExecutionRecord{}, fmt.Errorf(
			"reconstruct workflow execution: %w", err)
	}
	return record, nil
}

const updateWorkflowTransitionSQL = `
UPDATE workflow_runtime.workflow_executions
SET
	status = $6,
	validating_at = $7,
	queued_at = $8,
	started_at = $9,
	finished_at = $10,
	updated_at = $11,
	terminal_outputs = $12::jsonb,
	failure_summary = $13::jsonb,
	is_stalled = $14,
	next_sequence_number = $15,
	lock_version = $16
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND status = $3
  AND lock_version = $4
  AND next_sequence_number = $5
`

var _ repository.WorkflowTransitionStore = (*Store)(nil)

func (store *Store) ApplyWorkflowTransition(
	ctx context.Context, command repository.WorkflowTransitionCommand) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("workflow transition context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !command.IsValid() {
		return fmt.Errorf(
			"workflow transition command must be valid")
	}
	timeline := command.Timeline()
	workflowExecution := command.WorkflowExecution()
	calculatedNextSequence, err := nextSequenceAfterTimeline(
		command.ExpectedNextSequenceNumber(), len(timeline))
	if err != nil {
		return fmt.Errorf("calculate workflow transition sequence allocation: %w",
			err)
	}
	if calculatedNextSequence != workflowExecution.NextSequenceNumber() {
		return fmt.Errorf("workflow transition next sequence number is inconsistent")
	}
	return store.withinTransaction(
		ctx, "apply workflow transition", func(tx pgx.Tx) error {
			if err := updateWorkflowTransitionTransaction(ctx, tx,
				command); err != nil {
				return err
			}
			actualNextSequence, err :=
				insertTimelineEntriesTransaction(ctx, tx,
					command.ExpectedNextSequenceNumber(), timeline)
			if err != nil {
				return err
			}
			if actualNextSequence != workflowExecution.NextSequenceNumber() {
				return fmt.Errorf("workflow transition timeline sequence allocation is inconsistent")
			}
			if err := insertExecutionErrorsTransaction(
				ctx, tx, command.Errors(),
			); err != nil {
				return err
			}
			return nil
		},
	)
}
func updateWorkflowTransitionTransaction(ctx context.Context, tx pgx.Tx,
	command repository.WorkflowTransitionCommand) error {
	if tx == nil {
		return fmt.Errorf("workflow transition transaction must not be nil")
	}
	record := command.WorkflowExecution()
	commandTag, err := tx.Exec(ctx,
		updateWorkflowTransitionSQL, command.CompanyID().String(), command.WorkflowExecutionID().String(),
		command.ExpectedStatus().String(), command.ExpectedLockVersion(), command.ExpectedNextSequenceNumber().Int64(),
		record.Status().String(), optionalValue(record.ValidatingAt), optionalValue(record.QueuedAt), optionalValue(record.StartedAt), optionalValue(record.FinishedAt),
		record.UpdatedAt(), record.TerminalOutputs().String(),
		optionalStringValue(record.FailureSummary),
		record.IsStalled(), record.NextSequenceNumber().Int64(), record.LockVersion(),
	)
	if err != nil {
		return mapPostgreSQLError(
			"update", "workflow execution", err,
		)
	}
	if commandTag.RowsAffected() != 1 {
		return repository.NewStaleWriteError("update",
			"workflow execution", errors.New("workflow execution state no longer matches the expected status, lock version or sequence"))
	}
	return nil
}
