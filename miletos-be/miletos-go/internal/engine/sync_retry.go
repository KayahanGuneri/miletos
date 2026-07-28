package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

const (
	failureCodeNodeExecutorPanic      = "NODE_EXECUTOR_PANIC"
	failureCodeNodeExecutorError      = "NODE_EXECUTOR_ERROR"
	failureCodeRetryWaitFailed        = "RETRY_WAIT_FAILED"
	failureCodeRetryWaitReturnedEarly = "RETRY_WAIT_RETURNED_EARLY"
)

type nodeExecutorInvocationError struct {
	safeMessage     string
	technicalDetail string
	panicked        bool
}

type synchronousAttemptOutcome struct {
	result           runtime.NodeResult
	retryDecision    execution.RetryDecision
	hasRetryDecision bool
}

func (invocationError *nodeExecutorInvocationError) Error() string {
	if invocationError == nil || invocationError.safeMessage == "" {
		return "node executor invocation failed"
	}
	return invocationError.safeMessage
}

func (invocationError *nodeExecutorInvocationError) TechnicalDetail() string {
	if invocationError == nil {
		return ""
	}
	return invocationError.technicalDetail
}

func (invocationError *nodeExecutorInvocationError) Panicked() bool {
	return invocationError != nil && invocationError.panicked
}

func invokeSynchronousNodeExecutor(
	executor runtime.NodeExecutor,
	nodeContext *runtime.NodeExecutionContext,
	input runtime.NodeInput,
	configuration workflow.JSONObject,
) (result runtime.NodeResult, invocationError *nodeExecutorInvocationError) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		result = runtime.NodeResult{}
		invocationError = &nodeExecutorInvocationError{
			safeMessage: "node executor panicked",
			technicalDetail: fmt.Sprintf(
				"node executor panic type=%T value=%v\n%s",
				recovered, recovered, debug.Stack(),
			),
			panicked: true,
		}
	}()
	result, err := executor.Execute(nodeContext, input, configuration)
	if err != nil {
		return runtime.NodeResult{}, &nodeExecutorInvocationError{
			safeMessage:     "node executor returned a technical error",
			technicalDetail: err.Error(),
		}
	}
	return result, nil
}

