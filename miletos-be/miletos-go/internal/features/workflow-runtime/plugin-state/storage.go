package pluginstate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

type pluginNodeStateRecord struct {
	CompanyID        string    `gorm:"column:company_id"`
	WorkflowID       string    `gorm:"column:workflow_id"`
	WorkflowRevision uint64    `gorm:"column:workflow_revision"`
	NodeID           string    `gorm:"column:node_id"`
	StateKey         string    `gorm:"column:state_key"`
	StateValue       string    `gorm:"column:state_value;type:jsonb"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (pluginNodeStateRecord) TableName() string {
	return "workflow_runtime.plugin_node_state"
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
	var record pluginNodeStateRecord
	result := state.scopedQuery(key).Select("state_value").Limit(1).Find(&record)
	if result.Error != nil {
		return nil, false, fmt.Errorf("get plugin node state: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, false, nil
	}
	value, err := decodeStateValue(record.StateValue)
	if err != nil {
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
	record := pluginNodeStateRecord{
		CompanyID: state.scope.CompanyID, WorkflowID: state.scope.WorkflowID,
		WorkflowRevision: state.scope.WorkflowRevision, NodeID: state.scope.NodeID,
		StateKey: key, StateValue: string(encoded), CreatedAt: now, UpdatedAt: now,
	}
	err = state.repository.database.DB(state.ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "company_id"},
				{Name: "workflow_id"},
				{Name: "workflow_revision"},
				{Name: "node_id"},
				{Name: "state_key"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"state_value", "updated_at"}),
		}).
		Create(&record).Error
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
	var count int64
	if err := state.scopedQuery(key).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check plugin node state: %w", err)
	}
	return count > 0, nil
}

func (state *storage) Update(
	key string,
	updater plugin.StorageUpdater,
) (any, error) {
	key, err := state.validate(key)
	if err != nil {
		return nil, err
	}
	if updater == nil {
		return nil, fmt.Errorf("plugin node state updater is required")
	}
	transaction, err := state.repository.database.Begin(state.ctx)
	if err != nil {
		return nil, fmt.Errorf("begin plugin node state update: %w", err)
	}
	defer transaction.Rollback(state.ctx)
	lockIdentity, err := json.Marshal([]string{
		state.scope.CompanyID,
		state.scope.WorkflowID,
		fmt.Sprintf("%d", state.scope.WorkflowRevision),
		state.scope.NodeID,
		key,
	})
	if err != nil {
		return nil, fmt.Errorf("encode plugin node state lock identity: %w", err)
	}
	if _, err := transaction.Exec(
		state.ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))",
		string(lockIdentity),
	); err != nil {
		return nil, fmt.Errorf("lock plugin node state update: %w", err)
	}
	var record pluginNodeStateRecord
	result := transaction.DB(state.ctx).
		Where(
			"company_id = ? AND workflow_id = ? AND workflow_revision = ? AND node_id = ? AND state_key = ?",
			state.scope.CompanyID,
			state.scope.WorkflowID,
			state.scope.WorkflowRevision,
			state.scope.NodeID,
			key,
		).
		Take(&record)
	exists := result.Error == nil
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("get plugin node state for update: %w", result.Error)
	}
	var current any
	if exists {
		current, err = decodeStateValue(record.StateValue)
		if err != nil {
			return nil, fmt.Errorf("decode plugin node state for update: %w", err)
		}
	}
	next, err := updater(current, exists)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("plugin node state must be JSON-compatible: %w", err)
	}
	now := time.Now().UTC()
	updated := pluginNodeStateRecord{
		CompanyID: state.scope.CompanyID, WorkflowID: state.scope.WorkflowID,
		WorkflowRevision: state.scope.WorkflowRevision, NodeID: state.scope.NodeID,
		StateKey: key, StateValue: string(encoded), CreatedAt: now, UpdatedAt: now,
	}
	if exists {
		updated.CreatedAt = record.CreatedAt
	}
	if err := transaction.DB(state.ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "company_id"}, {Name: "workflow_id"},
			{Name: "workflow_revision"}, {Name: "node_id"}, {Name: "state_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"state_value", "updated_at"}),
	}).Create(&updated).Error; err != nil {
		return nil, fmt.Errorf("set plugin node state for update: %w", err)
	}
	if err := transaction.Commit(state.ctx); err != nil {
		return nil, fmt.Errorf("commit plugin node state update: %w", err)
	}
	return next, nil
}

func decodeStateValue(encoded string) (any, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(encoded))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func (state *storage) scopedQuery(key string) *gorm.DB {
	return state.repository.database.DB(state.ctx).
		Model(&pluginNodeStateRecord{}).
		Where(
			"company_id = ? AND workflow_id = ? AND workflow_revision = ? AND node_id = ? AND state_key = ?",
			state.scope.CompanyID,
			state.scope.WorkflowID,
			state.scope.WorkflowRevision,
			state.scope.NodeID,
			key,
		)
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
