package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/shared/database"

	"gorm.io/gorm"
)

type WorkflowRepository struct {
	dbClient *database.Client
}

func NewWorkflowRepository(dbClient *database.Client) *WorkflowRepository {
	return &WorkflowRepository{dbClient: dbClient}
}

func (repository *WorkflowRepository) Save(ctx context.Context, workflow Workflow) (WorkflowSnapshot, error) {
	definition, err := json.Marshal(workflow)
	if err != nil {
		return WorkflowSnapshot{}, fmt.Errorf("encode workflow definition: %w", err)
	}
	snapshot := WorkflowSnapshot{
		ID:        newID("snapshot_"),
		Workflow:  workflow,
		CreatedAt: time.Now().UTC(),
	}
	err = repository.dbClient.DB(ctx).
		Table("workflow_runtime.workflow_definition_snapshots").
		Create(&workflowSnapshotRecord{
			SnapshotID: snapshot.ID, CompanyID: workflow.CompanyID,
			WorkflowID: workflow.ID, WorkflowRevision: workflow.Revision,
			WorkflowName: workflow.Name, DefinitionJSON: definition,
			CreatedAt: snapshot.CreatedAt,
		}).Error
	if err != nil {
		return WorkflowSnapshot{}, fmt.Errorf("save workflow definition: %w", err)
	}
	return snapshot, nil
}

func (repository *WorkflowRepository) FindByExecutionID(
	ctx context.Context,
	companyID string,
	executionID string,
) (WorkflowSnapshot, error) {
	var record workflowSnapshotRecord
	err := repository.dbClient.DB(ctx).
		Table("workflow_runtime.workflow_definition_snapshots AS snapshot").
		Select("snapshot.snapshot_id, snapshot.definition_json, snapshot.created_at").
		Joins(`JOIN workflow_runtime.workflow_executions AS execution
			ON execution.snapshot_id = snapshot.snapshot_id
			AND execution.company_id = snapshot.company_id`).
		Where("execution.company_id = ? AND execution.workflow_execution_id = ?", companyID, executionID).
		Take(&record).Error
	if err != nil {
		return WorkflowSnapshot{}, mapNoRows(err)
	}
	return workflowSnapshotFromRecord(record)
}

func (repository *WorkflowRepository) FindBySnapshotID(
	ctx context.Context,
	companyID string,
	snapshotID string,
) (WorkflowSnapshot, error) {
	var records []workflowSnapshotRecord
	result := repository.dbClient.DB(ctx).
		Table("workflow_runtime.workflow_definition_snapshots").
		Select("snapshot_id, definition_json, created_at").
		Where("company_id = ? AND snapshot_id = ?", companyID, snapshotID).
		Limit(1).Find(&records)
	if result.Error != nil {
		return WorkflowSnapshot{}, fmt.Errorf("find workflow snapshot: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return WorkflowSnapshot{}, ErrNotFound
	}
	return workflowSnapshotFromRecord(records[0])
}

func newID(prefix string) string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err == nil {
		return prefix + hex.EncodeToString(value)
	}
	return fmt.Sprintf("%s%x", prefix, time.Now().UTC().UnixNano())
}

func mapNoRows(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

var ErrNotFound = database.ErrNotFound
