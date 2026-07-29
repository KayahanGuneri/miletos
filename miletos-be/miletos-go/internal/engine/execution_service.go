package engine

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/engine/modules"
	"miletos-go/internal/model"
	"miletos-go/internal/repository"
)

var ErrAsyncUnavailable = errors.New("asynchronous execution is unavailable")
var ErrSyncNonterminal = errors.New("synchronous execution did not reach a terminal state")

type ExecutionOutcome struct {
	Execution      model.Execution
	ScheduledRoots int
	Replayed       bool
}

type ExecutionService struct {
	workflows    *WorkflowService
	executions   *repository.ExecutionRepository
	scheduler    *Scheduler
	processor    *NodeProcessor
	asyncEnabled bool
}

func NewExecutionService(
	workflows *WorkflowService,
	executions *repository.ExecutionRepository,
	scheduler *Scheduler,
	processor *NodeProcessor,
	asyncEnabled bool,
) *ExecutionService {
	return &ExecutionService{
		workflows: workflows, executions: executions,
		scheduler: scheduler, processor: processor, asyncEnabled: asyncEnabled,
	}
}

func (service *ExecutionService) ExecuteAsync(
	ctx context.Context,
	workflow model.Workflow,
	initialVariables map[string]any,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (ExecutionOutcome, error) {
	if !service.asyncEnabled {
		return ExecutionOutcome{}, ErrAsyncUnavailable
	}
	if existing, found, err := service.executions.FindIdempotent(
		ctx, workflow.CompanyID, idempotencyKey, fingerprint,
	); err != nil || found {
		outcome := ExecutionOutcome{Execution: existing, Replayed: found}
		if isTerminalExecutionStatus(existing.Status) {
			return outcome, err
		}
		if err == nil && found {
			outcome.ScheduledRoots, err = service.scheduler.Activate(ctx, existing, initialVariables)
		}
		return outcome, err
	}
	snapshot, err := service.workflows.CreateWorkflow(ctx, workflow)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution, err := service.executions.Create(
		ctx, workflow, snapshot.ID, "ASYNC", correlationID, idempotencyKey, fingerprint,
	)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	scheduled, err := service.scheduler.Activate(ctx, execution, initialVariables)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution.Status = model.ExecutionQueued
	return ExecutionOutcome{Execution: execution, ScheduledRoots: scheduled}, nil
}

func (service *ExecutionService) ExecuteSync(
	ctx context.Context,
	workflow model.Workflow,
	initialVariables map[string]any,
	correlationID string,
) (ExecutionOutcome, error) {
	snapshot, err := service.workflows.CreateWorkflow(ctx, workflow)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution, err := service.executions.Create(
		ctx, workflow, snapshot.ID, "SYNC", correlationID, "", "",
	)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	if err := service.executions.MarkExecutionRunning(ctx, execution.ID); err != nil {
		return ExecutionOutcome{}, err
	}
	order, err := modules.TopologicalOrder(workflow)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	outputs := make(map[string]any)
	failed := make(map[string]bool)
	for _, nodeID := range order {
		node, _ := findWorkflowNode(workflow, nodeID)
		predecessors := modules.Predecessors(workflow, nodeID)
		if service.scheduler.Blocked(workflow, nodeID, failed) {
			failed[nodeID] = true
			if err := service.executions.MarkNodeSkipped(
				ctx, workflow.CompanyID, execution.ID, nodeID,
			); err != nil {
				return ExecutionOutcome{}, err
			}
			continue
		}
		state, changed, err := service.executions.MarkNodeQueued(
			ctx, workflow.CompanyID, execution.ID, nodeID,
		)
		if err != nil {
			return ExecutionOutcome{}, err
		}
		if !changed {
			continue
		}
		payload := syncInput(workflow, nodeID, outputs)
		if len(predecessors) == 0 {
			payload = initialVariables
		}
		job := model.NodeJob{
			CompanyID: workflow.CompanyID, WorkflowID: workflow.ID,
			ExecutionID: execution.ID, NodeID: nodeID, NodeExecutionID: state.ID,
			Attempt: state.Attempt, CorrelationID: correlationID, Payload: payload,
		}
		output, runErr := service.processor.ProcessSync(ctx, job, node)
		if runErr != nil {
			var nodeFailure *PersistedNodeFailure
			if errors.As(runErr, &nodeFailure) {
				failed[nodeID] = true
				continue
			}
			return ExecutionOutcome{}, runErr
		}
		outputs[nodeID] = output
	}
	if err := service.scheduler.Finalize(ctx, execution, workflow); err != nil {
		return ExecutionOutcome{}, err
	}
	completed, err := service.executions.FindByID(ctx, workflow.CompanyID, execution.ID)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	if completed.Status != model.ExecutionSucceeded &&
		completed.Status != model.ExecutionFailed {
		return ExecutionOutcome{}, fmt.Errorf(
			"%w: execution %s remained %s",
			ErrSyncNonterminal, completed.ID, completed.Status,
		)
	}
	return ExecutionOutcome{Execution: completed}, err
}

func syncInput(workflow model.Workflow, nodeID string, outputs map[string]any) any {
	values := make(map[string]any)
	for _, edge := range workflow.Edges {
		if edge.TargetNodeID == nodeID {
			values[edge.TargetInputPort] = outputs[edge.SourceNodeID]
		}
	}
	if len(values) == 1 {
		for _, value := range values {
			return value
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}
