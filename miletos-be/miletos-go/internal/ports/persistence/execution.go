package repository

import (
	"context"
	"math"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type CreateExecutionCommandParams struct {
	Snapshot          DefinitionSnapshot
	WorkflowExecution WorkflowExecutionRecord
	Timeline          []TimelineEntry
}
type CreateExecutionCommand struct {
	params CreateExecutionCommandParams
}

func NewCreateExecutionCommand(params CreateExecutionCommandParams) (CreateExecutionCommand, error) {
	if !params.Snapshot.IsValid() {
		return CreateExecutionCommand{}, newValidationError("snapshot",
			"must be valid")
	}
	if !params.WorkflowExecution.IsValid() {
		return CreateExecutionCommand{}, newValidationError(
			"workflowExecution", "must be valid")
	}
	if err := validateCreatedWorkflowExecution(
		params.Snapshot, params.WorkflowExecution); err != nil {
		return CreateExecutionCommand{}, err
	}
	timeline, err := validateExecutionCreationTimeline(params.WorkflowExecution, params.Timeline)
	if err != nil {
		return CreateExecutionCommand{}, err
	}
	return CreateExecutionCommand{params: CreateExecutionCommandParams{Snapshot: params.Snapshot, WorkflowExecution: params.WorkflowExecution, Timeline: timeline}}, nil
}
func validateCreatedWorkflowExecution(snapshot DefinitionSnapshot, workflowExecution WorkflowExecutionRecord,
) error {
	if workflowExecution.Status() != execution.WorkflowExecutionStatusCreated {
		return newValidationError("workflowExecution", "must have CREATED status")
	}
	if workflowExecution.LockVersion() != 0 {
		return newValidationError("workflowExecution",
			"must have zero lock version")
	}
	if workflowExecution.NextSequenceNumber() != SequenceNumber(1) {
		return newValidationError("workflowExecution", "must start with sequence number one")
	}
	if snapshot.CompanyID().String() != workflowExecution.CompanyID().String() {
		return newValidationError(
			"snapshot", "company ID must match workflow execution")
	}
	if snapshot.WorkflowID().String() !=
		workflowExecution.WorkflowID().String() {
		return newValidationError("snapshot",
			"workflow ID must match workflow execution")
	}
	if snapshot.WorkflowRevision() != workflowExecution.WorkflowRevision() {
		return newValidationError("snapshot", "workflow revision must match workflow execution")
	}
	if snapshot.ID().String() != workflowExecution.SnapshotID().String() {
		return newValidationError(
			"snapshot", "snapshot ID must match workflow execution")
	}
	if snapshot.CreatedAt().After(
		workflowExecution.CreatedAt()) {
		return newValidationError(
			"snapshot", "must not be created after workflow execution")
	}
	return nil
}
func validateExecutionCreationTimeline(
	workflowExecution WorkflowExecutionRecord, values []TimelineEntry) ([]TimelineEntry, error) {
	if len(values) == 0 {
		return nil, newValidationError("timeline",
			"must contain WORKFLOW_CREATED")
	}
	firstEvent, exists := values[0].Event()
	if !exists {
		return nil, newValidationError("timeline", "first entry must be WORKFLOW_CREATED")
	}
	if firstEvent.Type() != ExecutionEventTypeWorkflowCreated {
		return nil, newValidationError(
			"timeline", "first entry must be WORKFLOW_CREATED")
	}
	if firstEvent.CompanyID().String() !=
		workflowExecution.CompanyID().String() {
		return nil, newValidationError("timeline",
			"event company ID must match workflow execution")
	}
	if firstEvent.WorkflowExecutionID().String() != workflowExecution.ID().String() {
		return nil, newValidationError("timeline", "event workflow execution ID must match")
	}
	if _, exists := firstEvent.NodeExecutionID(); exists {
		return nil, newValidationError(
			"timeline", "WORKFLOW_CREATED must not reference a node execution")
	}
	if _, exists :=
		firstEvent.PreviousStatus(); exists {
		return nil, newValidationError("timeline",
			"WORKFLOW_CREATED must not have previous status")
	}
	newStatus, exists := firstEvent.NewStatus()
	if !exists ||
		newStatus != execution.WorkflowExecutionStatusCreated.String() {
		return nil, newValidationError(
			"timeline", "WORKFLOW_CREATED must set CREATED status")
	}
	if !firstEvent.CreatedAt().Equal(
		workflowExecution.CreatedAt()) {
		return nil, newValidationError(
			"timeline", "WORKFLOW_CREATED time must equal workflow creation time")
	}
	knownLogIDs := make(
		map[string]struct{}, len(values))
	normalized := make([]TimelineEntry,
		len(values))
	normalized[0] = values[0]
	for index := 1; index < len(values); index++ {
		entry := values[index]
		if !entry.IsValid() {
			return nil, newValidationError("timeline", "must contain only valid entries")
		}
		logDraft, exists := entry.Log()
		if !exists {
			return nil, newValidationError(
				"timeline", "only workflow logs may follow WORKFLOW_CREATED")
		}
		if logDraft.CompanyID().String() !=
			workflowExecution.CompanyID().String() {
			return nil, newValidationError("timeline",
				"log company ID must match workflow execution")
		}
		if logDraft.WorkflowExecutionID().String() != workflowExecution.ID().String() {
			return nil, newValidationError("timeline", "log workflow execution ID must match")
		}
		if _, exists := logDraft.NodeExecutionID(); exists {
			return nil, newValidationError(
				"timeline", "workflow creation log must not reference a node execution")
		}
		if logDraft.CreatedAt().Before(
			workflowExecution.CreatedAt()) {
			return nil, newValidationError(
				"timeline", "log time must not be before workflow creation")
		}
		logID := logDraft.ID().String()
		if _, exists := knownLogIDs[logID]; exists {
			return nil, newValidationError(
				"timeline", "must not contain duplicate log IDs")
		}
		knownLogIDs[logID] = struct{}{}
		normalized[index] = entry
	}
	return normalized, nil
}
func (command CreateExecutionCommand) Snapshot() DefinitionSnapshot {
	return command.params.Snapshot
}
func (command CreateExecutionCommand) WorkflowExecution() WorkflowExecutionRecord {
	return command.params.WorkflowExecution
}
func (command CreateExecutionCommand) Timeline() []TimelineEntry {
	return append([]TimelineEntry(nil), command.params.Timeline...,
	)
}
func (command CreateExecutionCommand) IsValid() bool {
	_, err := NewCreateExecutionCommand(command.params)
	return err == nil

}

type CreateNodeExecutionsCommandParams struct {
	CompanyID                   workflow.CompanyID
	WorkflowExecutionID         execution.WorkflowExecutionID
	ExpectedWorkflowStatus      execution.WorkflowExecutionStatus
	ExpectedWorkflowLockVersion int64
	ExpectedNextSequenceNumber  SequenceNumber
	NodeExecutions              []NodeExecutionRecord
	Timeline                    []TimelineEntry
}
type CreateNodeExecutionsCommand struct {
	params CreateNodeExecutionsCommandParams
}

func NewCreateNodeExecutionsCommand(
	params CreateNodeExecutionsCommandParams) (CreateNodeExecutionsCommand, error) {
	companyID, err := workflow.NewCompanyID(
		params.CompanyID.String())
	if err != nil {
		return CreateNodeExecutionsCommand{}, newValidationError("companyID",
			err.Error())
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(
		params.WorkflowExecutionID.String())
	if err != nil {
		return CreateNodeExecutionsCommand{}, newValidationError("workflowExecutionID",
			err.Error())
	}
	if params.ExpectedWorkflowStatus != execution.WorkflowExecutionStatusValidating {
		return CreateNodeExecutionsCommand{}, newValidationError("expectedWorkflowStatus",
			"must be VALIDATING")
	}
	if params.ExpectedWorkflowLockVersion < 0 {
		return CreateNodeExecutionsCommand{},
			newValidationError("expectedWorkflowLockVersion", "must not be negative")
	}
	if params.ExpectedWorkflowLockVersion == math.MaxInt64 {
		return CreateNodeExecutionsCommand{},
			newValidationError("expectedWorkflowLockVersion", "must allow lock version increment")
	}
	if !params.ExpectedNextSequenceNumber.IsValid() {
		return CreateNodeExecutionsCommand{}, newValidationError(
			"expectedNextSequenceNumber", "must be greater than zero")
	}
	nodeExecutions, err :=
		validateNodeExecutionCreationRecords(companyID, workflowExecutionID,
			params.NodeExecutions)
	if err != nil {
		return CreateNodeExecutionsCommand{}, err
	}
	timeline, err := validateNodeExecutionCreationTimeline(companyID,
		workflowExecutionID, nodeExecutions, params.Timeline,
	)
	if err != nil {
		return CreateNodeExecutionsCommand{}, err
	}
	if params.ExpectedNextSequenceNumber.Int64() >
		math.MaxInt64-int64(len(timeline)) {
		return CreateNodeExecutionsCommand{}, newValidationError(
			"expectedNextSequenceNumber", "must allow node creation sequence allocation")
	}
	return CreateNodeExecutionsCommand{params: CreateNodeExecutionsCommandParams{CompanyID: companyID, WorkflowExecutionID: workflowExecutionID, ExpectedWorkflowStatus: params.ExpectedWorkflowStatus, ExpectedWorkflowLockVersion: params.ExpectedWorkflowLockVersion, ExpectedNextSequenceNumber: params.ExpectedNextSequenceNumber, NodeExecutions: nodeExecutions, Timeline: timeline}}, nil
}
func validateNodeExecutionCreationRecords(companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID, values []NodeExecutionRecord) ([]NodeExecutionRecord, error) {
	if len(values) == 0 {
		return nil, newValidationError("nodeExecutions",
			"must contain at least one node execution")
	}
	knownExecutionIDs := make(map[string]struct{},
		len(values))
	knownNodeIDs := make(map[string]struct{}, len(values))
	normalized := make(
		[]NodeExecutionRecord, len(values))
	for index, nodeExecution := range values {
		if !nodeExecution.IsValid() {
			return nil, newValidationError("nodeExecutions", "must contain only valid node executions")
		}
		if nodeExecution.CompanyID().String() != companyID.String() {
			return nil, newValidationError(
				"nodeExecutions", "company ID must match the command")
		}
		if nodeExecution.WorkflowExecutionID().String() !=
			workflowExecutionID.String() {
			return nil, newValidationError("nodeExecutions",
				"workflow execution ID must match the command")
		}
		if nodeExecution.Status() != execution.NodeExecutionStatusPending {
			return nil, newValidationError("nodeExecutions", "initial node status must be PENDING")
		}
		if nodeExecution.Attempt() != 1 {
			return nil, newValidationError("nodeExecutions",
				"initial node attempt must be one")
		}
		if nodeExecution.LockVersion() != 0 {
			return nil, newValidationError(
				"nodeExecutions", "initial node lock version must be zero")
		}
		if _, exists :=
			nodeExecution.ReadyAt(); exists {
			return nil, newValidationError("nodeExecutions",
				"initial node must not have ready time")
		}
		if _, exists := nodeExecution.QueuedAt(); exists {
			return nil, newValidationError("nodeExecutions", "initial node must not have queued time")
		}
		if _, exists := nodeExecution.StartedAt(); exists {
			return nil, newValidationError(
				"nodeExecutions", "initial node must not have started time")
		}
		if _, exists :=
			nodeExecution.FinishedAt(); exists {
			return nil, newValidationError("nodeExecutions",
				"initial node must not have finished time")
		}
		if _, exists := nodeExecution.InputSummary(); exists {
			return nil, newValidationError("nodeExecutions", "initial node must not have input summary")
		}
		if _, exists := nodeExecution.OutputSummary(); exists {
			return nil, newValidationError(
				"nodeExecutions", "initial node must not have output summary")
		}
		if _, exists :=
			nodeExecution.FailureSummary(); exists {
			return nil, newValidationError("nodeExecutions",
				"initial node must not have failure summary")
		}
		executionID := nodeExecution.ID().String()
		if _, exists := knownExecutionIDs[executionID]; exists {
			return nil, newValidationError("nodeExecutions", "must not contain duplicate node execution IDs")
		}
		nodeID := nodeExecution.NodeID().String()
		if _, exists := knownNodeIDs[nodeID]; exists {
			return nil, newValidationError(
				"nodeExecutions", "must not contain duplicate node IDs")
		}
		knownExecutionIDs[executionID] =
			struct{}{}
		knownNodeIDs[nodeID] =
			struct{}{}
		normalized[index] =
			nodeExecution
	}
	return normalized, nil
}
func validateNodeExecutionCreationTimeline(companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutions []NodeExecutionRecord, values []TimelineEntry) ([]TimelineEntry, error) {
	if len(values) != len(nodeExecutions) {
		return nil, newValidationError("timeline",
			"must contain exactly one NODE_CREATED event per node execution")
	}
	knownEventIDs := make(map[string]struct{},
		len(values))
	normalized := make([]TimelineEntry, len(values))
	for index, entry := range values {
		if !entry.IsValid() {
			return nil, newValidationError("timeline",
				"must contain only valid entries")
		}
		eventDraft, exists := entry.Event()
		if !exists {
			return nil, newValidationError("timeline", "must contain only NODE_CREATED events")
		}
		nodeExecution := nodeExecutions[index]
		if eventDraft.Type() !=
			ExecutionEventTypeNodeCreated {
			return nil, newValidationError("timeline",
				"event type must be NODE_CREATED")
		}
		if eventDraft.CompanyID().String() != companyID.String() {
			return nil, newValidationError("timeline", "event company ID must match the command")
		}
		if eventDraft.WorkflowExecutionID().String() != workflowExecutionID.String() {
			return nil, newValidationError(
				"timeline", "event workflow execution ID must match the command")
		}
		eventNodeExecutionID, exists :=
			eventDraft.NodeExecutionID()
		if !exists || eventNodeExecutionID.String() !=
			nodeExecution.ID().String() {
			return nil, newValidationError("timeline",
				"NODE_CREATED event order must match node execution order")
		}
		if _, exists := eventDraft.PreviousStatus(); exists {
			return nil, newValidationError("timeline", "NODE_CREATED must not have previous status")
		}
		newStatus, exists := eventDraft.NewStatus()
		if !exists ||
			newStatus != execution.NodeExecutionStatusPending.String() {
			return nil, newValidationError(
				"timeline", "NODE_CREATED must set PENDING status")
		}
		if !eventDraft.CreatedAt().Equal(
			nodeExecution.CreatedAt()) {
			return nil, newValidationError(
				"timeline", "NODE_CREATED time must equal node creation time")
		}
		eventID := eventDraft.ID().String()
		if _, exists := knownEventIDs[eventID]; exists {
			return nil, newValidationError("timeline", "must not contain duplicate event IDs")
		}
		knownEventIDs[eventID] = struct{}{}
		normalized[index] = entry
	}
	return normalized, nil
}
func (command CreateNodeExecutionsCommand) CompanyID() workflow.CompanyID {
	return command.params.CompanyID
}
func (command CreateNodeExecutionsCommand) WorkflowExecutionID() execution.WorkflowExecutionID {
	return command.params.WorkflowExecutionID
}
func (command CreateNodeExecutionsCommand) ExpectedWorkflowStatus() execution.WorkflowExecutionStatus {
	return command.params.ExpectedWorkflowStatus
}
func (command CreateNodeExecutionsCommand) ExpectedWorkflowLockVersion() int64 {
	return command.params.ExpectedWorkflowLockVersion
}
func (command CreateNodeExecutionsCommand) ExpectedNextSequenceNumber() SequenceNumber {
	return command.params.ExpectedNextSequenceNumber
}
func (command CreateNodeExecutionsCommand) NodeExecutions() []NodeExecutionRecord {
	return append([]NodeExecutionRecord(nil), command.params.NodeExecutions...,
	)
}
func (command CreateNodeExecutionsCommand) Timeline() []TimelineEntry {
	return append([]TimelineEntry(nil), command.params.Timeline...,
	)
}
func (command CreateNodeExecutionsCommand) IsValid() bool {
	_, err := NewCreateNodeExecutionsCommand(command.params)
	return err == nil

}

type ExecutionCreationStore interface {
	CreateExecution(ctx context.Context,
		command CreateExecutionCommand) error
	CreateNodeExecutions(ctx context.Context, command CreateNodeExecutionsCommand,
	) error
}
type ExecutionLifecycleStore interface {
	ExecutionCreationStore
	WorkflowTransitionStore
	NodeTransitionStore
}
