package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"miletos-go/internal/model"
)

type WorkflowRepository struct {
	db *pgxpool.Pool
}

func NewWorkflowRepository(db *pgxpool.Pool) *WorkflowRepository {
	return &WorkflowRepository{db: db}
}

func (repository *WorkflowRepository) Save(ctx context.Context, workflow model.Workflow) (model.WorkflowSnapshot, error) {
	definition, err := json.Marshal(workflow)
	if err != nil {
		return model.WorkflowSnapshot{}, fmt.Errorf("encode workflow definition: %w", err)
	}
	snapshot := model.WorkflowSnapshot{
		ID:        newID("snapshot_"),
		Workflow:  workflow,
		CreatedAt: time.Now().UTC(),
	}
	_, err = repository.db.Exec(ctx, `
		INSERT INTO workflow_runtime.workflow_definition_snapshots (
			snapshot_id, company_id, workflow_id, workflow_revision,
			workflow_name, definition_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		snapshot.ID, workflow.CompanyID, workflow.ID, workflow.Revision,
		workflow.Name, definition, snapshot.CreatedAt,
	)
	if err != nil {
		return model.WorkflowSnapshot{}, fmt.Errorf("save workflow definition: %w", err)
	}
	return snapshot, nil
}

func (repository *WorkflowRepository) FindByExecutionID(
	ctx context.Context,
	companyID string,
	executionID string,
) (model.WorkflowSnapshot, error) {
	var snapshot model.WorkflowSnapshot
	var definition []byte
	err := repository.db.QueryRow(ctx, `
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
		return model.WorkflowSnapshot{}, mapNoRows(err)
	}
	if err := json.Unmarshal(definition, &snapshot.Workflow); err != nil {
		return model.WorkflowSnapshot{}, fmt.Errorf("decode workflow definition: %w", err)
	}
	return snapshot, nil
}
