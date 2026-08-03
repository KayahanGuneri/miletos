package persistence

import "time"

type ExecutionEventRecord struct {
	EventID         string    `gorm:"column:event_id"`
	ExecutionID     string    `gorm:"column:workflow_execution_id"`
	NodeExecutionID string    `gorm:"column:node_execution_id"`
	Sequence        int64     `gorm:"column:sequence_number"`
	EventType       string    `gorm:"column:event_type"`
	PreviousStatus  string    `gorm:"column:previous_status"`
	NewStatus       string    `gorm:"column:new_status"`
	CorrelationID   string    `gorm:"column:correlation_id"`
	CausationID     string    `gorm:"column:causation_id"`
	SafeMessage     string    `gorm:"column:safe_message"`
	Metadata        []byte    `gorm:"column:metadata"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

type ExecutionLogRecord struct {
	LogID           string    `gorm:"column:log_id"`
	ExecutionID     string    `gorm:"column:workflow_execution_id"`
	NodeExecutionID string    `gorm:"column:node_execution_id"`
	Sequence        int64     `gorm:"column:sequence_number"`
	Level           string    `gorm:"column:level"`
	Message         string    `gorm:"column:message"`
	Metadata        []byte    `gorm:"column:metadata"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

type ExecutionErrorRecord struct {
	ErrorID         string    `gorm:"column:error_id"`
	ExecutionID     string    `gorm:"column:workflow_execution_id"`
	CompanyID       string    `gorm:"column:company_id"`
	NodeExecutionID string    `gorm:"column:node_execution_id"`
	RelatedEventID  *string   `gorm:"column:related_event_id"`
	Category        string    `gorm:"column:category"`
	Code            string    `gorm:"column:code"`
	SafeMessage     string    `gorm:"column:safe_message"`
	Retryable       bool      `gorm:"column:retryable"`
	Details         []byte    `gorm:"column:details"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}
