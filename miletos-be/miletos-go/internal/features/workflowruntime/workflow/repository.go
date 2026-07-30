package workflow

import (
	"context"
	"crypto/rand"
	"database/sql"
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
	_, err = repository.dbClient.Exec(ctx, `
		INSERT INTO workflow_runtime.workflow_definition_snapshots (
			snapshot_id, company_id, workflow_id, workflow_revision,
			workflow_name, definition_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		snapshot.ID, workflow.CompanyID, workflow.ID, workflow.Revision,
		workflow.Name, definition, snapshot.CreatedAt,
	)
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
	var snapshot WorkflowSnapshot
	var definition []byte
	err := repository.dbClient.QueryRow(ctx, `
		SELECT snapshot.snapshot_id, snapshot.definition_json, snapshot.created_at
		FROM workflow_runtime.workflow_definition_snapshots snapshot
		JOIN workflow_runtime.workflow_executions execution
		  ON execution.snapshot_id = snapshot.snapshot_id
		 AND execution.company_id = snapshot.company_id
		WHERE execution.company_id = $1
		  AND execution.workflow_execution_id = $2`,
		companyID, executionID,
	).Scan(&snapshot.ID, &definition, &snapshot.CreatedAt)
	if err != nil {
		return WorkflowSnapshot{}, mapNoRows(err)
	}
	if err := json.Unmarshal(definition, &snapshot.Workflow); err != nil {
		return WorkflowSnapshot{}, fmt.Errorf("decode workflow definition: %w", err)
	}
	return snapshot, nil
}

func (repository *WorkflowRepository) FindBySnapshotID(
	ctx context.Context,
	companyID string,
	snapshotID string,
) (WorkflowSnapshot, error) {
	var snapshot WorkflowSnapshot
	var definition []byte
	err := repository.dbClient.QueryRow(ctx, `
		SELECT snapshot_id, definition_json, created_at
		FROM workflow_runtime.workflow_definition_snapshots
		WHERE company_id = $1 AND snapshot_id = $2`,
		companyID, snapshotID,
	).Scan(&snapshot.ID, &definition, &snapshot.CreatedAt)
	if err != nil {
		return WorkflowSnapshot{}, mapNoRows(err)
	}
	if err := json.Unmarshal(definition, &snapshot.Workflow); err != nil {
		return WorkflowSnapshot{}, fmt.Errorf("decode workflow definition: %w", err)
	}
	return snapshot, nil
}

func newID(prefix string) string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err == nil {
		return prefix + hex.EncodeToString(value)
	}
	return fmt.Sprintf("%s%x", prefix, time.Now().UTC().UnixNano())
}

func mapNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

var ErrNotFound = database.ErrNotFound
