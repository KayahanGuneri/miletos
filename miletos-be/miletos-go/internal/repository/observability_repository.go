package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"miletos-go/internal/model"
)

func (repository *ExecutionRepository) RecordEvent(
	ctx context.Context,
	executionID string,
	nodeExecutionID string,
	eventType string,
	previousStatus string,
	newStatus string,
	message string,
) error {
	return recordEvent(
		ctx, repository.db, executionID, nodeExecutionID, eventType,
		previousStatus, newStatus, message, map[string]any{},
	)
}

func recordEvent(
	ctx context.Context,
	executor sqlExecutor,
	executionID string,
	nodeExecutionID string,
	eventType string,
	previousStatus string,
	newStatus string,
	message string,
	eventMetadata map[string]any,
) error {
	metadata, err := encodeJSON(eventMetadata)
	if err != nil {
		return err
	}
	command, err := executor.Exec(ctx, `
		WITH sequence AS (
			UPDATE workflow_runtime.workflow_executions
			SET next_sequence_number = next_sequence_number + 1
			WHERE workflow_execution_id = $1
			RETURNING company_id, correlation_id, next_sequence_number - 1 AS number
		)
		INSERT INTO workflow_runtime.execution_events (
			event_id, workflow_execution_id, company_id, node_execution_id,
			sequence_number, event_type, previous_status, new_status,
			correlation_id, safe_message, metadata, created_at
		)
		SELECT $2, $1, company_id, NULLIF($3, ''), number, $4,
			NULLIF($5, ''), NULLIF($6, ''), correlation_id, NULLIF($7, ''), $8, $9
		FROM sequence`,
		executionID, newID("event_"), nodeExecutionID, eventType,
		previousStatus, newStatus, message, metadata, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: execution event parent is unavailable", ErrStateTransition)
	}
	return nil
}

func (repository *ExecutionRepository) RecordLog(
	ctx context.Context,
	executionID string,
	nodeExecutionID string,
	level string,
	message string,
	metadata map[string]any,
) error {
	return recordLog(
		ctx, repository.db, executionID, nodeExecutionID, level, message, metadata,
	)
}

func recordLog(
	ctx context.Context,
	executor sqlExecutor,
	executionID string,
	nodeExecutionID string,
	level string,
	message string,
	metadata map[string]any,
) error {
	encoded, err := encodeJSON(metadata)
	if err != nil {
		return err
	}
	command, err := executor.Exec(ctx, `
		WITH sequence AS (
			UPDATE workflow_runtime.workflow_executions
			SET next_sequence_number = next_sequence_number + 1
			WHERE workflow_execution_id = $1
			RETURNING company_id, next_sequence_number - 1 AS number
		)
		INSERT INTO workflow_runtime.execution_logs (
			log_id, workflow_execution_id, company_id, node_execution_id,
			sequence_number, level, message, metadata, created_at
		)
		SELECT $2, $1, company_id, NULLIF($3, ''), number, $4, $5, $6, $7
		FROM sequence`,
		executionID, newID("log_"), nodeExecutionID, level, message, encoded, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: execution log parent is unavailable", ErrStateTransition)
	}
	return nil
}

func (repository *ExecutionRepository) RecordError(
	ctx context.Context,
	job model.NodeJob,
	failure map[string]any,
	retryable bool,
	createdAt time.Time,
) error {
	return recordError(ctx, repository.db, job, failure, retryable, createdAt)
}

