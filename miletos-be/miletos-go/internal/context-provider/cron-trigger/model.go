package crontrigger

import "time"

type BindingStatus string
type ResolvedMode string

const (
	StatusActive   BindingStatus = "ACTIVE"
	StatusDisabled BindingStatus = "DISABLED"
	ModeAsync      ResolvedMode  = "ASYNC"
)

type Binding struct {
	ID               string
	CompanyID        string
	WorkflowID       string
	WorkflowRevision uint64
	SnapshotID       string
	TriggerNodeID    string
	Expression       string
	Timezone         string
	Status           BindingStatus
	NextFireAt       time.Time
	LastScheduledAt  *time.Time
	LastFiredAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DisabledAt       *time.Time
	LockVersion      int64
}

type DueOccurrence struct {
	Binding
	OccurrenceID string
	ScheduledAt  time.Time
	FiredAt      time.Time
}

type OccurrenceStatus string

const (
	OccurrencePending    OccurrenceStatus = "PENDING"
	OccurrenceProcessing OccurrenceStatus = "PROCESSING"
	OccurrenceSucceeded  OccurrenceStatus = "SUCCEEDED"
	OccurrenceFailed     OccurrenceStatus = "FAILED"
)

type Occurrence struct {
	ID             string
	TriggerID      string
	CompanyID      string
	WorkflowID     string
	SnapshotID     string
	ScheduledAt    time.Time
	Status         OccurrenceStatus
	AttemptCount   int
	NextAttemptAt  time.Time
	ExecutionID    *string
	FailureCode    *string
	FailureMessage *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LockVersion    int64
}
