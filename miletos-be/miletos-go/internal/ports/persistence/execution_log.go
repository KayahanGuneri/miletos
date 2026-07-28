package repository

import (
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strings"
	"time"
)

type ExecutionLogDraftParams struct {
	ID                  ExecutionLogID
	WorkflowExecutionID execution.WorkflowExecutionID
	CompanyID           workflow.CompanyID
	NodeExecutionID     execution.NodeExecutionID
	Level               ExecutionLogLevel
	Message             string
	Metadata            []byte
	CreatedAt           time.Time
}
type ExecutionLogDraft = ExecutionLogRecord

func NewExecutionLogDraft(params ExecutionLogDraftParams) (ExecutionLogDraft, error) {
	record, err := NewExecutionLogRecord(ExecutionLogRecordParams{ID: params.ID, WorkflowExecutionID: params.WorkflowExecutionID, CompanyID: params.CompanyID, NodeExecutionID: params.NodeExecutionID, SequenceNumber: SequenceNumber(1), Level: params.Level, Message: params.Message, Metadata: params.Metadata, CreatedAt: params.CreatedAt})
	if err != nil {
		return ExecutionLogDraft{}, err
	}
	return record, nil
}
func (draft ExecutionLogRecord) Record(sequenceNumber SequenceNumber) (ExecutionLogRecord, error) {
	if !sequenceNumber.IsValid() {
		return ExecutionLogRecord{}, newValidationError("sequenceNumber", "must be greater than zero")
	}
	return NewExecutionLogRecord(ExecutionLogRecordParams{ID: draft.params.ID, WorkflowExecutionID: draft.params.WorkflowExecutionID, CompanyID: draft.params.CompanyID, NodeExecutionID: draft.params.NodeExecutionID, SequenceNumber: sequenceNumber, Level: draft.params.Level, Message: draft.params.Message, Metadata: draft.metadata.Bytes(), CreatedAt: draft.params.CreatedAt})
}

const maximumExecutionLogMessageCharacters = 4000

type ExecutionLogRecordParams struct {
	ID                  ExecutionLogID
	WorkflowExecutionID execution.WorkflowExecutionID
	CompanyID           workflow.CompanyID
	NodeExecutionID     execution.NodeExecutionID
	SequenceNumber      SequenceNumber
	Level               ExecutionLogLevel
	Message             string
	Metadata            []byte
	CreatedAt           time.Time
}
type ExecutionLogRecord struct {
	params             ExecutionLogRecordParams
	hasNodeExecutionID bool
	metadata           JSONObject
}

func NewExecutionLogRecord(params ExecutionLogRecordParams) (ExecutionLogRecord, error) {
	normalizedID, err := NewExecutionLogID(params.ID.String())
	if err != nil {
		return ExecutionLogRecord{}, err
	}
	normalizedWorkflowExecutionID, err := execution.NewWorkflowExecutionID(params.WorkflowExecutionID.String())
	if err != nil {
		return ExecutionLogRecord{}, newValidationError("workflowExecutionID", err.Error())
	}
	normalizedCompanyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return ExecutionLogRecord{}, newValidationError("companyID", err.Error())
	}
	normalizedNodeExecutionID, hasNodeExecutionID, err := normalizeOptionalNodeExecutionID(params.NodeExecutionID)
	if err != nil {
		return ExecutionLogRecord{}, err
	}
	if !params.SequenceNumber.IsValid() {
		return ExecutionLogRecord{}, newValidationError("sequenceNumber", "must be greater than zero")
	}
	normalizedLevel, err := ParseExecutionLogLevel(params.Level.String())
	if err != nil {
		return ExecutionLogRecord{}, err
	}
	normalizedMessage, err := normalizeRequiredBoundedString("message", params.Message, maximumExecutionLogMessageCharacters)
	if err != nil {
		return ExecutionLogRecord{}, err
	}
	metadata, err := newJSONObject("metadata", params.Metadata)
	if err != nil {
		return ExecutionLogRecord{}, err
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return ExecutionLogRecord{}, err
	}
	return ExecutionLogRecord{params: ExecutionLogRecordParams{ID: normalizedID, WorkflowExecutionID: normalizedWorkflowExecutionID, CompanyID: normalizedCompanyID, NodeExecutionID: normalizedNodeExecutionID, SequenceNumber: params.SequenceNumber, Level: normalizedLevel, Message: normalizedMessage, Metadata: metadata.Bytes(), CreatedAt: createdAt}, hasNodeExecutionID: hasNodeExecutionID, metadata: metadata}, nil
}
func (record ExecutionLogRecord) ID() ExecutionLogID {
	return record.params.ID
}
func (record ExecutionLogRecord) WorkflowExecutionID() execution.WorkflowExecutionID {
	return record.params.WorkflowExecutionID
}
func (record ExecutionLogRecord) CompanyID() workflow.CompanyID {
	return record.params.CompanyID
}
func (record ExecutionLogRecord) NodeExecutionID() (execution.NodeExecutionID, bool) {
	if !record.hasNodeExecutionID {
		return "", false
	}
	return record.params.NodeExecutionID, true
}
func (record ExecutionLogRecord) SequenceNumber() SequenceNumber {
	return record.params.SequenceNumber
}
func (record ExecutionLogRecord) Level() ExecutionLogLevel {
	return record.params.Level
}
func (record ExecutionLogRecord) Message() string {
	return record.params.Message
}
func (record ExecutionLogRecord) Metadata() JSONObject {
	return record.metadata
}
func (record ExecutionLogRecord) CreatedAt() time.Time {
	return record.params.CreatedAt
}
func (record ExecutionLogRecord) IsValid() bool {
	_, err := NewExecutionLogRecord(record.params)
	return err == nil
}

type ExecutionLogLevel string

const (
	ExecutionLogLevelDebug ExecutionLogLevel = "DEBUG"
	ExecutionLogLevelInfo  ExecutionLogLevel = "INFO"
	ExecutionLogLevelWarn  ExecutionLogLevel = "WARN"
	ExecutionLogLevelError ExecutionLogLevel = "ERROR"
)

func ParseExecutionLogLevel(value string) (ExecutionLogLevel, error) {
	normalized := ExecutionLogLevel(strings.ToUpper(strings.TrimSpace(value)))
	if !normalized.IsValid() {
		return "", newValidationError("logLevel", "must be one of DEBUG, INFO, WARN, ERROR")
	}
	return normalized, nil
}
func (level ExecutionLogLevel) String() string {
	return string(level)
}
func (level ExecutionLogLevel) IsValid() bool {
	switch level {
	case ExecutionLogLevelDebug, ExecutionLogLevelInfo, ExecutionLogLevelWarn, ExecutionLogLevelError:
		return true
	default:
		return false
	}
}
