package repository

import (
	"fmt"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strconv"
	"time"
)

type AsyncWorkflowCompletion struct {
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	ExpectedLockVersion int64
	Status              execution.WorkflowExecutionStatus
	FinishedAt          time.Time
}

func (completion AsyncWorkflowCompletion) IsValid() bool {
	return completion.CompanyID.String() != "" && completion.WorkflowExecutionID.String() != "" && completion.ExpectedLockVersion >= 0 && completion.Status.IsTerminal() && completion.Status != execution.WorkflowExecutionStatusRejected && !completion.FinishedAt.IsZero()
}

const maximumAsyncPortCharacters = 128

type AsyncNodeInputParams struct {
	CompanyID             workflow.CompanyID
	WorkflowExecutionID   execution.WorkflowExecutionID
	TargetNodeExecutionID execution.NodeExecutionID
	SourceNodeExecutionID execution.NodeExecutionID
	TargetNodeID          workflow.NodeID
	SourceNodeID          workflow.NodeID
	EdgeID                workflow.EdgeID
	SourceOutputPort      string
	TargetInputPort       string
	SourceAttempt         int16
	Payload               runtime.Payload
	CreatedAt             time.Time
}
type AsyncNodeInput struct {
	params   AsyncNodeInputParams
	identity string
}

func NewAsyncNodeInput(params AsyncNodeInputParams) (AsyncNodeInput, error) {
	companyID, workflowExecutionID, err := normalizeAsyncExecutionScope(params.CompanyID, params.WorkflowExecutionID)
	if err != nil {
		return AsyncNodeInput{}, err
	}
	targetNodeExecutionID, err := execution.NewNodeExecutionID(params.TargetNodeExecutionID.String())
	if err != nil {
		return AsyncNodeInput{}, newValidationError("targetNodeExecutionID", err.Error())
	}
	sourceNodeExecutionID, err := execution.NewNodeExecutionID(params.SourceNodeExecutionID.String())
	if err != nil {
		return AsyncNodeInput{}, newValidationError("sourceNodeExecutionID", err.Error())
	}
	targetNodeID, err := workflow.NewNodeID(params.TargetNodeID.String())
	if err != nil {
		return AsyncNodeInput{}, newValidationError("targetNodeID", err.Error())
	}
	sourceNodeID, err := workflow.NewNodeID(params.SourceNodeID.String())
	if err != nil {
		return AsyncNodeInput{}, newValidationError("sourceNodeID", err.Error())
	}
	edgeID, err := workflow.NewEdgeID(params.EdgeID.String())
	if err != nil {
		return AsyncNodeInput{}, newValidationError("edgeID", err.Error())
	}
	sourceOutputPort, err := normalizeRequiredBoundedString("sourceOutputPort", params.SourceOutputPort, maximumAsyncPortCharacters)
	if err != nil {
		return AsyncNodeInput{}, err
	}
	targetInputPort, err := normalizeRequiredBoundedString("targetInputPort", params.TargetInputPort, maximumAsyncPortCharacters)
	if err != nil {
		return AsyncNodeInput{}, err
	}
	if params.SourceAttempt <= 0 {
		return AsyncNodeInput{}, newValidationError("sourceAttempt", "must be greater than zero")
	}
	payload, err := cloneBoundedAsyncPayload(params.Payload)
	if err != nil {
		return AsyncNodeInput{}, newValidationError("payload", err.Error())
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return AsyncNodeInput{}, err
	}
	identity := deterministicAsyncIdentity("async-input", companyID.String(), workflowExecutionID.String(), edgeID.String(), strconv.Itoa(int(params.SourceAttempt)))
	return AsyncNodeInput{params: AsyncNodeInputParams{CompanyID: companyID, WorkflowExecutionID: workflowExecutionID, TargetNodeExecutionID: targetNodeExecutionID, SourceNodeExecutionID: sourceNodeExecutionID, TargetNodeID: targetNodeID, SourceNodeID: sourceNodeID, EdgeID: edgeID, SourceOutputPort: sourceOutputPort, TargetInputPort: targetInputPort, SourceAttempt: params.SourceAttempt, Payload: payload, CreatedAt: createdAt}, identity: identity}, nil
}
func cloneBoundedAsyncPayload(payload runtime.Payload) (runtime.Payload, error) {
	if inlineData, inline := payload.InlineData(); inline {
		return runtime.NewInlinePayload(payload.ContentType(), inlineData, payload.Metadata(), MaximumAsyncEncodedPayloadBytes)
	}
	artifact, exists := payload.Artifact()
	if !exists {
		return runtime.Payload{}, fmt.Errorf("must contain a valid inline or artifact source")
	}
	clonedArtifact, err := runtime.NewArtifactReference(artifact.ID(), artifact.Location(), artifact.ContentType(), artifact.SizeBytes(), artifact.Checksum(), artifact.Metadata())
	if err != nil {
		return runtime.Payload{}, err
	}
	return runtime.NewArtifactPayload(clonedArtifact, payload.Metadata())
}
func (input AsyncNodeInput) CompanyID() workflow.CompanyID {
	return input.params.CompanyID
}
func (input AsyncNodeInput) WorkflowExecutionID() execution.WorkflowExecutionID {
	return input.params.WorkflowExecutionID
}
func (input AsyncNodeInput) TargetNodeExecutionID() execution.NodeExecutionID {
	return input.params.TargetNodeExecutionID
}
func (input AsyncNodeInput) SourceNodeExecutionID() execution.NodeExecutionID {
	return input.params.SourceNodeExecutionID
}
func (input AsyncNodeInput) TargetNodeID() workflow.NodeID {
	return input.params.TargetNodeID
}
func (input AsyncNodeInput) SourceNodeID() workflow.NodeID {
	return input.params.SourceNodeID
}
func (input AsyncNodeInput) EdgeID() workflow.EdgeID {
	return input.params.EdgeID
}
func (input AsyncNodeInput) SourceOutputPort() string {
	return input.params.SourceOutputPort
}
func (input AsyncNodeInput) TargetInputPort() string {
	return input.params.TargetInputPort
}
func (input AsyncNodeInput) SourceAttempt() int16 {
	return input.params.SourceAttempt
}
func (input AsyncNodeInput) Payload() runtime.Payload {
	payload, _ := cloneBoundedAsyncPayload(input.params.Payload)
	return payload
}
func (input AsyncNodeInput) CreatedAt() time.Time {
	return input.params.CreatedAt
}
func (input AsyncNodeInput) IdentityKey() string {
	return input.identity
}
func (input AsyncNodeInput) IsValid() bool {
	normalized, err := NewAsyncNodeInput(input.params)
	return err == nil && normalized.identity == input.identity
}

