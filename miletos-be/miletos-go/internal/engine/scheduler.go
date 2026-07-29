package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"miletos-go/internal/engine/modules"
	"miletos-go/internal/model"
	"miletos-go/internal/queue"
	"miletos-go/internal/repository"
)

type Scheduler struct {
	workflows  *repository.WorkflowRepository
	executions *repository.ExecutionRepository
	queue      queue.Queue
	topic      string
	registry   *NodeRegistry
}

func NewScheduler(
	workflows *repository.WorkflowRepository,
	executions *repository.ExecutionRepository,
	nodeQueue queue.Queue,
	topic string,
	registries ...*NodeRegistry,
) *Scheduler {
	scheduler := &Scheduler{
		workflows: workflows, executions: executions, queue: nodeQueue, topic: topic,
	}
	if len(registries) > 0 {
		scheduler.registry = registries[0]
	}
	return scheduler
}

func (scheduler *Scheduler) Activate(
	ctx context.Context,
	execution model.Execution,
	initialVariables map[string]any,
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
	if err := scheduler.executions.MarkExecutionQueued(ctx, persisted.ID); err != nil {
		return 0, err
	}
	return scheduler.scheduleReady(ctx, persisted, snapshot.Workflow, initialVariables)
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

func isTerminalExecutionStatus(status string) bool {
	switch status {
	case model.ExecutionSucceeded, model.ExecutionFailed, model.ExecutionCancelled,
		model.ExecutionRejected, model.ExecutionTimedOut:
		return true
	default:
		return false
	}
}

func (scheduler *Scheduler) Blocked(
	workflow model.Workflow,
	nodeID string,
	failed map[string]bool,
) bool {
	for _, predecessorID := range modules.Predecessors(workflow, nodeID) {
		if failed[predecessorID] {
			return true
		}
	}
	return false
}

func (scheduler *Scheduler) scheduleReady(
	ctx context.Context,
	execution model.Execution,
	workflow model.Workflow,
	initialVariables map[string]any,
) (int, error) {
	if scheduler.registry != nil {
		if err := modules.ValidateWorkflowDefinition(
			workflow,
			scheduler.registry.Definition,
			scheduler.registry.HasType,
			scheduler.registry.ValidateConfiguration,
		); err != nil {
			var validationError *modules.WorkflowValidationError
			if errors.As(err, &validationError) {
				return 0, &WorkflowValidationError{Issues: validationError.Issues}
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
		predecessors := modules.Predecessors(workflow, node.ID)
		if state.Status == model.NodeQueued {
			payload := inputForNode(workflow, node.ID, byNodeID)
			if len(predecessors) == 0 && initialVariables != nil {
				payload = initialVariables
			}
			job := model.NodeJob{
				CompanyID: execution.CompanyID, WorkflowID: execution.WorkflowID,
				ExecutionID: execution.ID, NodeID: node.ID, NodeExecutionID: state.ID,
				Attempt: state.Attempt, CorrelationID: execution.CorrelationID, Payload: payload,
			}
			if err := scheduler.push(ctx, job); err != nil {
				return scheduled, err
			}
			scheduled++
			continue
		}
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
		payload := inputForNode(workflow, node.ID, byNodeID)
		if len(predecessors) == 0 && initialVariables != nil {
			payload = initialVariables
		}
		queued, changed, err := scheduler.executions.MarkNodeQueued(
			ctx, execution.CompanyID, execution.ID, node.ID,
		)
		if err != nil {
			return scheduled, err
		}
		if !changed {
			continue
		}
		job := model.NodeJob{
			CompanyID:       execution.CompanyID,
			WorkflowID:      execution.WorkflowID,
			ExecutionID:     execution.ID,
			NodeID:          node.ID,
			NodeExecutionID: queued.ID,
			Attempt:         queued.Attempt,
			CorrelationID:   execution.CorrelationID,
			Payload:         payload,
		}
		if err := scheduler.push(ctx, job); err != nil {
			if resetErr := scheduler.executions.ResetNodeQueue(
				ctx, execution.CompanyID, execution.ID, node.ID,
			); resetErr != nil {
				return scheduled, fmt.Errorf(
					"queue node failed and queued state could not be reset: %w",
					resetErr,
				)
			}
			return scheduled, err
		}
		scheduled++
	}
	return scheduled, nil
}

func (scheduler *Scheduler) Finalize(
	ctx context.Context,
	execution model.Execution,
	workflow model.Workflow,
) error {
	states, err := scheduler.executions.ListAllNodes(ctx, execution.CompanyID, execution.ID)
	if err != nil {
		return err
	}
	failed := false
	active := false
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
	if failed {
		blocked := modules.Downstream(workflow, failedNodeIDs)
		hasUnblockedPending := false
		for _, state := range states {
			if state.Status != model.NodePending {
				continue
			}
			if blocked[state.NodeID] {
				if err := scheduler.executions.MarkNodeSkipped(
					ctx, execution.CompanyID, execution.ID, state.NodeID,
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
			ctx, execution.ID, model.ExecutionFailed, map[string]any{},
			map[string]any{"message": "One or more nodes failed"},
		)
	}
	outputs := terminalOutputs(workflow, byNodeID)
	return scheduler.executions.Finalize(
		ctx, execution.ID, model.ExecutionSucceeded, outputs, nil,
	)
}

func (scheduler *Scheduler) push(ctx context.Context, job model.NodeJob) error {
	encoded, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode node job: %w", err)
	}
	if err := scheduler.queue.Push(ctx, scheduler.topic, job.ExecutionID, encoded); err != nil {
		return fmt.Errorf("queue node %s: %w", job.NodeID, err)
	}
	return nil
}

func inputForNode(
	workflow model.Workflow,
	nodeID string,
	states map[string]model.NodeExecution,
) any {
	inputs := make(map[string]any)
	for _, edge := range workflow.Edges {
		if edge.TargetNodeID != nodeID {
			continue
		}
		output := states[edge.SourceNodeID].Output
		value := any(output)
		if unwrapped, exists := output["value"]; exists && len(output) == 1 {
			value = unwrapped
		}
		inputs[edge.TargetInputPort] = value
	}
	if len(inputs) == 1 {
		for _, value := range inputs {
			return value
		}
	}
	if len(inputs) == 0 {
		return nil
	}
	return inputs
}

func terminalOutputs(
	workflow model.Workflow,
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
