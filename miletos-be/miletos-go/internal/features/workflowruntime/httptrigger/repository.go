package httptrigger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/workflow"
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
	workflow workflow.Workflow,
) error {
	definition, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("encode trigger workflow snapshot: %w", err)
	}
	transaction, err := triggerRepository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin HTTP trigger activation: %w", err)
	}
	defer transaction.Rollback(ctx)
	if _, err := transaction.Exec(ctx, `
		INSERT INTO workflow_runtime.workflow_definition_snapshots (
			snapshot_id, company_id, workflow_id, workflow_revision,
			workflow_name, definition_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		binding.SnapshotID, binding.CompanyID, binding.WorkflowID,
		binding.WorkflowRevision, workflow.Name, definition, binding.CreatedAt,
	); err != nil {
		return fmt.Errorf("persist trigger workflow snapshot: %w", err)
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO workflow_runtime.http_trigger_bindings (
			trigger_id, company_id, workflow_id, workflow_revision, snapshot_id,
			trigger_node_id, http_method, token_hash, status, resolved_mode,
			created_at, updated_at, lock_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'ACTIVE', $9, $10, $10, 0)`,
		binding.ID, binding.CompanyID, binding.WorkflowID, binding.WorkflowRevision,
		binding.SnapshotID, binding.TriggerNodeID, binding.Method, binding.TokenHash,
		binding.ResolvedMode, binding.CreatedAt,
	); err != nil {
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
	return scanBinding(triggerRepository.dbClient.QueryRow(ctx, bindingSelect+`
		WHERE company_id = $1 AND trigger_id = $2`,
		companyID, triggerID,
	))
}

func (triggerRepository *Repository) FindActiveByTokenHash(
	ctx context.Context,
	tokenHash []byte,
) (Binding, error) {
	return scanBinding(triggerRepository.dbClient.QueryRow(ctx, bindingSelect+`
		WHERE token_hash = $1 AND status = 'ACTIVE'`,
		tokenHash,
	))
}

func (triggerRepository *Repository) Disable(
	ctx context.Context,
	companyID string,
	triggerID string,
) (Binding, error) {
	now := time.Now().UTC()
	result, err := triggerRepository.dbClient.Exec(ctx, `
		UPDATE workflow_runtime.http_trigger_bindings
		SET status = 'DISABLED', disabled_at = $3, updated_at = $3,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND trigger_id = $2 AND status = 'ACTIVE'`,
		companyID, triggerID, now,
	)
	if err != nil {
		return Binding{}, fmt.Errorf("disable HTTP trigger: %w", err)
	}
	if result.RowsAffected() == 0 {
		binding, findErr := triggerRepository.FindByID(ctx, companyID, triggerID)
		if findErr != nil {
			return Binding{}, findErr
		}
		if binding.Status != StatusDisabled {
			return Binding{}, repository.ErrStateTransition
		}
		return binding, nil
	}
	return triggerRepository.FindByID(ctx, companyID, triggerID)
}

const bindingSelect = `
	SELECT trigger_id, company_id, workflow_id, workflow_revision, snapshot_id,
		trigger_node_id, http_method, token_hash, status, resolved_mode,
		created_at, updated_at, disabled_at, lock_version
	FROM workflow_runtime.http_trigger_bindings`

func scanBinding(row interface{ Scan(...any) error }) (Binding, error) {
	var binding Binding
	var disabledAt sql.NullTime
	err := row.Scan(
		&binding.ID, &binding.CompanyID, &binding.WorkflowID,
		&binding.WorkflowRevision, &binding.SnapshotID, &binding.TriggerNodeID,
		&binding.Method, &binding.TokenHash, &binding.Status, &binding.ResolvedMode,
		&binding.CreatedAt, &binding.UpdatedAt, &disabledAt, &binding.LockVersion,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Binding{}, repository.ErrNotFound
	}
	if err != nil {
		return Binding{}, err
	}
	if disabledAt.Valid {
		binding.DisabledAt = &disabledAt.Time
	}
	return binding, nil
}
