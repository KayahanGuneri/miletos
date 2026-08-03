package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/persistence"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/shared/database"

	"gorm.io/gorm"
)

type ExecutionRepository struct {
	dbClient *database.Client
}

func NewExecutionRepository(dbClient *database.Client) *ExecutionRepository {
	return &ExecutionRepository{dbClient: dbClient}
}

func (repository *ExecutionRepository) FindIdempotent(
	ctx context.Context,
	companyID string,
	key string,
	fingerprint string,
) (model.Execution, bool, error) {
	if key == "" {
		return model.Execution{}, false, nil
	}
	var keyRecord struct {
		RequestFingerprint  string `gorm:"column:request_fingerprint"`
		WorkflowExecutionID string `gorm:"column:workflow_execution_id"`
	}
	err := repository.dbClient.DB(ctx).
		Table("workflow_runtime.http_idempotency_keys").
		Select("request_fingerprint, workflow_execution_id").
		Where("company_id = ? AND idempotency_key = ?", companyID, key).
		Take(&keyRecord).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Execution{}, false, nil
	}
	if err != nil {
		return model.Execution{}, false, fmt.Errorf("find idempotent execution: %w", err)
	}
	if keyRecord.RequestFingerprint != fingerprint {
		return model.Execution{}, false, ErrIdempotencyConflict
	}
	execution, err := repository.FindByID(ctx, companyID, keyRecord.WorkflowExecutionID)
	return execution, true, err
}

