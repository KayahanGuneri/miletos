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

const executionLogSelectColumns = `
	log_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
	sequence_number,
	level,
	message,
	metadata,
	created_at
`

var _ repository.ExecutionLogReader = (*Store)(nil)

func (
	store *Store) ListExecutionLogs(ctx context.Context,
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest,
) (repository.Page[repository.ExecutionLogRecord], error) {
	if !store.IsValid() {
		return repository.Page[repository.ExecutionLogRecord]{},
			fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.Page[repository.ExecutionLogRecord]{}, fmt.Errorf("list execution logs context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.Page[repository.ExecutionLogRecord]{}, err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(
		companyID.String())
	if err != nil {
		return repository.Page[repository.ExecutionLogRecord]{}, fmt.Errorf("company ID must be valid: %w",
			err)
	}
	normalizedWorkflowExecutionID, err := execution.NewWorkflowExecutionID(
		workflowExecutionID.String())
	if err != nil {
		return repository.Page[repository.ExecutionLogRecord]{}, fmt.Errorf("workflow execution ID must be valid: %w",
			err)
	}
	if !pageRequest.IsValid() {
		return repository.Page[repository.ExecutionLogRecord]{},
			fmt.Errorf("execution log page request must be valid")
	}
	query, arguments, err := buildExecutionLogListQuery(
		normalizedCompanyID, normalizedWorkflowExecutionID, pageRequest,
	)
	if err != nil {
		return repository.Page[repository.ExecutionLogRecord]{},
			err
	}
	if err := store.requireWorkflowExecutionScope(ctx, normalizedCompanyID,
		normalizedWorkflowExecutionID); err != nil {
		return repository.Page[repository.ExecutionLogRecord]{},
			err
	}
	rows, err := store.pool.Query(ctx, query,
		arguments...)
	if err != nil {
		return repository.Page[repository.ExecutionLogRecord]{}, mapPostgreSQLError("list",
			"execution logs", err)
	}
	defer rows.Close()
	records := make([]repository.ExecutionLogRecord, 0,
		pageRequest.Limit()+1)
	for rows.Next() {
		record, scanErr := scanExecutionLogRecord(rows)
		if scanErr != nil {
			return repository.Page[repository.ExecutionLogRecord]{}, mapPostgreSQLError("list",
				"execution logs", scanErr)
		}
		records = append(
			records, record)
	}
	if err := rows.Err(); err != nil {
		return repository.Page[repository.ExecutionLogRecord]{}, mapPostgreSQLError("list",
			"execution logs", err)
	}
	var nextToken repository.PageToken
	if len(records) > pageRequest.Limit() {
		records = records[:pageRequest.Limit()]
		lastRecord := records[len(records)-1]
		nextToken, err = encodeSequenceCursor(cursorKindExecutionLogs, lastRecord.SequenceNumber())
		if err != nil {
			return repository.Page[repository.ExecutionLogRecord]{},
				fmt.Errorf("encode execution log page token: %w", err)
		}
	}
	page, err := repository.NewPage(records,
		nextToken)
	if err != nil {
		return repository.Page[repository.ExecutionLogRecord]{}, fmt.Errorf("create execution log page: %w",
			err)
	}
	return page, nil
}
func buildExecutionLogListQuery(companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest) (
	string, []any, error,
) {
	var builder strings.Builder
	builder.WriteString("SELECT\n")
	builder.WriteString(executionLogSelectColumns)
	builder.WriteString("\nFROM execution_logs\n")
	builder.WriteString("WHERE company_id = $1")
	builder.WriteString("\n  AND workflow_execution_id = $2")
	arguments := []any{companyID.String(), workflowExecutionID.String()}
	if token, exists := pageRequest.After(); exists {
		sequence, err := decodeSequenceCursor(cursorKindExecutionLogs, token)
		if err != nil {
			return "", nil,
				&repository.ValidationError{Field: "pageToken", Reason: "must contain a valid execution log cursor"}
		}
		arguments = append(arguments, sequence.Int64())
		builder.WriteString(
			fmt.Sprintf("\n  AND sequence_number > $%d", len(arguments)))
	}
	builder.WriteString("\nORDER BY sequence_number ASC")
	arguments = append(
		arguments, pageRequest.Limit()+1)
	builder.WriteString(fmt.Sprintf(
		"\nLIMIT $%d", len(arguments)),
	)
	return builder.String(),
		arguments, nil
}
func scanExecutionLogRecord(row rowScanner,
) (repository.ExecutionLogRecord, error) {
	if row == nil {
		return repository.ExecutionLogRecord{},
			fmt.Errorf("execution log row must not be nil")
	}
	var (
		logID               string
		workflowExecutionID string
		companyID           string
		nodeExecutionID     pgtype.Text
		sequenceNumber      int64
		level               string
		message             string
		metadata            []byte
		createdAt           time.Time
	)
	if err := row.Scan(&logID, &workflowExecutionID,
		&companyID, &nodeExecutionID, &sequenceNumber,
		&level, &message, &metadata,
		&createdAt); err != nil {
		return repository.ExecutionLogRecord{},
			err
	}
	normalizedSequence, err := repository.NewSequenceNumber(sequenceNumber)
	if err != nil {
		return repository.ExecutionLogRecord{},
			fmt.Errorf("execution log sequence is invalid: %w", err)
	}
	record, err := repository.NewExecutionLogRecord(repository.ExecutionLogRecordParams{ID: repository.ExecutionLogID(
		logID), WorkflowExecutionID: execution.WorkflowExecutionID(
		workflowExecutionID), CompanyID: workflow.CompanyID(
		companyID), NodeExecutionID: execution.NodeExecutionID(
		nullableTextValue(nodeExecutionID),
	), SequenceNumber: normalizedSequence, Level: repository.ExecutionLogLevel(
		level), Message: message,
		Metadata: metadata, CreatedAt: createdAt},
	)
	if err != nil {
		return repository.ExecutionLogRecord{},
			fmt.Errorf("reconstruct execution log: %w", err)
	}
	return record, nil
}

const insertExecutionLogTransactionSQL = `
INSERT INTO workflow_runtime.execution_logs (
	log_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
	sequence_number,
	level,
	message,
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
	$8::jsonb,
	$9
)
`

func insertExecutionLogTransaction(ctx context.Context, tx pgx.Tx,
	record repository.ExecutionLogRecord) error {
	_, err := tx.Exec(
		ctx, insertExecutionLogTransactionSQL, record.ID().String(),
		record.WorkflowExecutionID().String(), record.CompanyID().String(), optionalStringValue(
			record.NodeExecutionID), record.SequenceNumber().Int64(),
		record.Level().String(), record.Message(), record.Metadata().String(),
		record.CreatedAt())
	if err != nil {
		return mapPostgreSQLError("create", "execution log",
			err)
	}
	return nil
}
