package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"

	"gorm.io/gorm"
)

type recoveryRequestRecord struct {
	CompanyID           string    `gorm:"column:company_id"`
	IdempotencyKey      string    `gorm:"column:idempotency_key"`
	RequestFingerprint  string    `gorm:"column:request_fingerprint"`
	SourceExecutionID   string    `gorm:"column:source_workflow_execution_id"`
	RecoveryExecutionID string    `gorm:"column:recovery_workflow_execution_id"`
	RecoveryPlan        []byte    `gorm:"column:recovery_plan"`
	PreservedNodeCount  int       `gorm:"column:preserved_node_count"`
	ScheduledNodeCount  int       `gorm:"column:scheduled_node_count"`
	ResetNodeCount      int       `gorm:"column:reset_node_count"`
	CreatedAt           time.Time `gorm:"column:created_at"`
}

func (repository *ExecutionRepository) CopySuccessfulNodes(
	ctx context.Context,
	sourceExecutionID string,
	recoveryExecutionID string,
	nodeIDs []string,
) error {
	var record struct {
		CompanyID string `gorm:"column:company_id"`
	}
	err := repository.dbClient.DB(ctx).
		Table("workflow_runtime.workflow_executions AS source").
		Select("source.company_id").
		Joins("JOIN workflow_runtime.workflow_executions target ON target.company_id = source.company_id").
		Where("source.workflow_execution_id = ? AND target.workflow_execution_id = ?", sourceExecutionID, recoveryExecutionID).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return repository.CopySuccessfulNodesForCompany(
		ctx, record.CompanyID, sourceExecutionID, recoveryExecutionID, nodeIDs,
	)
}

func (repository *ExecutionRepository) CopySuccessfulNodesForCompany(
	ctx context.Context,
	companyID string,
	sourceExecutionID string,
	recoveryExecutionID string,
	nodeIDs []string,
) error {
	if len(nodeIDs) == 0 {
		return nil
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin preserved-node copy: %w", err)
	}
	defer transaction.Rollback(ctx)
	encodedNodeIDs, err := json.Marshal(nodeIDs)
	if err != nil {
		return fmt.Errorf("encode preserved node IDs: %w", err)
	}
	var sourceCount, targetCount int64
	if err := transaction.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (
				WHERE workflow_execution_id = $2 AND status = 'SUCCEEDED'
			),
			COUNT(*) FILTER (WHERE workflow_execution_id = $3)
		FROM workflow_runtime.node_executions
		WHERE company_id = $1
		  AND workflow_execution_id IN ($2, $3)
		  AND node_id IN (SELECT jsonb_array_elements_text($4::jsonb))`,
		companyID, sourceExecutionID, recoveryExecutionID, string(encodedNodeIDs),
	).Scan(&sourceCount, &targetCount); err != nil {
		return fmt.Errorf("verify preserved nodes: %w", err)
	}
	expectedCount := int64(len(nodeIDs))
	if sourceCount != expectedCount || targetCount != expectedCount {
		return fmt.Errorf("%w: preserved node set is incomplete", ErrStateTransition)
	}
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.node_executions target
		SET status = 'SUCCEEDED',
			output_summary = source.output_summary,
			failure_summary = NULL,
			started_at = target.created_at,
			finished_at = target.created_at,
			updated_at = target.created_at,
			next_attempt_at = NULL,
			lock_version = CASE
				WHEN target.status = 'SUCCEEDED' THEN target.lock_version
				ELSE target.lock_version + 1
			END
		FROM workflow_runtime.node_executions source
		WHERE target.company_id = $1
		  AND target.workflow_execution_id = $2
		  AND target.node_id IN (SELECT jsonb_array_elements_text($4::jsonb))
		  AND target.status IN ('PENDING', 'SUCCEEDED')
		  AND source.company_id = $1
		  AND source.workflow_execution_id = $3
		  AND source.node_id = target.node_id
		  AND source.status = 'SUCCEEDED'`,
		companyID, recoveryExecutionID, sourceExecutionID, string(encodedNodeIDs),
	)
	if err != nil {
		return fmt.Errorf("copy preserved nodes: %w", err)
	}
	if command.RowsAffected() != expectedCount {
		return fmt.Errorf("%w: preserved node copy was partial", ErrStateTransition)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit preserved-node copy: %w", err)
	}
	return nil
}