func executeSynchronousNodeAttempts(
	prepared *preparedExecution,
	nodeID workflow.NodeID,
	nodeExecution *execution.NodeExecution,
	executor runtime.NodeExecutor,
	input runtime.NodeInput,
	incomingEdgeIDs []workflow.EdgeID,
	configuration workflow.JSONObject,
	pluginIdentity string,
) (synchronousAttemptOutcome, error) {
	currentAttempt := execution.InitialAttemptNumber
	for {
		if contextErr := prepared.executionContext.Err(); contextErr != nil {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution, contextErr)
		}
		nodeContext, err := runtime.NewNodeExecutionContext(
			prepared.executionContext, *nodeExecution, incomingEdgeIDs,
		)
		if err != nil {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution,
				fmt.Errorf("create execution context for node %s: %w", nodeID, err),
			)
		}
		result, invocationError := invokeSynchronousNodeExecutor(
			executor, nodeContext, input, configuration,
		)
		if invocationError != nil {
			failure, err := newNodeExecutorInvocationFailure(
				nodeID, pluginIdentity, invocationError,
			)
			if err != nil {
				return synchronousAttemptOutcome{}, fmt.Errorf(
					"%w; create node executor failure: %v", invocationError, err)
			}
			if err := transitionNodeToTerminal(
				prepared, nodeID, nodeExecution,
				execution.NodeExecutionStatusFailed,
				nodeTransitionObservationDetails{
					failure:         failure,
					hasFailure:      true,
					technicalDetail: invocationError.TechnicalDetail(),
				},
			); err != nil {
				return synchronousAttemptOutcome{}, fmt.Errorf(
					"%w; persist node executor failure: %v", invocationError, err)
			}
			return synchronousAttemptOutcome{}, invocationError
		}
		if contextErr := prepared.executionContext.Err(); contextErr != nil {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution, contextErr)
		}
		if !result.IsValid() {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution,
				fmt.Errorf("executor %s returned an invalid result for node %s",
					pluginIdentity, nodeID),
			)
		}
		if !result.IsFailure() {
			return synchronousAttemptOutcome{result: result}, nil
		}
		failure, exists := result.Failure()
		if !exists || !failure.IsValid() {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution,
				fmt.Errorf("executor %s returned a failed result without a valid RuntimeFailure for node %s",
					pluginIdentity, nodeID),
			)
		}
		policy, hasPolicy := prepared.dependencies.RetryPolicy()
		if !hasPolicy {
			return synchronousAttemptOutcome{result: result}, nil
		}
		decisionTime, err := executionTime(
			prepared.dependencies.Clock(), "nodeExecution.retryDecisionAt",
		)
		if err != nil {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution, err)
		}
		retryContext := prepared.executionContext.Context()
		if contextErr := retryContext.Err(); contextErr != nil {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution, contextErr)
		}
		var deadline *time.Time
		if value, exists := retryContext.Deadline(); exists {
			normalized := value.UTC()
			deadline = &normalized
		}
		decision, err := DecideRetry(
			policy, currentAttempt, failure, decisionTime, deadline,
		)
		if err != nil {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution,
				fmt.Errorf("decide retry for node %s: %w", nodeID, err),
			)
		}
		if decision.Kind() != execution.RetryDecisionRetry {
			return synchronousAttemptOutcome{
				result:           result,
				retryDecision:    decision,
				hasRetryDecision: true,
			}, nil
		}
		backoff, hasBackoff := decision.Backoff()
		nextAttempt, hasNextAttempt := decision.NextAttempt()
		if !hasBackoff || !hasNextAttempt {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution,
				errors.New("retry decision is missing retry scheduling values"),
			)
		}
		nextAttemptAt := decisionTime.Add(backoff)
		if !nextAttemptAt.After(decisionTime) {
			return synchronousAttemptOutcome{}, completeActiveNodeWithError(
				prepared, nodeID, nodeExecution,
				errors.New("retry next attempt time must be after decision time"),
			)
		}
		if err := applyRecordedNodeTransition(
			prepared, nodeID, nodeExecution, decisionTime,
			func(candidate *execution.NodeExecution) error {
				return candidate.ScheduleRetry(decisionTime)
			},
			nodeTransitionObservationDetails{
				result:           result,
				hasResult:        true,
				failure:          failure,
				hasFailure:       true,
				retryDecision:    decision,
				hasRetryDecision: true,
				nextAttemptAt:    nextAttemptAt,
			},
		); err != nil {
			return synchronousAttemptOutcome{}, fmt.Errorf(
				"schedule retry for node %s: %w", nodeID, err)
		}
		if contextErr := retryContext.Err(); contextErr != nil {
			return synchronousAttemptOutcome{}, terminalizeRetryPendingForContext(
				prepared, nodeID, nodeExecution, contextErr)
		}
		waitErr := prepared.dependencies.RetryWaiter().Wait(
			retryContext, backoff,
		)
		if waitErr != nil {
			if contextErr := retryContext.Err(); contextErr != nil {
				return synchronousAttemptOutcome{}, terminalizeRetryPendingForContext(
					prepared, nodeID, nodeExecution, contextErr)
			}
			return synchronousAttemptOutcome{}, failRetryPendingForWaiter(
				prepared, nodeID, nodeExecution, pluginIdentity,
				failureCodeRetryWaitFailed, "Retry wait failed",
				"retry wait failed", waitErr.Error(),
			)
		}
		if contextErr := retryContext.Err(); contextErr != nil {
			return synchronousAttemptOutcome{}, terminalizeRetryPendingForContext(
				prepared, nodeID, nodeExecution, contextErr)
		}
		resumeAt, err := executionTime(
			prepared.dependencies.Clock(), "nodeExecution.retryResumedAt",
		)
		if err != nil {
			return synchronousAttemptOutcome{}, failRetryPendingForWaiter(
				prepared, nodeID, nodeExecution, pluginIdentity,
				failureCodeRetryWaitFailed, "Retry wait failed",
				"retry wait failed", err.Error(),
			)
		}
		if resumeAt.Before(nextAttemptAt) {
			return synchronousAttemptOutcome{}, failRetryPendingForWaiter(
				prepared, nodeID, nodeExecution, pluginIdentity,
				failureCodeRetryWaitReturnedEarly,
				"Retry wait returned early",
				"retry wait returned early",
				fmt.Sprintf("retry waiter returned at %s before %s",
					resumeAt.Format(time.RFC3339Nano),
					nextAttemptAt.Format(time.RFC3339Nano)),
			)
		}
		if err := applyRecordedNodeTransition(
			prepared, nodeID, nodeExecution, resumeAt,
			func(candidate *execution.NodeExecution) error {
				return candidate.ResumeRetry(resumeAt)
			},
			nodeTransitionObservationDetails{},
		); err != nil {
			return synchronousAttemptOutcome{}, fmt.Errorf(
				"resume retry for node %s: %w", nodeID, err)
		}
		currentAttempt = nextAttempt
	}
}

