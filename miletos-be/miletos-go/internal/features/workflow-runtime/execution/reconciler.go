package execution

import (
	"context"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

type Reconciler struct {
	executions        *repository.ExecutionRepository
	workflows         *workflow.WorkflowRepository
	scheduler         *Scheduler
	processor         *NodeProcessor
	outbox            *repository.OutboxRepository
	commandTopic      string
	interval          time.Duration
	queuedStale       time.Duration
	runningStale      time.Duration
	retryPendingStale time.Duration
}

func NewReconciler(
	executions *repository.ExecutionRepository,
	workflows *workflow.WorkflowRepository,
	scheduler *Scheduler,
	processor *NodeProcessor,
	outbox *repository.OutboxRepository,
	commandTopic string,
	interval time.Duration,
	queuedStale time.Duration,
	runningStale time.Duration,
	retryPendingStale time.Duration,
) (*Reconciler, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("reconciliation interval must be positive")
	}
	if queuedStale <= 0 {
		return nil, fmt.Errorf("reconciliation queued-stale duration must be positive")
	}
	if runningStale <= 0 {
		return nil, fmt.Errorf("reconciliation running-stale duration must be positive")
	}
	if retryPendingStale <= 0 {
		return nil, fmt.Errorf("reconciliation retry-pending-stale duration must be positive")
	}
	return &Reconciler{
		executions:        executions,
		workflows:         workflows,
		scheduler:         scheduler,
		processor:         processor,
		outbox:            outbox,
		commandTopic:      commandTopic,
		interval:          interval,
		queuedStale:       queuedStale,
		runningStale:      runningStale,
		retryPendingStale: retryPendingStale,
	}, nil
}

func (reconciler *Reconciler) Interval() time.Duration {
	return reconciler.interval
}

func (reconciler *Reconciler) RunOnce(ctx context.Context) error {
	now := time.Now().UTC()
	nodes, err := reconciler.executions.FindStaleNodes(
		ctx,
		repository.ReconciliationThresholds{
			QueuedBefore:       now.Add(-reconciler.queuedStale),
			RunningBefore:      now.Add(-reconciler.runningStale),
			RetryPendingBefore: now.Add(-reconciler.retryPendingStale),
		},
		100,
	)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if err := reconciler.reconcileNode(ctx, node); err != nil &&
			!errors.Is(err, repository.ErrStateTransition) {
			return err
		}
	}
	_, err = reconciler.executions.FindStaleNodes(
		ctx,
		repository.ReconciliationThresholds{
			QueuedBefore:       time.Now().UTC().Add(-reconciler.queuedStale),
			RunningBefore:      time.Now().UTC().Add(-reconciler.runningStale),
			RetryPendingBefore: time.Now().UTC().Add(-reconciler.retryPendingStale),
		},
		1,
	)
	return err
}

func (reconciler *Reconciler) reconcileNode(
	ctx context.Context,
	node model.NodeExecution,
) error {
	execution, err := reconciler.executions.FindByID(
		ctx, node.CompanyID, node.ExecutionID,
	)
	if err != nil || isTerminalExecutionStatus(execution.Status) {
		return err
	}
	if _, err := reconciler.workflows.FindByExecutionID(
		ctx, node.CompanyID, node.ExecutionID,
	); err != nil {
		return err
	}
	job := model.NodeJob{
		CompanyID:       node.CompanyID,
		WorkflowID:      execution.WorkflowID,
		ExecutionID:     node.ExecutionID,
		NodeID:          node.NodeID,
		NodeExecutionID: node.ID,
		Attempt:         node.Attempt,
		CorrelationID:   execution.CorrelationID,
		Origin:          execution.Origin,
		Payload:         unwrapSummary(node.Input),
	}
	switch node.Status {
	case model.NodeQueued:
		_, repairErr := reconciler.executions.RepairQueuedNodeCommand(
			ctx,
			node,
			job,
			reconciler.commandTopic,
			time.Now().UTC(),
		)
		return repairErr
	case model.NodeRetryPending:
		if node.NextAttemptAt == nil {
			return repository.ErrStateTransition
		}
		return reconciler.outbox.EnsureRetryCommand(
			ctx, job, reconciler.commandTopic, *node.NextAttemptAt,
		)
	case model.NodeRunning:
		if err := reconciler.processor.PersistInterrupted(ctx, job); err != nil {
			return err
		}
		return reconciler.scheduler.CompleteNode(ctx, job)
	default:
		return nil
	}
}