func (repository *ExecutionRepository) ReserveRecovery(
	ctx context.Context,
	workflow workflowfeature.Workflow,
	correlationID string,
	key string,
	fingerprint string,
	sourceExecutionID string,
	preserved int,
	scheduled int,
	reset int,
) (model.Execution, error) {
	definition, err := json.Marshal(workflow)
	if err != nil {
		return model.Execution{}, fmt.Errorf("encode recovery workflow: %w", err)
	}
	plan, err := encodeJSON(map[string]any{
		"preservedNodeCount": preserved,
		"scheduledNodeCount": scheduled,
		"resetNodeCount":     reset,
	})
	if err != nil {
		return model.Execution{}, err
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return model.Execution{}, fmt.Errorf("begin recovery reservation: %w", err)
	}
	defer transaction.Rollback(ctx)
	now := time.Now().UTC()
	snapshotID := newID("snapshot_")
	execution := model.Execution{
		ID: newID("exec_"), CompanyID: workflow.CompanyID, WorkflowID: workflow.ID,
		WorkflowRevision: workflow.Revision, SnapshotID: snapshotID, Mode: "ASYNC",
		CorrelationID: correlationID, Status: model.ExecutionCreated,
		CreatedAt: now, UpdatedAt: now, TerminalOutputs: map[string]any{},
	}
	snapshotRecord := map[string]any{
		"snapshot_id": snapshotID, "company_id": workflow.CompanyID,
		"workflow_id": workflow.ID, "workflow_revision": workflow.Revision,
		"workflow_name": workflow.Name, "definition_json": definition,
		"created_at": now,
	}
	if err := transaction.DB(ctx).
		Table("workflow_runtime.workflow_definition_snapshots").
		Create(snapshotRecord).Error; err != nil {
		return model.Execution{}, fmt.Errorf("save recovery snapshot: %w", err)
	}
	var storedCorrelationID any
	if correlationID != "" {
		storedCorrelationID = correlationID
	}
	executionRecord := map[string]any{
		"workflow_execution_id": execution.ID, "company_id": workflow.CompanyID,
		"workflow_id": workflow.ID, "workflow_revision": workflow.Revision,
		"snapshot_id": snapshotID, "mode": "ASYNC", "correlation_id": storedCorrelationID,
		"status": model.ExecutionCreated, "created_at": now, "updated_at": now,
	}
	if err := transaction.DB(ctx).
		Table("workflow_runtime.workflow_executions").
		Create(executionRecord).Error; err != nil {
		return model.Execution{}, fmt.Errorf("create recovery execution: %w", err)
	}
	for _, node := range workflow.Nodes {
		version := node.Version
		if version == "" {
			version = "v1"
		}
		nodeRecord := map[string]any{
			"node_execution_id":     newID("node_exec_"),
			"workflow_execution_id": execution.ID, "company_id": workflow.CompanyID,
			"node_id": node.ID, "plugin_type": node.Type, "plugin_version": version,
			"status": model.NodePending, "attempt": 1,
			"created_at": now, "updated_at": now,
		}
		if err := transaction.DB(ctx).
			Table("workflow_runtime.node_executions").Create(nodeRecord).Error; err != nil {
			return model.Execution{}, fmt.Errorf("create recovery node %s: %w", node.ID, err)
		}
	}
	recoveryRecord := recoveryRequestRecord{
		CompanyID: workflow.CompanyID, IdempotencyKey: key,
		RequestFingerprint: fingerprint, SourceExecutionID: sourceExecutionID,
		RecoveryExecutionID: execution.ID, RecoveryPlan: plan,
		PreservedNodeCount: preserved, ScheduledNodeCount: scheduled,
		ResetNodeCount: reset, CreatedAt: now,
	}
	if err := transaction.DB(ctx).
		Table("workflow_runtime.partial_recovery_requests").
		Create(&recoveryRecord).Error; err != nil {
		return model.Execution{}, mapRecoveryInsertError(err)
	}
	if err := recordEvent(
		ctx, transaction, execution.ID, "", "WORKFLOW_CREATED", "",
		model.ExecutionCreated, "Workflow execution created", map[string]any{},
	); err != nil {
		return model.Execution{}, fmt.Errorf("record recovery creation: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return model.Execution{}, fmt.Errorf("commit recovery reservation: %w", err)
	}
	return execution, nil
}

func (repository *ExecutionRepository) SaveRecoveryRequest(
	ctx context.Context,
	companyID string,
	key string,
	fingerprint string,
	sourceExecutionID string,
	recoveryExecutionID string,
	preserved int,
	scheduled int,
	reset int,
) error {
	plan, err := encodeJSON(map[string]any{
		"preservedNodeCount": preserved,
		"scheduledNodeCount": scheduled,
		"resetNodeCount":     reset,
	})
	if err != nil {
		return err
	}
	record := recoveryRequestRecord{
		CompanyID: companyID, IdempotencyKey: key, RequestFingerprint: fingerprint,
		SourceExecutionID: sourceExecutionID, RecoveryExecutionID: recoveryExecutionID,
		RecoveryPlan: plan, PreservedNodeCount: preserved,
		ScheduledNodeCount: scheduled, ResetNodeCount: reset, CreatedAt: time.Now().UTC(),
	}
	err = repository.dbClient.DB(ctx).
		Table("workflow_runtime.partial_recovery_requests").Create(&record).Error
	return mapRecoveryInsertError(err)
}

func (repository *ExecutionRepository) FindRecoveryBySource(
	ctx context.Context,
	companyID string,
	sourceExecutionID string,
) (key string, recoveryID string, fingerprint string, found bool, err error) {
	var record recoveryRequestRecord
	err = repository.dbClient.DB(ctx).
		Table("workflow_runtime.partial_recovery_requests").
		Where("company_id = ? AND source_workflow_execution_id = ?", companyID, sourceExecutionID).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", "", false, nil
	}
	return record.IdempotencyKey, record.RecoveryExecutionID,
		record.RequestFingerprint, err == nil, err
}

