package httptrigger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/shared/database"
)

type Repository struct {
	dbClient *database.Client
}

func NewRepository(dbClient *database.Client) *Repository {
	return &Repository{dbClient: dbClient}
}

func (triggerRepository *Repository) Create(
	ctx context.Context,
	binding Binding,
	definition workflow.Workflow,
) error {
	encodedDefinition, err := json.Marshal(definition)
	if err != nil {
		return fmt.Errorf("encode trigger workflow snapshot: %w", err)
	}
	transaction, err := triggerRepository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin HTTP trigger activation: %w", err)
	}
	defer transaction.Rollback(ctx)
	now := binding.CreatedAt
	if err := transaction.DB(ctx).
		Table("workflow_runtime.http_trigger_bindings").
		Where(
			"company_id = ? AND workflow_id = ? AND trigger_node_id = ? AND status = ?",
			binding.CompanyID, binding.WorkflowID, binding.TriggerNodeID, StatusActive,
		).
		Updates(map[string]any{
			"status": StatusDisabled, "disabled_at": now, "updated_at": now,
			"lock_version": gorm.Expr("lock_version + 1"),
		}).Error; err != nil {
		return fmt.Errorf("disable previous HTTP trigger for node: %w", err)
	}
	snapshot := map[string]any{
		"snapshot_id": binding.SnapshotID, "company_id": binding.CompanyID,
		"workflow_id": binding.WorkflowID, "workflow_revision": binding.WorkflowRevision,
		"workflow_name": definition.Name, "definition_json": encodedDefinition,
		"created_at": binding.CreatedAt,
	}
	if err := transaction.DB(ctx).
		Table("workflow_runtime.workflow_definition_snapshots").Create(snapshot).Error; err != nil {
		return fmt.Errorf("persist trigger workflow snapshot: %w", err)
	}
	record := triggerBindingRecordFromBinding(binding)
	if err := transaction.DB(ctx).
		Table("workflow_runtime.http_trigger_bindings").Create(&record).Error; err != nil {
		return fmt.Errorf("persist HTTP trigger binding: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit HTTP trigger activation: %w", err)
	}
	return nil
}

func (triggerRepository *Repository) FindByID(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	var records []triggerBindingRecord
	result := triggerRepository.dbClient.DB(ctx).
		Table("workflow_runtime.http_trigger_bindings").
		Where("company_id = ? AND trigger_id = ?", companyID, triggerID).
		Limit(1).Find(&records)
	if result.Error != nil {
		return Binding{}, fmt.Errorf("find HTTP trigger binding: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return Binding{}, repository.ErrNotFound
	}
	return bindingFromRecord(records[0]), nil
}

func (triggerRepository *Repository) FindActiveByTokenHash(
	ctx context.Context,
	tokenHash []byte,
) (Binding, error) {
	var records []triggerBindingRecord
	result := triggerRepository.dbClient.DB(ctx).
		Table("workflow_runtime.http_trigger_bindings").
		Where("token_hash = ? AND status = ?", tokenHash, StatusActive).
		Limit(1).Find(&records)
	if result.Error != nil {
		return Binding{}, fmt.Errorf("find active HTTP trigger binding: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return Binding{}, repository.ErrNotFound
	}
	return bindingFromRecord(records[0]), nil
}

func (triggerRepository *Repository) FindActiveByWorkflowAndNode(
	ctx context.Context,
	companyID string,
	workflowID string,
	triggerNodeID string,
) (Binding, error) {
	var records []triggerBindingRecord
	result := triggerRepository.dbClient.DB(ctx).
		Table("workflow_runtime.http_trigger_bindings").
		Where(
			"company_id = ? AND workflow_id = ? AND trigger_node_id = ? AND status = ?",
			companyID, workflowID, triggerNodeID, StatusActive,
		).
		Limit(1).Find(&records)
	if result.Error != nil {
		return Binding{}, fmt.Errorf(
			"find active HTTP trigger by workflow and node: %w", result.Error,
		)
	}
	if result.RowsAffected == 0 {
		return Binding{}, repository.ErrNotFound
	}
	return bindingFromRecord(records[0]), nil
}

func (triggerRepository *Repository) Disable(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, bool, error) {
	now := time.Now().UTC()
	var records []triggerBindingRecord
	result := triggerRepository.dbClient.DB(ctx).Raw(`
		UPDATE workflow_runtime.http_trigger_bindings
		SET status = ?,
			disabled_at = ?,
			updated_at = ?,
			lock_version = lock_version + 1
		WHERE company_id = ?
		  AND trigger_id = ?
		  AND status = ?
		RETURNING *`,
		StatusDisabled, now, now, companyID, triggerID, StatusActive,
	).Scan(&records)
	if result.Error != nil {
		return Binding{}, false, fmt.Errorf("disable HTTP trigger: %w", result.Error)
	}
	if len(records) == 1 {
		return bindingFromRecord(records[0]), true, nil
	}
	binding, err := triggerRepository.FindByID(ctx, companyID, triggerID)
	if err != nil {
		return Binding{}, false, err
	}
	return binding, false, nil
}

func (triggerRepository *Repository) DisableWorkflow(
	ctx context.Context,
	companyID string,
	workflowID string,
) (int, error) {
	now := time.Now().UTC()
	result := triggerRepository.dbClient.DB(ctx).
		Table("workflow_runtime.http_trigger_bindings").
		Where("company_id = ? AND workflow_id = ? AND status = ?", companyID, workflowID, StatusActive).
		Updates(map[string]any{
			"status": StatusDisabled, "disabled_at": now, "updated_at": now,
			"lock_version": gorm.Expr("lock_version + 1"),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("disable workflow HTTP triggers: %w", result.Error)
	}
	return int(result.RowsAffected), nil
}
