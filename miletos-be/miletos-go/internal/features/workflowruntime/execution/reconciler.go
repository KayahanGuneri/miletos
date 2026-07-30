package execution

import (
	"context"
	"errors"
	"time"

	"miletos-go/internal/features/workflowruntime/execution/model"
	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/workflow"
)

type Reconciler struct {
	executions        *repository.ExecutionRepository
	workflows         *workflow.WorkflowRepository
	scheduler         *Scheduler
	processor         *NodeProcessor
	outbox            *repository.OutboxRepository
	commandTopic      string
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
	queuedStale time.Duration,
	runningStale time.Duration,
	retryPendingStale time.Duration,
) *Reconciler {
	return &Reconciler{
		executions:        executions,
		workflows:         workflows,
		scheduler:         scheduler,
		processor:         processor,
		outbox:            outbox,
		commandTopic:      commandTopic,
		queuedStale:       queuedStale,
		runningStale:      runningStale,
		retryPendingStale: retryPendingStale,
	}
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
		reserved, reserveErr := reconciler.executions.ReserveQueuedRepublish(ctx, node)
		if reserveErr != nil || !reserved {
			return reserveErr
		}
		return reconciler.scheduler.push(ctx, job)
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
