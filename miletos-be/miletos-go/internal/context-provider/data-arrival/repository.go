package dataarrival

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm/clause"

	"miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/shared/database"
)

const (
	bindingStatusActive   = "ACTIVE"
	bindingStatusDisabled = "DISABLED"
)

type Repository struct {
	database *database.Client
}

type bindingRecord struct {
	CompanyID        string     `gorm:"column:company_id"`
	WorkflowID       string     `gorm:"column:workflow_id"`
	WorkflowRevision uint64     `gorm:"column:workflow_revision"`
	SnapshotID       string     `gorm:"column:snapshot_id"`
	NodeID           string     `gorm:"column:node_id"`
	PluginType       string     `gorm:"column:plugin_type"`
	Status           string     `gorm:"column:status"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
	DisabledAt       *time.Time `gorm:"column:disabled_at"`
}

func (bindingRecord) TableName() string {
	return "workflow_runtime.data_arrival_source_bindings"
}

func NewRepository(dbClient *database.Client) *Repository {
	return &Repository{database: dbClient}
}

func (repository *Repository) Activate(
	ctx context.Context,
	snapshot workflow.WorkflowSnapshot,
	bindings []Binding,
) error {
	transaction, err := repository.database.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin data-arrival source activation: %w", err)
	}
	defer transaction.Rollback(ctx)
	now := time.Now().UTC()
	if err := disableWorkflowBindings(
		ctx, transaction, snapshot.Workflow.CompanyID, snapshot.Workflow.ID, now,
	); err != nil {
		return err
	}
	if len(bindings) > 0 {
		definitionJSON, err := json.Marshal(snapshot.Workflow)
		if err != nil {
			return fmt.Errorf("encode data-arrival workflow snapshot: %w", err)
		}
		snapshotRecord := map[string]any{
			"snapshot_id": snapshot.ID, "company_id": snapshot.Workflow.CompanyID,
			"workflow_id": snapshot.Workflow.ID, "workflow_revision": snapshot.Workflow.Revision,
			"workflow_name": snapshot.Workflow.Name, "definition_json": definitionJSON,
			"created_at": snapshot.CreatedAt,
		}
		if err := transaction.DB(ctx).
			Table("workflow_runtime.workflow_definition_snapshots").
			Clauses(clause.OnConflict{DoNothing: true}).
			Create(snapshotRecord).Error; err != nil {
			return fmt.Errorf("persist data-arrival workflow snapshot: %w", err)
		}
		for _, binding := range bindings {
			record := bindingRecord{
				CompanyID: binding.CompanyID, WorkflowID: binding.WorkflowID,
				WorkflowRevision: binding.WorkflowRevision, SnapshotID: binding.SnapshotID,
				NodeID: binding.NodeID, PluginType: binding.PluginType,
				Status: bindingStatusActive, CreatedAt: now, UpdatedAt: now,
			}
			if err := transaction.DB(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "company_id"}, {Name: "workflow_id"},
					{Name: "workflow_revision"}, {Name: "node_id"},
				},
				DoUpdates: clause.Assignments(map[string]any{
					"snapshot_id": snapshot.ID, "plugin_type": binding.PluginType,
					"status": bindingStatusActive, "updated_at": now, "disabled_at": nil,
				}),
			}).Create(&record).Error; err != nil {
				return fmt.Errorf("persist data-arrival source binding: %w", err)
			}
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit data-arrival source activation: %w", err)
	}
	return nil
}

func (repository *Repository) ListActive(ctx context.Context) ([]Binding, error) {
	var records []bindingRecord
	if err := repository.database.DB(ctx).
		Where("status = ?", bindingStatusActive).
		Order("company_id, workflow_id, workflow_revision, node_id").
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list active data-arrival source bindings: %w", err)
	}
	bindings := make([]Binding, 0, len(records))
	for _, record := range records {
		bindings = append(bindings, bindingFromRecord(record))
	}
	return bindings, nil
}

func (repository *Repository) IsActive(ctx context.Context, binding Binding) (bool, error) {
	var count int64
	err := repository.database.DB(ctx).Model(&bindingRecord{}).
		Where(
			"company_id = ? AND workflow_id = ? AND workflow_revision = ? AND node_id = ? AND status = ?",
			binding.CompanyID, binding.WorkflowID, binding.WorkflowRevision,
			binding.NodeID, bindingStatusActive,
		).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check data-arrival source binding: %w", err)
	}
	return count == 1, nil
}

func (repository *Repository) DisableWorkflow(
	ctx context.Context,
	companyID string,
	workflowID string,
) (int, error) {
	now := time.Now().UTC()
	result := repository.database.DB(ctx).Model(&bindingRecord{}).
		Where(
			"company_id = ? AND workflow_id = ? AND status = ?",
			companyID, workflowID, bindingStatusActive,
		).
		Updates(map[string]any{
			"status": bindingStatusDisabled, "updated_at": now, "disabled_at": now,
		})
	if result.Error != nil {
		return 0, fmt.Errorf("disable workflow data-arrival sources: %w", result.Error)
	}
	return int(result.RowsAffected), nil
}

func disableWorkflowBindings(
	ctx context.Context,
	transaction *database.Transaction,
	companyID string,
	workflowID string,
	now time.Time,
) error {
	result := transaction.DB(ctx).Model(&bindingRecord{}).
		Where(
			"company_id = ? AND workflow_id = ? AND status = ?",
			companyID, workflowID, bindingStatusActive,
		).
		Updates(map[string]any{
			"status": bindingStatusDisabled, "updated_at": now, "disabled_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("disable obsolete data-arrival source bindings: %w", result.Error)
	}
	return nil
}

func bindingFromRecord(record bindingRecord) Binding {
	return Binding{
		CompanyID: record.CompanyID, WorkflowID: record.WorkflowID,
		WorkflowRevision: record.WorkflowRevision, SnapshotID: record.SnapshotID,
		NodeID: record.NodeID, PluginType: record.PluginType,
	}
}
