package engine

import (
	"context"
	"errors"
	"fmt"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"sort"
)

func prepareNodeInput(prepared *preparedExecution, nodeID workflow.NodeID,
) (runtime.NodeInput, error,
) {
	if prepared == nil {
		return runtime.NodeInput{},
			newValidationError("preparedExecution", "must not be nil")
	}
	if prepared.executionContext == nil {
		return runtime.NodeInput{}, newValidationError(
			"preparedExecution.executionContext", "must not be nil")
	}
	if _, exists := prepared.plan.graph.Node(nodeID); !exists {
		return runtime.NodeInput{}, newValidationError("nodeID",
			fmt.Sprintf("node %q is not present in the execution plan", nodeID.String()))
	}
	incomingEdges := prepared.plan.graph.IncomingEdges(nodeID)
	if len(incomingEdges) == 0 {
		input, err := runtime.NewNodeInput(nil)
		if err != nil {
			return runtime.NodeInput{}, fmt.Errorf(
				"create empty node input: %w", err)
		}
		return input, nil
	}
	sort.Slice(
		incomingEdges, func(left int, right int) bool {
			return incomingEdges[left].
				ID().String() < incomingEdges[right].
				ID().String()
		},
	)
	payloadsByPort := make(
		map[string][]runtime.Payload)
	edgeRuntimes := make([]*runtime.EdgeRuntime, 0,
		len(incomingEdges))
	for _, edge := range incomingEdges {
		edgeRuntime, exists, err := prepared.executionContext.Edge(
			edge.ID())
		if err != nil {
			return runtime.NodeInput{}, fmt.Errorf("lookup runtime edge %s: %w",
				edge.ID(), err)
		}
		if !exists || edgeRuntime == nil {
			return runtime.NodeInput{}, fmt.Errorf("runtime edge %s is unavailable",
				edge.ID())
		}
		if edgeRuntime.TargetNodeID() != nodeID {
			return runtime.NodeInput{},
				fmt.Errorf("runtime edge %s targets node %s instead of node %s", edge.ID(),
					edgeRuntime.TargetNodeID(), nodeID)
		}
		queue := edgeRuntime.Queue()
		if queue == nil {
			return runtime.NodeInput{}, fmt.Errorf(
				"runtime edge %s contains no queue", edge.ID())
		}
		queueLength := queue.Len()
		if queueLength == 0 {
			return runtime.NodeInput{},
				fmt.Errorf("runtime edge %s contains no payload", edge.ID())
		}
		if queueLength > 1 {
			return runtime.NodeInput{}, fmt.Errorf(
				"runtime edge %s contains %d payloads; single-shot execution requires exactly one", edge.ID(), queueLength,
			)
		}
		payload, exists := queue.Peek()
		if !exists {
			return runtime.NodeInput{},
				fmt.Errorf("runtime edge %s reported one payload but Peek returned empty", edge.ID())
		}
		targetPort := edgeRuntime.TargetInputPort()
		payloadsByPort[targetPort] = append(
			payloadsByPort[targetPort], payload)
		edgeRuntimes = append(edgeRuntimes,
			edgeRuntime)
	}
	input, err := runtime.NewNodeInput(payloadsByPort)
	if err != nil {
		return runtime.NodeInput{},
			fmt.Errorf("build node input for node %s: %w", nodeID,
				err)
	}
	for _, edgeRuntime := range edgeRuntimes {
		queue := edgeRuntime.Queue()
		_, exists := queue.Pop()
		if !exists {
			return runtime.NodeInput{}, fmt.Errorf("runtime edge %s was empty during committed input consumption",
				edgeRuntime.ID())
		}
	}
	return input, nil
}