func (repository *ExecutionRepository) Create(
	ctx context.Context,
	workflow workflowfeature.Workflow,
	snapshotID string,
	mode string,
	origin model.ExecutionOrigin,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (model.Execution, error) {
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return model.Execution{}, fmt.Errorf("begin execution creation: %w", err)
	}
	defer transaction.Rollback(ctx)

	now := time.Now().UTC()
	execution := model.Execution{
		ID:               newID("exec_"),
		CompanyID:        workflow.CompanyID,
		WorkflowID:       workflow.ID,
		WorkflowRevision: workflow.Revision,
		SnapshotID:       snapshotID,
		Mode:             mode,
		Origin:           origin,
		CorrelationID:    correlationID,
		Status:           model.ExecutionCreated,
		CreatedAt:        now,
		UpdatedAt:        now,
		TerminalOutputs:  map[string]any{},
	}
	executionRecord := map[string]any{
		"workflow_execution_id": execution.ID, "company_id": workflow.CompanyID,
		"workflow_id": workflow.ID, "workflow_revision": workflow.Revision,
		"snapshot_id": snapshotID, "mode": mode, "execution_origin": origin,
		"status": execution.Status, "created_at": now, "updated_at": now,
	}
	if correlationID != "" {
		executionRecord["correlation_id"] = correlationID
	}
	err = transaction.DB(ctx).
		Table("workflow_runtime.workflow_executions").Create(executionRecord).Error
	if err != nil {
		return model.Execution{}, fmt.Errorf("create execution: %w", err)
	}
	for _, node := range workflow.Nodes {
		version := node.Version
		if version == "" {
			version = "v1"
		}
		err = transaction.DB(ctx).Table("workflow_runtime.node_executions").Create(map[string]any{
			"node_execution_id":     newID("node_exec_"),
			"workflow_execution_id": execution.ID, "company_id": workflow.CompanyID,
			"node_id": node.ID, "plugin_type": node.Type, "plugin_version": version,
			"status": model.NodePending, "attempt": 1, "created_at": now, "updated_at": now,
		}).Error
		if err != nil {
			return model.Execution{}, fmt.Errorf("create node execution %s: %w", node.ID, err)
		}
	}
	if idempotencyKey != "" {
		err = transaction.DB(ctx).Table("workflow_runtime.http_idempotency_keys").Create(map[string]any{
			"company_id": workflow.CompanyID, "idempotency_key": idempotencyKey,
			"request_fingerprint": fingerprint, "workflow_execution_id": execution.ID,
			"state": "ACCEPTED", "created_at": now, "accepted_at": now,
		}).Error
		if err != nil {
			return model.Execution{}, fmt.Errorf("save idempotency key: %w", err)
		}
	}
	if err := recordEvent(
		ctx, transaction, execution.ID, "", "WORKFLOW_CREATED", "",
		model.ExecutionCreated, "Workflow execution created", map[string]any{},
	); err != nil {
		return model.Execution{}, fmt.Errorf("record execution creation: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return model.Execution{}, fmt.Errorf("commit execution creation: %w", err)
	}
	return execution, nil
}

func (repository *ExecutionRepository) FindByID(
	ctx context.Context,
	companyID string,
	executionID string,
) (model.Execution, error) {
	var record persistence.ExecutionRecord
	err := repository.dbClient.DB(ctx).
		Table("workflow_runtime.workflow_executions").
		Where("company_id = ? AND workflow_execution_id = ?", companyID, executionID).
		Take(&record).Error
	if err != nil {
		return model.Execution{}, mapNoRows(err)
	}
	return persistence.ToExecution(record), nil
}

func (repository *ExecutionRepository) List(
	ctx context.Context,
	companyID string,
	workflowID string,
	status model.ExecutionStatus,
	after string,
	limit int,
) (model.Page[model.Execution], error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	query := repository.dbClient.DB(ctx).Table("workflow_runtime.workflow_executions").
		Where("company_id = ?", companyID)
	if workflowID != "" {
		query = query.Where("workflow_id = ?", workflowID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if after != "" {
		var cursor persistence.ExecutionRecord
		if err := repository.dbClient.DB(ctx).Table("workflow_runtime.workflow_executions").
			Select("workflow_execution_id, created_at").
			Where("company_id = ? AND workflow_execution_id = ?", companyID, after).
			Take(&cursor).Error; err != nil {
			return model.Page[model.Execution]{}, mapNoRows(err)
		}
		query = query.Where(
			"created_at < ? OR (created_at = ? AND workflow_execution_id < ?)",
			cursor.CreatedAt, cursor.CreatedAt, cursor.WorkflowExecutionID,
		)
	}
	var records []persistence.ExecutionRecord
	if err := query.Order("created_at DESC, workflow_execution_id DESC").
		Limit(limit + 1).Find(&records).Error; err != nil {
		return model.Page[model.Execution]{}, fmt.Errorf("list executions: %w", err)
	}
	items := persistence.ToExecutions(records)
	page := model.Page[model.Execution]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}

func (repository *ExecutionRepository) MarkExecutionQueued(
	ctx context.Context,
	companyID string,
	executionID string,
) error {
	now := time.Now().UTC()
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin execution queue transition: %w", err)
	}
	defer transaction.Rollback(ctx)
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.workflow_executions
		SET status = 'QUEUED', queued_at = COALESCE(queued_at, $3), updated_at = $3,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2 AND status = 'CREATED'`,
		companyID, executionID, now,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return nil
	}
	if err := recordEvent(
		ctx, transaction, executionID, "", "WORKFLOW_QUEUED",
		model.ExecutionCreated, model.ExecutionQueued, "Workflow queued", map[string]any{},
	); err != nil {
		return fmt.Errorf("record execution queued event: %w", err)
	}
	return transaction.Commit(ctx)
}

func (repository *ExecutionRepository) MarkExecutionRunning(
	ctx context.Context,
	companyID string,
	executionID string,
) error {
	now := time.Now().UTC()
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin execution start transition: %w", err)
	}
	defer transaction.Rollback(ctx)
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.workflow_executions
		SET status = 'RUNNING', started_at = COALESCE(started_at, $3), updated_at = $3,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND status IN ('CREATED', 'QUEUED')`,
		companyID, executionID, now,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return nil
	}
	if err := recordEvent(
		ctx, transaction, executionID, "", "WORKFLOW_STARTED",
		"", model.ExecutionRunning, "Workflow started", map[string]any{},
	); err != nil {
		return fmt.Errorf("record execution started event: %w", err)
	}
	return transaction.Commit(ctx)
}

func (repository *ExecutionRepository) Finalize(
	ctx context.Context,
	companyID string,
	executionID string,
	status model.ExecutionStatus,
	outputs map[string]any,
	failure map[string]any,
) error {
	now := time.Now().UTC()
	encodedOutputs, err := encodeJSON(outputs)
	if err != nil {
		return err
	}
	var encodedFailure any
	if failure != nil {
		encodedFailure, err = encodeJSON(failure)
		if err != nil {
			return err
		}
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin execution finalization: %w", err)
	}
	defer transaction.Rollback(ctx)
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.workflow_executions
		SET status = $3, terminal_outputs = $4, failure_summary = $5,
			finished_at = $6, updated_at = $6, is_stalled = FALSE,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND status NOT IN ('SUCCEEDED', 'FAILED', 'CANCELLED', 'TIMED_OUT')`,
		companyID, executionID, status, encodedOutputs, encodedFailure, now,
	)
	if err != nil {
		return fmt.Errorf("finalize execution: %w", err)
	}
	if command.RowsAffected() == 0 {
		var current struct {
			Status string `gorm:"column:status"`
		}
		if err := transaction.DB(ctx).
			Table("workflow_runtime.workflow_executions").
			Select("status").
			Where("company_id = ? AND workflow_execution_id = ?", companyID, executionID).
			Take(&current).Error; err != nil {
			return fmt.Errorf("inspect final execution state: %w", mapNoRows(err))
		}
		if model.ExecutionStatus(current.Status) == status {
			return nil
		}
		return fmt.Errorf(
			"%w: cannot finalize execution from status %s",
			ErrStateTransition,
			current.Status,
		)
	}
	eventType, message := "WORKFLOW_SUCCEEDED", "Workflow succeeded"
	if status == model.ExecutionFailed {
		eventType, message = "WORKFLOW_FAILED", "Workflow failed"
	}
	if err := recordEvent(
		ctx, transaction, executionID, "", eventType,
		model.ExecutionRunning, status, message, map[string]any{},
	); err != nil {
		return fmt.Errorf("record workflow finalization event: %w", err)
	}
	if status == model.ExecutionFailed {
		if err := recordLog(
			ctx, transaction, executionID, "", "ERROR", "Workflow execution failed",
			map[string]any{"status": status},
		); err != nil {
			return fmt.Errorf("record workflow failure log: %w", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit execution finalization: %w", err)
	}
	return nil
}
