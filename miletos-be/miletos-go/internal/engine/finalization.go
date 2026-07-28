package engine

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

func finalizePreparedExecutionFromNodeStates(prepared *preparedExecution,
	state *schedulerExecutionState) (ExecutionResult,
	error) {
	if state == nil {
		return ExecutionResult{}, newValidationError("schedulerExecutionState",
			"must not be nil")
	}
	summary, err := summarizeNodeExecutions(prepared)
	if err != nil {
		return failPreparedExecutionWithInternalError(
			prepared, state, fmt.Errorf(
				"summarize node execution states: %w", err),
		)
	}
	switch {
	case summary.failed > 0:
		failure, err := runtime.NewRuntimeFailure(runtime.FailureCategoryExecution,
			failureCodeWorkflowNodeFailure, "One or more workflow nodes failed", false,
			map[string]string{"failedNodeCount": fmt.Sprintf("%d",
				summary.failed),
				"skippedNodeCount": fmt.Sprintf("%d", summary.skipped), "recordedFailureCount": fmt.Sprintf(
					"%d", len(state.nodeFailures)),
			})
		if err != nil {
			return failPreparedExecutionWithInternalError(prepared, state,
				err)
		}
		if state.workflowFailure == nil {
			if err := state.recordWorkflowFailure(
				failure, false); err != nil {
				return failPreparedExecutionWithInternalError(prepared, state,
					err)
			}
		}
		if err := transitionWorkflowExecution(
			prepared, execution.WorkflowExecutionStatusFailed, state,
		); err != nil {
			return failPreparedExecutionWithInternalError(prepared,
				state, err)
		}
	case summary.timedOut > 0:
		failure, err := runtime.NewRuntimeFailure(
			runtime.FailureCategoryTimeout, failureCodeWorkflowTimedOut, "Workflow execution timed out",
			false, map[string]string{"timedOutNodeCount": fmt.Sprintf(
				"%d", summary.timedOut),
			})
		if err != nil {
			return failPreparedExecutionWithInternalError(prepared, state,
				err)
		}
		if state.workflowFailure == nil {
			if err := state.recordWorkflowFailure(
				failure, false); err != nil {
				return failPreparedExecutionWithInternalError(prepared, state,
					err)
			}
		}
		if err := transitionWorkflowExecution(
			prepared, execution.WorkflowExecutionStatusTimedOut, state,
		); err != nil {
			return failPreparedExecutionWithInternalError(prepared,
				state, err)
		}
	case summary.cancelled > 0:
		failure, err := runtime.NewRuntimeFailure(runtime.FailureCategoryCanceled, failureCodeWorkflowCanceled,
			"Workflow execution was canceled", false, map[string]string{
				"cancelledNodeCount": fmt.Sprintf("%d", summary.cancelled)})
		if err != nil {
			return failPreparedExecutionWithInternalError(prepared,
				state, err)
		}
		if state.workflowFailure == nil {
			if err := state.recordWorkflowFailure(failure, false); err != nil {
				return failPreparedExecutionWithInternalError(prepared,
					state, err)
			}
		}
		if err := transitionWorkflowExecution(prepared, execution.WorkflowExecutionStatusCancelled,
			state); err != nil {
			return failPreparedExecutionWithInternalError(
				prepared, state, err,
			)
		}
	default:
		if err := transitionWorkflowExecution(prepared, execution.WorkflowExecutionStatusSucceeded,
			state); err != nil {
			return failPreparedExecutionWithInternalError(
				prepared, state, err,
			)
		}
	}
	result, err := newExecutionResultFromPrepared(prepared,
		*state)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("build execution result: %w",
			err)
	}
	return result, nil
}
func finalizePreparedExecutionForContext(prepared *preparedExecution,
	state *schedulerExecutionState, contextErr error) (
	ExecutionResult, error) {
	if state == nil {
		return ExecutionResult{}, newValidationError(
			"schedulerExecutionState", "must not be nil")
	}
	if contextErr == nil {
		return ExecutionResult{}, newValidationError("contextError",
			"must not be nil")
	}
	if err := terminalizeRemainingNodesForContext(prepared,
		contextErr); err != nil {
		return failPreparedExecutionWithInternalError(
			prepared, state, fmt.Errorf(
				"terminalize nodes after context completion: %w", err),
		)
	}
	category := runtime.FailureCategoryCanceled
	code := failureCodeWorkflowCanceled
	message := "Workflow execution was canceled"
	targetStatus := execution.WorkflowExecutionStatusCancelled
	if errors.Is(contextErr, context.DeadlineExceeded) {
		category = runtime.FailureCategoryTimeout
		code = failureCodeWorkflowTimedOut
		message = "Workflow execution timed out"
		targetStatus = execution.WorkflowExecutionStatusTimedOut
	}
	failure, err := runtime.NewRuntimeFailure(
		category, code, message,
		false, map[string]string{"contextError": contextErr.Error()})
	if err != nil {
		return failPreparedExecutionWithInternalError(prepared, state,
			err)
	}
	if state.workflowFailure == nil {
		if err := state.recordWorkflowFailure(
			failure, false); err != nil {
			return failPreparedExecutionWithInternalError(prepared, state,
				err)
		}
	}
	if err := transitionWorkflowExecution(
		prepared, targetStatus, state,
	); err != nil {
		return failPreparedExecutionWithInternalError(prepared,
			state, err)
	}
	result, err := newExecutionResultFromPrepared(
		prepared, *state)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf(
			"build context-terminated execution result: %w", err)
	}
	return result, nil
}
func finalizeStalledPreparedExecution(
	prepared *preparedExecution, state *schedulerExecutionState) (
	ExecutionResult, error) {
	summary, err := summarizeNodeExecutions(prepared)
	if err != nil {
		return failPreparedExecutionWithInternalError(prepared,
			state, err)
	}
	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryInternal, failureCodeWorkflowStalled, "Workflow execution stalled because no node was ready",
		false, map[string]string{"pendingNodeCount": fmt.Sprintf(
			"%d", summary.pending),
			"readyNodeCount": fmt.Sprintf("%d",
				summary.ready),
			"queuedNodeCount": fmt.Sprintf("%d", summary.queued), "runningNodeCount": fmt.Sprintf(
				"%d", summary.running),
		})
	if err != nil {
		return failPreparedExecutionWithInternalError(prepared, state,
			err)
	}
	if err := state.recordWorkflowFailure(failure,
		true); err != nil {
		return failPreparedExecutionWithInternalError(
			prepared, state, err,
		)
	}
	if err := terminalizeRemainingNodesForFailure(prepared); err != nil {
		return failPreparedExecutionWithInternalError(prepared, state,
			fmt.Errorf("terminalize stalled node executions: %w", err))
	}
	if err := transitionWorkflowExecution(prepared,
		execution.WorkflowExecutionStatusFailed, state); err != nil {
		return failPreparedExecutionWithInternalError(prepared, state,
			err)
	}
	result, err := newExecutionResultFromPrepared(prepared,
		*state)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("build stalled execution result: %w",
			err)
	}
	return result, nil
}
func failPreparedExecutionWithInternalError(prepared *preparedExecution,
	state *schedulerExecutionState, cause error) (
	ExecutionResult, error) {
	if cause == nil {
		cause = errors.New("scheduler failed without an error")
	}
	if state == nil {
		newState := newSchedulerExecutionState()
		state = &newState
	}
	finalizationErrors := []error{
		cause}
	failure, failureErr := runtime.NewRuntimeFailure(runtime.FailureCategoryInternal,
		failureCodeSchedulerInternal, "Workflow scheduler encountered an internal error", false,
		map[string]string{"error": cause.Error()},
	)
	if failureErr != nil {
		finalizationErrors = append(finalizationErrors, failureErr)
	} else if state.workflowFailure == nil {
		if err := state.recordWorkflowFailure(
			failure, false); err != nil {
			finalizationErrors = append(finalizationErrors, err)
		}
	}
	if prepared != nil {
		if err := terminalizeRemainingNodesForFailure(
			prepared); err != nil {
			finalizationErrors = append(
				finalizationErrors, err)
		}
		if prepared.workflowExecution != nil &&
			!prepared.workflowExecution.IsTerminal() {
			if err := transitionWorkflowExecution(prepared,
				execution.WorkflowExecutionStatusFailed, state); err != nil {
				finalizationErrors = append(finalizationErrors, err)
			}
		}
		result, resultErr := newExecutionResultFromPrepared(
			prepared, *state)
		if resultErr == nil {
			return result,
				errors.Join(finalizationErrors...)
		}
		finalizationErrors = append(
			finalizationErrors, resultErr)
	}
	return ExecutionResult{},
		errors.Join(finalizationErrors...)
}
func newTechnicalNodeFailure(
	nodeID workflow.NodeID, cause error) (
	runtime.RuntimeFailure, error) {
	if cause == nil {
		return runtime.RuntimeFailure{}, newValidationError(
			"cause", "must not be nil")
	}
	return runtime.NewRuntimeFailure(
		runtime.FailureCategoryInternal, failureCodeNodeExecutionError, "Node executor returned a technical error",
		false, map[string]string{"nodeID": nodeID.String(),
			"error": cause.Error()},
	)
}

func transitionWorkflowExecution(prepared *preparedExecution,
	target execution.WorkflowExecutionStatus, states ...*schedulerExecutionState) error {
	if len(states) > 1 {
		return newValidationError("schedulerExecutionStates",
			"must contain at most one state")
	}
	var state *schedulerExecutionState
	if len(states) == 1 {
		state = states[0]
	}
	return applyRecordedWorkflowTransition(prepared,
		target, state)
}
