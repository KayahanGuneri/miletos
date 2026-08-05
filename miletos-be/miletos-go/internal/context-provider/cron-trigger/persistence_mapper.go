package crontrigger

import "time"

type bindingRecord struct {
	TriggerID        string        `gorm:"column:trigger_id"`
	CompanyID        string        `gorm:"column:company_id"`
	WorkflowID       string        `gorm:"column:workflow_id"`
	WorkflowRevision uint64        `gorm:"column:workflow_revision"`
	SnapshotID       string        `gorm:"column:snapshot_id"`
	TriggerNodeID    string        `gorm:"column:trigger_node_id"`
	CronExpression   string        `gorm:"column:cron_expression"`
	Timezone         string        `gorm:"column:timezone"`
	Status           BindingStatus `gorm:"column:status"`
	NextFireAt       time.Time     `gorm:"column:next_fire_at"`
	LastScheduledAt  *time.Time    `gorm:"column:last_scheduled_at"`
	LastFiredAt      *time.Time    `gorm:"column:last_fired_at"`
	CreatedAt        time.Time     `gorm:"column:created_at"`
	UpdatedAt        time.Time     `gorm:"column:updated_at"`
	DisabledAt       *time.Time    `gorm:"column:disabled_at"`
	LockVersion      int64         `gorm:"column:lock_version"`
}

type occurrenceRecord struct {
	OccurrenceID   string           `gorm:"column:occurrence_id"`
	TriggerID      string           `gorm:"column:trigger_id"`
	CompanyID      string           `gorm:"column:company_id"`
	WorkflowID     string           `gorm:"column:workflow_id"`
	SnapshotID     string           `gorm:"column:snapshot_id"`
	ScheduledAt    time.Time        `gorm:"column:scheduled_at"`
	Status         OccurrenceStatus `gorm:"column:status"`
	AttemptCount   int              `gorm:"column:attempt_count"`
	NextAttemptAt  time.Time        `gorm:"column:next_attempt_at"`
	ExecutionID    *string          `gorm:"column:execution_id"`
	FailureCode    *string          `gorm:"column:failure_code"`
	FailureMessage *string          `gorm:"column:failure_message"`
	CreatedAt      time.Time        `gorm:"column:created_at"`
	UpdatedAt      time.Time        `gorm:"column:updated_at"`
	LockVersion    int64            `gorm:"column:lock_version"`
}

func occurrenceFromRecord(record occurrenceRecord) Occurrence {
	return Occurrence{ID: record.OccurrenceID, TriggerID: record.TriggerID, CompanyID: record.CompanyID,
		WorkflowID: record.WorkflowID, SnapshotID: record.SnapshotID, ScheduledAt: record.ScheduledAt,
		Status: record.Status, AttemptCount: record.AttemptCount, NextAttemptAt: record.NextAttemptAt,
		ExecutionID: record.ExecutionID, FailureCode: record.FailureCode, FailureMessage: record.FailureMessage,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, LockVersion: record.LockVersion}
}

func recordFromBinding(binding Binding) bindingRecord {
	return bindingRecord{
		TriggerID: binding.ID, CompanyID: binding.CompanyID,
		WorkflowID: binding.WorkflowID, WorkflowRevision: binding.WorkflowRevision,
		SnapshotID: binding.SnapshotID, TriggerNodeID: binding.TriggerNodeID,
		CronExpression: binding.Expression, Timezone: binding.Timezone,
		Status: binding.Status, NextFireAt: binding.NextFireAt,
		LastScheduledAt: binding.LastScheduledAt, LastFiredAt: binding.LastFiredAt,
		CreatedAt: binding.CreatedAt, UpdatedAt: binding.UpdatedAt,
		DisabledAt: binding.DisabledAt, LockVersion: binding.LockVersion,
	}
}

func bindingFromRecord(record bindingRecord) Binding {
	return Binding{
		ID: record.TriggerID, CompanyID: record.CompanyID,
		WorkflowID: record.WorkflowID, WorkflowRevision: record.WorkflowRevision,
		SnapshotID: record.SnapshotID, TriggerNodeID: record.TriggerNodeID,
		Expression: record.CronExpression, Timezone: record.Timezone,
		Status: record.Status, NextFireAt: record.NextFireAt,
		LastScheduledAt: record.LastScheduledAt, LastFiredAt: record.LastFiredAt,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		DisabledAt: record.DisabledAt, LockVersion: record.LockVersion,
	}
}
