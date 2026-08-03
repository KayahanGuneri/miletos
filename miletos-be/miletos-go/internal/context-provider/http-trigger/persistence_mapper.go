package httptrigger

import "time"

type triggerBindingRecord struct {
	TriggerID        string        `gorm:"column:trigger_id"`
	CompanyID        string        `gorm:"column:company_id"`
	WorkflowID       string        `gorm:"column:workflow_id"`
	WorkflowRevision uint64        `gorm:"column:workflow_revision"`
	SnapshotID       string        `gorm:"column:snapshot_id"`
	TriggerNodeID    string        `gorm:"column:trigger_node_id"`
	HTTPMethod       string        `gorm:"column:http_method"`
	TokenHash        []byte        `gorm:"column:token_hash"`
	Status           BindingStatus `gorm:"column:status"`
	ResolvedMode     ResolvedMode  `gorm:"column:resolved_mode"`
	CreatedAt        time.Time     `gorm:"column:created_at"`
	UpdatedAt        time.Time     `gorm:"column:updated_at"`
	DisabledAt       *time.Time    `gorm:"column:disabled_at"`
	LockVersion      int64         `gorm:"column:lock_version"`
}

func triggerBindingRecordFromBinding(binding Binding) triggerBindingRecord {
	return triggerBindingRecord{
		TriggerID: binding.ID, CompanyID: binding.CompanyID,
		WorkflowID: binding.WorkflowID, WorkflowRevision: binding.WorkflowRevision,
		SnapshotID: binding.SnapshotID, TriggerNodeID: binding.TriggerNodeID,
		HTTPMethod: binding.Method, TokenHash: binding.TokenHash,
		Status: StatusActive, ResolvedMode: binding.ResolvedMode,
		CreatedAt: binding.CreatedAt, UpdatedAt: binding.UpdatedAt,
		LockVersion: binding.LockVersion,
	}
}

func bindingFromRecord(record triggerBindingRecord) Binding {
	return Binding{
		ID: record.TriggerID, CompanyID: record.CompanyID,
		WorkflowID: record.WorkflowID, WorkflowRevision: record.WorkflowRevision,
		SnapshotID: record.SnapshotID, TriggerNodeID: record.TriggerNodeID,
		Method: record.HTTPMethod, TokenHash: record.TokenHash,
		Status: record.Status, ResolvedMode: record.ResolvedMode,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		DisabledAt: record.DisabledAt, LockVersion: record.LockVersion,
	}
}