func mapRecoveryInsertError(err error) error {
	if err == nil {
		return nil
	}
	var postgresError interface{ SQLState() string }
	if errors.As(err, &postgresError) && postgresError.SQLState() == "23505" {
		switch {
		case strings.Contains(err.Error(), "partial_recovery_requests_pk"):
			return ErrIdempotencyConflict
		case strings.Contains(err.Error(), "partial_recovery_requests_source_uk"),
			strings.Contains(err.Error(), "partial_recovery_requests_recovery_uk"):
			return ErrRecoveryConflict
		}
	}
	return err
}

func (repository *ExecutionRepository) FindRecovery(
	ctx context.Context,
	companyID string,
	key string,
) (
	sourceID, recoveryID, fingerprint string,
	preserved, scheduled, reset int,
	createdAt time.Time,
	found bool,
	err error,
) {
	var record recoveryRequestRecord
	err = repository.dbClient.DB(ctx).
		Table("workflow_runtime.partial_recovery_requests").
		Where("company_id = ? AND idempotency_key = ?", companyID, key).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", "", 0, 0, 0, time.Time{}, false, nil
	}
	return record.SourceExecutionID, record.RecoveryExecutionID, record.RequestFingerprint,
		record.PreservedNodeCount, record.ScheduledNodeCount, record.ResetNodeCount,
		record.CreatedAt, err == nil, err
}
