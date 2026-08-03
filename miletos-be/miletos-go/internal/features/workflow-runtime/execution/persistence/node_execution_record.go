package persistence

import "time"

type NodeExecutionRecord struct {
	NodeExecutionID string     `gorm:"column:node_execution_id"`
	ExecutionID     string     `gorm:"column:workflow_execution_id"`
	CompanyID       string     `gorm:"column:company_id"`
	NodeID          string     `gorm:"column:node_id"`
	PluginType      string     `gorm:"column:plugin_type"`
	PluginVersion   string     `gorm:"column:plugin_version"`
	Status          string     `gorm:"column:status"`
	Attempt         int        `gorm:"column:attempt"`
	CreatedAt       time.Time  `gorm:"column:created_at"`
	ReadyAt         *time.Time `gorm:"column:ready_at"`
	QueuedAt        *time.Time `gorm:"column:queued_at"`
	StartedAt       *time.Time `gorm:"column:started_at"`
	FinishedAt      *time.Time `gorm:"column:finished_at"`
	NextAttemptAt   *time.Time `gorm:"column:next_attempt_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
	InputSummary    []byte     `gorm:"column:input_summary"`
	OutputSummary   []byte     `gorm:"column:output_summary"`
	FailureSummary  []byte     `gorm:"column:failure_summary"`
	LockVersion     int64      `gorm:"column:lock_version"`
}
