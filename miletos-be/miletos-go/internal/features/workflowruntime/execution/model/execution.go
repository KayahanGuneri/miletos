package model

import "time"

const (
	ExecutionCreated    = "CREATED"
	ExecutionValidating = "VALIDATING"
	ExecutionRejected   = "REJECTED"
	ExecutionQueued     = "QUEUED"
	ExecutionRunning    = "RUNNING"
	ExecutionSucceeded  = "SUCCEEDED"
	ExecutionFailed     = "FAILED"
	ExecutionCancelled  = "CANCELLED"
	ExecutionTimedOut   = "TIMED_OUT"

	NodePending      = "PENDING"
	NodeReady        = "READY"
	NodeQueued       = "QUEUED"
	NodeRunning      = "RUNNING"
	NodeRetryPending = "RETRY_PENDING"
	NodeSucceeded    = "SUCCEEDED"
	NodeFailed       = "FAILED"
	NodeSkipped      = "SKIPPED"
	NodeCancelled    = "CANCELLED"
	NodeTimedOut     = "TIMED_OUT"
)

type Execution struct {
	ID               string         `json:"executionId"`
	CompanyID        string         `json:"-"`
	WorkflowID       string         `json:"workflowId"`
	WorkflowRevision uint64         `json:"workflowRevision"`
	SnapshotID       string         `json:"snapshotId,omitempty"`
	Mode             string         `json:"mode"`
	Origin           string         `json:"origin,omitempty"`
	CorrelationID    string         `json:"correlationId"`
	Status           string         `json:"status"`
	CreatedAt        time.Time      `json:"createdAt"`
	ValidatingAt     *time.Time     `json:"validatingAt,omitempty"`
	QueuedAt         *time.Time     `json:"queuedAt,omitempty"`
	StartedAt        *time.Time     `json:"startedAt,omitempty"`
	FinishedAt       *time.Time     `json:"finishedAt,omitempty"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	TerminalOutputs  map[string]any `json:"terminalOutputs,omitempty"`
	Failure          map[string]any `json:"failureSummary,omitempty"`
	IsStalled        bool           `json:"isStalled"`
}

type NodeExecution struct {
	ID            string         `json:"nodeExecutionId"`
	ExecutionID   string         `json:"workflowExecutionId"`
	CompanyID     string         `json:"-"`
	NodeID        string         `json:"nodeId"`
	Type          string         `json:"pluginType"`
	Version       string         `json:"pluginVersion"`
	Status        string         `json:"status"`
	Attempt       int            `json:"attempt"`
	CreatedAt     time.Time      `json:"createdAt"`
	ReadyAt       *time.Time     `json:"readyAt,omitempty"`
	QueuedAt      *time.Time     `json:"queuedAt,omitempty"`
	StartedAt     *time.Time     `json:"startedAt,omitempty"`
	FinishedAt    *time.Time     `json:"finishedAt,omitempty"`
	NextAttemptAt *time.Time     `json:"nextAttemptAt,omitempty"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	Input         map[string]any `json:"inputSummary,omitempty"`
	Output        map[string]any `json:"outputSummary,omitempty"`
	Failure       map[string]any `json:"failureSummary,omitempty"`
	LockVersion   int64          `json:"-"`
}

const (
	ExecutionOriginManualDirect = "MANUAL_DIRECT"
	ExecutionOriginHTTPWebhook  = "HTTP_WEBHOOK"
)

type ExecutionEvent struct {
	ID              string         `json:"eventId"`
	ExecutionID     string         `json:"workflowExecutionId"`
	NodeExecutionID string         `json:"nodeExecutionId,omitempty"`
	Sequence        int64          `json:"sequenceNumber"`
	Type            string         `json:"type"`
	PreviousStatus  string         `json:"previousStatus,omitempty"`
	NewStatus       string         `json:"newStatus,omitempty"`
	CorrelationID   string         `json:"correlationId,omitempty"`
	CausationID     string         `json:"causationId,omitempty"`
	Message         string         `json:"safeMessage,omitempty"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type ExecutionLog struct {
	ID              string         `json:"logId"`
	ExecutionID     string         `json:"workflowExecutionId"`
	NodeExecutionID string         `json:"nodeExecutionId,omitempty"`
	Sequence        int64          `json:"sequenceNumber"`
	Level           string         `json:"level"`
	Message         string         `json:"message"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type ExecutionError struct {
	ID              string         `json:"errorId"`
	ExecutionID     string         `json:"workflowExecutionId"`
	NodeExecutionID string         `json:"nodeExecutionId,omitempty"`
	RelatedEventID  string         `json:"relatedEventId,omitempty"`
	Category        string         `json:"category"`
	Code            string         `json:"code"`
	Message         string         `json:"safeMessage"`
	Retryable       bool           `json:"retryable"`
	Details         map[string]any `json:"details"`
	CreatedAt       time.Time      `json:"createdAt"`
}

type Page[T any] struct {
	Items   []T    `json:"items"`
	Next    string `json:"next"`
	HasNext bool   `json:"hasNext"`
}
