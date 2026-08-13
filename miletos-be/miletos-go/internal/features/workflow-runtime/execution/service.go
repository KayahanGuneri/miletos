package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

func (service *ExecutionService) AsyncAvailable() bool {
	return service.asyncEnabled
}

func (service *ExecutionService) Validate(definition workflow.Workflow) error {
	return service.workflows.Validate(definition)
}

// Fingerprint hashes the stable logical values of a request so that a replay of
// the same Idempotency-Key can be distinguished from a conflicting reuse.
// Callers must supply only stable values: no request identifiers, correlation
// identifiers, timestamps, secrets or transport control headers.
func Fingerprint(request map[string]any) string {
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
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
		return ExecutionOutcome{Execution: existing, Replayed: found}, err
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
				string(model.SkipReasonDependencyFailed),
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
	return service.ExecuteTriggerFromSnapshot(
		ctx, snapshot, triggerNodeID, startInput,
		model.ExecutionOriginHTTPWebhook, correlationID, idempotencyKey, fingerprint,
	)
}

func (service *ExecutionService) ExecuteTriggerFromSnapshot(
	ctx context.Context,
	snapshot workflow.WorkflowSnapshot,
	triggerNodeID string,
	startInput map[string]any,
	origin model.ExecutionOrigin,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (ExecutionOutcome, error) {
	if !service.asyncEnabled {
		return ExecutionOutcome{}, ErrAsyncUnavailable
	}
	if origin == model.ExecutionOriginManualDirect {
		return ExecutionOutcome{}, ErrInvalidExecutionOrigin
	}
	snapshotWorkflow := snapshot.Workflow
	if err := service.workflows.Validate(snapshotWorkflow); err != nil {
		return ExecutionOutcome{}, err
	}
	triggerNode, isRoot := findRootNode(snapshotWorkflow, triggerNodeID)
	if !isRoot {
		return ExecutionOutcome{}, ErrInvalidExecutionOrigin
	}
	if !service.scheduler.registry.DeclaresExecutionSource(
		triggerNode.Type,
		triggerNode.Version,
		string(origin),
	) {
		return ExecutionOutcome{}, ErrInvalidExecutionOrigin
	}
	// A matching idempotent request is returned as-is. Re-activating it would
	// schedule the root nodes a second time; the reconciler and the outbox own
	// durable recovery of an execution that has not progressed.
	if existing, found, err := service.executions.FindIdempotent(
		ctx, snapshotWorkflow.CompanyID, idempotencyKey, fingerprint,
	); err != nil || found {
		return ExecutionOutcome{Execution: existing, Replayed: found}, err
	}
	execution, err := service.executions.Create(
		ctx, snapshotWorkflow, snapshot.ID, "ASYNC", origin,
		correlationID, idempotencyKey, fingerprint,
	)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	inScope := workflow.Downstream(snapshotWorkflow, []string{triggerNodeID})
	for _, node := range snapshotWorkflow.Nodes {
		if inScope[node.ID] {
			continue
		}
		if err := service.executions.MarkNodeSkipped(
			ctx, snapshotWorkflow.CompanyID, execution.ID, node.ID,
			string(model.SkipReasonOutOfTriggerScope),
		); err != nil {
			return ExecutionOutcome{}, err
		}
	}
	scheduled, err := service.scheduler.Activate(ctx, execution, startInput)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution.Status = model.ExecutionQueued
	return ExecutionOutcome{Execution: execution, ScheduledEntryNodes: scheduled}, nil
}

func findRootNode(definition workflow.Workflow, nodeID string) (workflow.WorkflowNode, bool) {
	for _, root := range workflow.Roots(definition) {
		if root.ID == nodeID {
			return root, true
		}
	}
	return workflow.WorkflowNode{}, false
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