func recordError(
	ctx context.Context,
	executor sqlExecutor,
	job model.NodeJob,
	failure map[string]any,
	retryable bool,
	createdAt time.Time,
) error {
	category, _ := failure["category"].(string)
	if category == "" {
		category = "EXECUTION"
	}
	code, _ := failure["code"].(string)
	if code == "" {
		code = "NODE_EXECUTION_FAILED"
	}
	message, _ := failure["message"].(string)
	if message == "" {
		message = "Node execution failed"
	}
	details, err := encodeJSON(failure)
	if err != nil {
		return err
	}
	command, err := executor.Exec(ctx, `
		INSERT INTO workflow_runtime.execution_errors (
			error_id, workflow_execution_id, company_id, node_execution_id,
			category, code, safe_message, retryable, details, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		newID("error_"), job.ExecutionID, job.CompanyID, job.NodeExecutionID,
		category, code, message, retryable, details, createdAt,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: execution error was not recorded", ErrStateTransition)
	}
	return nil
}

func (repository *ExecutionRepository) ListEvents(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionEvent], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	afterSequence, _ := strconv.ParseInt(after, 10, 64)
	rows, err := repository.db.Query(ctx, `
		SELECT event_id, workflow_execution_id, COALESCE(node_execution_id, ''),
			sequence_number, event_type, COALESCE(previous_status, ''),
			COALESCE(new_status, ''), COALESCE(correlation_id, ''),
			COALESCE(causation_id, ''), COALESCE(safe_message, ''), metadata, created_at
		FROM workflow_runtime.execution_events
		WHERE company_id = $1 AND workflow_execution_id = $2 AND sequence_number > $3
		ORDER BY sequence_number LIMIT $4`,
		companyID, executionID, afterSequence, limit+1,
	)
	if err != nil {
		return model.Page[model.ExecutionEvent]{}, err
	}
	defer rows.Close()
	items := make([]model.ExecutionEvent, 0, limit+1)
	for rows.Next() {
		var item model.ExecutionEvent
		var metadata []byte
		if err := rows.Scan(
			&item.ID, &item.ExecutionID, &item.NodeExecutionID, &item.Sequence,
			&item.Type, &item.PreviousStatus, &item.NewStatus, &item.CorrelationID,
			&item.CausationID, &item.Message, &metadata, &item.CreatedAt,
		); err != nil {
			return model.Page[model.ExecutionEvent]{}, err
		}
		item.Metadata = decodeObject(metadata)
		items = append(items, item)
	}
	page := model.Page[model.ExecutionEvent]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = strconv.FormatInt(page.Items[len(page.Items)-1].Sequence, 10)
	}
	return page, rows.Err()
}

func (repository *ExecutionRepository) ListLogs(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionLog], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	afterSequence, _ := strconv.ParseInt(after, 10, 64)
	rows, err := repository.db.Query(ctx, `
		SELECT log_id, workflow_execution_id, COALESCE(node_execution_id, ''),
			sequence_number, level, message, metadata, created_at
		FROM workflow_runtime.execution_logs
		WHERE company_id = $1 AND workflow_execution_id = $2 AND sequence_number > $3
		ORDER BY sequence_number LIMIT $4`,
		companyID, executionID, afterSequence, limit+1,
	)
	if err != nil {
		return model.Page[model.ExecutionLog]{}, err
	}
	defer rows.Close()
	items := make([]model.ExecutionLog, 0, limit+1)
	for rows.Next() {
		var item model.ExecutionLog
		var metadata []byte
		if err := rows.Scan(
			&item.ID, &item.ExecutionID, &item.NodeExecutionID, &item.Sequence,
			&item.Level, &item.Message, &metadata, &item.CreatedAt,
		); err != nil {
			return model.Page[model.ExecutionLog]{}, err
		}
		item.Metadata = decodeObject(metadata)
		items = append(items, item)
	}
	page := model.Page[model.ExecutionLog]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = strconv.FormatInt(page.Items[len(page.Items)-1].Sequence, 10)
	}
	return page, rows.Err()
}

func (repository *ExecutionRepository) ListErrors(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionError], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	arguments := []any{companyID, executionID}
	query := `
		SELECT error_id, workflow_execution_id, COALESCE(node_execution_id, ''),
			COALESCE(related_event_id, ''), category, code, safe_message,
			retryable, details, created_at
		FROM workflow_runtime.execution_errors
		WHERE company_id = $1 AND workflow_execution_id = $2`
	if after != "" {
		arguments = append(arguments, after)
		query += fmt.Sprintf(" AND error_id > $%d", len(arguments))
	}
	arguments = append(arguments, limit+1)
	query += fmt.Sprintf(" ORDER BY error_id LIMIT $%d", len(arguments))
	rows, err := repository.db.Query(ctx, query, arguments...)
	if err != nil {
		return model.Page[model.ExecutionError]{}, err
	}
	defer rows.Close()
	items := make([]model.ExecutionError, 0, limit+1)
	for rows.Next() {
		var item model.ExecutionError
		var details []byte
		if err := rows.Scan(
			&item.ID, &item.ExecutionID, &item.NodeExecutionID, &item.RelatedEventID,
			&item.Category, &item.Code, &item.Message, &item.Retryable, &details, &item.CreatedAt,
		); err != nil {
			return model.Page[model.ExecutionError]{}, err
		}
		item.Details = decodeObject(details)
		items = append(items, item)
	}
	page := model.Page[model.ExecutionError]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = page.Items[len(page.Items)-1].ID
	}
	return page, rows.Err()
}
