package workflow

import (
	"encoding/json"
	"fmt"
	"time"
)

type workflowSnapshotRecord struct {
	SnapshotID       string    `gorm:"column:snapshot_id"`
	CompanyID        string    `gorm:"column:company_id"`
	WorkflowID       string    `gorm:"column:workflow_id"`
	WorkflowRevision uint64    `gorm:"column:workflow_revision"`
	WorkflowName     string    `gorm:"column:workflow_name"`
	DefinitionJSON   []byte    `gorm:"column:definition_json"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func workflowSnapshotFromRecord(record workflowSnapshotRecord) (WorkflowSnapshot, error) {
	snapshot := WorkflowSnapshot{ID: record.SnapshotID, CreatedAt: record.CreatedAt}
	if err := json.Unmarshal(record.DefinitionJSON, &snapshot.Workflow); err != nil {
		return WorkflowSnapshot{}, fmt.Errorf("decode workflow definition: %w", err)
	}
	return snapshot, nil
}
