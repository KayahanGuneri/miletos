package execution

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/queue"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
)

type Scheduler struct {
	workflows  *workflowfeature.WorkflowRepository
	executions *repository.ExecutionRepository
	queue      queue.Queue
	topic      string
	registry   *plugin.NodeRegistry
}

func NewScheduler(
	workflows *workflowfeature.WorkflowRepository,
	executions *repository.ExecutionRepository,
	nodeQueue queue.Queue,
	topic string,
	registry *plugin.NodeRegistry,
) *Scheduler {
	return &Scheduler{
		workflows: workflows, executions: executions, queue: nodeQueue, topic: topic,
		registry: registry,
	}
}

func (scheduler *Scheduler) Activate(
	ctx context.Context,
	execution model.Execution,
	startInput map[string]any,
) (int, error) {
	persisted, err := scheduler.executions.FindByID(ctx, execution.CompanyID, execution.ID)
	if err != nil {
		return 0, fmt.Errorf("load execution for scheduling: %w", err)
	}
	if isTerminalExecutionStatus(persisted.Status) {
		return 0, nil
	}
	if scheduler.queue == nil {
		return 0, fmt.Errorf("asynchronous execution is unavailable")
	}
	snapshot, err := scheduler.workflows.FindByExecutionID(ctx, persisted.CompanyID, persisted.ID)
	if err != nil {
		return 0, fmt.Errorf("load workflow for scheduling: %w", err)
	}
	if err := scheduler.executions.MarkExecutionQueued(
		ctx, persisted.CompanyID, persisted.ID,
	); err != nil {
		return 0, err
	}
	return scheduler.scheduleReady(ctx, persisted, snapshot.Workflow, startInput)
}

func (scheduler *Scheduler) CompleteNode(ctx context.Context, job model.NodeJob) error {
	execution, err := scheduler.executions.FindByID(ctx, job.CompanyID, job.ExecutionID)
	if err != nil {
		return fmt.Errorf("load execution after node completion: %w", err)
	}
	if isTerminalExecutionStatus(execution.Status) {
		return nil
	}
	snapshot, err := scheduler.workflows.FindByExecutionID(ctx, job.CompanyID, job.ExecutionID)
	if err != nil {
		return fmt.Errorf("load workflow after node completion: %w", err)
	}
	scheduled, err := scheduler.scheduleReady(ctx, execution, snapshot.Workflow, nil)
	if err != nil || scheduled > 0 {
		return err
	}
	return scheduler.Finalize(ctx, execution, snapshot.Workflow)
}

func isTerminalExecutionStatus(status model.ExecutionStatus) bool {
	switch status {
	case model.ExecutionSucceeded, model.ExecutionFailed, model.ExecutionCancelled,
		model.ExecutionRejected, model.ExecutionTimedOut:
		return true
	default:
		return false
	}
}

func (scheduler *Scheduler) Blocked(
	workflow workflowfeature.Workflow,
	nodeID string,
	failed map[string]bool,
) bool {
	for _, predecessorID := range workflowfeature.Predecessors(workflow, nodeID) {
		if failed[predecessorID] {
			return true
		}
	}
	return false
}

func (scheduler *Scheduler) scheduleReady(
	ctx context.Context,
	execution model.Execution,
	workflow workflowfeature.Workflow,
	startInput map[string]any,
) (int, error) {
	if scheduler.registry != nil {
		if err := workflowfeature.ValidateWorkflowDefinition(
			workflow,
			scheduler.registry.Definition,
			scheduler.registry.HasType,
			scheduler.registry.ValidateConfiguration,
		); err != nil {
			var validationError *workflowfeature.WorkflowDefinitionValidationError
			if errors.As(err, &validationError) {
				return 0, &workflowfeature.WorkflowValidationError{Issues: validationError.Issues}
			}
			return 0, err
		}
	}
	states, err := scheduler.executions.ListAllNodes(ctx, execution.CompanyID, execution.ID)
	if err != nil {
		return 0, fmt.Errorf("load node states: %w", err)
	}
	byNodeID := make(map[string]model.NodeExecution, len(states))
	for _, state := range states {
		byNodeID[state.NodeID] = state
	}
	scheduled := 0
	for _, node := range workflow.Nodes {
		state := byNodeID[node.ID]
		predecessors := workflowfeature.Predecessors(workflow, node.ID)
		if state.Status != model.NodePending {
			continue
		}
		ready := true
		for _, predecessorID := range predecessors {
			if byNodeID[predecessorID].Status != model.NodeSucceeded {
				ready = false
				break
			}
		}
		if !ready {
			continue
		}
		payload := buildExecutionNodeInput(
			workflow, node.ID, nodeOutputs(byNodeID), scheduler.registry, startInput,
			execution.Origin,
		)
		_, changed, err := scheduler.executions.QueueNodeCommand(
			ctx,
			execution,
			node.ID,
			payload,
			scheduler.topic,
		)
		if err != nil {
			return scheduled, err
		}
		if !changed {
			continue
		}
		scheduled++
	}
	return scheduled, nil
}

