package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	runtimefailure "miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

const executionErrorSelectColumns = `
	error_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
	related_event_id,
	category,
	code,
	safe_message,
	technical_detail,
	retryable,
	details,
	created_at
`

var _ repository.ExecutionErrorReader = (*Store)(nil)

func (store *Store) ListExecutionErrors(
	ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	pageRequest repository.PageRequest) (repository.Page[repository.ExecutionErrorRecord], error) {
	if !store.IsValid() {
		return repository.Page[repository.ExecutionErrorRecord]{}, fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, fmt.Errorf(
			"list execution errors context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{},
			err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, fmt.Errorf(
			"company ID must be valid: %w", err)
	}
	normalizedWorkflowExecutionID, err :=
		execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, fmt.Errorf(
			"workflow execution ID must be valid: %w", err)
	}
	if !pageRequest.IsValid() {
		return repository.Page[repository.ExecutionErrorRecord]{}, fmt.Errorf("execution error page request must be valid")
	}
	query, arguments, err := buildExecutionErrorListQuery(normalizedCompanyID, normalizedWorkflowExecutionID,
		pageRequest)
	if err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, err
	}
	if err := store.requireWorkflowExecutionScope(ctx,
		normalizedCompanyID, normalizedWorkflowExecutionID); err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, err
	}
	rows, err := store.pool.Query(ctx,
		query, arguments...)
	if err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, mapPostgreSQLError(
			"list", "execution errors", err,
		)
	}
	defer rows.Close()
	records := make([]repository.ExecutionErrorRecord,
		0, pageRequest.Limit()+1)
	for rows.Next() {
		record, scanErr := scanExecutionErrorRecord(
			rows)
		if scanErr != nil {
			return repository.Page[repository.ExecutionErrorRecord]{}, mapPostgreSQLError("list",
				"execution errors", scanErr)
		}
		records = append(
			records, record)
	}
	if err := rows.Err(); err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, mapPostgreSQLError("list",
			"execution errors", err)
	}
	var nextToken repository.PageToken
	if len(records) > pageRequest.Limit() {
		records = records[:pageRequest.Limit()]
		lastRecord := records[len(records)-1]
		nextToken, err = encodeTimestampCursor(cursorKindExecutionErrors, lastRecord.CreatedAt(),
			lastRecord.ID().String())
		if err != nil {
			return repository.Page[repository.ExecutionErrorRecord]{}, fmt.Errorf("encode execution error page token: %w",
				err)
		}
	}
	page, err := repository.NewPage(
		records, nextToken)
	if err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, fmt.Errorf(
			"create execution error page: %w", err)
	}
	return page, nil
}
func buildExecutionErrorListQuery(
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest,
) (string, []any,
	error) {
	var builder strings.Builder
	builder.WriteString("SELECT\n")
	builder.WriteString(executionErrorSelectColumns)
	builder.WriteString("\nFROM execution_errors\n")
	builder.WriteString("WHERE company_id = $1")
	builder.WriteString("\n  AND workflow_execution_id = $2")
	arguments := []any{companyID.String(),
		workflowExecutionID.String()}
	if token, exists := pageRequest.After(); exists {
		createdAt, errorID, err := decodeTimestampCursor(
			cursorKindExecutionErrors, token)
		if err != nil {
			return "", nil, &repository.ValidationError{
				Field: "pageToken", Reason: "must contain a valid execution error cursor"}
		}
		arguments = append(
			arguments, createdAt, errorID,
		)
		createdAtPosition := len(arguments) - 1
		errorIDPosition := len(arguments)
		builder.WriteString(
			fmt.Sprintf("\n  AND "+"(created_at, error_id) "+
				"> ($%d, $%d)", createdAtPosition, errorIDPosition,
			))
	}
	builder.WriteString("\nORDER BY created_at ASC, error_id ASC")
	arguments = append(
		arguments, pageRequest.Limit()+1)
	builder.WriteString(fmt.Sprintf(
		"\nLIMIT $%d", len(arguments)),
	)
	return builder.String(),
		arguments, nil
}
func scanExecutionErrorRecord(row rowScanner,
) (repository.ExecutionErrorRecord, error) {
	if row == nil {
		return repository.ExecutionErrorRecord{},
			fmt.Errorf("execution error row must not be nil")
	}
	var (
		errorID             string
		workflowExecutionID string
		companyID           string
		nodeExecutionID     pgtype.Text
		relatedEventID      pgtype.Text
		category            string
		code                string
		safeMessage         string
		technicalDetail     pgtype.Text
		retryable           bool
		details             []byte
		createdAt           time.Time
	)
	if err := row.Scan(
		&errorID, &workflowExecutionID, &companyID,
		&nodeExecutionID, &relatedEventID, &category,
		&code, &safeMessage, &technicalDetail,
		&retryable, &details, &createdAt,
	); err != nil {
		return repository.ExecutionErrorRecord{}, err
	}
	record, err := repository.NewExecutionErrorRecord(
		repository.ExecutionErrorRecordParams{ID: repository.ExecutionErrorID(errorID), WorkflowExecutionID: execution.WorkflowExecutionID(workflowExecutionID),
			CompanyID: workflow.CompanyID(companyID), NodeExecutionID: execution.NodeExecutionID(nullableTextValue(
				nodeExecutionID)),
			RelatedEventID: repository.ExecutionEventID(nullableTextValue(relatedEventID)), Category: runtimefailure.FailureCategory(
				category), Code: code,
			SafeMessage: safeMessage, TechnicalDetail: nullableTextValue(technicalDetail), Retryable: retryable, Details: details,
			CreatedAt: createdAt})
	if err != nil {
		return repository.ExecutionErrorRecord{}, fmt.Errorf(
			"reconstruct execution error: %w", err)
	}
	return record, nil
}

const insertExecutionErrorTransactionSQL = `
INSERT INTO workflow_runtime.execution_errors (
	error_id,
	workflow_execution_id,
	company_id,
	node_execution_id,
	related_event_id,
	category,
	code,
	safe_message,
	technical_detail,
	retryable,
	details,
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
	$11::jsonb,
	$12
)
`

func insertExecutionErrorsTransaction(ctx context.Context, tx pgx.Tx,
	records []repository.ExecutionErrorRecord) error {
	if tx == nil {
		return fmt.Errorf("execution error transaction must not be nil")
	}
	for _, record := range records {
		if err := insertExecutionErrorTransaction(ctx, tx,
			record); err != nil {
			return err
		}
	}
	return nil
}
func insertExecutionErrorTransaction(ctx context.Context, tx pgx.Tx,
	record repository.ExecutionErrorRecord) error {
	if tx == nil {
		return fmt.Errorf("execution error transaction must not be nil")
	}
	if !record.IsValid() {
		return fmt.Errorf("execution error record must be valid")
	}
	_, err := tx.Exec(
		ctx, insertExecutionErrorTransactionSQL, record.ID().String(),
		record.WorkflowExecutionID().String(), record.CompanyID().String(), optionalStringValue(
			record.NodeExecutionID), optionalStringValue(
			record.RelatedEventID), record.Category().String(),
		record.Code(), record.SafeMessage(), optionalValue(
			record.TechnicalDetail), record.Retryable(),
		record.Details().String(), record.CreatedAt())
	if err != nil {
		return mapPostgreSQLError("create",
			"execution error", err)
	}
	return nil
}
