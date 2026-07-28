package repository

import (
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strings"
	"time"
)

type ExecutionEventType string

const (
	ExecutionEventTypeWorkflowCreated    ExecutionEventType = "WORKFLOW_CREATED"
	ExecutionEventTypeWorkflowValidating ExecutionEventType = "WORKFLOW_VALIDATING"
	ExecutionEventTypeWorkflowRejected   ExecutionEventType = "WORKFLOW_REJECTED"
	ExecutionEventTypeWorkflowQueued     ExecutionEventType = "WORKFLOW_QUEUED"
	ExecutionEventTypeWorkflowStarted    ExecutionEventType = "WORKFLOW_STARTED"
	ExecutionEventTypeWorkflowSucceeded  ExecutionEventType = "WORKFLOW_SUCCEEDED"
	ExecutionEventTypeWorkflowFailed     ExecutionEventType = "WORKFLOW_FAILED"
	ExecutionEventTypeWorkflowCancelled  ExecutionEventType = "WORKFLOW_CANCELLED"
	ExecutionEventTypeWorkflowTimedOut   ExecutionEventType = "WORKFLOW_TIMED_OUT"
	ExecutionEventTypeWorkflowStalled    ExecutionEventType = "WORKFLOW_STALLED"
	ExecutionEventTypeNodeCreated        ExecutionEventType = "NODE_CREATED"
	ExecutionEventTypeNodeReady          ExecutionEventType = "NODE_READY"
	ExecutionEventTypeNodeQueued         ExecutionEventType = "NODE_QUEUED"
	ExecutionEventTypeNodeStarted        ExecutionEventType = "NODE_STARTED"
	ExecutionEventTypeNodeRetryPending   ExecutionEventType = "NODE_RETRY_PENDING"
	ExecutionEventTypeNodeSucceeded      ExecutionEventType = "NODE_SUCCEEDED"
	ExecutionEventTypeNodeFailed         ExecutionEventType = "NODE_FAILED"
	ExecutionEventTypeNodeSkipped        ExecutionEventType = "NODE_SKIPPED"
	ExecutionEventTypeNodeCancelled      ExecutionEventType = "NODE_CANCELLED"
	ExecutionEventTypeNodeTimedOut       ExecutionEventType = "NODE_TIMED_OUT"
)

func ParseExecutionEventType(value string) (ExecutionEventType, error) {
	normalized := ExecutionEventType(strings.ToUpper(strings.TrimSpace(value)))
	if !normalized.IsValid() {
		return "", newValidationError("eventType", "must contain a supported execution event type")
	}
	return normalized, nil
}
func (eventType ExecutionEventType) String() string {
	return string(eventType)
}
func (eventType ExecutionEventType) IsValid() bool {
	return eventType.IsWorkflowScoped() || eventType.IsNodeScoped()
}
func (eventType ExecutionEventType) IsWorkflowScoped() bool {
	switch eventType {
	case ExecutionEventTypeWorkflowCreated, ExecutionEventTypeWorkflowValidating, ExecutionEventTypeWorkflowRejected, ExecutionEventTypeWorkflowQueued, ExecutionEventTypeWorkflowStarted, ExecutionEventTypeWorkflowSucceeded, ExecutionEventTypeWorkflowFailed, ExecutionEventTypeWorkflowCancelled, ExecutionEventTypeWorkflowTimedOut, ExecutionEventTypeWorkflowStalled:
		return true
	default:
		return false
	}
}
func (eventType ExecutionEventType) IsNodeScoped() bool {
	switch eventType {
	case ExecutionEventTypeNodeCreated, ExecutionEventTypeNodeReady, ExecutionEventTypeNodeQueued, ExecutionEventTypeNodeStarted, ExecutionEventTypeNodeRetryPending, ExecutionEventTypeNodeSucceeded, ExecutionEventTypeNodeFailed, ExecutionEventTypeNodeSkipped, ExecutionEventTypeNodeCancelled, ExecutionEventTypeNodeTimedOut:
		return true
	default:
		return false
	}
}
func normalizeOptionalExecutionEventID(value ExecutionEventID) (ExecutionEventID, bool, error) {
	if value.String() == "" {
		return "", false, nil
	}
	normalized, err := NewExecutionEventID(value.String())
	if err != nil {
		return "", false, newValidationError("relatedEventID", err.Error())
	}
	return normalized, true, nil
}