func executeReadyNode(prepared *preparedExecution,
	nodeID workflow.NodeID) (runtime.NodeResult,
	error) {
	if prepared == nil {
		return runtime.NodeResult{}, newValidationError("preparedExecution",
			"must not be nil")
	}
	if prepared.executionContext == nil {
		return runtime.NodeResult{},
			newValidationError("preparedExecution.executionContext", "must not be nil")
	}
	if prepared.workflowExecution == nil {
		return runtime.NodeResult{}, newValidationError(
			"preparedExecution.workflowExecution", "must not be nil")
	}
	if !prepared.dependencies.IsValid() {
		return runtime.NodeResult{}, newValidationError("preparedExecution.dependencies",
			"must be valid")
	}
	if contextErr := prepared.executionContext.Err(); contextErr != nil {
		return runtime.NodeResult{}, fmt.Errorf("execution context ended before node %s started: %w",
			nodeID, contextErr)
	}
	nodeDefinition, exists :=
		prepared.plan.graph.Node(nodeID)
	if !exists {
		return runtime.NodeResult{},
			newValidationError("nodeID", fmt.Sprintf(
				"node %q is not present in the execution plan", nodeID.String()),
			)
	}
	nodeExecution, exists := prepared.nodeExecutionsByNode[nodeID]
	if !exists || nodeExecution == nil {
		return runtime.NodeResult{}, fmt.Errorf("node execution for node %s is unavailable",
			nodeID)
	}
	if nodeExecution.Status() != execution.NodeExecutionStatusPending {
		return runtime.NodeResult{}, newValidationError("nodeExecution.status",
			fmt.Sprintf("node %s must be PENDING before execution; current status is %s", nodeID,
				nodeExecution.Status()))
	}
	readiness, err := assessNodeReadiness(
		prepared, nodeID)
	if err != nil {
		return runtime.NodeResult{}, fmt.Errorf(
			"assess readiness for node %s: %w", nodeID, err,
		)
	}
	if !readiness.IsReady() {
		return runtime.NodeResult{}, newValidationError(
			"nodeExecution.readiness", fmt.Sprintf("node %s must be READY before execution; current readiness is %s",
				nodeID, readiness.state),
		)
	}
	descriptor, exists := prepared.plan.descriptorsByNode[nodeID]
	if !exists {
		return runtime.NodeResult{}, fmt.Errorf("plugin descriptor for node %s is unavailable",
			nodeID)
	}
	executor, exists := prepared.
		dependencies.ExecutorRegistry().Lookup(
		descriptor.Identity())
	if !exists || executor == nil {
		return runtime.NodeResult{}, fmt.Errorf(
			"executor %s for node %s is unavailable", descriptor.Identity(), nodeID,
		)
	}
	incomingEdgeIDs := incomingEdgeIDsForNode(prepared,
		nodeID)
	readyAt, err := executionTime(prepared.dependencies.Clock(), "nodeExecution.readyAt")
	if err != nil {
		return runtime.NodeResult{}, err
	}
	if err := applyRecordedNodeTransition(
		prepared, nodeID, nodeExecution,
		readyAt, func(candidate *execution.NodeExecution) error {
			return candidate.MarkReady(
				readyAt)
		},
		nodeTransitionObservationDetails{}); err != nil {
		return runtime.NodeResult{},
			fmt.Errorf("mark node %s ready: %w", nodeID,
				err)
	}
	startedAt, err := executionTime(prepared.dependencies.Clock(),
		"nodeExecution.startedAt")
	if err != nil {
		return runtime.NodeResult{}, err
	}
	if err := applyRecordedNodeTransition(prepared, nodeID,
		nodeExecution, startedAt, func(candidate *execution.NodeExecution) error {
			return candidate.Start(startedAt)
		}, nodeTransitionObservationDetails{}); err != nil {
		return runtime.NodeResult{}, fmt.Errorf("start node %s: %w",
			nodeID, err)
	}
	/*
		The RUNNING transition has now been durably recorded.
		The plugin executes only after that persistence succeeds.
		No database transaction is kept open during this call.
	*/
	input, err := prepareNodeInput(prepared,
		nodeID)
	if err != nil {
		return runtime.NodeResult{}, completeActiveNodeWithError(prepared,
			nodeID, nodeExecution, fmt.Errorf(
				"prepare input for node %s: %w", nodeID, err,
			))
	}
	attemptOutcome, err := executeSynchronousNodeAttempts(
		prepared, nodeID, nodeExecution, executor, input,
		incomingEdgeIDs, nodeDefinition.Configuration(),
		descriptor.Identity().String(),
	)
	if err != nil {
		return runtime.NodeResult{}, err
	}
	result := attemptOutcome.result
	if result.IsFailure() {
		failure, exists := result.Failure()
		if !exists {
			return runtime.NodeResult{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution,
				fmt.Errorf("executor %s returned a failed result without a RuntimeFailure for node %s", descriptor.Identity(),
					nodeID))
		}
		if err := transitionNodeToTerminal(
			prepared, nodeID, nodeExecution,
			nodeTerminalStatusForFailure(failure), nodeTransitionObservationDetails{result: result,
				hasResult: true, failure: failure, hasFailure: true,
				retryDecision:    attemptOutcome.retryDecision,
				hasRetryDecision: attemptOutcome.hasRetryDecision,
			}); err != nil {
			return runtime.NodeResult{},
				fmt.Errorf("complete controlled failure for node %s: %w", nodeID,
					err)
		}
		return result, nil
	}
	if !result.IsSuccess() {
		return runtime.NodeResult{},
			completeActiveNodeWithError(prepared, nodeID,
				nodeExecution, fmt.Errorf("executor %s returned an unsupported result status %q for node %s",
					descriptor.Identity(), result.Status(), nodeID,
				))
	}
	_, err = buildOutputRoutingOperations(prepared,
		nodeID, result)
	if err != nil {
		return runtime.NodeResult{}, completeActiveNodeWithError(
			prepared, nodeID, nodeExecution,
			fmt.Errorf("validate outputs for node %s: %w", nodeID,
				err))
	}
	if err := prepared.
		executionContext.ApplyContextChanges(result.ContextChanges()); err != nil {
		return runtime.NodeResult{}, completeActiveNodeWithError(
			prepared, nodeID, nodeExecution,
			fmt.Errorf("apply context changes for node %s: %w", nodeID,
				err))
	}
	if err := routeNodeOutputs(
		prepared, nodeID, result,
	); err != nil {
		return runtime.NodeResult{}, completeActiveNodeWithError(
			prepared, nodeID, nodeExecution,
			fmt.Errorf("route outputs for node %s: %w", nodeID,
				err))
	}
	if err := transitionNodeToTerminal(
		prepared, nodeID, nodeExecution,
		execution.NodeExecutionStatusSucceeded, nodeTransitionObservationDetails{result: result,
			hasResult: true}); err != nil {
		return runtime.NodeResult{}, fmt.Errorf("mark node %s succeeded: %w",
			nodeID, err)
	}
	return result, nil
}