func newNodeExecutorInvocationFailure(
	nodeID workflow.NodeID,
	pluginIdentity string,
	invocationError *nodeExecutorInvocationError,
) (runtime.RuntimeFailure, error) {
	code := failureCodeNodeExecutorError
	message := "Node executor returned a technical error"
	if invocationError.Panicked() {
		code = failureCodeNodeExecutorPanic
		message = "Node executor panicked"
	}
	return runtime.NewRuntimeFailure(
		runtime.FailureCategoryInternal,
		code,
		message,
		false,
		map[string]string{
			"nodeID":         nodeID.String(),
			"pluginIdentity": pluginIdentity,
		},
	)
}

func terminalizeRetryPendingForContext(
	prepared *preparedExecution,
	nodeID workflow.NodeID,
	nodeExecution *execution.NodeExecution,
	contextErr error,
) error {
	targetStatus := execution.NodeExecutionStatusCancelled
	if errors.Is(contextErr, context.DeadlineExceeded) {
		targetStatus = execution.NodeExecutionStatusTimedOut
	}
	failure, err := newObservedTechnicalNodeFailure(
		nodeID, targetStatus, contextErr,
	)
	if err != nil {
		return fmt.Errorf("%w; create retry cleanup failure: %v", contextErr, err)
	}
	if err := transitionNodeToTerminal(
		prepared, nodeID, nodeExecution, targetStatus,
		nodeTransitionObservationDetails{
			failure:           failure,
			hasFailure:        true,
			technicalDetail:   contextErr.Error(),
			useCleanupContext: true,
		},
	); err != nil {
		return fmt.Errorf(
			"%w; terminalize retry-pending node %s: %v",
			contextErr, nodeID, err,
		)
	}
	return contextErr
}

func failRetryPendingForWaiter(
	prepared *preparedExecution,
	nodeID workflow.NodeID,
	nodeExecution *execution.NodeExecution,
	pluginIdentity string,
	code string,
	failureMessage string,
	safeError string,
	technicalDetail string,
) error {
	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryInternal,
		code,
		failureMessage,
		false,
		map[string]string{
			"nodeID":         nodeID.String(),
			"pluginIdentity": pluginIdentity,
		},
	)
	if err != nil {
		return fmt.Errorf("%s; create retry wait failure: %v", safeError, err)
	}
	if err := transitionNodeToTerminal(
		prepared, nodeID, nodeExecution,
		execution.NodeExecutionStatusFailed,
		nodeTransitionObservationDetails{
			failure:           failure,
			hasFailure:        true,
			technicalDetail:   technicalDetail,
			useCleanupContext: prepared.executionContext.Err() != nil,
		},
	); err != nil {
		return fmt.Errorf(
			"%s; persist retry wait failure: %v", safeError, err)
	}
	return errors.New(safeError)
}
