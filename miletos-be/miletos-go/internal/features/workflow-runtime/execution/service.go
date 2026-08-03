package execution

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

var ErrAsyncUnavailable = errors.New("asynchronous execution is unavailable")
var ErrSyncNonterminal = errors.New("synchronous execution did not reach a terminal state")
var ErrStartInputNotAccepted = errors.New("start input is not accepted")
var ErrInvalidExecutionOrigin = errors.New("execution origin is incompatible with the workflow")

type ExecutionOutcome struct {
	Execution           model.Execution
	ScheduledEntryNodes int
	Replayed            bool
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

func (service *ExecutionService) ExecuteAsyncCommand(
	ctx context.Context,
	command ExecutionCommand,
) (ExecutionOutcome, error) {
	return service.ExecuteAsync(
		ctx, command.Definition, command.StartInput,
		command.CorrelationID, command.IdempotencyKey, command.Fingerprint,
	)
}

func (service *ExecutionService) ExecuteSyncCommand(
	ctx context.Context,
	command ExecutionCommand,
) (ExecutionOutcome, error) {
	return service.ExecuteSync(
		ctx, command.Definition, command.StartInput,
		command.CorrelationID, command.IdempotencyKey, command.Fingerprint,
	)
}

func (service *ExecutionService) ExecuteAsync(
	ctx context.Context,
	definition workflow.Workflow,
	startInput map[string]any,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (ExecutionOutcome, error) {
	if err := service.workflows.Validate(definition); err != nil {
		return ExecutionOutcome{}, err
	}
	if err := service.validateExecutionStart(
		definition, startInput, model.ExecutionOriginManualDirect,
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
			outcome.ScheduledEntryNodes, err = service.scheduler.Activate(ctx, existing, startInput)
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
	scheduled, err := service.scheduler.Activate(ctx, execution, startInput)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution.Status = model.ExecutionQueued
	return ExecutionOutcome{Execution: execution, ScheduledEntryNodes: scheduled}, nil
}

func (service *ExecutionService) ExecuteSync(
	ctx context.Context,
	definition workflow.Workflow,
	startInput map[string]any,
	correlationID string,
	idempotency ...string,
) (ExecutionOutcome, error) {
	if err := service.workflows.Validate(definition); err != nil {
		return ExecutionOutcome{}, err
	}
	if err := service.validateExecutionStart(
		definition, startInput, model.ExecutionOriginManualDirect,
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
		payload := buildExecutionNodeInput(
			definition, nodeID, outputs, service.scheduler.registry, startInput,
			model.ExecutionOriginManualDirect,
		)
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
	startInput map[string]any,
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
	if err := service.validateExecutionStart(
		snapshotWorkflow, startInput, model.ExecutionOriginHTTPWebhook,
	); err != nil {
		return ExecutionOutcome{}, err
	}
	roots := workflow.Roots(snapshotWorkflow)
	if len(roots) != 1 || roots[0].ID != triggerNodeID {
		return ExecutionOutcome{}, ErrInvalidExecutionOrigin
	}
	if existing, found, err := service.executions.FindIdempotent(
		ctx, snapshotWorkflow.CompanyID, idempotencyKey, fingerprint,
	); err != nil || found {
		outcome := ExecutionOutcome{Execution: existing, Replayed: found}
		if found && err == nil && !isTerminalExecutionStatus(existing.Status) {
			outcome.ScheduledEntryNodes, err = service.scheduler.Activate(
				ctx, existing, startInput,
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
	scheduled, err := service.scheduler.Activate(ctx, execution, startInput)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution.Status = model.ExecutionQueued
	return ExecutionOutcome{Execution: execution, ScheduledEntryNodes: scheduled}, nil
}

func (service *ExecutionService) validateExecutionStart(
	definition workflow.Workflow,
	startInput map[string]any,
	source model.ExecutionOrigin,
) error {
	acceptsManualEntryInput := false
	for _, node := range definition.Nodes {
		if len(workflow.Predecessors(definition, node.ID)) != 0 {
			continue
		}
		if !service.scheduler.registry.CanStartFrom(
			node.Type, node.Version, string(source),
		) {
			return ErrInvalidExecutionOrigin
		}
		if source == model.ExecutionOriginManualDirect && len(startInput) > 0 {
			descriptor, exists := service.scheduler.registry.Definition(
				node.Type, node.Version,
			)
			if exists && plugin.CanReceiveEntryInput(descriptor) {
				acceptsManualEntryInput = true
			}
		}
	}
	if source == model.ExecutionOriginManualDirect &&
		len(startInput) > 0 && !acceptsManualEntryInput {
		return ErrStartInputNotAccepted
	}
	return nil
}
