package repository

import (
	"context"
	"math"
	"time"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type WorkflowTransitionCommandParams struct {
	CompanyID                  workflow.CompanyID
	WorkflowExecutionID        execution.WorkflowExecutionID
	ExpectedStatus             execution.WorkflowExecutionStatus
	ExpectedLockVersion        int64
	ExpectedNextSequenceNumber SequenceNumber
	WorkflowExecution          WorkflowExecutionRecord
	Timeline                   []TimelineEntry
	Errors                     []ExecutionErrorRecord
}
type WorkflowTransitionCommand struct {
	params WorkflowTransitionCommandParams
}

func NewWorkflowTransitionCommand(params WorkflowTransitionCommandParams,
) (WorkflowTransitionCommand, error) {
	companyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return WorkflowTransitionCommand{},
			newValidationError("companyID", err.Error())
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(params.WorkflowExecutionID.String())
	if err != nil {
		return WorkflowTransitionCommand{},
			newValidationError("workflowExecutionID", err.Error())
	}
	if !params.ExpectedStatus.IsValid() {
		return WorkflowTransitionCommand{}, newValidationError(
			"expectedStatus", "must contain a supported workflow execution status")
	}
	if params.ExpectedLockVersion < 0 {
		return WorkflowTransitionCommand{}, newValidationError("expectedLockVersion",
			"must not be negative")
	}
	if params.ExpectedLockVersion == math.MaxInt64 {
		return WorkflowTransitionCommand{}, newValidationError("expectedLockVersion",
			"must allow lock version increment")
	}
	if !params.ExpectedNextSequenceNumber.IsValid() {
		return WorkflowTransitionCommand{},
			newValidationError("expectedNextSequenceNumber", "must be greater than zero")
	}
	if !params.WorkflowExecution.IsValid() {
		return WorkflowTransitionCommand{}, newValidationError(
			"workflowExecution", "must be valid")
	}
	if params.WorkflowExecution.CompanyID().String() !=
		companyID.String() {
		return WorkflowTransitionCommand{}, newValidationError(
			"workflowExecution", "company ID must match the command")
	}
	if params.WorkflowExecution.ID().String() !=
		workflowExecutionID.String() {
		return WorkflowTransitionCommand{}, newValidationError(
			"workflowExecution", "workflow execution ID must match the command")
	}
	targetStatus :=
		params.WorkflowExecution.Status()
	if !params.ExpectedStatus.CanTransitionTo(
		targetStatus) {
		return WorkflowTransitionCommand{},
			newValidationError("expectedStatus", "does not allow the requested workflow transition")
	}
	expectedUpdatedLockVersion := params.ExpectedLockVersion + 1
	if params.WorkflowExecution.LockVersion() != expectedUpdatedLockVersion {
		return WorkflowTransitionCommand{},
			newValidationError("workflowExecution", "lock version must increment by one")
	}
	transitionAt, exists := workflowTransitionTimestamp(params.WorkflowExecution)
	if !exists {
		return WorkflowTransitionCommand{},
			newValidationError("workflowExecution", "must contain the transition timestamp")
	}
	if !transitionAt.Equal(params.WorkflowExecution.UpdatedAt()) {
		return WorkflowTransitionCommand{}, newValidationError("workflowExecution",
			"updated time must equal the transition time")
	}
	if err := validateWorkflowTransitionFailureState(params.WorkflowExecution); err != nil {
		return WorkflowTransitionCommand{}, err
	}
	timeline, transitionEventID, err := validateWorkflowTransitionTimeline(
		companyID, workflowExecutionID, params.ExpectedStatus,
		params.WorkflowExecution, transitionAt, params.Timeline,
	)
	if err != nil {
		return WorkflowTransitionCommand{}, err
	}
	if err := validateWorkflowTransitionSequence(
		params.ExpectedNextSequenceNumber, params.WorkflowExecution.NextSequenceNumber(), len(timeline),
	); err != nil {
		return WorkflowTransitionCommand{}, err
	}
	normalizedErrors, err := validateWorkflowTransitionErrors(
		companyID, workflowExecutionID, targetStatus,
		transitionAt, transitionEventID, params.Errors,
	)
	if err != nil {
		return WorkflowTransitionCommand{}, err
	}
	return WorkflowTransitionCommand{params: WorkflowTransitionCommandParams{CompanyID: companyID, WorkflowExecutionID: workflowExecutionID, ExpectedStatus: params.ExpectedStatus, ExpectedLockVersion: params.ExpectedLockVersion, ExpectedNextSequenceNumber: params.ExpectedNextSequenceNumber, WorkflowExecution: params.WorkflowExecution, Timeline: timeline, Errors: normalizedErrors}}, nil
}
func validateWorkflowTransitionSequence(
	expected SequenceNumber, updated SequenceNumber, timelineLength int,
) error {
	if timelineLength <= 0 {
		return newValidationError(
			"timeline", "must contain at least one entry")
	}
	increment := int64(timelineLength)
	if expected.Int64() > math.MaxInt64-increment {
		return newValidationError("expectedNextSequenceNumber", "must allow timeline sequence allocation")
	}
	expectedUpdated := SequenceNumber(expected.Int64() + increment)
	if updated != expectedUpdated {
		return newValidationError("workflowExecution", "next sequence number must advance by the timeline entry count")
	}
	return nil
}
func workflowTransitionTimestamp(record WorkflowExecutionRecord) (time.Time, bool) {
	switch record.Status() {
	case execution.WorkflowExecutionStatusValidating:
		return record.ValidatingAt()
	case execution.WorkflowExecutionStatusQueued:
		return record.QueuedAt()
	case execution.WorkflowExecutionStatusRunning:
		return record.StartedAt()
	case execution.WorkflowExecutionStatusRejected, execution.WorkflowExecutionStatusSucceeded,
		execution.WorkflowExecutionStatusFailed, execution.WorkflowExecutionStatusCancelled, execution.WorkflowExecutionStatusTimedOut:
		return record.FinishedAt()
	default:
		return time.Time{}, false
	}
}
func workflowTransitionEventType(status execution.WorkflowExecutionStatus,
) (ExecutionEventType, bool) {
	switch status {
	case execution.WorkflowExecutionStatusValidating:
		return ExecutionEventTypeWorkflowValidating, true
	case execution.WorkflowExecutionStatusRejected:
		return ExecutionEventTypeWorkflowRejected, true
	case execution.WorkflowExecutionStatusQueued:
		return ExecutionEventTypeWorkflowQueued,
			true
	case execution.WorkflowExecutionStatusRunning:
		return ExecutionEventTypeWorkflowStarted, true
	case execution.WorkflowExecutionStatusSucceeded:
		return ExecutionEventTypeWorkflowSucceeded, true
	case execution.WorkflowExecutionStatusFailed:
		return ExecutionEventTypeWorkflowFailed,
			true
	case execution.WorkflowExecutionStatusCancelled:
		return ExecutionEventTypeWorkflowCancelled, true
	case execution.WorkflowExecutionStatusTimedOut:
		return ExecutionEventTypeWorkflowTimedOut, true
	default:
		return "", false
	}
}
func validateWorkflowTransitionTimeline(companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	expectedStatus execution.WorkflowExecutionStatus, workflowExecution WorkflowExecutionRecord, transitionAt time.Time,
	values []TimelineEntry) ([]TimelineEntry,
	ExecutionEventID, error) {
	if len(values) == 0 {
		return nil, "", newValidationError("timeline",
			"must contain a workflow transition event")
	}
	firstEvent, exists := values[0].Event()
	if !exists {
		return nil, "", newValidationError("timeline",
			"first entry must be the workflow transition event")
	}
	if firstEvent.CompanyID().String() != companyID.String() {
		return nil, "", newValidationError("timeline", "event company ID must match the command")
	}
	if firstEvent.WorkflowExecutionID().String() != workflowExecutionID.String() {
		return nil, "", newValidationError(
			"timeline", "event workflow execution ID must match the command")
	}
	if _, exists :=
		firstEvent.NodeExecutionID(); exists {
		return nil, "", newValidationError("timeline",
			"workflow transition event must not reference a node execution")
	}
	expectedEventType, exists := workflowTransitionEventType(
		workflowExecution.Status())
	if !exists {
		return nil, "", newValidationError("timeline", "workflow transition does not have a supported event type")
	}
	if firstEvent.Type() != expectedEventType {
		return nil, "", newValidationError(
			"timeline", "first event type must match the workflow transition")
	}
	previousStatus, exists :=
		firstEvent.PreviousStatus()
	if !exists || previousStatus != expectedStatus.String() {
		return nil, "", newValidationError("timeline", "transition event previous status must match the expected status")
	}
	newStatus, exists := firstEvent.NewStatus()
	if !exists ||
		newStatus != workflowExecution.Status().String() {
		return nil, "", newValidationError(
			"timeline", "transition event new status must match the updated workflow status")
	}
	if !firstEvent.CreatedAt().Equal(
		transitionAt) {
		return nil, "", newValidationError(
			"timeline", "transition event time must equal the workflow transition time")
	}
	logIDs := make(
		map[string]struct{}, len(values))
	normalized := make([]TimelineEntry,
		len(values))
	normalized[0] = values[0]
	for index := 1; index < len(values); index++ {
		entry := values[index]
		if !entry.IsValid() {
			return nil, "", newValidationError("timeline", "must contain only valid entries")
		}
		logDraft, exists := entry.Log()
		if !exists {
			return nil, "", newValidationError("timeline", "only log entries may follow the workflow transition event")
		}
		if logDraft.CompanyID().String() != companyID.String() {
			return nil, "", newValidationError(
				"timeline", "log company ID must match the command")
		}
		if logDraft.WorkflowExecutionID().String() !=
			workflowExecutionID.String() {
			return nil, "", newValidationError("timeline",
				"log workflow execution ID must match the command")
		}
		if _, exists := logDraft.NodeExecutionID(); exists {
			return nil, "", newValidationError("timeline", "workflow transition log must not reference a node execution")
		}
		if logDraft.CreatedAt().Before(transitionAt) {
			return nil, "", newValidationError("timeline", "log time must not be before the workflow transition time")
		}
		logID := logDraft.ID().String()
		if _, exists := logIDs[logID]; exists {
			return nil, "", newValidationError("timeline", "must not contain duplicate log IDs")
		}
		logIDs[logID] = struct{}{}
		normalized[index] = entry
	}
	return normalized, firstEvent.ID(), nil
}
func validateWorkflowTransitionFailureState(record WorkflowExecutionRecord,
) error {
	_, hasFailureSummary := record.FailureSummary()
	requiresFailure := workflowStatusRequiresStructuredError(
		record.Status())
	allowsFailure := workflowStatusAllowsStructuredError(record.Status())
	if requiresFailure &&
		!hasFailureSummary {
		return newValidationError("workflowExecution",
			"failure summary must be provided for the target status")
	}
	if !allowsFailure && hasFailureSummary {
		return newValidationError("workflowExecution", "failure summary must be absent for the target status")
	}
	return nil
}
func workflowStatusRequiresStructuredError(status execution.WorkflowExecutionStatus) bool {
	switch status {
	case execution.WorkflowExecutionStatusRejected, execution.WorkflowExecutionStatusFailed,
		execution.WorkflowExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
func workflowStatusAllowsStructuredError(
	status execution.WorkflowExecutionStatus) bool {
	switch status {
	case execution.WorkflowExecutionStatusRejected, execution.WorkflowExecutionStatusFailed, execution.WorkflowExecutionStatusCancelled,
		execution.WorkflowExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
func validateWorkflowTransitionErrors(
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, targetStatus execution.WorkflowExecutionStatus,
	transitionAt time.Time, transitionEventID ExecutionEventID, values []ExecutionErrorRecord,
) ([]ExecutionErrorRecord, error) {
	requiresErrors := workflowStatusRequiresStructuredError(
		targetStatus)
	allowsErrors := workflowStatusAllowsStructuredError(targetStatus)
	if requiresErrors &&
		len(values) == 0 {
		return nil, newValidationError("errors",
			"must contain a structured error for the target status")
	}
	if !allowsErrors && len(values) > 0 {
		return nil, newValidationError("errors", "must be empty for the target status")
	}
	knownErrorIDs := make(map[string]struct{}, len(values))
	normalized := make(
		[]ExecutionErrorRecord, len(values))
	for index, executionError := range values {
		if !executionError.IsValid() {
			return nil, newValidationError("errors", "must contain only valid structured errors")
		}
		if executionError.CompanyID().String() != companyID.String() {
			return nil, newValidationError(
				"errors", "company ID must match the command")
		}
		if executionError.WorkflowExecutionID().String() !=
			workflowExecutionID.String() {
			return nil, newValidationError("errors",
				"workflow execution ID must match the command")
		}
		if _, exists := executionError.NodeExecutionID(); exists {
			return nil, newValidationError("errors", "workflow transition error must not reference a node execution")
		}
		relatedEventID, exists := executionError.RelatedEventID()
		if !exists {
			return nil, newValidationError("errors", "must reference the workflow transition event")
		}
		if relatedEventID.String() != transitionEventID.String() {
			return nil, newValidationError(
				"errors", "related event ID must match the workflow transition event")
		}
		if executionError.CreatedAt().Before(
			transitionAt) {
			return nil, newValidationError(
				"errors", "creation time must not be before the workflow transition")
		}
		errorID :=
			executionError.ID().String()
		if _, exists :=
			knownErrorIDs[errorID]; exists {
			return nil, newValidationError("errors",
				"must not contain duplicate error IDs")
		}
		knownErrorIDs[errorID] = struct{}{}
		normalized[index] = executionError
	}
	return normalized, nil
}
func (
	command WorkflowTransitionCommand) CompanyID() workflow.CompanyID {
	return command.params.CompanyID
}
func (
	command WorkflowTransitionCommand) WorkflowExecutionID() execution.WorkflowExecutionID {
	return command.params.WorkflowExecutionID
}
func (
	command WorkflowTransitionCommand) ExpectedStatus() execution.WorkflowExecutionStatus {
	return command.params.ExpectedStatus
}
func (
	command WorkflowTransitionCommand) ExpectedLockVersion() int64 {
	return command.params.ExpectedLockVersion
}
func (
	command WorkflowTransitionCommand) ExpectedNextSequenceNumber() SequenceNumber {
	return command.params.ExpectedNextSequenceNumber
}
func (
	command WorkflowTransitionCommand) WorkflowExecution() WorkflowExecutionRecord {
	return command.params.WorkflowExecution
}
func (
	command WorkflowTransitionCommand) Timeline() []TimelineEntry {
	return append(
		[]TimelineEntry(nil), command.params.Timeline...)
}
func (
	command WorkflowTransitionCommand) Errors() []ExecutionErrorRecord {
	return append(
		[]ExecutionErrorRecord(nil), command.params.Errors...)
}
func (
	command WorkflowTransitionCommand) IsValid() bool {
	_, err := NewWorkflowTransitionCommand(command.params)
	return err == nil

}

type WorkflowTransitionStore interface {
	ApplyWorkflowTransition(ctx context.Context,
		command WorkflowTransitionCommand) error
}