func unwrapSummary(summary map[string]any) any {
	return normalizeOutput(summary)
}

func (scheduler *Scheduler) Finalize(
	ctx context.Context,
	execution model.Execution,
	workflow workflowfeature.Workflow,
) error {
	states, err := scheduler.executions.ListAllNodes(ctx, execution.CompanyID, execution.ID)
	if err != nil {
		return err
	}
	failed := false
	active := false
	pending := false
	invalidRetry := false
	failedNodeIDs := make([]string, 0)
	byNodeID := make(map[string]model.NodeExecution, len(states))
	for _, state := range states {
		byNodeID[state.NodeID] = state
		switch state.Status {
		case model.NodeFailed, model.NodeTimedOut, model.NodeCancelled:
			failed = true
			failedNodeIDs = append(failedNodeIDs, state.NodeID)
		case model.NodeQueued, model.NodeRunning:
			active = true
		case model.NodePending, model.NodeReady:
			pending = true
		case model.NodeRetryPending:
			if state.NextAttemptAt == nil {
				invalidRetry = true
			} else {
				active = true
			}
		}
	}
	if invalidRetry {
		return fmt.Errorf("%w: retry-pending node has no retry schedule", repository.ErrStateTransition)
	}
	if active {
		return nil
	}
	if pending && !failed {
		return fmt.Errorf("execution has pending nodes awaiting scheduling")
	}
	if failed {
		blocked := workflowfeature.Downstream(workflow, failedNodeIDs)
		hasUnblockedPending := false
		for _, state := range states {
			if state.Status != model.NodePending {
				continue
			}
			if blocked[state.NodeID] {
				if err := scheduler.executions.MarkNodeSkipped(
					ctx, execution.CompanyID, execution.ID, state.NodeID,
					string(model.SkipReasonDependencyFailed),
				); err != nil {
					return err
				}
			} else {
				hasUnblockedPending = true
			}
		}
		if hasUnblockedPending {
			return fmt.Errorf(
				"%w: execution has schedulable pending nodes",
				repository.ErrStateTransition,
			)
		}
		return scheduler.executions.Finalize(
			ctx, execution.CompanyID, execution.ID,
			model.ExecutionFailed, map[string]any{},
			map[string]any{"message": "One or more nodes failed"},
		)
	}
	outputs := terminalOutputs(workflow, byNodeID)
	return scheduler.executions.Finalize(
		ctx, execution.CompanyID, execution.ID,
		model.ExecutionSucceeded, outputs, nil,
	)
}

func nodeOutputs(states map[string]model.NodeExecution) map[string]any {
	outputs := make(map[string]any, len(states))
	for nodeID, state := range states {
		outputs[nodeID] = state.Output
	}
	return outputs
}

func terminalOutputs(
	workflow workflowfeature.Workflow,
	states map[string]model.NodeExecution,
) map[string]any {
	hasOutgoing := make(map[string]bool)
	for _, edge := range workflow.Edges {
		hasOutgoing[edge.SourceNodeID] = true
	}
	outputs := make(map[string]any)
	for _, node := range workflow.Nodes {
		state := states[node.ID]
		if !hasOutgoing[node.ID] && state.Status == model.NodeSucceeded {
			outputs[node.ID] = state.Output
		}
	}
	return outputs
}
