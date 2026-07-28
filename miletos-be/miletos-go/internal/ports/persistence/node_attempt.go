package repository

import (
	"time"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type NodeExecutionAttemptRecordParams struct {
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	NodeExecutionID     execution.NodeExecutionID
	Attempt             execution.AttemptNumber
	Status              execution.NodeExecutionStatus
	StartedAt           time.Time
	FinishedAt          time.Time
	OutputSummary       []byte
	FailureSummary      []byte
	RetryDecision       execution.RetryDecision
	NextAttemptAt       time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type NodeExecutionAttemptRecord struct {
	params NodeExecutionAttemptRecordParams

	outputSummary     JSONObject
	hasOutputSummary  bool
	failureSummary    JSONObject
	hasFailureSummary bool
	retryDecision     execution.RetryDecision
	hasRetryDecision  bool
}

func NewNodeExecutionAttemptRecord(
	params NodeExecutionAttemptRecordParams,
) (NodeExecutionAttemptRecord, error) {
	companyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return NodeExecutionAttemptRecord{},
			newValidationError("companyID", err.Error())
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(
		params.WorkflowExecutionID.String(),
	)
	if err != nil {
		return NodeExecutionAttemptRecord{},
			newValidationError("workflowExecutionID", err.Error())
	}
	nodeExecutionID, err := execution.NewNodeExecutionID(
		params.NodeExecutionID.String(),
	)
	if err != nil {
		return NodeExecutionAttemptRecord{},
			newValidationError("nodeExecutionID", err.Error())
	}
	attempt, err := execution.NewAttemptNumber(params.Attempt.Int16())
	if err != nil {
		return NodeExecutionAttemptRecord{},
			newValidationError("attempt", err.Error())
	}
	if !isNodeExecutionAttemptStatus(params.Status) {
		return NodeExecutionAttemptRecord{}, newValidationError(
			"status", "must contain a supported node execution attempt status")
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	startedAt, err := normalizeRequiredRecordTime("startedAt", params.StartedAt)
	if err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	if startedAt.Before(createdAt) {
		return NodeExecutionAttemptRecord{},
			newValidationError("startedAt", "must not be before createdAt")
	}
	finishedAt, err := normalizeOptionalRecordTime(
		"finishedAt", params.FinishedAt, createdAt,
	)
	if err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	if params.Status == execution.NodeExecutionStatusRunning {
		if !finishedAt.IsZero() {
			return NodeExecutionAttemptRecord{},
				newValidationError("finishedAt", "must be absent for RUNNING status")
		}
	} else if finishedAt.IsZero() {
		return NodeExecutionAttemptRecord{},
			newValidationError("finishedAt", "must be provided for a terminal attempt status")
	}
	if !finishedAt.IsZero() && finishedAt.Before(startedAt) {
		return NodeExecutionAttemptRecord{},
			newValidationError("finishedAt", "must not be before startedAt")
	}
	updatedAt, err := normalizeRequiredRecordTime("updatedAt", params.UpdatedAt)
	if err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	if err := validateUpdatedRecordTime(
		updatedAt, createdAt, startedAt, finishedAt,
	); err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	outputSummary, hasOutputSummary, err := normalizeOptionalJSONObject(
		"outputSummary", params.OutputSummary,
	)
	if err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	failureSummary, hasFailureSummary, err := normalizeOptionalJSONObject(
		"failureSummary", params.FailureSummary,
	)
	if err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	if params.Status != execution.NodeExecutionStatusSucceeded && hasOutputSummary {
		return NodeExecutionAttemptRecord{}, newValidationError(
			"outputSummary", "must be absent unless status is SUCCEEDED")
	}
	switch params.Status {
	case execution.NodeExecutionStatusSucceeded:
		if hasFailureSummary {
			return NodeExecutionAttemptRecord{}, newValidationError(
				"failureSummary", "must be absent for SUCCEEDED status")
		}
	case execution.NodeExecutionStatusFailed, execution.NodeExecutionStatusTimedOut:
		if !hasFailureSummary {
			return NodeExecutionAttemptRecord{}, newValidationError(
				"failureSummary", "must be provided for the attempt status")
		}
	}
	retryDecision, hasRetryDecision, err := normalizeOptionalRetryDecision(
		params.RetryDecision,
	)
	if err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	nextAttemptAt, err := normalizeOptionalRecordTime(
		"nextAttemptAt", params.NextAttemptAt, createdAt,
	)
	if err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	if err := validateNodeExecutionAttemptRetryDecision(
		params.Status, retryDecision, hasRetryDecision,
	); err != nil {
		return NodeExecutionAttemptRecord{}, err
	}
	if hasRetryDecision {
		if retryDecision.CurrentAttempt() != attempt {
			return NodeExecutionAttemptRecord{}, newValidationError(
				"retryDecision", "current attempt must match the record attempt")
		}
		if retryDecision.Kind() == execution.RetryDecisionRetry {
			if nextAttemptAt.IsZero() {
				return NodeExecutionAttemptRecord{}, newValidationError(
					"nextAttemptAt", "must be provided for a retry decision")
			}
			if finishedAt.IsZero() || !nextAttemptAt.After(finishedAt) {
				return NodeExecutionAttemptRecord{}, newValidationError(
					"nextAttemptAt", "must be after finishedAt for a retry decision")
			}
		} else if !nextAttemptAt.IsZero() {
			return NodeExecutionAttemptRecord{}, newValidationError(
				"nextAttemptAt", "must be absent when retry is not scheduled")
		}
	} else if !nextAttemptAt.IsZero() {
		return NodeExecutionAttemptRecord{}, newValidationError(
			"nextAttemptAt", "must be absent without a retry decision")
	}
	normalizedParams := NodeExecutionAttemptRecordParams{
		CompanyID:           companyID,
		WorkflowExecutionID: workflowExecutionID,
		NodeExecutionID:     nodeExecutionID,
		Attempt:             attempt,
		Status:              params.Status,
		StartedAt:           startedAt,
		FinishedAt:          finishedAt,
		OutputSummary:       optionalJSONObjectBytes(outputSummary, hasOutputSummary),
		FailureSummary:      optionalJSONObjectBytes(failureSummary, hasFailureSummary),
		RetryDecision:       retryDecision,
		NextAttemptAt:       nextAttemptAt,
		CreatedAt:           createdAt,
		UpdatedAt:           updatedAt,
	}
	return NodeExecutionAttemptRecord{
		params:            normalizedParams,
		outputSummary:     outputSummary,
		hasOutputSummary:  hasOutputSummary,
		failureSummary:    failureSummary,
		hasFailureSummary: hasFailureSummary,
		retryDecision:     retryDecision,
		hasRetryDecision:  hasRetryDecision,
	}, nil
}

func validateNodeExecutionAttemptRetryDecision(
	status execution.NodeExecutionStatus,
	decision execution.RetryDecision,
	hasDecision bool,
) error {
	if !hasDecision {
		return nil
	}
	switch status {
	case execution.NodeExecutionStatusRunning,
		execution.NodeExecutionStatusSucceeded:
		return newValidationError(
			"retryDecision", "must be absent for RUNNING and SUCCEEDED statuses")
	case execution.NodeExecutionStatusFailed,
		execution.NodeExecutionStatusTimedOut:
		return nil
	case execution.NodeExecutionStatusCancelled:
		if decision.Kind() != execution.RetryDecisionDoNotRetry ||
			decision.Reason() != execution.RetryReasonCategoryNotRetryable {
			return newValidationError(
				"retryDecision",
				"must be CATEGORY_NOT_RETRYABLE when provided for CANCELLED status",
			)
		}
		return nil
	default:
		return newValidationError(
			"retryDecision", "is not supported for the attempt status")
	}
}

func isNodeExecutionAttemptStatus(status execution.NodeExecutionStatus) bool {
	switch status {
	case execution.NodeExecutionStatusRunning,
		execution.NodeExecutionStatusSucceeded,
		execution.NodeExecutionStatusFailed,
		execution.NodeExecutionStatusCancelled,
		execution.NodeExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}

func normalizeOptionalRetryDecision(
	value execution.RetryDecision,
) (execution.RetryDecision, bool, error) {
	if value.Kind() == "" && value.Reason() == "" &&
		value.CurrentAttempt() == 0 {
		return execution.RetryDecision{}, false, nil
	}
	if !value.IsValid() {
		return execution.RetryDecision{}, false,
			newValidationError("retryDecision", "must be valid when provided")
	}
	nextAttempt, _ := value.NextAttempt()
	backoff, _ := value.Backoff()
	normalized, err := execution.NewRetryDecision(
		value.Kind(), value.Reason(), value.CurrentAttempt(), nextAttempt, backoff,
	)
	if err != nil {
		return execution.RetryDecision{}, false,
			newValidationError("retryDecision", err.Error())
	}
	return normalized, true, nil
}

func (record NodeExecutionAttemptRecord) CompanyID() workflow.CompanyID {
	return record.params.CompanyID
}

func (record NodeExecutionAttemptRecord) WorkflowExecutionID() execution.WorkflowExecutionID {
	return record.params.WorkflowExecutionID
}

func (record NodeExecutionAttemptRecord) NodeExecutionID() execution.NodeExecutionID {
	return record.params.NodeExecutionID
}

func (record NodeExecutionAttemptRecord) Attempt() execution.AttemptNumber {
	return record.params.Attempt
}

func (record NodeExecutionAttemptRecord) Status() execution.NodeExecutionStatus {
	return record.params.Status
}

func (record NodeExecutionAttemptRecord) StartedAt() time.Time {
	return record.params.StartedAt
}

func (record NodeExecutionAttemptRecord) FinishedAt() (time.Time, bool) {
	return optionalRecordTime(record.params.FinishedAt)
}

func (record NodeExecutionAttemptRecord) OutputSummary() (JSONObject, bool) {
	if !record.hasOutputSummary {
		return JSONObject{}, false
	}
	return record.outputSummary, true
}

func (record NodeExecutionAttemptRecord) FailureSummary() (JSONObject, bool) {
	if !record.hasFailureSummary {
		return JSONObject{}, false
	}
	return record.failureSummary, true
}

func (record NodeExecutionAttemptRecord) RetryDecision() (execution.RetryDecision, bool) {
	if !record.hasRetryDecision {
		return execution.RetryDecision{}, false
	}
	return record.retryDecision, true
}

func (record NodeExecutionAttemptRecord) NextAttemptAt() (time.Time, bool) {
	return optionalRecordTime(record.params.NextAttemptAt)
}

func (record NodeExecutionAttemptRecord) CreatedAt() time.Time {
	return record.params.CreatedAt
}

func (record NodeExecutionAttemptRecord) UpdatedAt() time.Time {
	return record.params.UpdatedAt
}

func (record NodeExecutionAttemptRecord) IsValid() bool {
	_, err := NewNodeExecutionAttemptRecord(record.params)
	return err == nil
}
