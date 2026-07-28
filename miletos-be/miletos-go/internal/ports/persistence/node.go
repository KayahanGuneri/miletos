package repository

import (
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"time"
)

type NodeExecutionRecordParams struct {
	ID                  execution.NodeExecutionID
	WorkflowExecutionID execution.WorkflowExecutionID
	CompanyID           workflow.CompanyID
	NodeID              workflow.NodeID
	PluginType          workflow.PluginType
	PluginVersion       workflow.PluginVersion
	Status              execution.NodeExecutionStatus
	Attempt             int16
	RetryPolicy         execution.RetryPolicy
	NextAttemptAt       time.Time
	CreatedAt           time.Time
	ReadyAt             time.Time
	QueuedAt            time.Time
	StartedAt           time.Time
	FinishedAt          time.Time
	UpdatedAt           time.Time
	InputSummary        []byte
	OutputSummary       []byte
	FailureSummary      []byte
	LockVersion         int64
}
type NodeExecutionRecord struct {
	params NodeExecutionRecordParams

	inputSummary      JSONObject
	hasInputSummary   bool
	outputSummary     JSONObject
	hasOutputSummary  bool
	failureSummary    JSONObject
	hasFailureSummary bool
	retryPolicy       execution.RetryPolicy
	hasRetryPolicy    bool
}

