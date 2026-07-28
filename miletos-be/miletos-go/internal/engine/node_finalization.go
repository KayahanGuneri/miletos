package engine

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

const failureCodeNodeSchedulerTerminalized = "NODE_SCHEDULER_TERMINALIZED"

type nodeExecutionSummary struct {
	pending      int
	ready        int
	queued       int
	running      int
	retryPending int
	succeeded    int
	failed       int
	skipped      int
	cancelled    int
	timedOut     int
}

func (
	summary nodeExecutionSummary) NonTerminalCount() int {
	return summary.pending +
		summary.ready + summary.queued + summary.running +
		summary.retryPending
}
func (
	summary nodeExecutionSummary) TerminalCount() int {
	return summary.succeeded +
		summary.failed + summary.skipped + summary.cancelled +
		summary.timedOut
}
func skipBlockedPendingNodes(prepared *preparedExecution) (
	int, error) {
	if prepared == nil {
		return 0, newValidationError(
			"preparedExecution", "must not be nil")
	}
	skippedCount := 0
	for _, nodeID := range prepared.nodeExecutionOrder {
		nodeExecution, exists :=
			prepared.nodeExecutionsByNode[nodeID]
		if !exists ||
			nodeExecution == nil {
			return skippedCount, fmt.Errorf(
				"node execution for node %s is unavailable", nodeID)
		}
		if nodeExecution.Status() !=
			execution.NodeExecutionStatusPending {
			continue
		}
		assessment, err := assessNodeReadiness(
			prepared, nodeID)
		if err != nil {
			return skippedCount, fmt.Errorf(
				"assess blocked state for node %s: %w", nodeID, err,
			)
		}
		if !assessment.IsBlocked() {
			continue
		}
		skippedAt, err := executionTime(prepared.dependencies.Clock(),
			"nodeExecution.skippedAt")
		if err != nil {
			return skippedCount, err
		}
		if err := applyRecordedNodeTransition(prepared, nodeID,
			nodeExecution, skippedAt, func(
				candidate *execution.NodeExecution) error {
				return candidate.Skip(
					skippedAt)
			},
			nodeTransitionObservationDetails{}); err != nil {
			return skippedCount,
				fmt.Errorf("skip blocked node %s: %w", nodeID,
					err)
		}
		skippedCount++
	}
	return skippedCount, nil
}
func summarizeNodeExecutions(prepared *preparedExecution,
) (nodeExecutionSummary, error,
) {
	if prepared == nil {
		return nodeExecutionSummary{},
			newValidationError("preparedExecution", "must not be nil")
	}
	summary := nodeExecutionSummary{}
	for _, nodeID := range prepared.nodeExecutionOrder {
		nodeExecution, exists := prepared.nodeExecutionsByNode[nodeID]
		if !exists || nodeExecution == nil {
			return nodeExecutionSummary{},
				fmt.Errorf("node execution for node %s is unavailable", nodeID)
		}
		switch nodeExecution.Status() {
		case execution.NodeExecutionStatusPending:
			summary.pending++
		case execution.NodeExecutionStatusReady:
			summary.ready++
		case execution.NodeExecutionStatusQueued:
			summary.queued++
		case execution.NodeExecutionStatusRunning:
			summary.running++
		case execution.NodeExecutionStatusRetryPending:
			summary.retryPending++
		case execution.NodeExecutionStatusSucceeded:
			summary.succeeded++
		case execution.NodeExecutionStatusFailed:
			summary.failed++
		case execution.NodeExecutionStatusSkipped:
			summary.skipped++
		case execution.NodeExecutionStatusCancelled:
			summary.cancelled++
		case execution.NodeExecutionStatusTimedOut:
			summary.timedOut++
		default:
			return nodeExecutionSummary{},
				fmt.Errorf("node %s contains unsupported execution status %q", nodeID,
					nodeExecution.Status())
		}
	}
	return summary, nil
}
func allNodeExecutionsTerminal(
	prepared *preparedExecution) (bool,
	error) {
	summary, err :=
		summarizeNodeExecutions(prepared)
	if err != nil {
		return false, err
	}
	return summary.NonTerminalCount() == 0, nil
}
func nodeExecutionStarted(
	prepared *preparedExecution, nodeID workflow.NodeID) (
	bool, error) {
	if prepared == nil {
		return false, newValidationError(
			"preparedExecution", "must not be nil")
	}
	nodeExecution, exists :=
		prepared.nodeExecutionsByNode[nodeID]
	if !exists ||
		nodeExecution == nil {
		return false, fmt.Errorf(
			"node execution for node %s is unavailable", nodeID)
	}
	startedAt, started :=
		nodeExecution.StartedAt()
	return started &&
		!startedAt.IsZero(), nil
}
func terminalizeRemainingNodesForContext(prepared *preparedExecution,
	contextErr error) error {
	if prepared == nil {
		return newValidationError("preparedExecution", "must not be nil")
	}
	if contextErr == nil {
		return newValidationError("contextError",
			"must not be nil")
	}
	isTimeout := errors.Is(
		contextErr, context.DeadlineExceeded)
	for _, nodeID := range prepared.nodeExecutionOrder {
		nodeExecution, exists :=
			prepared.nodeExecutionsByNode[nodeID]
		if !exists ||
			nodeExecution == nil {
			return fmt.Errorf("node execution for node %s is unavailable",
				nodeID)
		}
		if nodeExecution.IsTerminal() {
			continue
		}
		if isTimeout {
			if err := timeoutNodeExecution(prepared, nodeID,
				nodeExecution, contextErr); err != nil {
				return err
			}
			continue
		}
		cancelledAt, err := executionTime(prepared.dependencies.Clock(), "nodeExecution.cancelledAt")
		if err != nil {
			return err
		}
		failure, err :=
			newObservedTechnicalNodeFailure(nodeID, execution.NodeExecutionStatusCancelled,
				contextErr)
		if err != nil {
			return fmt.Errorf("create cancellation failure for node %s: %w", nodeID,
				err)
		}
		if err := applyRecordedNodeTransition(prepared,
			nodeID, nodeExecution, cancelledAt,
			func(candidate *execution.NodeExecution) error {
				return candidate.Cancel(cancelledAt)
			}, nodeTransitionObservationDetails{failure: failure,
				hasFailure:      true,
				technicalDetail: contextErr.Error(), useCleanupContext: true,
			}); err != nil {
			return fmt.Errorf(
				"cancel node %s: %w", nodeID, err,
			)
		}
	}
	return nil
}
func timeoutNodeExecution(prepared *preparedExecution,
	nodeID workflow.NodeID, nodeExecution *execution.NodeExecution, cause error,
) error {
	if prepared == nil {
		return newValidationError(
			"preparedExecution", "must not be nil")
	}
	if nodeExecution == nil {
		return newValidationError("nodeExecution", "must not be nil")
	}
	if cause == nil {
		cause = context.DeadlineExceeded
	}
	if nodeExecution.Status() ==
		execution.NodeExecutionStatusPending {
		readyAt, err := executionTime(prepared.dependencies.Clock(),
			"nodeExecution.readyAt")
		if err != nil {
			return err
		}
		if err := applyRecordedNodeTransition(prepared, nodeID,
			nodeExecution, readyAt, func(
				candidate *execution.NodeExecution) error {
				return candidate.MarkReady(
					readyAt)
			},
			nodeTransitionObservationDetails{useCleanupContext: true},
		); err != nil {
			return fmt.Errorf("mark node %s ready before timeout: %w",
				nodeID, err)
		}
	}
	timedOutAt, err := executionTime(prepared.dependencies.Clock(), "nodeExecution.timedOutAt")
	if err != nil {
		return err
	}
	failure, err :=
		newObservedTechnicalNodeFailure(nodeID, execution.NodeExecutionStatusTimedOut,
			cause)
	if err != nil {
		return fmt.Errorf("create timeout failure for node %s: %w", nodeID,
			err)
	}
	if err := applyRecordedNodeTransition(prepared,
		nodeID, nodeExecution, timedOutAt,
		func(candidate *execution.NodeExecution) error {
			return candidate.Timeout(timedOutAt)
		}, nodeTransitionObservationDetails{failure: failure,
			hasFailure:      true,
			technicalDetail: cause.Error(), useCleanupContext: true,
		}); err != nil {
		return fmt.Errorf(
			"timeout node %s: %w", nodeID, err,
		)
	}
	return nil
}
func terminalizeRemainingNodesForFailure(prepared *preparedExecution) error {
	if prepared == nil {
		return newValidationError("preparedExecution",
			"must not be nil")
	}
	useCleanupContext := prepared.executionContext != nil &&
		prepared.executionContext.Err() != nil
	for _, nodeID := range prepared.nodeExecutionOrder {
		nodeExecution, exists := prepared.nodeExecutionsByNode[nodeID]
		if !exists || nodeExecution == nil {
			return fmt.Errorf(
				"node execution for node %s is unavailable", nodeID)
		}
		if nodeExecution.IsTerminal() {
			continue
		}
		finishedAt, err := executionTime(prepared.dependencies.Clock(), "nodeExecution.finishedAt")
		if err != nil {
			return err
		}
		switch nodeExecution.Status() {
		case execution.NodeExecutionStatusPending, execution.NodeExecutionStatusReady:
			if err := applyRecordedNodeTransition(prepared, nodeID,
				nodeExecution, finishedAt, func(
					candidate *execution.NodeExecution) error {
					return candidate.Skip(
						finishedAt)
				},
				nodeTransitionObservationDetails{useCleanupContext: useCleanupContext},
			); err != nil {
				return fmt.Errorf("skip remaining node %s: %w",
					nodeID, err)
			}
		case execution.NodeExecutionStatusQueued,
			execution.NodeExecutionStatusRunning,
			execution.NodeExecutionStatusRetryPending:
			failure, err :=
				newSchedulerTerminalizationFailure(nodeID, nodeExecution.Status())
			if err != nil {
				return fmt.Errorf(
					"create scheduler terminalization failure for node %s: %w", nodeID, err,
				)
			}
			if err := applyRecordedNodeTransition(prepared, nodeID,
				nodeExecution, finishedAt, func(
					candidate *execution.NodeExecution) error {
					return candidate.Fail(
						finishedAt)
				},
				nodeTransitionObservationDetails{failure: failure,
					hasFailure: true, technicalDetail: "scheduler terminalized a remaining non-terminal node",
					useCleanupContext: useCleanupContext},
			); err != nil {
				return fmt.Errorf("fail remaining node %s: %w",
					nodeID, err)
			}
		default:
			return fmt.Errorf("cannot terminalize node %s from unsupported status %q", nodeID,
				nodeExecution.Status())
		}
	}
	return nil
}
func newSchedulerTerminalizationFailure(
	nodeID workflow.NodeID, previousStatus execution.NodeExecutionStatus) (
	runtime.RuntimeFailure, error) {
	return runtime.NewRuntimeFailure(runtime.FailureCategoryInternal, failureCodeNodeSchedulerTerminalized,
		"Node execution stopped because the workflow scheduler failed", false, map[string]string{
			"nodeID": nodeID.String(), "previousStatus": previousStatus.String(),
		})
}