func nodeTerminalStatusForFailure(
	failure runtime.RuntimeFailure,
) execution.NodeExecutionStatus {
	if failure.Category() == runtime.FailureCategoryTimeout {
		return execution.NodeExecutionStatusTimedOut
	}
	return execution.NodeExecutionStatusFailed
}

func incomingEdgeIDsForNode(
	prepared *preparedExecution, nodeID workflow.NodeID) []workflow.EdgeID {
	if prepared == nil {
		return nil
	}
	incomingEdges := prepared.plan.graph.IncomingEdges(nodeID)
	if len(incomingEdges) == 0 {
		return nil
	}
	sort.Slice(
		incomingEdges, func(left int, right int) bool {
			return incomingEdges[left].
				ID().String() < incomingEdges[right].
				ID().String()
		},
	)
	edgeIDs := make(
		[]workflow.EdgeID, 0, len(incomingEdges),
	)
	for _, edge := range incomingEdges {
		edgeIDs = append(edgeIDs, edge.ID())
	}
	return edgeIDs
}
func completeActiveNodeWithError(prepared *preparedExecution, nodeID workflow.NodeID,
	nodeExecution *execution.NodeExecution, cause error) error {
	if cause == nil {
		cause = errors.New("node execution failed without an error")
	}
	targetStatus := execution.NodeExecutionStatusFailed
	executionContextErr := error(nil)
	if prepared != nil &&
		prepared.executionContext != nil {
		executionContextErr = prepared.executionContext.Err()
	}
	switch {
	case errors.Is(cause, context.DeadlineExceeded) || errors.Is(executionContextErr,
		context.DeadlineExceeded):
		targetStatus =
			execution.NodeExecutionStatusTimedOut
	case errors.Is(
		cause, context.Canceled) ||
		errors.Is(executionContextErr, context.Canceled):
		targetStatus = execution.NodeExecutionStatusCancelled
	}
	failure, failureErr :=
		newObservedTechnicalNodeFailure(nodeID, targetStatus,
			cause)
	if failureErr != nil {
		return fmt.Errorf("%w; create structured node failure: %v", cause,
			failureErr)
	}
	transitionErr := transitionNodeToTerminal(prepared,
		nodeID, nodeExecution, targetStatus,
		nodeTransitionObservationDetails{failure: failure, hasFailure: true,
			technicalDetail: cause.Error()})
	if transitionErr != nil {
		return fmt.Errorf("%w; transition node %s to %s also failed: %v",
			cause, nodeID, targetStatus,
			transitionErr)
	}
	return cause
}

