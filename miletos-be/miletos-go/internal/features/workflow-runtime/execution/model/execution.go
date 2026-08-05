package model

import "time"

type ExecutionStatus string
type NodeStatus string
type ExecutionOrigin string
type FailureCategory string
type FailureCode string
type RetryDecisionKind string
type RetryDecisionReason string
type ObservationResource string

const (
	ExecutionCreated    ExecutionStatus = "CREATED"
	ExecutionValidating ExecutionStatus = "VALIDATING"
	ExecutionRejected   ExecutionStatus = "REJECTED"
	ExecutionQueued     ExecutionStatus = "QUEUED"
	ExecutionRunning    ExecutionStatus = "RUNNING"
	ExecutionSucceeded  ExecutionStatus = "SUCCEEDED"
	ExecutionFailed     ExecutionStatus = "FAILED"
	ExecutionCancelled  ExecutionStatus = "CANCELLED"
	ExecutionTimedOut   ExecutionStatus = "TIMED_OUT"

	NodePending      NodeStatus = "PENDING"
	NodeReady        NodeStatus = "READY"
	NodeQueued       NodeStatus = "QUEUED"
	NodeRunning      NodeStatus = "RUNNING"
	NodeRetryPending NodeStatus = "RETRY_PENDING"
	NodeSucceeded    NodeStatus = "SUCCEEDED"
	NodeFailed       NodeStatus = "FAILED"
	NodeSkipped      NodeStatus = "SKIPPED"
	NodeCancelled    NodeStatus = "CANCELLED"
	NodeTimedOut     NodeStatus = "TIMED_OUT"
)

const (
	FailureCategoryValidation FailureCategory = "VALIDATION"
	FailureCategoryExecution  FailureCategory = "EXECUTION"
	FailureCategoryInternal   FailureCategory = "INTERNAL"
	FailureCategoryCanceled   FailureCategory = "CANCELED"
	FailureCategoryTimeout    FailureCategory = "TIMEOUT"

	FailureCodeExecutionFailed      FailureCode = "NODE_EXECUTION_FAILED"
	FailureCodeHandlerNotFound      FailureCode = "NODE_HANDLER_NOT_FOUND"
	FailureCodeHandlerPanicked      FailureCode = "NODE_HANDLER_PANICKED"
	FailureCodeExecutionCancelled   FailureCode = "NODE_EXECUTION_CANCELLED"
	FailureCodeExecutionTimeout     FailureCode = "NODE_EXECUTION_TIMEOUT"
	FailureCodeRetryExhausted       FailureCode = "NODE_RETRY_EXHAUSTED"
	FailureCodeNotRetryable         FailureCode = "NODE_FAILURE_NOT_RETRYABLE"
	FailureCodeExecutionInterrupted FailureCode = "NODE_EXECUTION_INTERRUPTED"
	FailureCodeJobNotDurable        FailureCode = "NODE_JOB_NOT_DURABLE"

	RetryKindRetry      RetryDecisionKind = "RETRY"
	RetryKindExhausted  RetryDecisionKind = "EXHAUSTED"
	RetryKindDoNotRetry RetryDecisionKind = "DO_NOT_RETRY"

	RetryReasonScheduled    RetryDecisionReason = "RETRY_SCHEDULED"
	RetryReasonMaxAttempts  RetryDecisionReason = "MAX_ATTEMPTS_REACHED"
	RetryReasonNotRetryable RetryDecisionReason = "FAILURE_NOT_RETRYABLE"
	RetryReasonCategory     RetryDecisionReason = "CATEGORY_NOT_RETRYABLE"
	RetryReasonDeadline     RetryDecisionReason = "DEADLINE_WOULD_BE_EXCEEDED"

	ObservationNodes  ObservationResource = "nodes"
	ObservationEvents ObservationResource = "events"
	ObservationLogs   ObservationResource = "logs"
	ObservationErrors ObservationResource = "errors"
)

type Execution struct {
	ID               string          `json:"executionId"`
	CompanyID        string          `json:"-"`
	WorkflowID       string          `json:"workflowId"`
	WorkflowRevision uint64          `json:"workflowRevision"`
	SnapshotID       string          `json:"snapshotId,omitempty"`
	Mode             string          `json:"mode"`
	Origin           ExecutionOrigin `json:"origin,omitempty"`
	CorrelationID    string          `json:"correlationId"`
	Status           ExecutionStatus `json:"status"`
	CreatedAt        time.Time       `json:"createdAt"`
	ValidatingAt     *time.Time      `json:"validatingAt,omitempty"`
	QueuedAt         *time.Time      `json:"queuedAt,omitempty"`
	StartedAt        *time.Time      `json:"startedAt,omitempty"`
	FinishedAt       *time.Time      `json:"finishedAt,omitempty"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	TerminalOutputs  map[string]any  `json:"terminalOutputs,omitempty"`
	Failure          map[string]any  `json:"failureSummary,omitempty"`
	IsStalled        bool            `json:"isStalled"`
}

type NodeExecution struct {
	ID            string         `json:"nodeExecutionId"`
	ExecutionID   string         `json:"workflowExecutionId"`
	CompanyID     string         `json:"-"`
	NodeID        string         `json:"nodeId"`
	Type          string         `json:"pluginType"`
	Version       string         `json:"pluginVersion"`
	Configuration map[string]any `json:"configuration,omitempty"`
	Status        NodeStatus     `json:"status"`
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
	ExecutionOriginManualDirect ExecutionOrigin = "MANUAL_DIRECT"
	ExecutionOriginHTTPWebhook  ExecutionOrigin = "HTTP_WEBHOOK"
	ExecutionOriginCron         ExecutionOrigin = "CRON"
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
	ID              string          `json:"errorId"`
	ExecutionID     string          `json:"workflowExecutionId"`
	NodeExecutionID string          `json:"nodeExecutionId,omitempty"`
	RelatedEventID  string          `json:"relatedEventId,omitempty"`
	Category        FailureCategory `json:"category"`
	Code            FailureCode     `json:"code"`
	Message         string          `json:"safeMessage"`
	Retryable       bool            `json:"retryable"`
	Details         map[string]any  `json:"details"`
	CreatedAt       time.Time       `json:"createdAt"`
}

type Page[T any] struct {
	Items   []T    `json:"items"`
	Next    string `json:"next"`
	HasNext bool   `json:"hasNext"`
}
