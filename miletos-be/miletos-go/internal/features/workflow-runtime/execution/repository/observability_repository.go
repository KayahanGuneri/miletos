package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/persistence"
)

func (repository *ExecutionRepository) RecordEvent(
	ctx context.Context,
	executionID string,
	nodeExecutionID string,
	eventType string,
	previousStatus any,
	newStatus any,
	message string,
) error {
	return recordEvent(
		ctx, repository.dbClient, executionID, nodeExecutionID, eventType,
		previousStatus, newStatus, message, map[string]any{},
	)
}

func recordEvent(
	ctx context.Context,
	executor sqlExecutor,
	executionID string,
	nodeExecutionID string,
	eventType string,
	previousStatus any,
	newStatus any,
	message string,
	eventMetadata map[string]any,
) error {
	metadata, err := encodeJSON(eventMetadata)
	if err != nil {
		return err
	}
	// Raw SQL atomically allocates the per-execution sequence and inserts the event.
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
		ctx, repository.dbClient, executionID, nodeExecutionID, level, message, metadata,
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
	return recordError(ctx, repository.dbClient, job, failure, retryable, createdAt)
}

func recordError(
	ctx context.Context,
	executor gormExecutor,
	job model.NodeJob,
	failure map[string]any,
	retryable bool,
	createdAt time.Time,
) error {
	category := failureCategoryValue(failure["category"])
	if category == "" {
		category = model.FailureCategoryExecution
	}
	code := failureCodeValue(failure["code"])
	if code == "" {
		code = model.FailureCodeExecutionFailed
	}
	message, _ := failure["message"].(string)
	if message == "" {
		message = "Node execution failed"
	}
	details, err := encodeJSON(failure)
	if err != nil {
		return err
	}
	record := persistence.ExecutionErrorRecord{
		ErrorID: newID("error_"), ExecutionID: job.ExecutionID,
		CompanyID: job.CompanyID, NodeExecutionID: job.NodeExecutionID,
		Category: string(category), Code: string(code), SafeMessage: message,
		Retryable: retryable, Details: details, CreatedAt: createdAt,
	}
	result := executor.DB(ctx).
		Table("workflow_runtime.execution_errors").Create(&record)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%w: execution error was not recorded", ErrStateTransition)
	}
	return nil
}

func failureCategoryValue(value any) model.FailureCategory {
	var raw string
	switch typed := value.(type) {
	case model.FailureCategory:
		raw = string(typed)
	case string:
		raw = typed
	default:
		return ""
	}
	category := model.FailureCategory(strings.TrimSpace(raw))
	switch category {
	case model.FailureCategoryValidation,
		model.FailureCategoryExecution,
		model.FailureCategoryInternal,
		model.FailureCategoryCanceled,
		model.FailureCategoryTimeout:
		return category
	default:
		return ""
	}
}

func failureCodeValue(value any) model.FailureCode {
	switch typed := value.(type) {
	case model.FailureCode:
		return model.FailureCode(strings.TrimSpace(string(typed)))
	case string:
		return model.FailureCode(strings.TrimSpace(typed))
	default:
		return ""
	}
}

func (repository *ExecutionRepository) ListEvents(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionEvent], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	afterSequence, _ := strconv.ParseInt(after, 10, 64)
	var records []persistence.ExecutionEventRecord
	err := repository.dbClient.DB(ctx).Table("workflow_runtime.execution_events").
		Where("company_id = ? AND workflow_execution_id = ? AND sequence_number > ?", companyID, executionID, afterSequence).
		Order("sequence_number").Limit(limit + 1).Find(&records).Error
	if err != nil {
		return model.Page[model.ExecutionEvent]{}, err
	}
	items := persistence.ToExecutionEvents(records)
	page := model.Page[model.ExecutionEvent]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = strconv.FormatInt(page.Items[len(page.Items)-1].Sequence, 10)
	}
	return page, nil
}

func (repository *ExecutionRepository) ListLogs(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionLog], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	afterSequence, _ := strconv.ParseInt(after, 10, 64)
	var records []persistence.ExecutionLogRecord
	err := repository.dbClient.DB(ctx).Table("workflow_runtime.execution_logs").
		Where("company_id = ? AND workflow_execution_id = ? AND sequence_number > ?", companyID, executionID, afterSequence).
		Order("sequence_number").Limit(limit + 1).Find(&records).Error
	if err != nil {
		return model.Page[model.ExecutionLog]{}, err
	}
	items := persistence.ToExecutionLogs(records)
	page := model.Page[model.ExecutionLog]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = strconv.FormatInt(page.Items[len(page.Items)-1].Sequence, 10)
	}
	return page, nil
}

func (repository *ExecutionRepository) ListErrors(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionError], error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := repository.dbClient.DB(ctx).Table("workflow_runtime.execution_errors").
		Where("company_id = ? AND workflow_execution_id = ?", companyID, executionID)
	if after != "" {
		query = query.Where("error_id > ?", after)
	}
	var records []persistence.ExecutionErrorRecord
	if err := query.Order("error_id").Limit(limit + 1).Find(&records).Error; err != nil {
		return model.Page[model.ExecutionError]{}, err
	}
	items := persistence.ToExecutionErrors(records)
	page := model.Page[model.ExecutionError]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}
