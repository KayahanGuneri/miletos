package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

const executionEventSelectColumns = `
	event_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
	sequence_number,
	event_type,
	previous_status,
	new_status,
	correlation_id,
	causation_id,
	safe_message,
	metadata,
	created_at
`

var _ repository.ExecutionEventReader = (*Store)(nil)

func (store *Store) ListExecutionEvents(
	ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	pageRequest repository.PageRequest) (repository.Page[repository.ExecutionEventRecord], error) {
	if !store.IsValid() {
		return repository.Page[repository.ExecutionEventRecord]{}, fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.Page[repository.ExecutionEventRecord]{}, fmt.Errorf(
			"list execution events context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.Page[repository.ExecutionEventRecord]{},
			err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return repository.Page[repository.ExecutionEventRecord]{}, fmt.Errorf(
			"company ID must be valid: %w", err)
	}
	normalizedWorkflowExecutionID, err :=
		execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return repository.Page[repository.ExecutionEventRecord]{}, fmt.Errorf(
			"workflow execution ID must be valid: %w", err)
	}
	if !pageRequest.IsValid() {
		return repository.Page[repository.ExecutionEventRecord]{}, fmt.Errorf("execution event page request must be valid")
	}
	query, arguments, err := buildExecutionEventListQuery(normalizedCompanyID, normalizedWorkflowExecutionID,
		pageRequest)
	if err != nil {
		return repository.Page[repository.ExecutionEventRecord]{}, err
	}
	if err := store.requireWorkflowExecutionScope(ctx,
		normalizedCompanyID, normalizedWorkflowExecutionID); err != nil {
		return repository.Page[repository.ExecutionEventRecord]{}, err
	}
	rows, err := store.pool.Query(ctx,
		query, arguments...)
	if err != nil {
		return repository.Page[repository.ExecutionEventRecord]{}, mapPostgreSQLError(
			"list", "execution events", err,
		)
	}
	defer rows.Close()
	records := make([]repository.ExecutionEventRecord,
		0, pageRequest.Limit()+1)
	for rows.Next() {
		record, scanErr := scanExecutionEventRecord(rows)
		if scanErr != nil {
			return repository.Page[repository.ExecutionEventRecord]{}, mapPostgreSQLError(
				"list", "execution events", scanErr,
			)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return repository.Page[repository.ExecutionEventRecord]{}, mapPostgreSQLError(
			"list", "execution events", err,
		)
	}
	var nextToken repository.PageToken
	if len(records) > pageRequest.Limit() {
		records = records[:pageRequest.Limit()]
		lastRecord := records[len(records)-1]
		nextToken, err = encodeSequenceCursor(cursorKindExecutionEvents,
			lastRecord.SequenceNumber())
		if err != nil {
			return repository.Page[repository.ExecutionEventRecord]{}, fmt.Errorf("encode execution event page token: %w",
				err)
		}
	}
	page, err := repository.NewPage(
		records, nextToken)
	if err != nil {
		return repository.Page[repository.ExecutionEventRecord]{}, fmt.Errorf(
			"create execution event page: %w", err)
	}
	return page, nil
}
func buildExecutionEventListQuery(
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest,
) (string, []any,
	error) {
	var builder strings.Builder
	builder.WriteString("SELECT\n")
	builder.WriteString(executionEventSelectColumns)
	builder.WriteString("\nFROM execution_events\n")
	builder.WriteString("WHERE company_id = $1")
	builder.WriteString("\n  AND workflow_execution_id = $2")
	arguments := []any{companyID.String(),
		workflowExecutionID.String()}
	if token, exists := pageRequest.After(); exists {
		sequence, err := decodeSequenceCursor(cursorKindExecutionEvents,
			token)
		if err != nil {
			return "", nil, &repository.ValidationError{Field: "pageToken",
				Reason: "must contain a valid execution event cursor"}
		}
		arguments = append(arguments,
			sequence.Int64())
		builder.WriteString(fmt.Sprintf("\n  AND sequence_number > $%d",
			len(arguments)))
	}
	builder.WriteString(
		"\nORDER BY sequence_number ASC")
	arguments = append(arguments, pageRequest.Limit()+1)
	builder.WriteString(
		fmt.Sprintf("\nLIMIT $%d", len(arguments)))
	return builder.String(), arguments, nil
}
func scanExecutionEventRecord(
	row rowScanner) (repository.ExecutionEventRecord, error) {
	if row == nil {
		return repository.ExecutionEventRecord{}, fmt.Errorf("execution event row must not be nil")
	}
	var (
		eventID             string
		workflowExecutionID string
		companyID           string
		nodeExecutionID     pgtype.Text
		sequenceNumber      int64
		eventType           string
		previousStatus      pgtype.Text
		newStatus           pgtype.Text
		correlationID       pgtype.Text
		causationID         pgtype.Text
		safeMessage         pgtype.Text
		metadata            []byte
		createdAt           time.Time
	)
	if err := row.Scan(
		&eventID, &workflowExecutionID, &companyID,
		&nodeExecutionID, &sequenceNumber, &eventType,
		&previousStatus, &newStatus, &correlationID,
		&causationID, &safeMessage, &metadata,
		&createdAt); err != nil {
		return repository.ExecutionEventRecord{},
			err
	}
	normalizedSequence, err := repository.NewSequenceNumber(sequenceNumber)
	if err != nil {
		return repository.ExecutionEventRecord{},
			fmt.Errorf("execution event sequence is invalid: %w", err)
	}
	record, err := repository.NewExecutionEventRecord(repository.ExecutionEventRecordParams{ID: repository.ExecutionEventID(
		eventID), WorkflowExecutionID: execution.WorkflowExecutionID(
		workflowExecutionID), CompanyID: workflow.CompanyID(
		companyID), NodeExecutionID: execution.NodeExecutionID(
		nullableTextValue(nodeExecutionID),
	), SequenceNumber: normalizedSequence, Type: repository.ExecutionEventType(
		eventType), PreviousStatus: nullableTextValue(
		previousStatus), NewStatus: nullableTextValue(
		newStatus), CorrelationID: nullableTextValue(
		correlationID), CausationID: nullableTextValue(
		causationID), SafeMessage: nullableTextValue(
		safeMessage), Metadata: metadata,
		CreatedAt: createdAt})
	if err != nil {
		return repository.ExecutionEventRecord{}, fmt.Errorf(
			"reconstruct execution event: %w", err)
	}
	return record, nil
}

const insertExecutionEventTransactionSQL = `
INSERT INTO workflow_runtime.execution_events (
	event_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
	sequence_number,
	event_type,
	previous_status,
	new_status,
	correlation_id,
	causation_id,
	safe_message,
	metadata,
	created_at
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
	$12::jsonb,
	$13
)
`

func insertExecutionEventTransaction(ctx context.Context, tx pgx.Tx,
	record repository.ExecutionEventRecord) error {
	_, err := tx.Exec(
		ctx, insertExecutionEventTransactionSQL, record.ID().String(),
		record.WorkflowExecutionID().String(), record.CompanyID().String(), optionalStringValue(
			record.NodeExecutionID), record.SequenceNumber().Int64(),
		record.Type().String(), optionalValue(record.PreviousStatus), optionalValue(record.NewStatus), optionalValue(record.CorrelationID), optionalValue(record.CausationID),
		optionalValue(record.SafeMessage), record.Metadata().String(), record.CreatedAt(),
	)
	if err != nil {
		return mapPostgreSQLError(
			"create", "execution event", err,
		)
	}
	return nil
}