func transitionRunningNode(
	prepared *preparedExecution,
	nodeID workflow.NodeID,
	nodeExecution *execution.NodeExecution,
	targetStatus execution.NodeExecutionStatus,
	details nodeTransitionObservationDetails,
) error {
	if nodeExecution == nil {
		return newValidationError("nodeExecution", "must not be nil")
	}
	if nodeExecution.Status() != execution.NodeExecutionStatusRunning {
		return newValidationError("nodeExecution.status", fmt.Sprintf(
			"node %s must be RUNNING before terminal transition; current status is %s",
			nodeID, nodeExecution.Status(),
		))
	}
	return transitionNodeToTerminal(
		prepared, nodeID, nodeExecution, targetStatus, details)
}

func transitionNodeToTerminal(prepared *preparedExecution,
	nodeID workflow.NodeID, nodeExecution *execution.NodeExecution, targetStatus execution.NodeExecutionStatus,
	details nodeTransitionObservationDetails) error {
	if prepared == nil {
		return newValidationError("preparedExecution", "must not be nil")
	}
	if nodeExecution == nil {
		return newValidationError("nodeExecution",
			"must not be nil")
	}
	if nodeExecution.Status() != execution.NodeExecutionStatusRunning &&
		nodeExecution.Status() != execution.NodeExecutionStatusRetryPending {
		return newValidationError("nodeExecution.status", fmt.Sprintf(
			"node %s must be RUNNING or RETRY_PENDING before terminal transition; current status is %s", nodeID, nodeExecution.Status(),
		))
	}
	previousStatus := nodeExecution.Status()
	finishedAt, err := executionTime(prepared.dependencies.Clock(),
		"nodeExecution.finishedAt")
	if err != nil {
		return err
	}
	var mutate nodeExecutionTransitionMutation
	switch targetStatus {
	case execution.NodeExecutionStatusSucceeded:
		mutate = func(candidate *execution.NodeExecution,
		) error {
			return candidate.Succeed(finishedAt)
		}
	case execution.NodeExecutionStatusFailed:
		mutate = func(candidate *execution.NodeExecution,
		) error {
			return candidate.Fail(finishedAt)
		}
	case execution.NodeExecutionStatusCancelled:
		mutate = func(candidate *execution.NodeExecution,
		) error {
			return candidate.Cancel(finishedAt)
		}
	case execution.NodeExecutionStatusTimedOut:
		mutate = func(candidate *execution.NodeExecution,
		) error {
			return candidate.Timeout(finishedAt)
		}
	default:
		return newValidationError("targetStatus",
			fmt.Sprintf("status %q is not a supported running-node terminal status", targetStatus))
	}
	if err := applyRecordedNodeTransition(prepared,
		nodeID, nodeExecution, finishedAt,
		mutate, details); err != nil {
		return fmt.Errorf("transition node %s from %s to %s: %w", nodeID,
			previousStatus, targetStatus, err)
	}
	return nil
}

func newObservedTechnicalNodeFailure(nodeID workflow.NodeID, targetStatus execution.NodeExecutionStatus,
	cause error) (runtime.RuntimeFailure,
	error) {
	if cause == nil {
		cause = errors.New("node execution failed without an error")
	}
	switch targetStatus {
	case execution.NodeExecutionStatusFailed:
		return newTechnicalNodeFailure(nodeID,
			cause)
	case execution.NodeExecutionStatusCancelled:
		return runtime.NewRuntimeFailure(runtime.FailureCategoryCanceled,
			failureCodeNodeExecutionCanceled, "Node execution was canceled", false,
			map[string]string{"nodeID": nodeID.String(), "error": cause.Error()})
	case execution.NodeExecutionStatusTimedOut:
		return runtime.NewRuntimeFailure(runtime.FailureCategoryTimeout,
			failureCodeNodeExecutionTimedOut, "Node execution timed out", false,
			map[string]string{"nodeID": nodeID.String(), "error": cause.Error()})
	default:
		return runtime.RuntimeFailure{}, newValidationError(
			"targetStatus", fmt.Sprintf("status %q does not support a technical node failure",
				targetStatus))
	}
}
