package executionhttp

import (
	"encoding/json"
	"time"
)

type ExecutionDefinitionResponse struct {
	ExecutionID      string          `json:"executionId"`
	SnapshotID       string          `json:"snapshotId"`
	WorkflowID       string          `json:"workflowId"`
	WorkflowRevision uint64          `json:"workflowRevision"`
	WorkflowName     string          `json:"workflowName"`
	Definition       json.RawMessage `json:"definition"`
	CreatedAt        string          `json:"createdAt"`
}

type ExecutionResponse struct {
	ExecutionID      string  `json:"executionId"`
	WorkflowID       string  `json:"workflowId"`
	WorkflowRevision uint64  `json:"workflowRevision"`
	Mode             string  `json:"mode"`
	Status           string  `json:"status"`
	CorrelationID    string  `json:"correlationId"`
	CreatedAt        string  `json:"createdAt"`
	StartedAt        *string `json:"startedAt,omitempty"`
	FinishedAt       *string `json:"finishedAt,omitempty"`
	ScheduledRoots   int     `json:"scheduledRoots,omitempty"`
}

type PartialRecoveryResponse struct {
	SourceExecutionID   string `json:"sourceExecutionId"`
	RecoveryExecutionID string `json:"recoveryExecutionId"`
	Status              string `json:"status"`
	PreservedNodeCount  int    `json:"preservedNodeCount"`
	ScheduledNodeCount  int    `json:"scheduledNodeCount"`
	ResetNodeCount      int    `json:"resetNodeCount"`
	CreatedAt           string `json:"createdAt"`
	Replayed            bool   `json:"replayed"`
}

var executionErrorSafeDetailKeys = map[string]struct{}{
	"issueCount": {}, "structuralIssueCount": {}, "pluginIssueCount": {},
	"engineIssueCount": {}, "failedNodeCount": {}, "skippedNodeCount": {},
	"pendingNodeCount": {}, "readyNodeCount": {}}

type ExecutionErrorResponse struct {
	ErrorID             string          `json:"errorId"`
	WorkflowExecutionID string          `json:"workflowExecutionId"`
	NodeExecutionID     string          `json:"nodeExecutionId,omitempty"`
	RelatedEventID      string          `json:"relatedEventId,omitempty"`
	Category            string          `json:"category"`
	Code                string          `json:"code"`
	SafeMessage         string          `json:"safeMessage"`
	Retryable           bool            `json:"retryable"`
	Details             json.RawMessage `json:"details"`
	CreatedAt           string          `json:"createdAt"`
}

type ExecutionErrorPageResponse struct {
	Items   []ExecutionErrorResponse `json:"items"`
	Next    string                   `json:"next"`
	HasNext bool                     `json:"hasNext"`
}

type ExecutionEventResponse struct {
	EventID             string          `json:"eventId"`
	WorkflowExecutionID string          `json:"workflowExecutionId"`
	NodeExecutionID     string          `json:"nodeExecutionId,omitempty"`
	SequenceNumber      int64           `json:"sequenceNumber"`
	Type                string          `json:"type"`
	PreviousStatus      string          `json:"previousStatus,omitempty"`
	NewStatus           string          `json:"newStatus,omitempty"`
	CorrelationID       string          `json:"correlationId,omitempty"`
	CausationID         string          `json:"causationId,omitempty"`
	SafeMessage         string          `json:"safeMessage,omitempty"`
	Metadata            json.RawMessage `json:"metadata"`
	CreatedAt           string          `json:"createdAt"`
}

type ExecutionEventPageResponse struct {
	Items   []ExecutionEventResponse `json:"items"`
	Next    string                   `json:"next"`
	HasNext bool                     `json:"hasNext"`
}

type ExecutionLogResponse struct {
	LogID               string          `json:"logId"`
	WorkflowExecutionID string          `json:"workflowExecutionId"`
	NodeExecutionID     string          `json:"nodeExecutionId,omitempty"`
	SequenceNumber      int64           `json:"sequenceNumber"`
	Level               string          `json:"level"`
	Message             string          `json:"message"`
	Metadata            json.RawMessage `json:"metadata"`
	CreatedAt           string          `json:"createdAt"`
}

type ExecutionLogPageResponse struct {
	Items   []ExecutionLogResponse `json:"items"`
	Next    string                 `json:"next"`
	HasNext bool                   `json:"hasNext"`
}

type NodeExecutionResponse struct {
	NodeExecutionID     string          `json:"nodeExecutionId"`
	WorkflowExecutionID string          `json:"workflowExecutionId"`
	NodeID              string          `json:"nodeId"`
	PluginType          string          `json:"pluginType"`
	PluginVersion       string          `json:"pluginVersion"`
	Status              string          `json:"status"`
	Attempt             int16           `json:"attempt"`
	CreatedAt           string          `json:"createdAt"`
	ReadyAt             string          `json:"readyAt,omitempty"`
	QueuedAt            string          `json:"queuedAt,omitempty"`
	StartedAt           string          `json:"startedAt,omitempty"`
	FinishedAt          string          `json:"finishedAt,omitempty"`
	UpdatedAt           string          `json:"updatedAt"`
	InputSummary        json.RawMessage `json:"inputSummary,omitempty"`
	OutputSummary       json.RawMessage `json:"outputSummary,omitempty"`
	FailureSummary      json.RawMessage `json:"failureSummary,omitempty"`
}

type NodeExecutionPageResponse struct {
	Items   []NodeExecutionResponse `json:"items"`
	Next    string                  `json:"next"`
	HasNext bool                    `json:"hasNext"`
}

type ExecutionSummaryResponse struct {
	ExecutionID      string  `json:"executionId"`
	WorkflowID       string  `json:"workflowId"`
	WorkflowRevision uint64  `json:"workflowRevision"`
	Mode             string  `json:"mode"`
	Status           string  `json:"status"`
	CorrelationID    string  `json:"correlationId"`
	CreatedAt        string  `json:"createdAt"`
	ValidatingAt     *string `json:"validatingAt,omitempty"`
	QueuedAt         *string `json:"queuedAt,omitempty"`
	StartedAt        *string `json:"startedAt,omitempty"`
	FinishedAt       *string `json:"finishedAt,omitempty"`
	UpdatedAt        string  `json:"updatedAt"`
	IsStalled        bool    `json:"isStalled"`
}

type ExecutionPageResponse struct {
	Items   []ExecutionSummaryResponse `json:"items"`
	Next    string                     `json:"next"`
	HasNext bool                       `json:"hasNext"`
}

const timeRFC3339Nano = time.RFC3339Nano
