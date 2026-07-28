package repository

import (
	"context"
	"math"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"time"
)

type NodeTransitionCommandParams struct {
	CompanyID                   workflow.CompanyID
	WorkflowExecutionID         execution.WorkflowExecutionID
	NodeExecutionID             execution.NodeExecutionID
	ExecutionMode               execution.ExecutionMode
	ExpectedWorkflowStatus      execution.WorkflowExecutionStatus
	ExpectedWorkflowLockVersion int64
	ExpectedNextSequenceNumber  SequenceNumber
	ExpectedNodeStatus          execution.NodeExecutionStatus
	ExpectedNodeLockVersion     int64
	ExpectedNodeAttempt         int16
	NodeExecution               NodeExecutionRecord
	AttemptMutation             NodeAttemptMutation
	HasAttemptMutation          bool
	Timeline                    []TimelineEntry
	Errors                      []ExecutionErrorRecord
}
type NodeTransitionCommand struct {
	params NodeTransitionCommandParams
}

func NewNodeTransitionCommand(
	params NodeTransitionCommandParams) (NodeTransitionCommand, error) {
	companyID, err := workflow.NewCompanyID(
		params.CompanyID.String())
	if err != nil {
		return NodeTransitionCommand{}, newValidationError("companyID",
			err.Error())
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(
		params.WorkflowExecutionID.String())
	if err != nil {
		return NodeTransitionCommand{}, newValidationError("workflowExecutionID",
			err.Error())
	}
	nodeExecutionID, err := execution.NewNodeExecutionID(
		params.NodeExecutionID.String())
	if err != nil {
		return NodeTransitionCommand{}, newValidationError("nodeExecutionID",
			err.Error())
	}
	if !params.ExecutionMode.IsValid() {
		return NodeTransitionCommand{}, newValidationError(
			"executionMode", "must contain a supported execution mode")
	}
	if params.ExpectedWorkflowStatus != execution.WorkflowExecutionStatusRunning {
		return NodeTransitionCommand{}, newValidationError("expectedWorkflowStatus",
			"must be RUNNING for a node transition")
	}
	if params.ExpectedWorkflowLockVersion < 0 {
		return NodeTransitionCommand{},
			newValidationError("expectedWorkflowLockVersion", "must not be negative")
	}
	if params.ExpectedWorkflowLockVersion == math.MaxInt64 {
		return NodeTransitionCommand{},
			newValidationError("expectedWorkflowLockVersion", "must allow lock version increment")
	}
	if !params.ExpectedNextSequenceNumber.IsValid() {
		return NodeTransitionCommand{}, newValidationError(
			"expectedNextSequenceNumber", "must be greater than zero")
	}
	if !params.ExpectedNodeStatus.IsValid() {
		return NodeTransitionCommand{}, newValidationError("expectedNodeStatus",
			"must contain a supported node execution status")
	}
	if params.ExpectedNodeLockVersion < 0 {
		return NodeTransitionCommand{},
			newValidationError("expectedNodeLockVersion", "must not be negative")
	}
	if params.ExpectedNodeLockVersion == math.MaxInt64 {
		return NodeTransitionCommand{},
			newValidationError("expectedNodeLockVersion", "must allow lock version increment")
	}
	if params.ExpectedNodeAttempt <= 0 {
		return NodeTransitionCommand{},
			newValidationError("expectedNodeAttempt", "must be greater than zero")
	}
	if !params.NodeExecution.IsValid() {
		return NodeTransitionCommand{}, newValidationError(
			"nodeExecution", "must be valid")
	}
	if params.NodeExecution.CompanyID().String() !=
		companyID.String() {
		return NodeTransitionCommand{}, newValidationError(
			"nodeExecution", "company ID must match the command")
	}
	if params.NodeExecution.WorkflowExecutionID().String() !=
		workflowExecutionID.String() {
		return NodeTransitionCommand{}, newValidationError(
			"nodeExecution", "workflow execution ID must match the command")
	}
	if params.NodeExecution.ID().String() !=
		nodeExecutionID.String() {
		return NodeTransitionCommand{}, newValidationError(
			"nodeExecution", "node execution ID must match the command")
	}
	targetStatus := params.NodeExecution.Status()
	if !params.ExpectedNodeStatus.CanTransitionTo(targetStatus) {
		return NodeTransitionCommand{}, newValidationError(
			"expectedNodeStatus", "does not allow the requested node transition")
	}
	isRetryResume := params.ExpectedNodeStatus == execution.NodeExecutionStatusRetryPending &&
		targetStatus == execution.NodeExecutionStatusRunning
	if isRetryResume {
		if params.ExpectedNodeAttempt == math.MaxInt16 {
			return NodeTransitionCommand{}, newValidationError(
				"expectedNodeAttempt", "must allow retry attempt increment")
		}
		if params.NodeExecution.Attempt() != params.ExpectedNodeAttempt+1 {
			return NodeTransitionCommand{}, newValidationError(
				"nodeExecution", "attempt must increment by exactly one for retry resume")
		}
	} else if params.NodeExecution.Attempt() != params.ExpectedNodeAttempt {
		return NodeTransitionCommand{}, newValidationError("nodeExecution",
			"attempt must match the expected node attempt")
	}
	expectedUpdatedNodeLockVersion :=
		params.ExpectedNodeLockVersion + 1
	if params.NodeExecution.LockVersion() !=
		expectedUpdatedNodeLockVersion {
		return NodeTransitionCommand{}, newValidationError(
			"nodeExecution", "lock version must increment by one")
	}
	transitionAt, exists :=
		nodeTransitionTimestamp(
			params.ExpectedNodeStatus, params.NodeExecution)
	if !exists {
		return NodeTransitionCommand{}, newValidationError(
			"nodeExecution", "must contain the transition timestamp")
	}
	if !transitionAt.Equal(
		params.NodeExecution.UpdatedAt()) {
		return NodeTransitionCommand{},
			newValidationError("nodeExecution", "updated time must equal the transition time")
	}
	if err := validateNodeTransitionSummaries(params.NodeExecution); err != nil {
		return NodeTransitionCommand{}, err
	}
	if err := validateNodeTransitionAttemptMutationMatrix(
		params.ExecutionMode, params.ExpectedNodeStatus, targetStatus,
		params.AttemptMutation, params.HasAttemptMutation,
	); err != nil {
		return NodeTransitionCommand{}, err
	}
	attemptMutation, err := validateNodeTransitionAttemptMutation(
		companyID, workflowExecutionID, nodeExecutionID,
		params.ExpectedNodeStatus, params.ExpectedNodeAttempt,
		params.NodeExecution, transitionAt,
		params.AttemptMutation, params.HasAttemptMutation,
	)
	if err != nil {
		return NodeTransitionCommand{}, err
	}
	timeline, transitionEventID, err := validateNodeTransitionTimeline(companyID,
		workflowExecutionID, nodeExecutionID, params.ExpectedNodeStatus,
		params.NodeExecution, transitionAt, params.Timeline,
	)
	if err != nil {
		return NodeTransitionCommand{}, err
	}
	if err := validateNodeTimelineSequenceCapacity(
		params.ExpectedNextSequenceNumber, len(timeline)); err != nil {
		return NodeTransitionCommand{}, err
	}
	normalizedErrors, err := validateNodeTransitionErrors(companyID,
		workflowExecutionID, nodeExecutionID, targetStatus,
		transitionAt, transitionEventID, params.Errors,
	)
	if err != nil {
		return NodeTransitionCommand{}, err
	}
	return NodeTransitionCommand{params: NodeTransitionCommandParams{CompanyID: companyID, WorkflowExecutionID: workflowExecutionID, NodeExecutionID: nodeExecutionID, ExecutionMode: params.ExecutionMode, ExpectedWorkflowStatus: params.ExpectedWorkflowStatus, ExpectedWorkflowLockVersion: params.ExpectedWorkflowLockVersion, ExpectedNextSequenceNumber: params.ExpectedNextSequenceNumber, ExpectedNodeStatus: params.ExpectedNodeStatus, ExpectedNodeLockVersion: params.ExpectedNodeLockVersion, ExpectedNodeAttempt: params.ExpectedNodeAttempt, NodeExecution: params.NodeExecution, AttemptMutation: attemptMutation, HasAttemptMutation: params.HasAttemptMutation, Timeline: timeline, Errors: normalizedErrors}}, nil
}
func validateNodeTransitionAttemptMutationMatrix(
	mode execution.ExecutionMode,
	expectedStatus execution.NodeExecutionStatus,
	targetStatus execution.NodeExecutionStatus,
	mutation NodeAttemptMutation,
	hasMutation bool,
) error {
	if mode == execution.ExecutionModeAsync {
		if hasMutation || mutation.Kind() != "" {
			return newValidationError(
				"attemptMutation", "must be absent for ASYNC execution mode")
		}
		return nil
	}
	if mode != execution.ExecutionModeSync {
		return newValidationError(
			"executionMode", "must contain a supported execution mode")
	}

	var requiredKind NodeAttemptMutationKind
	switch {
	case targetStatus == execution.NodeExecutionStatusRunning &&
		(expectedStatus == execution.NodeExecutionStatusReady ||
			expectedStatus == execution.NodeExecutionStatusQueued ||
			expectedStatus == execution.NodeExecutionStatusRetryPending):
		requiredKind = NodeAttemptMutationStart
	case expectedStatus == execution.NodeExecutionStatusRunning &&
		(targetStatus == execution.NodeExecutionStatusRetryPending ||
			targetStatus == execution.NodeExecutionStatusSucceeded ||
			targetStatus == execution.NodeExecutionStatusFailed ||
			targetStatus == execution.NodeExecutionStatusCancelled ||
			targetStatus == execution.NodeExecutionStatusTimedOut):
		requiredKind = NodeAttemptMutationComplete
	}
	if requiredKind == "" {
		if hasMutation || mutation.Kind() != "" {
			return newValidationError(
				"attemptMutation", "must be absent for the node transition")
		}
		return nil
	}
	if !hasMutation {
		return newValidationError(
			"attemptMutation", string(requiredKind)+" must be provided for the node transition")
	}
	if mutation.Kind() != requiredKind {
		return newValidationError(
			"attemptMutation", string(requiredKind)+" is required for the node transition")
	}
	return nil
}
func nodeTransitionTimestamp(
	expectedStatus execution.NodeExecutionStatus,
	record NodeExecutionRecord) (time.Time, bool) {
	switch record.Status() {
	case execution.NodeExecutionStatusReady:
		return record.ReadyAt()
	case execution.NodeExecutionStatusQueued:
		return record.QueuedAt()
	case execution.NodeExecutionStatusRunning:
		if expectedStatus == execution.NodeExecutionStatusRetryPending {
			return record.UpdatedAt(), true
		}
		return record.StartedAt()
	case execution.NodeExecutionStatusRetryPending:
		return record.UpdatedAt(), true
	case execution.NodeExecutionStatusSucceeded, execution.NodeExecutionStatusFailed, execution.NodeExecutionStatusSkipped,
		execution.NodeExecutionStatusCancelled, execution.NodeExecutionStatusTimedOut:
		return record.FinishedAt()
	default:
		return time.Time{}, false
	}
}
func nodeTransitionEventType(status execution.NodeExecutionStatus) (ExecutionEventType, bool) {
	switch status {
	case execution.NodeExecutionStatusReady:
		return ExecutionEventTypeNodeReady,
			true
	case execution.NodeExecutionStatusQueued:
		return ExecutionEventTypeNodeQueued, true
	case execution.NodeExecutionStatusRunning:
		return ExecutionEventTypeNodeStarted, true
	case execution.NodeExecutionStatusRetryPending:
		return ExecutionEventTypeNodeRetryPending, true
	case execution.NodeExecutionStatusSucceeded:
		return ExecutionEventTypeNodeSucceeded,
			true
	case execution.NodeExecutionStatusFailed:
		return ExecutionEventTypeNodeFailed, true
	case execution.NodeExecutionStatusSkipped:
		return ExecutionEventTypeNodeSkipped, true
	case execution.NodeExecutionStatusCancelled:
		return ExecutionEventTypeNodeCancelled,
			true
	case execution.NodeExecutionStatusTimedOut:
		return ExecutionEventTypeNodeTimedOut, true
	default:
		return "", false
	}
}
func validateNodeTransitionSummaries(
	record NodeExecutionRecord) error {
	_, hasOutputSummary :=
		record.OutputSummary()
	_, hasFailureSummary :=
		record.FailureSummary()
	switch record.Status() {
	case execution.NodeExecutionStatusSucceeded:
		if hasFailureSummary {
			return newValidationError(
				"nodeExecution", "failure summary must be absent for SUCCEEDED status")
		}
	case execution.NodeExecutionStatusRetryPending, execution.NodeExecutionStatusFailed,
		execution.NodeExecutionStatusTimedOut:
		if !hasFailureSummary {
			return newValidationError(
				"nodeExecution", "failure summary must be provided for the target status")
		}
		if hasOutputSummary {
			return newValidationError("nodeExecution", "output summary must be absent for the target status")
		}
	case execution.NodeExecutionStatusCancelled:
		if hasOutputSummary {
			return newValidationError(
				"nodeExecution", "output summary must be absent for CANCELLED status")
		}
	case execution.NodeExecutionStatusSkipped:
		if hasOutputSummary || hasFailureSummary {
			return newValidationError(
				"nodeExecution", "output and failure summaries must be absent for SKIPPED status")
		}
	default:
		if hasOutputSummary || hasFailureSummary {
			return newValidationError(
				"nodeExecution", "terminal summaries must be absent for a non-terminal status")
		}
	}
	return nil
}
func validateNodeTimelineSequenceCapacity(expected SequenceNumber, timelineLength int,
) error {
	if timelineLength <= 0 {
		return newValidationError(
			"timeline", "must contain at least one entry")
	}
	increment := int64(timelineLength)
	if expected.Int64() > math.MaxInt64-increment {
		return newValidationError("expectedNextSequenceNumber", "must allow timeline sequence allocation")
	}
	return nil
}
func validateNodeTransitionTimeline(companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID, expectedStatus execution.NodeExecutionStatus, nodeExecution NodeExecutionRecord,
	transitionAt time.Time, values []TimelineEntry) (
	[]TimelineEntry, ExecutionEventID, error,
) {
	if len(values) == 0 {
		return nil, "", newValidationError(
			"timeline", "must contain a node transition event")
	}
	firstEvent, exists :=
		values[0].Event()
	if !exists {
		return nil, "", newValidationError(
			"timeline", "first entry must be the node transition event")
	}
	if firstEvent.CompanyID().String() !=
		companyID.String() {
		return nil, "", newValidationError("timeline",
			"event company ID must match the command")
	}
	if firstEvent.WorkflowExecutionID().String() != workflowExecutionID.String() {
		return nil, "", newValidationError("timeline", "event workflow execution ID must match the command")
	}
	eventNodeExecutionID, exists := firstEvent.NodeExecutionID()
	if !exists ||
		eventNodeExecutionID.String() != nodeExecutionID.String() {
		return nil, "", newValidationError(
			"timeline", "event node execution ID must match the command")
	}
	expectedEventType, exists :=
		nodeTransitionEventType(nodeExecution.Status())
	if !exists {
		return nil, "", newValidationError("timeline",
			"node transition does not have a supported event type")
	}
	if firstEvent.Type() != expectedEventType {
		return nil, "", newValidationError("timeline", "first event type must match the node transition")
	}
	previousStatus, exists := firstEvent.PreviousStatus()
	if !exists ||
		previousStatus != expectedStatus.String() {
		return nil, "", newValidationError("timeline",
			"transition event previous status must match the expected status")
	}
	newStatus, exists := firstEvent.NewStatus()
	if !exists || newStatus != nodeExecution.Status().String() {
		return nil, "", newValidationError("timeline", "transition event new status must match the updated node status")
	}
	if !firstEvent.CreatedAt().Equal(transitionAt) {
		return nil, "", newValidationError("timeline", "transition event time must equal the node transition time")
	}
	logIDs := make(map[string]struct{}, len(values))
	normalized := make(
		[]TimelineEntry, len(values))
	normalized[0] = values[0]
	for index := 1; index < len(values); index++ {
		entry := values[index]
		if !entry.IsValid() {
			return nil, "", newValidationError("timeline",
				"must contain only valid entries")
		}
		logDraft, exists := entry.Log()
		if !exists {
			return nil, "", newValidationError("timeline",
				"only log entries may follow the node transition event")
		}
		if logDraft.CompanyID().String() != companyID.String() {
			return nil, "", newValidationError("timeline", "log company ID must match the command")
		}
		if logDraft.WorkflowExecutionID().String() != workflowExecutionID.String() {
			return nil, "", newValidationError(
				"timeline", "log workflow execution ID must match the command")
		}
		logNodeExecutionID, exists :=
			logDraft.NodeExecutionID()
		if !exists || logNodeExecutionID.String() !=
			nodeExecutionID.String() {
			return nil, "", newValidationError("timeline",
				"log node execution ID must match the command")
		}
		if logDraft.CreatedAt().Before(transitionAt) {
			return nil, "", newValidationError("timeline",
				"log time must not be before the node transition time")
		}
		logID := logDraft.ID().String()
		if _, exists := logIDs[logID]; exists {
			return nil, "", newValidationError("timeline",
				"must not contain duplicate log IDs")
		}
		logIDs[logID] = struct{}{}
		normalized[index] = entry
	}
	return normalized, firstEvent.ID(), nil
}
func nodeStatusRequiresStructuredError(
	status execution.NodeExecutionStatus) bool {
	switch status {
	case execution.NodeExecutionStatusRetryPending, execution.NodeExecutionStatusFailed, execution.NodeExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
func nodeStatusAllowsStructuredError(status execution.NodeExecutionStatus) bool {
	switch status {
	case execution.NodeExecutionStatusRetryPending, execution.NodeExecutionStatusFailed, execution.NodeExecutionStatusCancelled,
		execution.NodeExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
func validateNodeTransitionErrors(
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, nodeExecutionID execution.NodeExecutionID,
	targetStatus execution.NodeExecutionStatus, transitionAt time.Time, transitionEventID ExecutionEventID,
	values []ExecutionErrorRecord) ([]ExecutionErrorRecord, error) {
	requiresErrors :=
		nodeStatusRequiresStructuredError(targetStatus)
	allowsErrors := nodeStatusAllowsStructuredError(
		targetStatus)
	if requiresErrors && len(values) == 0 {
		return nil, newValidationError(
			"errors", "must contain a structured error for the target status")
	}
	if !allowsErrors &&
		len(values) > 0 {
		return nil, newValidationError("errors",
			"must be empty for the target status")
	}
	knownErrorIDs := make(map[string]struct{},
		len(values))
	normalized := make([]ExecutionErrorRecord, len(values))
	for index, executionError := range values {
		if !executionError.IsValid() {
			return nil, newValidationError("errors",
				"must contain only valid structured errors")
		}
		if executionError.CompanyID().String() != companyID.String() {
			return nil, newValidationError("errors", "company ID must match the command")
		}
		if executionError.WorkflowExecutionID().String() != workflowExecutionID.String() {
			return nil, newValidationError(
				"errors", "workflow execution ID must match the command")
		}
		errorNodeExecutionID, exists :=
			executionError.NodeExecutionID()
		if !exists || errorNodeExecutionID.String() !=
			nodeExecutionID.String() {
			return nil, newValidationError("errors",
				"node execution ID must match the command")
		}
		relatedEventID, exists := executionError.RelatedEventID()
		if !exists {
			return nil, newValidationError("errors",
				"must reference the node transition event")
		}
		if relatedEventID.String() != transitionEventID.String() {
			return nil, newValidationError("errors", "related event ID must match the node transition event")
		}
		if executionError.CreatedAt().Before(transitionAt) {
			return nil, newValidationError("errors", "creation time must not be before the node transition")
		}
		errorID := executionError.ID().String()
		if _, exists := knownErrorIDs[errorID]; exists {
			return nil, newValidationError(
				"errors", "must not contain duplicate error IDs")
		}
		knownErrorIDs[errorID] =
			struct{}{}
		normalized[index] =
			executionError
	}
	return normalized, nil
}
func (command NodeTransitionCommand) CompanyID() workflow.CompanyID {
	return command.params.CompanyID
}
func (command NodeTransitionCommand) WorkflowExecutionID() execution.WorkflowExecutionID {
	return command.params.WorkflowExecutionID
}
func (command NodeTransitionCommand) NodeExecutionID() execution.NodeExecutionID {
	return command.params.NodeExecutionID
}
func (command NodeTransitionCommand) ExecutionMode() execution.ExecutionMode {
	return command.params.ExecutionMode
}
func (command NodeTransitionCommand) ExpectedWorkflowStatus() execution.WorkflowExecutionStatus {
	return command.params.ExpectedWorkflowStatus
}
func (command NodeTransitionCommand) ExpectedWorkflowLockVersion() int64 {
	return command.params.ExpectedWorkflowLockVersion
}
func (command NodeTransitionCommand) ExpectedNextSequenceNumber() SequenceNumber {
	return command.params.ExpectedNextSequenceNumber
}
func (command NodeTransitionCommand) ExpectedNodeStatus() execution.NodeExecutionStatus {
	return command.params.ExpectedNodeStatus
}
func (command NodeTransitionCommand) ExpectedNodeLockVersion() int64 {
	return command.params.ExpectedNodeLockVersion
}
func (command NodeTransitionCommand) ExpectedNodeAttempt() int16 {
	return command.params.ExpectedNodeAttempt
}
func (command NodeTransitionCommand) NodeExecution() NodeExecutionRecord {
	return command.params.NodeExecution
}
func (command NodeTransitionCommand) AttemptMutation() (NodeAttemptMutation, bool) {
	if !command.params.HasAttemptMutation {
		return NodeAttemptMutation{}, false
	}
	return command.params.AttemptMutation, true
}
func (command NodeTransitionCommand) Timeline() []TimelineEntry {
	return append([]TimelineEntry(nil), command.params.Timeline...,
	)
}
func (command NodeTransitionCommand) Errors() []ExecutionErrorRecord {
	return append([]ExecutionErrorRecord(nil), command.params.Errors...,
	)
}
func (command NodeTransitionCommand) IsValid() bool {
	_, err := NewNodeTransitionCommand(command.params)
	return err == nil

}

type NodeTransitionStore interface {
	ApplyNodeTransition(
		ctx context.Context, command NodeTransitionCommand) error
}
