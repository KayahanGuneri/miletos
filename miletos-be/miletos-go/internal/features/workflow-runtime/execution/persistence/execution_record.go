package persistence

import "time"

type ExecutionRecord struct {
	WorkflowExecutionID string     `gorm:"column:workflow_execution_id"`
	CompanyID           string     `gorm:"column:company_id"`
	WorkflowID          string     `gorm:"column:workflow_id"`
	WorkflowRevision    uint64     `gorm:"column:workflow_revision"`
	SnapshotID          string     `gorm:"column:snapshot_id"`
	Mode                string     `gorm:"column:mode"`
	ExecutionOrigin     string     `gorm:"column:execution_origin"`
	CorrelationID       string     `gorm:"column:correlation_id"`
	Status              string     `gorm:"column:status"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	ValidatingAt        *time.Time `gorm:"column:validating_at"`
	QueuedAt            *time.Time `gorm:"column:queued_at"`
	StartedAt           *time.Time `gorm:"column:started_at"`
	FinishedAt          *time.Time `gorm:"column:finished_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
	TerminalOutputs     []byte     `gorm:"column:terminal_outputs"`
	FailureSummary      []byte     `gorm:"column:failure_summary"`
	IsStalled           bool       `gorm:"column:is_stalled"`
}