type ExecutionEventDraftParams struct {
	ID                  ExecutionEventID
	WorkflowExecutionID execution.WorkflowExecutionID
	CompanyID           workflow.CompanyID
	NodeExecutionID     execution.NodeExecutionID
	Type                ExecutionEventType
	PreviousStatus      string
	NewStatus           string
	CorrelationID       string
	CausationID         string
	SafeMessage         string
	Metadata            []byte
	CreatedAt           time.Time
}
type ExecutionEventDraft = ExecutionEventRecord

func NewExecutionEventDraft(params ExecutionEventDraftParams) (ExecutionEventDraft, error) {
	record, err := NewExecutionEventRecord(ExecutionEventRecordParams{ID: params.ID, WorkflowExecutionID: params.WorkflowExecutionID, CompanyID: params.CompanyID, NodeExecutionID: params.NodeExecutionID, SequenceNumber: SequenceNumber(1), Type: params.Type, PreviousStatus: params.PreviousStatus, NewStatus: params.NewStatus, CorrelationID: params.CorrelationID, CausationID: params.CausationID, SafeMessage: params.SafeMessage, Metadata: params.Metadata, CreatedAt: params.CreatedAt})
	if err != nil {
		return ExecutionEventDraft{}, err
	}
	return record, nil
}
func (draft ExecutionEventRecord) Record(sequenceNumber SequenceNumber) (ExecutionEventRecord, error) {
	if !sequenceNumber.IsValid() {
		return ExecutionEventRecord{}, newValidationError("sequenceNumber", "must be greater than zero")
	}
	return NewExecutionEventRecord(ExecutionEventRecordParams{ID: draft.params.ID, WorkflowExecutionID: draft.params.WorkflowExecutionID, CompanyID: draft.params.CompanyID, NodeExecutionID: draft.params.NodeExecutionID, SequenceNumber: sequenceNumber, Type: draft.params.Type, PreviousStatus: draft.params.PreviousStatus, NewStatus: draft.params.NewStatus, CorrelationID: draft.params.CorrelationID, CausationID: draft.params.CausationID, SafeMessage: draft.params.SafeMessage, Metadata: draft.metadata.Bytes(), CreatedAt: draft.params.CreatedAt})
}

const maximumExecutionEventMessageCharacters = 2000

type ExecutionEventRecordParams struct {
	ID                  ExecutionEventID
	WorkflowExecutionID execution.WorkflowExecutionID
	CompanyID           workflow.CompanyID
	NodeExecutionID     execution.NodeExecutionID
	SequenceNumber      SequenceNumber
	Type                ExecutionEventType
	PreviousStatus      string
	NewStatus           string
	CorrelationID       string
	CausationID         string
	SafeMessage         string
	Metadata            []byte
	CreatedAt           time.Time
}
type ExecutionEventRecord struct {
	params             ExecutionEventRecordParams
	hasNodeExecutionID bool
	metadata           JSONObject
}