func NewNodeExecutionRecord(params NodeExecutionRecordParams) (NodeExecutionRecord, error) {
	normalizedID, err := execution.NewNodeExecutionID(params.ID.String())
	if err != nil {
		return NodeExecutionRecord{}, newValidationError("nodeExecutionID",
			err.Error())
	}
	normalizedWorkflowExecutionID, err := execution.NewWorkflowExecutionID(
		params.WorkflowExecutionID.String())
	if err != nil {
		return NodeExecutionRecord{}, newValidationError("workflowExecutionID", err.Error())
	}
	normalizedCompanyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return NodeExecutionRecord{}, newValidationError("companyID",
			err.Error())
	}
	normalizedNodeID, err := workflow.NewNodeID(params.NodeID.String())
	if err != nil {
		return NodeExecutionRecord{}, newValidationError(
			"nodeID", err.Error())
	}
	normalizedPluginType, err := workflow.NewPluginType(
		params.PluginType.String())
	if err != nil {
		return NodeExecutionRecord{}, newValidationError("pluginType", err.Error())
	}
	normalizedPluginVersion, err := workflow.NewPluginVersion(params.PluginVersion.String())
	if err != nil {
		return NodeExecutionRecord{}, newValidationError("pluginVersion",
			err.Error())
	}
	if !params.Status.IsValid() {
		return NodeExecutionRecord{}, newValidationError(
			"status", "must contain a supported node execution status")
	}
	if params.Attempt <= 0 {
		return NodeExecutionRecord{}, newValidationError("attempt", "must be greater than zero")
	}
	retryPolicy, hasRetryPolicy, err := normalizeOptionalRetryPolicy(params.RetryPolicy)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	readyAt, err := normalizeOptionalRecordTime(
		"readyAt", params.ReadyAt, createdAt,
	)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	queuedAt, err := normalizeOptionalRecordTime(
		"queuedAt", params.QueuedAt, createdAt,
	)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	startedAt, err := normalizeOptionalRecordTime(
		"startedAt", params.StartedAt, createdAt,
	)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	finishedAt, err := normalizeOptionalRecordTime(
		"finishedAt", params.FinishedAt, createdAt,
	)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	updatedAt, err := normalizeRequiredRecordTime(
		"updatedAt", params.UpdatedAt)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	nextAttemptAt, err := normalizeOptionalRecordTime(
		"nextAttemptAt", params.NextAttemptAt, createdAt,
	)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	if err := validateNodeExecutionRecordTimes(params.Status,
		createdAt, readyAt, queuedAt,
		startedAt, finishedAt, updatedAt, nextAttemptAt,
	); err != nil {
		return NodeExecutionRecord{}, err
	}
	inputSummary, hasInputSummary, err := normalizeOptionalJSONObject(
		"inputSummary", params.InputSummary)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	outputSummary, hasOutputSummary, err := normalizeOptionalJSONObject(
		"outputSummary", params.OutputSummary)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	failureSummary, hasFailureSummary, err := normalizeOptionalJSONObject(
		"failureSummary", params.FailureSummary)
	if err != nil {
		return NodeExecutionRecord{}, err
	}
	if params.Status == execution.NodeExecutionStatusRetryPending {
		if !hasRetryPolicy {
			return NodeExecutionRecord{}, newValidationError(
				"retryPolicy", "must be provided for RETRY_PENDING status")
		}
		if params.Attempt >= retryPolicy.MaxAttempts().Int16() {
			return NodeExecutionRecord{}, newValidationError(
				"attempt", "must be lower than retryPolicy max attempts for RETRY_PENDING status")
		}
		if hasOutputSummary {
			return NodeExecutionRecord{}, newValidationError(
				"outputSummary", "must be absent for RETRY_PENDING status")
		}
		if !hasFailureSummary {
			return NodeExecutionRecord{}, newValidationError(
				"failureSummary", "must be provided for RETRY_PENDING status")
		}
	}
	if params.LockVersion < 0 {
		return NodeExecutionRecord{}, newValidationError(
			"lockVersion", "must not be negative")
	}
	return NodeExecutionRecord{params: NodeExecutionRecordParams{ID: normalizedID, WorkflowExecutionID: normalizedWorkflowExecutionID, CompanyID: normalizedCompanyID, NodeID: normalizedNodeID, PluginType: normalizedPluginType, PluginVersion: normalizedPluginVersion, Status: params.Status, Attempt: params.Attempt, RetryPolicy: retryPolicy, NextAttemptAt: nextAttemptAt, CreatedAt: createdAt, ReadyAt: readyAt, QueuedAt: queuedAt, StartedAt: startedAt, FinishedAt: finishedAt, UpdatedAt: updatedAt, InputSummary: optionalJSONObjectBytes(inputSummary,
		hasInputSummary), OutputSummary: optionalJSONObjectBytes(outputSummary, hasOutputSummary), FailureSummary: optionalJSONObjectBytes(failureSummary, hasFailureSummary), LockVersion: params.LockVersion}, inputSummary: inputSummary, hasInputSummary: hasInputSummary, outputSummary: outputSummary, hasOutputSummary: hasOutputSummary, failureSummary: failureSummary, hasFailureSummary: hasFailureSummary,
		retryPolicy: retryPolicy, hasRetryPolicy: hasRetryPolicy,
	}, nil
}
func normalizeOptionalRetryPolicy(
	value execution.RetryPolicy,
) (execution.RetryPolicy, bool, error) {
	if value.MaxAttempts().Int16() == 0 &&
		value.InitialBackoff() == 0 && value.MaxBackoff() == 0 {
		return execution.RetryPolicy{}, false, nil
	}
	if !value.IsValid() {
		return execution.RetryPolicy{}, false,
			newValidationError("retryPolicy", "must be valid when provided")
	}
	normalized, err := execution.NewRetryPolicy(
		value.MaxAttempts(), value.InitialBackoff(), value.MaxBackoff(),
	)
	if err != nil {
		return execution.RetryPolicy{}, false,
			newValidationError("retryPolicy", err.Error())
	}
	return normalized, true, nil
}
func validateNodeExecutionRecordTimes(status execution.NodeExecutionStatus, createdAt time.Time,
	readyAt time.Time, queuedAt time.Time, startedAt time.Time,
	finishedAt time.Time, updatedAt time.Time, nextAttemptAt time.Time) error {
	if status == execution.NodeExecutionStatusReady && readyAt.IsZero() {
		return newValidationError(
			"readyAt", "must be provided for READY status")
	}
	if status == execution.NodeExecutionStatusQueued {
		if readyAt.IsZero() {
			return newValidationError("readyAt",
				"must be provided for QUEUED status")
		}
		if queuedAt.IsZero() {
			return newValidationError(
				"queuedAt", "must be provided for QUEUED status")
		}
	}
	if status == execution.NodeExecutionStatusRunning {
		if readyAt.IsZero() {
			return newValidationError(
				"readyAt", "must be provided for RUNNING status")
		}
		if startedAt.IsZero() {
			return newValidationError("startedAt", "must be provided for RUNNING status")
		}
	}
	if status == execution.NodeExecutionStatusRetryPending {
		if readyAt.IsZero() {
			return newValidationError(
				"readyAt", "must be provided for RETRY_PENDING status")
		}
		if startedAt.IsZero() {
			return newValidationError(
				"startedAt", "must be provided for RETRY_PENDING status")
		}
		if nextAttemptAt.IsZero() {
			return newValidationError(
				"nextAttemptAt", "must be provided for RETRY_PENDING status")
		}
		if !nextAttemptAt.After(updatedAt) {
			return newValidationError(
				"nextAttemptAt", "must be after updatedAt")
		}
	} else if !nextAttemptAt.IsZero() {
		return newValidationError(
			"nextAttemptAt", "must be absent unless status is RETRY_PENDING")
	}
	switch status {
	case execution.NodeExecutionStatusSucceeded:
		if readyAt.IsZero() {
			return newValidationError("readyAt",
				"must be provided for SUCCEEDED status")
		}
		if startedAt.IsZero() {
			return newValidationError(
				"startedAt", "must be provided for SUCCEEDED status")
		}
	case execution.NodeExecutionStatusFailed,
		execution.NodeExecutionStatusTimedOut:
		if readyAt.IsZero() {
			return newValidationError(
				"readyAt", "must be provided for the node status")
		}
	}
	if status.IsTerminal() && finishedAt.IsZero() {
		return newValidationError("finishedAt",
			"must be provided for a terminal status")
	}
	if !status.IsTerminal() && !finishedAt.IsZero() {
		return newValidationError(
			"finishedAt", "must be absent for a non-terminal status")
	}
	if !readyAt.IsZero() &&
		!queuedAt.IsZero() && queuedAt.Before(readyAt) {
		return newValidationError(
			"queuedAt", "must not be before readyAt")
	}
	if !readyAt.IsZero() &&
		!startedAt.IsZero() && startedAt.Before(readyAt) {
		return newValidationError(
			"startedAt", "must not be before readyAt")
	}
	if !queuedAt.IsZero() &&
		!startedAt.IsZero() && startedAt.Before(queuedAt) {
		return newValidationError(
			"startedAt", "must not be before queuedAt")
	}
	if !startedAt.IsZero() &&
		!finishedAt.IsZero() && finishedAt.Before(startedAt) {
		return newValidationError(
			"finishedAt", "must not be before startedAt")
	}
	return validateUpdatedRecordTime(
		updatedAt, createdAt, readyAt,
		queuedAt, startedAt, finishedAt,
	)
}
func (record NodeExecutionRecord) ID() execution.NodeExecutionID { return record.params.ID }
func (record NodeExecutionRecord) WorkflowExecutionID() execution.WorkflowExecutionID {
	return record.params.WorkflowExecutionID
}
func (record NodeExecutionRecord) CompanyID() workflow.CompanyID {
	return record.params.CompanyID
}
func (record NodeExecutionRecord) NodeID() workflow.NodeID { return record.params.NodeID }
func (record NodeExecutionRecord) PluginType() workflow.PluginType {
	return record.params.PluginType
}
func (record NodeExecutionRecord) PluginVersion() workflow.PluginVersion {
	return record.params.PluginVersion
}
func (record NodeExecutionRecord) Status() execution.NodeExecutionStatus { return record.params.Status }
func (record NodeExecutionRecord) Attempt() int16 {
	return record.params.Attempt
}
func (record NodeExecutionRecord) RetryPolicy() (execution.RetryPolicy, bool) {
	if !record.hasRetryPolicy {
		return execution.RetryPolicy{}, false
	}
	return record.retryPolicy, true
}
func (record NodeExecutionRecord) NextAttemptAt() (time.Time, bool) {
	return optionalRecordTime(record.params.NextAttemptAt)
}
func (record NodeExecutionRecord) CreatedAt() time.Time {
	return record.params.CreatedAt
}
func (record NodeExecutionRecord) ReadyAt() (time.Time, bool) {
	return optionalRecordTime(record.params.ReadyAt)
}
func (record NodeExecutionRecord) QueuedAt() (time.Time, bool) {
	return optionalRecordTime(record.params.QueuedAt)
}
func (record NodeExecutionRecord) StartedAt() (time.Time, bool) {
	return optionalRecordTime(record.params.StartedAt)
}
func (record NodeExecutionRecord) FinishedAt() (time.Time, bool) {
	return optionalRecordTime(record.params.FinishedAt)
}
func (record NodeExecutionRecord) UpdatedAt() time.Time {
	return record.params.UpdatedAt
}
func (record NodeExecutionRecord) InputSummary() (JSONObject, bool) {
	if !record.hasInputSummary {
		return JSONObject{}, false
	}
	return record.inputSummary, true
}
func (record NodeExecutionRecord) OutputSummary() (JSONObject, bool) {
	if !record.hasOutputSummary {
		return JSONObject{}, false
	}
	return record.outputSummary, true
}
func (record NodeExecutionRecord) FailureSummary() (JSONObject, bool) {
	if !record.hasFailureSummary {
		return JSONObject{}, false
	}
	return record.failureSummary, true
}
func (record NodeExecutionRecord) LockVersion() int64 {
	return record.params.LockVersion
}
func (record NodeExecutionRecord) IsValid() bool {
	_, err := NewNodeExecutionRecord(record.params)
	return err == nil

}
