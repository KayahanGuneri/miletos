package repository

import (
	"math"
	"time"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type WorkflowExecutionRecordParams struct {
	ID                 execution.WorkflowExecutionID
	CompanyID          workflow.CompanyID
	WorkflowID         workflow.WorkflowID
	WorkflowRevision   uint64
	SnapshotID         DefinitionSnapshotID
	Mode               execution.ExecutionMode
	CorrelationID      string
	Status             execution.WorkflowExecutionStatus
	CreatedAt          time.Time
	ValidatingAt       time.Time
	QueuedAt           time.Time
	StartedAt          time.Time
	FinishedAt         time.Time
	UpdatedAt          time.Time
	TerminalOutputs    []byte
	FailureSummary     []byte
	IsStalled          bool
	NextSequenceNumber SequenceNumber
	LockVersion        int64
}
type WorkflowExecutionRecord struct {
	params WorkflowExecutionRecordParams

	terminalOutputs   JSONObject
	failureSummary    JSONObject
	hasFailureSummary bool
}

func NewWorkflowExecutionRecord(params WorkflowExecutionRecordParams,
) (WorkflowExecutionRecord, error) {
	normalizedID, err := execution.NewWorkflowExecutionID(params.ID.String())
	if err != nil {
		return WorkflowExecutionRecord{}, newValidationError(
			"workflowExecutionID", err.Error())
	}
	normalizedCompanyID, err := workflow.NewCompanyID(
		params.CompanyID.String())
	if err != nil {
		return WorkflowExecutionRecord{}, newValidationError("companyID", err.Error())
	}
	normalizedWorkflowID, err := workflow.NewWorkflowID(params.WorkflowID.String())
	if err != nil {
		return WorkflowExecutionRecord{}, newValidationError("workflowID",
			err.Error())
	}
	if params.WorkflowRevision == 0 {
		return WorkflowExecutionRecord{}, newValidationError(
			"workflowRevision", "must be greater than zero")
	}
	if params.WorkflowRevision > uint64(math.MaxInt64) {
		return WorkflowExecutionRecord{}, newValidationError("workflowRevision", "must fit in a PostgreSQL BIGINT")
	}
	normalizedSnapshotID, err := NewDefinitionSnapshotID(params.SnapshotID.String())
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	if !params.Mode.IsValid() {
		return WorkflowExecutionRecord{}, newValidationError(
			"mode", "must contain a supported execution mode")
	}
	normalizedCorrelationID, err := normalizeOptionalString(
		"correlationID", params.CorrelationID)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	if !params.Status.IsValid() {
		return WorkflowExecutionRecord{}, newValidationError(
			"status", "must contain a supported workflow execution status")
	}
	createdAt, err := normalizeRequiredRecordTime(
		"createdAt", params.CreatedAt)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	validatingAt, err := normalizeOptionalRecordTime("validatingAt",
		params.ValidatingAt, createdAt)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	queuedAt, err := normalizeOptionalRecordTime("queuedAt",
		params.QueuedAt, createdAt)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	startedAt, err := normalizeOptionalRecordTime("startedAt",
		params.StartedAt, createdAt)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	finishedAt, err := normalizeOptionalRecordTime("finishedAt",
		params.FinishedAt, createdAt)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	updatedAt, err := normalizeRequiredRecordTime("updatedAt",
		params.UpdatedAt)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	if err := validateWorkflowExecutionRecordTimes(params.Status, createdAt,
		validatingAt, queuedAt, startedAt,
		finishedAt, updatedAt); err != nil {
		return WorkflowExecutionRecord{}, err
	}
	terminalOutputs, err := newJSONObject("terminalOutputs", params.TerminalOutputs)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	failureSummary, hasFailureSummary, err :=
		normalizeOptionalJSONObject("failureSummary", params.FailureSummary)
	if err != nil {
		return WorkflowExecutionRecord{}, err
	}
	if !params.NextSequenceNumber.IsValid() {
		return WorkflowExecutionRecord{}, newValidationError("nextSequenceNumber", "must be greater than zero")
	}
	if params.LockVersion < 0 {
		return WorkflowExecutionRecord{}, newValidationError("lockVersion",
			"must not be negative")
	}
	return WorkflowExecutionRecord{params: WorkflowExecutionRecordParams{ID: normalizedID, CompanyID: normalizedCompanyID, WorkflowID: normalizedWorkflowID, WorkflowRevision: params.WorkflowRevision, SnapshotID: normalizedSnapshotID, Mode: params.Mode, CorrelationID: normalizedCorrelationID, Status: params.Status, CreatedAt: createdAt, ValidatingAt: validatingAt, QueuedAt: queuedAt, StartedAt: startedAt, FinishedAt: finishedAt, UpdatedAt: updatedAt, TerminalOutputs: terminalOutputs.Bytes(), FailureSummary: optionalJSONObjectBytes(failureSummary, hasFailureSummary), IsStalled: params.IsStalled, NextSequenceNumber: params.NextSequenceNumber, LockVersion: params.LockVersion}, terminalOutputs: terminalOutputs, failureSummary: failureSummary, hasFailureSummary: hasFailureSummary}, nil
}
func validateWorkflowExecutionRecordTimes(status execution.WorkflowExecutionStatus,
	createdAt time.Time, validatingAt time.Time, queuedAt time.Time,
	startedAt time.Time, finishedAt time.Time, updatedAt time.Time,
) error {
	if status == execution.WorkflowExecutionStatusValidating && validatingAt.IsZero() {
		return newValidationError("validatingAt", "must be provided for VALIDATING status")
	}
	if status == execution.WorkflowExecutionStatusQueued && queuedAt.IsZero() {
		return newValidationError(
			"queuedAt", "must be provided for QUEUED status")
	}
	if status == execution.WorkflowExecutionStatusRunning &&
		startedAt.IsZero() {
		return newValidationError("startedAt",
			"must be provided for RUNNING status")
	}
	if status.IsTerminal() && finishedAt.IsZero() {
		return newValidationError(
			"finishedAt", "must be provided for a terminal status")
	}
	if !status.IsTerminal() && !finishedAt.IsZero() {
		return newValidationError("finishedAt", "must be absent for a non-terminal status")
	}
	if !startedAt.IsZero() && !finishedAt.IsZero() && finishedAt.Before(startedAt) {
		return newValidationError("finishedAt", "must not be before startedAt")
	}
	return validateUpdatedRecordTime(updatedAt, createdAt,
		validatingAt, queuedAt, startedAt,
		finishedAt)
}
func (record WorkflowExecutionRecord) ID() execution.WorkflowExecutionID {
	return record.params.ID
}
func (record WorkflowExecutionRecord) CompanyID() workflow.CompanyID {
	return record.params.CompanyID
}
func (record WorkflowExecutionRecord) WorkflowID() workflow.WorkflowID {
	return record.params.WorkflowID
}
func (record WorkflowExecutionRecord) WorkflowRevision() uint64 {
	return record.params.WorkflowRevision
}
func (record WorkflowExecutionRecord) SnapshotID() DefinitionSnapshotID {
	return record.params.SnapshotID
}
func (record WorkflowExecutionRecord) Mode() execution.ExecutionMode { return record.params.Mode }
func (record WorkflowExecutionRecord) CorrelationID() (string, bool) {
	if record.params.CorrelationID == "" {
		return "", false
	}
	return record.params.CorrelationID, true
}
func (record WorkflowExecutionRecord) Status() execution.WorkflowExecutionStatus {
	return record.params.Status
}
func (record WorkflowExecutionRecord) CreatedAt() time.Time {
	return record.params.CreatedAt
}
func (record WorkflowExecutionRecord) ValidatingAt() (time.Time, bool) {
	return optionalRecordTime(record.params.ValidatingAt)
}
func (record WorkflowExecutionRecord) QueuedAt() (time.Time, bool) {
	return optionalRecordTime(record.params.QueuedAt)
}
func (record WorkflowExecutionRecord) StartedAt() (time.Time, bool) {
	return optionalRecordTime(record.params.StartedAt)
}
func (record WorkflowExecutionRecord) FinishedAt() (time.Time, bool) {
	return optionalRecordTime(record.params.FinishedAt)
}
func (record WorkflowExecutionRecord) UpdatedAt() time.Time { return record.params.UpdatedAt }
func (record WorkflowExecutionRecord) TerminalOutputs() JSONObject {
	return record.terminalOutputs
}
func (record WorkflowExecutionRecord) FailureSummary() (JSONObject, bool) {
	if !record.hasFailureSummary {
		return JSONObject{}, false
	}
	return record.failureSummary, true
}
func (record WorkflowExecutionRecord) IsStalled() bool { return record.params.IsStalled }
func (record WorkflowExecutionRecord) NextSequenceNumber() SequenceNumber {
	return record.params.NextSequenceNumber
}
func (record WorkflowExecutionRecord) LockVersion() int64 {
	return record.params.LockVersion
}
func (record WorkflowExecutionRecord) IsValid() bool {
	_, err := NewWorkflowExecutionRecord(record.params)
	return err == nil

}
func optionalRecordTime(value time.Time,
) (time.Time, bool) {
	if value.IsZero() {
		return time.Time{}, false
	}
	return value, true
}