func NewExecutionEventRecord(params ExecutionEventRecordParams) (ExecutionEventRecord, error) {
	normalizedID, err := NewExecutionEventID(params.ID.String())
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	normalizedWorkflowExecutionID, err := execution.NewWorkflowExecutionID(params.WorkflowExecutionID.String())
	if err != nil {
		return ExecutionEventRecord{}, newValidationError("workflowExecutionID", err.Error())
	}
	normalizedCompanyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return ExecutionEventRecord{}, newValidationError("companyID", err.Error())
	}
	normalizedNodeExecutionID, hasNodeExecutionID, err := normalizeOptionalNodeExecutionID(params.NodeExecutionID)
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	if !params.SequenceNumber.IsValid() {
		return ExecutionEventRecord{}, newValidationError("sequenceNumber", "must be greater than zero")
	}
	normalizedEventType, err := ParseExecutionEventType(params.Type.String())
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	if err := validateExecutionEventScope(normalizedEventType, hasNodeExecutionID); err != nil {
		return ExecutionEventRecord{}, err
	}
	previousStatus, err := normalizeExecutionEventStatus("previousStatus", params.PreviousStatus, normalizedEventType)
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	newStatus, err := normalizeExecutionEventStatus("newStatus", params.NewStatus, normalizedEventType)
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	correlationID, err := normalizeOptionalString("correlationID", params.CorrelationID)
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	causationID, err := normalizeOptionalString("causationID", params.CausationID)
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	safeMessage, err := normalizeOptionalBoundedString("safeMessage", params.SafeMessage, maximumExecutionEventMessageCharacters)
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	metadata, err := newJSONObject("metadata", params.Metadata)
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return ExecutionEventRecord{}, err
	}
	return ExecutionEventRecord{params: ExecutionEventRecordParams{ID: normalizedID, WorkflowExecutionID: normalizedWorkflowExecutionID, CompanyID: normalizedCompanyID, NodeExecutionID: normalizedNodeExecutionID, SequenceNumber: params.SequenceNumber, Type: normalizedEventType, PreviousStatus: previousStatus, NewStatus: newStatus, CorrelationID: correlationID, CausationID: causationID, SafeMessage: safeMessage, Metadata: metadata.Bytes(), CreatedAt: createdAt}, hasNodeExecutionID: hasNodeExecutionID, metadata: metadata}, nil
}
func validateExecutionEventScope(eventType ExecutionEventType, hasNodeExecutionID bool) error {
	if eventType.IsNodeScoped() && !hasNodeExecutionID {
		return newValidationError("nodeExecutionID", "must be provided for a node-scoped event")
	}
	if eventType.IsWorkflowScoped() && hasNodeExecutionID {
		return newValidationError("nodeExecutionID", "must be absent for a workflow-scoped event")
	}
	return nil
}
func normalizeExecutionEventStatus(field string, value string, eventType ExecutionEventType) (string, error) {
	if value == "" {
		return "", nil
	}
	normalized := strings.ToUpper(strings.TrimSpace(value))
	if normalized == "" {
		return "", newValidationError(field, "must not be blank when provided")
	}
	if eventType.IsWorkflowScoped() {
		status := execution.WorkflowExecutionStatus(normalized)
		if !status.IsValid() {
			return "", newValidationError(field, "must contain a supported workflow execution status")
		}
		return status.String(), nil
	}
	status := execution.NodeExecutionStatus(normalized)
	if !status.IsValid() {
		return "", newValidationError(field, "must contain a supported node execution status")
	}
	return status.String(), nil
}
func (record ExecutionEventRecord) ID() ExecutionEventID {
	return record.params.ID
}
func (record ExecutionEventRecord) WorkflowExecutionID() execution.WorkflowExecutionID {
	return record.params.WorkflowExecutionID
}
func (record ExecutionEventRecord) CompanyID() workflow.CompanyID {
	return record.params.CompanyID
}
func (record ExecutionEventRecord) NodeExecutionID() (execution.NodeExecutionID, bool) {
	if !record.hasNodeExecutionID {
		return "", false
	}
	return record.params.NodeExecutionID, true
}
func (record ExecutionEventRecord) SequenceNumber() SequenceNumber {
	return record.params.SequenceNumber
}
func (record ExecutionEventRecord) Type() ExecutionEventType {
	return record.params.Type
}
func (record ExecutionEventRecord) PreviousStatus() (string, bool) {
	if record.params.PreviousStatus == "" {
		return "", false
	}
	return record.params.PreviousStatus, true
}
func (record ExecutionEventRecord) NewStatus() (string, bool) {
	if record.params.NewStatus == "" {
		return "", false
	}
	return record.params.NewStatus, true
}
func (record ExecutionEventRecord) CorrelationID() (string, bool) {
	if record.params.CorrelationID == "" {
		return "", false
	}
	return record.params.CorrelationID, true
}
func (record ExecutionEventRecord) CausationID() (string, bool) {
	if record.params.CausationID == "" {
		return "", false
	}
	return record.params.CausationID, true
}
func (record ExecutionEventRecord) SafeMessage() (string, bool) {
	if record.params.SafeMessage == "" {
		return "", false
	}
	return record.params.SafeMessage, true
}
func (record ExecutionEventRecord) Metadata() JSONObject {
	return record.metadata
}
func (record ExecutionEventRecord) CreatedAt() time.Time {
	return record.params.CreatedAt
}
func (record ExecutionEventRecord) IsValid() bool {
	_, err := NewExecutionEventRecord(record.params)
	return err == nil
}
