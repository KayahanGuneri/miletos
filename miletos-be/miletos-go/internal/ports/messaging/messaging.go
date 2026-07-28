package messaging

import (
	"fmt"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strings"
	"time"
)

const (
	maximumMessageIdentifierLength = 200
	maximumTraceIdentifierLength   = 200
)

type MessageMetadataParams struct {
	MessageID           string
	CreatedAt           time.Time
	CompanyID           workflow.CompanyID
	WorkflowID          workflow.WorkflowID
	WorkflowExecutionID execution.WorkflowExecutionID
	NodeID              workflow.NodeID
	NodeExecutionID     execution.NodeExecutionID
	Attempt             int16
	CorrelationID       string
	CausationID         string
}
type MessageMetadata struct {
	messageID           string
	createdAt           time.Time
	companyID           workflow.CompanyID
	workflowID          workflow.WorkflowID
	workflowExecutionID execution.WorkflowExecutionID
	nodeID              workflow.NodeID
	nodeExecutionID     execution.NodeExecutionID
	attempt             int16
	correlationID       string
	causationID         string
}

func NewMessageMetadata(params MessageMetadataParams,
) (MessageMetadata, error) {
	messageID, err := normalizeRequiredIdentifier("messageID",
		params.MessageID, maximumMessageIdentifierLength)
	if err != nil {
		return MessageMetadata{}, err
	}
	createdAt, err := normalizeRequiredTime("createdAt", params.CreatedAt)
	if err != nil {
		return MessageMetadata{}, err
	}
	companyID, err := workflow.NewCompanyID(params.CompanyID.String())
	if err != nil {
		return MessageMetadata{}, fmt.Errorf("companyID: %w", err)
	}
	workflowID, err := workflow.NewWorkflowID(params.WorkflowID.String())
	if err != nil {
		return MessageMetadata{}, fmt.Errorf("workflowID: %w", err)
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(params.WorkflowExecutionID.String())
	if err != nil {
		return MessageMetadata{}, fmt.Errorf("workflowExecutionID: %w", err)
	}
	nodeID, err := workflow.NewNodeID(params.NodeID.String())
	if err != nil {
		return MessageMetadata{}, fmt.Errorf("nodeID: %w", err)
	}
	nodeExecutionID, err := execution.NewNodeExecutionID(params.NodeExecutionID.String())
	if err != nil {
		return MessageMetadata{}, fmt.Errorf("nodeExecutionID: %w", err)
	}
	if params.Attempt <= 0 {
		return MessageMetadata{}, fmt.Errorf("attempt must be greater than zero")
	}
	correlationID, err := normalizeRequiredIdentifier("correlationID", params.CorrelationID,
		maximumTraceIdentifierLength)
	if err != nil {
		return MessageMetadata{}, err
	}
	causationID, err := normalizeRequiredIdentifier("causationID", params.CausationID,
		maximumTraceIdentifierLength)
	if err != nil {
		return MessageMetadata{}, err
	}
	return MessageMetadata{messageID: messageID, createdAt: createdAt,
		companyID: companyID, workflowID: workflowID,
		workflowExecutionID: workflowExecutionID, nodeID: nodeID, nodeExecutionID: nodeExecutionID,
		attempt:       params.Attempt,
		correlationID: correlationID, causationID: causationID}, nil
}
func (metadata MessageMetadata) MessageID() string {
	return metadata.messageID
}
func (metadata MessageMetadata) CreatedAt() time.Time { return metadata.createdAt }
func (metadata MessageMetadata) CompanyID() workflow.CompanyID {
	return metadata.companyID
}
func (metadata MessageMetadata) WorkflowID() workflow.WorkflowID {
	return metadata.workflowID
}
func (metadata MessageMetadata) WorkflowExecutionID() execution.WorkflowExecutionID {
	return metadata.workflowExecutionID
}
func (metadata MessageMetadata) NodeID() workflow.NodeID {
	return metadata.nodeID
}
func (metadata MessageMetadata) NodeExecutionID() execution.NodeExecutionID {
	return metadata.nodeExecutionID
}
func (metadata MessageMetadata) Attempt() int16 { return metadata.attempt }
func (metadata MessageMetadata) CorrelationID() string {
	return metadata.correlationID
}
func (metadata MessageMetadata) CausationID() string {
	return metadata.causationID
}
func (metadata MessageMetadata) IsValid() bool {
	_, err := NewMessageMetadata(MessageMetadataParams{
		MessageID: metadata.messageID, CreatedAt: metadata.createdAt,
		CompanyID: metadata.companyID, WorkflowID: metadata.workflowID, WorkflowExecutionID: metadata.workflowExecutionID,
		NodeID: metadata.nodeID, NodeExecutionID: metadata.nodeExecutionID,
		Attempt: metadata.attempt, CorrelationID: metadata.correlationID,
		CausationID: metadata.causationID})
	return err == nil
}

type NodeCommand struct {
	metadata       MessageMetadata
	nodeDefinition workflow.NodeDefinition
	input          runtime.NodeInput
	variables      map[string]runtime.RuntimeValue
	deadlineAt     time.Time
	hasDeadlineAt  bool
}

func NewNodeCommand(metadata MessageMetadata, nodeDefinition workflow.NodeDefinition,
	input runtime.NodeInput, variables map[string]runtime.RuntimeValue, deadlineAt *time.Time,
) (NodeCommand, error) {
	if !metadata.IsValid() {
		return NodeCommand{}, fmt.Errorf("message metadata must be valid")
	}
	if metadata.NodeID() != nodeDefinition.ID() {
		return NodeCommand{}, fmt.Errorf("message node ID must match node definition")
	}
	normalizedNode, err := workflow.NewNodeDefinition(nodeDefinition.ID(), nodeDefinition.PluginType(),
		nodeDefinition.PluginVersion(), nodeDefinition.Configuration().Bytes(), nil,
	)
	if err != nil {
		return NodeCommand{}, fmt.Errorf("node definition: %w", err)
	}
	normalizedInput, err := cloneNodeInput(input)
	if err != nil {
		return NodeCommand{}, fmt.Errorf("node input: %w", err)
	}
	normalizedVariables, err := cloneRuntimeValues(variables)
	if err != nil {
		return NodeCommand{}, fmt.Errorf("variables: %w", err)
	}
	var normalizedDeadline time.Time
	hasDeadline := false
	if deadlineAt != nil {
		normalizedDeadline, err = normalizeRequiredTime("deadlineAt", *deadlineAt)
		if err != nil {
			return NodeCommand{}, err
		}
		if !normalizedDeadline.After(metadata.CreatedAt()) {
			return NodeCommand{}, fmt.Errorf("deadlineAt must be after message creation")
		}
		hasDeadline = true
	}
	return NodeCommand{metadata: metadata,
		nodeDefinition: normalizedNode, input: normalizedInput,
		variables: normalizedVariables, deadlineAt: normalizedDeadline,
		hasDeadlineAt: hasDeadline}, nil
}
func (command NodeCommand) Metadata() MessageMetadata {
	return command.metadata
}
func (command NodeCommand) NodeDefinition() workflow.NodeDefinition {
	return command.nodeDefinition
}
func (command NodeCommand) Input() runtime.NodeInput {
	cloned, _ := cloneNodeInput(command.input)
	return cloned
}
func (command NodeCommand) Variables() map[string]runtime.RuntimeValue {
	cloned, _ := cloneRuntimeValues(command.variables)
	return cloned
}
func (command NodeCommand) DeadlineAt() (time.Time, bool) {
	if !command.hasDeadlineAt {
		return time.Time{}, false
	}
	return command.deadlineAt, true
}
func (command NodeCommand) IsValid() bool {
	var deadline *time.Time
	if command.hasDeadlineAt {
		value := command.deadlineAt
		deadline = &value
	}
	_, err := NewNodeCommand(command.metadata,
		command.nodeDefinition, command.input, command.variables,
		deadline)
	return err == nil
}

type NodeResultStatus string

const (
	NodeResultStatusSucceeded NodeResultStatus = "SUCCEEDED"
	NodeResultStatusFailed    NodeResultStatus = "FAILED"
	NodeResultStatusCancelled NodeResultStatus = "CANCELLED"
	NodeResultStatusTimedOut  NodeResultStatus = "TIMED_OUT"
)

func (status NodeResultStatus) String() string { return string(status) }
func (status NodeResultStatus) IsValid() bool {
	switch status {
	case NodeResultStatusSucceeded, NodeResultStatusFailed, NodeResultStatusCancelled,
		NodeResultStatusTimedOut:
		return true
	default:
		return false
	}
}

type NodeResultEvent struct {
	metadata   MessageMetadata
	status     NodeResultStatus
	result     runtime.NodeResult
	hasResult  bool
	failure    runtime.RuntimeFailure
	hasFailure bool
	startedAt  time.Time
	finishedAt time.Time
}

func NewNodeResultEvent(metadata MessageMetadata, result runtime.NodeResult,
	startedAt time.Time, finishedAt time.Time) (NodeResultEvent, error) {
	if !metadata.IsValid() {
		return NodeResultEvent{}, fmt.Errorf("message metadata must be valid")
	}
	normalizedResult, err := cloneNodeResult(result)
	if err != nil {
		return NodeResultEvent{}, fmt.Errorf("node result: %w", err)
	}
	status := NodeResultStatusSucceeded
	if normalizedResult.IsFailure() {
		status = NodeResultStatusFailed
	}
	started, finished, err := normalizeResultTimes(startedAt, finishedAt)
	if err != nil {
		return NodeResultEvent{}, err
	}
	if metadata.CreatedAt().Before(finished) {
		return NodeResultEvent{}, fmt.Errorf("message creation must not be before result completion")
	}
	return NodeResultEvent{metadata: metadata,
		status: status, result: normalizedResult, hasResult: true,
		startedAt: started, finishedAt: finished}, nil
}
func NewInterruptedNodeResultEvent(
	metadata MessageMetadata, status NodeResultStatus, failure runtime.RuntimeFailure,
	startedAt time.Time, finishedAt time.Time) (NodeResultEvent, error) {
	if !metadata.IsValid() {
		return NodeResultEvent{}, fmt.Errorf("message metadata must be valid")
	}
	if status != NodeResultStatusCancelled && status != NodeResultStatusTimedOut {
		return NodeResultEvent{}, fmt.Errorf("interrupted result status must be CANCELLED or TIMED_OUT")
	}
	normalizedFailure, err := cloneRuntimeFailure(failure)
	if err != nil {
		return NodeResultEvent{}, fmt.Errorf("runtime failure: %w", err)
	}
	if status == NodeResultStatusCancelled && normalizedFailure.Category() != runtime.FailureCategoryCanceled {
		return NodeResultEvent{}, fmt.Errorf("CANCELLED result requires CANCELED failure category")
	}
	if status == NodeResultStatusTimedOut && normalizedFailure.Category() != runtime.FailureCategoryTimeout {
		return NodeResultEvent{}, fmt.Errorf("TIMED_OUT result requires TIMEOUT failure category")
	}
	started, finished, err := normalizeResultTimes(startedAt, finishedAt)
	if err != nil {
		return NodeResultEvent{}, err
	}
	if metadata.CreatedAt().Before(finished) {
		return NodeResultEvent{}, fmt.Errorf("message creation must not be before result completion")
	}
	return NodeResultEvent{metadata: metadata,
		status: status, failure: normalizedFailure,
		hasFailure: true, startedAt: started,
		finishedAt: finished}, nil
}
func (event NodeResultEvent) Metadata() MessageMetadata {
	return event.metadata
}
func (event NodeResultEvent) Status() NodeResultStatus {
	return event.status
}
func (event NodeResultEvent) Result() (runtime.NodeResult, bool) {
	if !event.hasResult {
		return runtime.NodeResult{}, false
	}
	cloned, err := cloneNodeResult(event.result)
	if err != nil {
		return runtime.NodeResult{}, false
	}
	return cloned, true
}
func (event NodeResultEvent) Failure() (runtime.RuntimeFailure, bool) {
	if event.hasResult {
		return event.result.Failure()
	}
	if !event.hasFailure {
		return runtime.RuntimeFailure{}, false
	}
	cloned, err := cloneRuntimeFailure(event.failure)
	if err != nil {
		return runtime.RuntimeFailure{}, false
	}
	return cloned, true
}
func (event NodeResultEvent) StartedAt() time.Time { return event.startedAt }
func (event NodeResultEvent) FinishedAt() time.Time {
	return event.finishedAt
}
func (event NodeResultEvent) IsValid() bool {
	if event.hasResult {
		_, err := NewNodeResultEvent(event.metadata,
			event.result, event.startedAt, event.finishedAt,
		)
		return err == nil
	}
	if event.hasFailure {
		_, err := NewInterruptedNodeResultEvent(
			event.metadata, event.status, event.failure,
			event.startedAt, event.finishedAt)
		return err == nil
	}
	return false
}
func cloneNodeInput(input runtime.NodeInput) (runtime.NodeInput, error) {
	if !input.IsValid() {
		return runtime.NodeInput{}, fmt.Errorf("must be valid")
	}
	values := make(map[string][]runtime.Payload, input.PortCount())
	for _, port := range input.Ports() {
		payloads, exists, err := input.Payloads(port)
		if err != nil {
			return runtime.NodeInput{}, err
		}
		if !exists {
			return runtime.NodeInput{}, fmt.Errorf("port %q is unavailable", port)
		}
		values[port] = payloads
	}
	return runtime.NewNodeInput(values)
}
func cloneRuntimeValues(
	values map[string]runtime.RuntimeValue) (map[string]runtime.RuntimeValue, error) {
	if len(values) == 0 {
		return nil, nil
	}
	changes, err := runtime.NewContextChanges(values, nil)
	if err != nil {
		return nil, err
	}
	return changes.SetValues(), nil
}
func cloneNodeResult(result runtime.NodeResult) (runtime.NodeResult, error) {
	if !result.IsValid() {
		return runtime.NodeResult{}, fmt.Errorf("must be valid")
	}
	if result.IsFailure() {
		failure, exists := result.Failure()
		if !exists {
			return runtime.NodeResult{}, fmt.Errorf("failed result must contain a failure")
		}
		return runtime.NewNodeFailureResult(failure)
	}
	changes := result.ContextChanges()
	if terminal, exists := result.TerminalOutput(); exists {
		return runtime.NewTerminalNodeSuccessResult(terminal, changes)
	}
	return runtime.NewNodeSuccessResult(result.OutputsSnapshot(), changes)
}
func cloneRuntimeFailure(
	failure runtime.RuntimeFailure) (runtime.RuntimeFailure, error) {
	if !failure.IsValid() {
		return runtime.RuntimeFailure{}, fmt.Errorf("must be valid")
	}
	return runtime.NewRuntimeFailure(failure.Category(), failure.Code(),
		failure.Message(), failure.Retryable(), failure.Details(),
	)
}
func normalizeRequiredIdentifier(field string, value string,
	maximumLength int) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", fmt.Errorf("%s must not be blank", field)
	}
	if len(normalized) > maximumLength {
		return "", fmt.Errorf("%s must not exceed %d bytes", field, maximumLength)
	}
	return normalized, nil
}
func normalizeRequiredTime(field string, value time.Time,
) (time.Time, error) {
	if value.IsZero() {
		return time.Time{}, fmt.Errorf("%s must not be zero", field)
	}
	return value.UTC(), nil
}
func normalizeResultTimes(startedAt time.Time,
	finishedAt time.Time) (time.Time, time.Time, error) {
	started, err := normalizeRequiredTime("startedAt", startedAt)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	finished, err := normalizeRequiredTime("finishedAt", finishedAt)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if finished.Before(started) {
		return time.Time{}, time.Time{}, fmt.Errorf("finishedAt must not be before startedAt")
	}
	return started, finished, nil
}