// AsyncNodeSchedule is the atomic persistence command for making one pending
// node publishable. The node state and its NODE_COMMAND outbox record must be
// committed together.
type AsyncNodeSchedule struct {
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	NodeExecutionID     execution.NodeExecutionID
	NodeID              workflow.NodeID
	Attempt             int16
	ExpectedLockVersion int64
	QueuedAt            time.Time
	OutboxMessage       OutboxMessage
}

func (schedule AsyncNodeSchedule) IsValid() bool {
	if schedule.CompanyID.String() == "" || schedule.WorkflowExecutionID.String() == "" || schedule.NodeExecutionID.String() == "" || schedule.NodeID.String() == "" || schedule.Attempt <= 0 || schedule.ExpectedLockVersion < 0 || schedule.QueuedAt.IsZero() || !schedule.OutboxMessage.IsValid() {
		return false
	}
	messageNodeExecutionID, hasNodeExecution := schedule.OutboxMessage.NodeExecutionID()
	messageNodeID, hasNode := schedule.OutboxMessage.NodeID()
	messageAttempt, hasAttempt := schedule.OutboxMessage.Attempt()
	return hasNodeExecution && hasNode && hasAttempt && schedule.OutboxMessage.OperationKind() == OutboxOperationNodeCommand && schedule.OutboxMessage.CompanyID() == schedule.CompanyID && schedule.OutboxMessage.WorkflowExecutionID() == schedule.WorkflowExecutionID && messageNodeExecutionID == schedule.NodeExecutionID && messageNodeID == schedule.NodeID && messageAttempt == schedule.Attempt && schedule.OutboxMessage.PublicationState() == OutboxPublicationPending &&
		schedule.OutboxMessage.CreatedAt().Equal(schedule.QueuedAt) &&
		schedule.OutboxMessage.AvailableAt().Equal(schedule.QueuedAt)
}
func normalizeAsyncExecutionScope(companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID) (workflow.CompanyID, execution.WorkflowExecutionID, error) {
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return "", "", newValidationError("companyID", err.Error())
	}
	normalizedWorkflowExecutionID, err := execution.NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return "", "", newValidationError("workflowExecutionID", err.Error())
	}
	return normalizedCompanyID, normalizedWorkflowExecutionID, nil
}

// AsyncNodeSchedule is the atomic persistence command for making one pending
// node publishable. The node state and its NODE_COMMAND outbox record must be
// committed together.
