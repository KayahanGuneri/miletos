package pluginstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/shared/database"
)

type Scope struct {
	CompanyID        string
	WorkflowID       string
	WorkflowRevision uint64
	NodeID           string
}

type Repository struct {
	database *database.Client
}

type storage struct {
	ctx        context.Context
	repository *Repository
	scope      Scope
}

func NewRepository(dbClient *database.Client) *Repository {
	return &Repository{database: dbClient}
}

func NewStorage(
	ctx context.Context,
	repository *Repository,
	scope Scope,
) plugin.Storage {
	return &storage{ctx: ctx, repository: repository, scope: scope}
}

func (state *storage) Get(key string) (any, bool, error) {
	key, err := state.validate(key)
	if err != nil {
		return nil, false, err
	}
	var encoded []byte
	err = state.repository.database.QueryRow(
		state.ctx,
		`SELECT state_value
		 FROM workflow_runtime.plugin_node_state
		 WHERE company_id = $1
		   AND workflow_id = $2
		   AND workflow_revision = $3
		   AND node_id = $4
		   AND state_key = $5`,
		state.scope.CompanyID,
		state.scope.WorkflowID,
		state.scope.WorkflowRevision,
		state.scope.NodeID,
		key,
	).Scan(&encoded)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get plugin node state: %w", err)
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil, false, fmt.Errorf("decode plugin node state: %w", err)
	}
	return value, true, nil
}

func (state *storage) Set(key string, value any) error {
	key, err := state.validate(key)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("plugin node state must be JSON-compatible: %w", err)
	}
	now := time.Now().UTC()
	_, err = state.repository.database.Exec(
		state.ctx,
		`INSERT INTO workflow_runtime.plugin_node_state (
		    company_id, workflow_id, workflow_revision, node_id,
		    state_key, state_value, created_at, updated_at
		 ) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $7)
		 ON CONFLICT (
		    company_id, workflow_id, workflow_revision, node_id, state_key
		 ) DO UPDATE SET
		    state_value = EXCLUDED.state_value,
		    updated_at = EXCLUDED.updated_at`,
		state.scope.CompanyID,
		state.scope.WorkflowID,
		state.scope.WorkflowRevision,
		state.scope.NodeID,
		key,
		string(encoded),
		now,
	)
	if err != nil {
		return fmt.Errorf("set plugin node state: %w", err)
	}
	return nil
}

func (state *storage) Has(key string) (bool, error) {
	key, err := state.validate(key)
	if err != nil {
		return false, err
	}
	var exists bool
	err = state.repository.database.QueryRow(
		state.ctx,
		`SELECT EXISTS (
		    SELECT 1
		    FROM workflow_runtime.plugin_node_state
		    WHERE company_id = $1
		      AND workflow_id = $2
		      AND workflow_revision = $3
		      AND node_id = $4
		      AND state_key = $5
		)`,
		state.scope.CompanyID,
		state.scope.WorkflowID,
		state.scope.WorkflowRevision,
		state.scope.NodeID,
		key,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check plugin node state: %w", err)
	}
	return exists, nil
}

func (state *storage) validate(key string) (string, error) {
	if state.repository == nil || state.repository.database == nil {
		return "", fmt.Errorf("plugin node state repository is unavailable")
	}
	if strings.TrimSpace(state.scope.CompanyID) == "" ||
		strings.TrimSpace(state.scope.WorkflowID) == "" ||
		state.scope.WorkflowRevision == 0 ||
		strings.TrimSpace(state.scope.NodeID) == "" {
		return "", fmt.Errorf("plugin node state scope is invalid")
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 255 {
		return "", fmt.Errorf("plugin node state key must contain between 1 and 255 bytes")
	}
	return key, nil
}
