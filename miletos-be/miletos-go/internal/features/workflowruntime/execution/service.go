package execution

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/features/workflowruntime/execution/model"
	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/workflow"
)

var ErrAsyncUnavailable = errors.New("asynchronous execution is unavailable")
var ErrSyncNonterminal = errors.New("synchronous execution did not reach a terminal state")
var ErrInitialVariablesNotAccepted = errors.New("initial variables are not accepted")
var ErrInvalidExecutionOrigin = errors.New("execution origin is incompatible with the workflow")

type ExecutionOutcome struct {
	Execution      model.Execution
	ScheduledRoots int
	Replayed       bool
}

type ExecutionService struct {
	workflows    *workflow.WorkflowService
	executions   *repository.ExecutionRepository
	scheduler    *Scheduler
	processor    *NodeProcessor
	asyncEnabled bool
}

func NewExecutionService(
	workflows *workflow.WorkflowService,
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
	definition workflow.Workflow,
	initialVariables map[string]any,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (ExecutionOutcome, error) {
	if err := service.workflows.Validate(definition); err != nil {
		return ExecutionOutcome{}, err
	}
	if err := service.validateInput(
		definition, initialVariables, model.ExecutionOriginManualDirect,
	); err != nil {
		return ExecutionOutcome{}, err
	}
	if !service.asyncEnabled {
		return ExecutionOutcome{}, ErrAsyncUnavailable
	}
	if existing, found, err := service.executions.FindIdempotent(
		ctx, definition.CompanyID, idempotencyKey, fingerprint,
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
	snapshot, err := service.workflows.CreateWorkflow(ctx, definition)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution, err := service.executions.Create(
		ctx, definition, snapshot.ID, "ASYNC", model.ExecutionOriginManualDirect,
		correlationID, idempotencyKey, fingerprint,
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
	definition workflow.Workflow,
	initialVariables map[string]any,
	correlationID string,
	idempotency ...string,
) (ExecutionOutcome, error) {
	if err := service.workflows.Validate(definition); err != nil {
		return ExecutionOutcome{}, err
	}
	if err := service.validateInput(
		definition, initialVariables, model.ExecutionOriginManualDirect,
	); err != nil {
		return ExecutionOutcome{}, err
	}
	idempotencyKey := ""
	fingerprint := ""
	if len(idempotency) > 0 {
		idempotencyKey = idempotency[0]
	}
	if len(idempotency) > 1 {
		fingerprint = idempotency[1]
	}
	if existing, found, err := service.executions.FindIdempotent(
		ctx, definition.CompanyID, idempotencyKey, fingerprint,
	); err != nil || found {
		return ExecutionOutcome{
			Execution: existing,
			Replayed:  found,
		}, err
	}
	snapshot, err := service.workflows.CreateWorkflow(ctx, definition)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution, err := service.executions.Create(
		ctx, definition, snapshot.ID, "SYNC", model.ExecutionOriginManualDirect,
		correlationID, idempotencyKey, fingerprint,
	)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	if err := service.executions.MarkExecutionRunning(
		ctx, execution.CompanyID, execution.ID,
	); err != nil {
		return ExecutionOutcome{}, err
	}
	order, err := workflow.TopologicalOrder(definition)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	outputs := make(map[string]any)
	failed := make(map[string]bool)
	for _, nodeID := range order {
		node, _ := findWorkflowNode(definition, nodeID)
		predecessors := workflow.Predecessors(definition, nodeID)
		if service.scheduler.Blocked(definition, nodeID, failed) {
			failed[nodeID] = true
			if err := service.executions.MarkNodeSkipped(
				ctx, definition.CompanyID, execution.ID, nodeID,
			); err != nil {
				return ExecutionOutcome{}, err
			}
			continue
		}
		state, changed, err := service.executions.MarkNodeQueued(
			ctx, definition.CompanyID, execution.ID, nodeID,
		)
		if err != nil {
			return ExecutionOutcome{}, err
		}
		if !changed {
			continue
		}
		payload := BuildNodeInput(
			definition, nodeID, outputs, service.scheduler.registry,
		)
		if len(predecessors) == 0 {
			descriptor, _ := service.scheduler.registry.Definition(node.Type, node.Version)
			if descriptor.AcceptsInitialVariables {
				payload = initialVariables
			}
		}
		job := model.NodeJob{
			CompanyID: definition.CompanyID, WorkflowID: definition.ID,
			ExecutionID: execution.ID, NodeID: nodeID, NodeExecutionID: state.ID,
			Attempt: state.Attempt, CorrelationID: correlationID,
			Origin: model.ExecutionOriginManualDirect, Payload: payload,
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
	if err := service.scheduler.Finalize(ctx, execution, definition); err != nil {
		return ExecutionOutcome{}, err
	}
	completed, err := service.executions.FindByID(ctx, definition.CompanyID, execution.ID)
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

func (service *ExecutionService) ExecuteAsyncFromSnapshot(
	ctx context.Context,
	snapshot workflow.WorkflowSnapshot,
	triggerNodeID string,
	initialVariables map[string]any,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (ExecutionOutcome, error) {
	if !service.asyncEnabled {
		return ExecutionOutcome{}, ErrAsyncUnavailable
	}
	snapshotWorkflow := snapshot.Workflow
	if err := service.workflows.Validate(snapshotWorkflow); err != nil {
		return ExecutionOutcome{}, err
	}
	if err := service.validateInput(
		snapshotWorkflow, initialVariables, model.ExecutionOriginHTTPWebhook,
	); err != nil {
		return ExecutionOutcome{}, err
	}
	roots := make([]workflow.WorkflowNode, 0)
	for _, node := range snapshotWorkflow.Nodes {
		if len(workflow.Predecessors(snapshotWorkflow, node.ID)) == 0 {
			roots = append(roots, node)
		}
	}
	if len(roots) != 1 || roots[0].ID != triggerNodeID ||
		roots[0].Type != "core.http-trigger" {
		return ExecutionOutcome{}, ErrInvalidExecutionOrigin
	}
	if existing, found, err := service.executions.FindIdempotent(
		ctx, snapshotWorkflow.CompanyID, idempotencyKey, fingerprint,
	); err != nil || found {
		outcome := ExecutionOutcome{Execution: existing, Replayed: found}
		if found && err == nil && !isTerminalExecutionStatus(existing.Status) {
			outcome.ScheduledRoots, err = service.scheduler.Activate(
				ctx, existing, initialVariables,
			)
		}
		return outcome, err
	}
	execution, err := service.executions.Create(
		ctx, snapshotWorkflow, snapshot.ID, "ASYNC", model.ExecutionOriginHTTPWebhook,
		correlationID, idempotencyKey, fingerprint,
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

func (service *ExecutionService) validateInput(
	definition workflow.Workflow,
	initialVariables map[string]any,
	origin string,
) error {
	acceptingRoot := false
	for _, node := range definition.Nodes {
		if node.Type == "core.http-trigger" &&
			origin != model.ExecutionOriginHTTPWebhook {
			return ErrInvalidExecutionOrigin
		}
		if len(workflow.Predecessors(definition, node.ID)) != 0 {
			continue
		}
		descriptor, exists := service.scheduler.registry.Definition(
			node.Type, node.Version,
		)
		if exists && descriptor.AcceptsInitialVariables {
			acceptingRoot = true
		}
	}
	if len(initialVariables) > 0 && !acceptingRoot {
		return ErrInitialVariablesNotAccepted
	}
	return nil
}
