package engine

import (
	"context"
	"fmt"
	"time"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type WorkflowCreationObservation struct {
	Request           ExecutionRequest
	WorkflowExecution execution.WorkflowExecution
}
type WorkflowTransitionObservation struct {
	Request         ExecutionRequest
	Before          execution.WorkflowExecution
	After           execution.WorkflowExecution
	TransitionAt    time.Time
	Failure         runtime.RuntimeFailure
	HasFailure      bool
	TechnicalDetail string
	Stalled         bool
}
type NodeExecutionCreationItem struct {
	Definition     workflow.NodeDefinition
	Execution      execution.NodeExecution
	RetryPolicy    execution.RetryPolicy
	HasRetryPolicy bool
}
type NodeExecutionsCreationObservation struct {
	Request           ExecutionRequest
	WorkflowExecution execution.WorkflowExecution
	Items             []NodeExecutionCreationItem
}
type NodeTransitionObservation struct {
	Request           ExecutionRequest
	WorkflowExecution execution.WorkflowExecution
	Definition        workflow.NodeDefinition
	Before            execution.NodeExecution
	After             execution.NodeExecution
	TransitionAt      time.Time
	Result            runtime.NodeResult
	HasResult         bool
	Failure           runtime.RuntimeFailure
	HasFailure        bool
	RetryDecision     execution.RetryDecision
	HasRetryDecision  bool
	TechnicalDetail   string
	NextAttemptAt     time.Time
}
type LifecycleRecorder interface {
	RecordWorkflowCreation(
		ctx context.Context, observation WorkflowCreationObservation) error
	RecordWorkflowTransition(ctx context.Context,
		observation WorkflowTransitionObservation) error
	RecordNodeExecutionsCreation(ctx context.Context, observation NodeExecutionsCreationObservation,
	) error
	RecordNodeTransition(
		ctx context.Context, observation NodeTransitionObservation) error
}
type NoopLifecycleRecorder struct{}

var _ LifecycleRecorder = NoopLifecycleRecorder{}

func (NoopLifecycleRecorder) RecordWorkflowCreation(
	ctx context.Context, _ WorkflowCreationObservation) error {
	return lifecycleRecorderContextError(ctx)
}
func (NoopLifecycleRecorder) RecordWorkflowTransition(
	ctx context.Context, _ WorkflowTransitionObservation) error {
	return lifecycleRecorderContextError(ctx)
}
func (NoopLifecycleRecorder) RecordNodeExecutionsCreation(
	ctx context.Context, _ NodeExecutionsCreationObservation) error {
	return lifecycleRecorderContextError(ctx)
}
func (NoopLifecycleRecorder) RecordNodeTransition(
	ctx context.Context, _ NodeTransitionObservation) error {
	return lifecycleRecorderContextError(ctx)
}
func lifecycleRecorderContextError(ctx context.Context) error {
	if ctx == nil {
		return newValidationError("context",
			"must not be nil")
	}
	return ctx.Err()
}

const (
	failureCodeNodeExecutionCanceled        = "NODE_EXECUTION_CANCELED"
	failureCodeNodeExecutionTimedOut        = "NODE_EXECUTION_TIMED_OUT"
	terminalNodeLifecyclePersistenceTimeout = 5 * time.Second
)

type nodeTransitionObservationDetails struct {
	result            runtime.NodeResult
	hasResult         bool
	failure           runtime.RuntimeFailure
	hasFailure        bool
	retryDecision     execution.RetryDecision
	hasRetryDecision  bool
	technicalDetail   string
	nextAttemptAt     time.Time
	useCleanupContext bool
}
type nodeExecutionTransitionMutation func(nodeExecution *execution.NodeExecution,
) error

func applyRecordedNodeTransition(
	prepared *preparedExecution, nodeID workflow.NodeID, nodeExecution *execution.NodeExecution,
	transitionAt time.Time, mutate nodeExecutionTransitionMutation, details nodeTransitionObservationDetails,
) error {
	if prepared == nil {
		return newValidationError(
			"preparedExecution", "must not be nil")
	}
	if prepared.workflowExecution == nil {
		return newValidationError("preparedExecution.workflowExecution", "must not be nil")
	}
	if prepared.executionContext == nil {
		return newValidationError("preparedExecution.executionContext",
			"must not be nil")
	}
	if !prepared.dependencies.IsValid() {
		return newValidationError(
			"preparedExecution.dependencies", "must be valid")
	}
	if nodeExecution == nil {
		return newValidationError("nodeExecution", "must not be nil")
	}
	if transitionAt.IsZero() {
		return newValidationError("transitionAt",
			"must not be zero")
	}
	if mutate == nil {
		return newValidationError(
			"transitionMutation", "must not be nil")
	}
	if nodeExecution.NodeID() != nodeID {
		return newValidationError("nodeExecution.nodeID", "must match the transition node ID")
	}
	if nodeExecution.WorkflowExecutionID() != prepared.workflowExecution.ID() {
		return newValidationError(
			"nodeExecution.workflowExecutionID", "must match the prepared workflow execution")
	}
	if details.hasResult &&
		!details.result.IsValid() {
		return newValidationError("nodeTransition.result",
			"must be valid when provided")
	}
	if details.hasFailure && !details.failure.IsValid() {
		return newValidationError("nodeTransition.failure", "must be valid when provided")
	}
	if details.hasRetryDecision && !details.retryDecision.IsValid() {
		return newValidationError(
			"nodeTransition.retryDecision", "must be valid when provided")
	}
	nodeDefinition, exists := prepared.plan.graph.Node(nodeID)
	if !exists {
		return fmt.Errorf("node definition %s is unavailable", nodeID)
	}
	before := *nodeExecution
	after := before
	if err := mutate(&after); err != nil {
		return fmt.Errorf("prepare node %s lifecycle transition: %w",
			nodeID, err)
	}
	if after.Status() == before.Status() {
		return newValidationError("nodeTransition.status", "must change the node execution status")
	}
	if after.Status() == execution.NodeExecutionStatusRetryPending {
		if details.nextAttemptAt.IsZero() {
			return newValidationError(
				"nodeTransition.nextAttemptAt", "must be provided for RETRY_PENDING status")
		}
	} else if !details.nextAttemptAt.IsZero() {
		return newValidationError(
			"nodeTransition.nextAttemptAt", "must be absent unless target status is RETRY_PENDING")
	}
	observation := NodeTransitionObservation{Request: prepared.request,
		WorkflowExecution: *prepared.workflowExecution, Definition: nodeDefinition,
		Before: before,
		After:  after, TransitionAt: transitionAt,
		Result:    details.result,
		HasResult: details.hasResult, Failure: details.failure,
		HasFailure:       details.hasFailure,
		RetryDecision:    details.retryDecision,
		HasRetryDecision: details.hasRetryDecision,
		TechnicalDetail:  details.technicalDetail,
		NextAttemptAt:    details.nextAttemptAt}
	recorderContext, cancelRecorderContext, err := nodeTransitionRecorderContext(prepared.executionContext.Context(),
		after.Status(), details.useCleanupContext)
	if err != nil {
		return fmt.Errorf("prepare recorder context for node %s transition: %w",
			nodeID, err)
	}
	defer cancelRecorderContext()
	if err := prepared.dependencies.LifecycleRecorder().
		RecordNodeTransition(recorderContext, observation); err != nil {
		return fmt.Errorf("record node %s transition from %s to %s: %w",
			nodeID, before.Status(), after.Status(),
			err)
	}
	*nodeExecution = after
	return nil
}
func nodeTransitionRecorderContext(parent context.Context, targetStatus execution.NodeExecutionStatus,
	useCleanupContext bool) (context.Context,
	context.CancelFunc, error) {
	if parent == nil {
		return nil, nil,
			newValidationError("nodeTransition.context", "must not be nil")
	}
	if parent.Err() == nil {
		return parent, func() {},
			nil
	}
	requiresCleanupContext := useCleanupContext || targetStatus ==
		execution.NodeExecutionStatusCancelled || targetStatus == execution.NodeExecutionStatusTimedOut
	if !requiresCleanupContext {
		return parent,
			func() {}, nil
	}
	cleanupContext := context.WithoutCancel(parent)
	boundedContext, cancel := context.WithTimeout(
		cleanupContext, terminalNodeLifecyclePersistenceTimeout)
	return boundedContext, cancel,
		nil
}

const terminalWorkflowLifecyclePersistenceTimeout = 5 * time.Second

func applyRecordedWorkflowTransition(
	prepared *preparedExecution, target execution.WorkflowExecutionStatus, state *schedulerExecutionState,
) error {
	if prepared == nil {
		return newValidationError(
			"preparedExecution", "must not be nil")
	}
	if prepared.workflowExecution == nil {
		return newValidationError("preparedExecution.workflowExecution", "must not be nil")
	}
	if prepared.executionContext == nil {
		return newValidationError("preparedExecution.executionContext",
			"must not be nil")
	}
	if !prepared.dependencies.IsValid() {
		return newValidationError(
			"preparedExecution.dependencies", "must be valid")
	}
	if prepared.workflowExecution.Status() ==
		target {
		return nil
	}
	if prepared.workflowExecution.IsTerminal() {
		return fmt.Errorf(
			"workflow execution is already terminal with status %q", prepared.workflowExecution.Status())
	}
	finishedAt, err := executionTime(
		prepared.dependencies.Clock(), "workflowExecution.finishedAt")
	if err != nil {
		return err
	}
	before := *prepared.workflowExecution
	after := before
	if err := applyWorkflowTerminalMutation(&after,
		target, finishedAt); err != nil {
		return fmt.Errorf("prepare workflow transition from %s to %s: %w", before.Status(),
			target, err)
	}
	failure, hasFailure, stalled, technicalDetail, err :=
		workflowTransitionObservationDetails(state)
	if err != nil {
		return err
	}
	observation := WorkflowTransitionObservation{Request: prepared.request,
		Before: before,
		After:  after, TransitionAt: finishedAt,
		Failure:    failure,
		HasFailure: hasFailure, TechnicalDetail: technicalDetail,
		Stalled: stalled}
	recorderContext, cancelRecorderContext, err := workflowTransitionRecorderContext(
		prepared.executionContext.Context(), target)
	if err != nil {
		return fmt.Errorf("prepare workflow transition recorder context: %w",
			err)
	}
	defer cancelRecorderContext()
	if err := prepared.
		dependencies.LifecycleRecorder().RecordWorkflowTransition(
		recorderContext, observation); err != nil {
		return fmt.Errorf("record workflow transition from %s to %s: %w", before.Status(),
			after.Status(), err)
	}
	/*
		The workflow is mutated only after the recorder successfully
		completes the durable transition.
	*/
	*prepared.workflowExecution = after
	return nil
}
func applyWorkflowTerminalMutation(workflowExecution *execution.WorkflowExecution, target execution.WorkflowExecutionStatus,
	finishedAt time.Time) error {
	if workflowExecution == nil {
		return newValidationError("workflowExecution", "must not be nil")
	}
	if finishedAt.IsZero() {
		return newValidationError("finishedAt",
			"must not be zero")
	}
	switch target {
	case execution.WorkflowExecutionStatusSucceeded:
		return workflowExecution.Succeed(finishedAt)
	case execution.WorkflowExecutionStatusFailed:
		return workflowExecution.Fail(
			finishedAt)
	case execution.WorkflowExecutionStatusCancelled:
		return workflowExecution.Cancel(finishedAt)
	case execution.WorkflowExecutionStatusTimedOut:
		return workflowExecution.Timeout(finishedAt)
	default:
		return newValidationError(
			"targetStatus", fmt.Sprintf("workflow status %q is not a supported terminal status",
				target))
	}
}
func workflowTransitionObservationDetails(state *schedulerExecutionState) (
	runtime.RuntimeFailure, bool, bool,
	string, error) {
	if state == nil || state.workflowFailure == nil {
		return runtime.RuntimeFailure{},
			false, false, "",
			nil
	}
	failure := *state.workflowFailure
	if !failure.IsValid() {
		return runtime.RuntimeFailure{}, false,
			false, "", newValidationError(
				"workflowFailure", "must be valid")
	}
	technicalDetail := ""
	if failure.Category() == runtime.FailureCategoryInternal {
		technicalDetail = failure.Details()["error"]
	}
	return failure, true,
		state.stalled, technicalDetail, nil
}
func workflowTransitionRecorderContext(
	parent context.Context, target execution.WorkflowExecutionStatus) (
	context.Context, context.CancelFunc, error,
) {
	if parent == nil {
		return nil,
			nil, newValidationError("workflowTransition.context",
				"must not be nil")
	}
	if parent.Err() == nil {
		return parent,
			func() {}, nil
	}
	if !isSupportedWorkflowTerminalTarget(target) {
		return parent, func() {},
			nil
	}
	cleanupContext := context.WithoutCancel(parent)
	boundedContext, cancel := context.WithTimeout(cleanupContext,
		terminalWorkflowLifecyclePersistenceTimeout)
	return boundedContext, cancel, nil
}
func isSupportedWorkflowTerminalTarget(
	target execution.WorkflowExecutionStatus) bool {
	switch target {
	case execution.WorkflowExecutionStatusSucceeded, execution.WorkflowExecutionStatusFailed, execution.WorkflowExecutionStatusCancelled,
		execution.WorkflowExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
