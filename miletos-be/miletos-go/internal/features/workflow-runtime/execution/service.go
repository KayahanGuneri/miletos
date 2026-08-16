package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	pluginstate "miletos-go/internal/features/workflow-runtime/plugin-state"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

var ErrAsyncUnavailable = errors.New("asynchronous execution is unavailable")
var ErrIdempotencyRequestInProgress = errors.New("idempotent request is still in progress")
var ErrSyncNonterminal = errors.New("synchronous execution did not reach a terminal state")
var ErrStartInputNotAccepted = errors.New("start input is not accepted")
var ErrInvalidExecutionOrigin = errors.New("execution origin is incompatible with the workflow")

type ExecutionOutcome struct {
	Execution           model.Execution
	ScheduledEntryNodes int
	Replayed            bool
}

type SourceDispatch struct {
	Output  any
	Routing model.NodeRoutingOutcome
}

type ExecutionService struct {
	workflows         *workflow.WorkflowService
	executions        *repository.ExecutionRepository
	scheduler         *Scheduler
	processor         *NodeProcessor
	recordSourceState *pluginstate.Repository
	asyncEnabled      bool
	nodeConcurrency   int
}

func NewExecutionService(
	workflows *workflow.WorkflowService,
	executions *repository.ExecutionRepository,
	scheduler *Scheduler,
	processor *NodeProcessor,
	recordSourceState *pluginstate.Repository,
	asyncEnabled bool,
	nodeConcurrency ...int,
) *ExecutionService {
	readyNodeConcurrency := 1
	if len(nodeConcurrency) > 0 && nodeConcurrency[0] > 0 {
		readyNodeConcurrency = nodeConcurrency[0]
	}
	return &ExecutionService{
		workflows: workflows, executions: executions,
		scheduler: scheduler, processor: processor, asyncEnabled: asyncEnabled,
		recordSourceState: recordSourceState,
		nodeConcurrency:   readyNodeConcurrency,
	}
}

func (service *ExecutionService) AsyncAvailable() bool {
	return service.asyncEnabled
}

func (service *ExecutionService) Validate(definition workflow.Workflow) error {
	return service.workflows.Validate(definition)
}

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
	reservation, created, err := service.executions.ReserveHTTPIdempotency(
		ctx, definition.CompanyID, idempotencyKey, fingerprint,
	)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	if !created {
		return service.replayHTTPIdempotency(ctx, reservation, fingerprint)
	}
	snapshot, err := service.workflows.CreateWorkflow(ctx, definition)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	execution, err := service.executions.CreateReserved(
		ctx, definition, snapshot.ID, "ASYNC", model.ExecutionOriginManualDirect,
		correlationID, idempotencyKey, fingerprint, reservation.WorkflowExecutionID,
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

func (service *ExecutionService) replayHTTPIdempotency(
	ctx context.Context,
	reservation repository.HTTPIdempotencyReservation,
	fingerprint string,
) (ExecutionOutcome, error) {
	if reservation.RequestFingerprint != fingerprint {
		return ExecutionOutcome{}, repository.ErrIdempotencyConflict
	}
	switch reservation.State {
	case repository.HTTPIdempotencyReserved:
		return ExecutionOutcome{}, ErrIdempotencyRequestInProgress
	case repository.HTTPIdempotencyAccepted:
		existing, err := service.executions.FindByID(
			ctx, reservation.CompanyID, reservation.WorkflowExecutionID,
		)
		if errors.Is(err, repository.ErrNotFound) {
			return ExecutionOutcome{}, fmt.Errorf(
				"%w: accepted HTTP idempotency key references a missing execution",
				repository.ErrStateTransition,
			)
		}
		return ExecutionOutcome{Execution: existing, Replayed: err == nil}, err
	default:
		return ExecutionOutcome{}, fmt.Errorf(
			"%w: unknown HTTP idempotency state %q",
			repository.ErrStateTransition, reservation.State,
		)
	}
}

func (service *ExecutionService) ExecuteSync(
	ctx context.Context,
	definition workflow.Workflow,
	startInput map[string]any,
	correlationID string,
	idempotency ...string,
) (ExecutionOutcome, error) {
	return service.ExecuteSyncWithStartInput(
		ctx,
		definition,
		startInput,
		correlationID,
		idempotency...,
	)
}

func (service *ExecutionService) ExecuteSyncWithStartInput(
	ctx context.Context,
	definition workflow.Workflow,
	startInput any,
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
	if err := service.executeReadyWaves(
		ctx, execution, definition, startInput, correlationID,
	); err != nil {
		return ExecutionOutcome{}, err
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

func (service *ExecutionService) ExecuteReferenced(
	ctx context.Context,
	companyID string,
	parentWorkflowID string,
	workflowID string,
	startInput any,
) (ExecutionOutcome, error) {
	workflowID = strings.TrimSpace(workflowID)
	parentWorkflowID = strings.TrimSpace(parentWorkflowID)
	if workflowID == "" {
		return ExecutionOutcome{}, plugin.ErrWorkflowNotFound
	}
	if parentWorkflowID != "" && workflowID == parentWorkflowID {
		return ExecutionOutcome{}, plugin.ErrWorkflowSelfReference
	}
	ctx, err := contextWithReferencedWorkflow(ctx, parentWorkflowID, workflowID)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	catalog, err := service.workflows.FindCatalog(ctx, companyID, workflowID)
	if err != nil {
		if errors.Is(err, workflow.ErrNotFound) {
			return ExecutionOutcome{}, plugin.ErrWorkflowNotFound
		}
		return ExecutionOutcome{}, err
	}
	if catalog.Status != workflow.CatalogStatusActive {
		return ExecutionOutcome{}, plugin.ErrWorkflowNotCallable
	}
	if err := validateReferencedWorkflowResult(catalog.Workflow, service.scheduler.registry); err != nil {
		return ExecutionOutcome{}, err
	}
	outcome, err := service.ExecuteSyncWithStartInput(ctx, catalog.Workflow, startInput, "")
	if err != nil {
		if isUncallableReferencedWorkflow(err) {
			return ExecutionOutcome{}, plugin.ErrWorkflowNotCallable
		}
		return ExecutionOutcome{}, err
	}
	return outcome, nil
}

func validateReferencedWorkflowResult(
	definition workflow.Workflow,
	registry *plugin.NodeRegistry,
) error {
	if registry == nil {
		return plugin.ErrWorkflowReturnMissing
	}
	count := 0
	for _, node := range definition.Nodes {
		registration, exists := registry.Get(node.Type)
		if exists && registration.ProvidesWorkflowResult {
			count++
		}
	}
	if count == 0 {
		return plugin.ErrWorkflowReturnMissing
	}
	if count > 1 {
		return plugin.ErrWorkflowReturnAmbiguous
	}
	return nil
}

func isUncallableReferencedWorkflow(err error) bool {
	var validationError *workflow.WorkflowValidationError
	return errors.As(err, &validationError) ||
		errors.Is(err, workflow.ErrInvalidWorkflow) ||
		errors.Is(err, ErrStartInputNotAccepted) ||
		errors.Is(err, ErrInvalidExecutionOrigin)
}

type readySyncNode struct {
	node      workflow.WorkflowNode
	readiness routeReadiness
}

func (service *ExecutionService) executeReadyWaves(
	ctx context.Context,
	execution model.Execution,
	definition workflow.Workflow,
	startInput any,
	correlationID string,
) error {
	outcomes := make(map[string]nodeOutcome)
	for {
		ready := make([]readySyncNode, 0)
		for _, node := range definition.Nodes {
			if _, exists := outcomes[node.ID]; exists {
				continue
			}
			readiness := resolveNodeRoutes(definition, node.ID, outcomes)
			if readiness.failed {
				if err := service.executions.MarkNodeSkipped(
					ctx, definition.CompanyID, execution.ID, node.ID,
					string(model.SkipReasonDependencyFailed),
				); err != nil {
					return err
				}
				outcomes[node.ID] = nodeOutcome{
					status: model.NodeSkipped, skipReason: model.SkipReasonDependencyFailed,
				}
				continue
			}
			if readiness.inactive {
				if err := service.executions.MarkNodeSkipped(
					ctx, definition.CompanyID, execution.ID, node.ID,
					string(model.SkipReasonNoActiveRoute),
				); err != nil {
					return err
				}
				outcomes[node.ID] = nodeOutcome{
					status: model.NodeSkipped, skipReason: model.SkipReasonNoActiveRoute,
				}
				continue
			}
			if readiness.ready {
				ready = append(ready, readySyncNode{node: node, readiness: readiness})
			}
		}
		if len(ready) == 0 {
			for _, node := range definition.Nodes {
				if _, exists := outcomes[node.ID]; !exists {
					return fmt.Errorf(
						"%w: node %s has unresolved incoming routes",
						repository.ErrStateTransition, node.ID,
					)
				}
			}
			return nil
		}
		if err := service.runReadySyncNodes(
			ctx, execution, definition, startInput, correlationID, ready, outcomes,
		); err != nil {
			return err
		}
	}
}

func (service *ExecutionService) runReadySyncNodes(
	ctx context.Context,
	execution model.Execution,
	definition workflow.Workflow,
	startInput any,
	correlationID string,
	ready []readySyncNode,
	outcomes map[string]nodeOutcome,
) error {
	var mutex sync.Mutex
	var waitGroup sync.WaitGroup
	slots := make(chan struct{}, service.nodeConcurrency)
	runErrors := make([]error, len(ready))
	for index, item := range ready {
		index, item := index, item
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				runErrors[index] = ctx.Err()
				return
			}
			defer func() { <-slots }()
			outcome, err := service.runSyncNode(
				ctx, execution, definition, startInput, correlationID, item,
			)
			if err != nil {
				runErrors[index] = err
				return
			}
			mutex.Lock()
			outcomes[item.node.ID] = outcome
			mutex.Unlock()
		}()
	}
	waitGroup.Wait()
	for _, err := range runErrors {
		if err != nil {
			return err
		}
	}
	return nil
}

func (service *ExecutionService) runSyncNode(
	ctx context.Context,
	execution model.Execution,
	definition workflow.Workflow,
	startInput any,
	correlationID string,
	item readySyncNode,
) (nodeOutcome, error) {
	state, changed, err := service.executions.MarkNodeQueued(
		ctx, definition.CompanyID, execution.ID, item.node.ID,
	)
	if err != nil {
		return nodeOutcome{}, err
	}
	if !changed {
		return nodeOutcome{}, fmt.Errorf(
			"%w: node %s could not be queued",
			repository.ErrStateTransition, item.node.ID,
		)
	}
	payload := buildExecutionNodeInput(
		definition, item.node.ID, item.readiness.edgePayloads, service.scheduler.registry, startInput,
		"",
		model.ExecutionOriginManualDirect,
	)
	job := model.NodeJob{
		CompanyID: definition.CompanyID, WorkflowID: definition.ID,
		ExecutionID: execution.ID, NodeID: item.node.ID, NodeExecutionID: state.ID,
		Attempt: state.Attempt, CorrelationID: correlationID,
		Origin: model.ExecutionOriginManualDirect, Payload: payload,
	}
	result, runErr := service.processor.ProcessSync(ctx, job, definition, item.node)
	if runErr != nil {
		var nodeFailure *PersistedNodeFailure
		if errors.As(runErr, &nodeFailure) {
			return nodeOutcome{status: model.NodeFailed}, nil
		}
		return nodeOutcome{}, runErr
	}
	return nodeOutcome{
		status: model.NodeSucceeded, output: result.output, routing: result.routing,
	}, nil
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
	return service.executeTriggerFromSnapshot(
		ctx, snapshot, triggerNodeID, startInput, nil, origin,
		correlationID, idempotencyKey, fingerprint,
	)
}

func (service *ExecutionService) ExecuteDispatchedFromSnapshot(
	ctx context.Context,
	snapshot workflow.WorkflowSnapshot,
	sourceNodeID string,
	dispatch SourceDispatch,
	origin model.ExecutionOrigin,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (ExecutionOutcome, error) {
	return service.executeTriggerFromSnapshot(
		ctx, snapshot, sourceNodeID, nil, &dispatch, origin,
		correlationID, idempotencyKey, fingerprint,
	)
}

func (service *ExecutionService) executeTriggerFromSnapshot(
	ctx context.Context,
	snapshot workflow.WorkflowSnapshot,
	triggerNodeID string,
	startInput map[string]any,
	dispatch *SourceDispatch,
	origin model.ExecutionOrigin,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (ExecutionOutcome, error) {
	if !service.asyncEnabled {
		return ExecutionOutcome{}, ErrAsyncUnavailable
	}
	if origin == model.ExecutionOriginManualDirect && dispatch == nil {
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
		string(origin),
	) {
		return ExecutionOutcome{}, ErrInvalidExecutionOrigin
	}

	if existing, found, err := service.executions.FindIdempotent(
		ctx, snapshotWorkflow.CompanyID, idempotencyKey, fingerprint,
	); err != nil {
		return ExecutionOutcome{}, err
	} else if found {
		if dispatch != nil {
			if err := service.resumeSourceDispatch(
				ctx, existing, triggerNodeID, *dispatch,
			); err != nil {
				return ExecutionOutcome{}, err
			}
			if _, err := service.scheduler.ActivateFromRoot(
				ctx, existing, nil, triggerNodeID,
			); err != nil {
				return ExecutionOutcome{}, err
			}
		}
		return ExecutionOutcome{Execution: existing, Replayed: true}, nil
	}
	inScope := workflow.Downstream(snapshotWorkflow, []string{triggerNodeID})
	if err := expandRequiredInputs(
		snapshotWorkflow,
		inScope,
		triggerNodeID,
		service.scheduler.registry,
	); err != nil {
		return ExecutionOutcome{}, err
	}
	execution, err := service.executions.Create(
		ctx, snapshotWorkflow, snapshot.ID, "ASYNC", origin,
		correlationID, idempotencyKey, fingerprint,
	)
	if err != nil {
		return ExecutionOutcome{}, err
	}
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
	if dispatch != nil {
		if err := service.executions.MarkExecutionQueued(
			ctx,
			execution.CompanyID,
			execution.ID,
		); err != nil {
			return ExecutionOutcome{}, err
		}
		if err := service.persistSourceDispatch(
			ctx,
			execution,
			triggerNodeID,
			*dispatch,
		); err != nil {
			return ExecutionOutcome{}, err
		}
		startInput = nil
	}
	scheduled, err := service.scheduler.ActivateFromRoot(
		ctx, execution, startInput, triggerNodeID,
	)
	if err != nil {
		return ExecutionOutcome{}, err
	}
	if dispatch != nil {
		scheduled = 1
	}
	execution.Status = model.ExecutionQueued
	return ExecutionOutcome{Execution: execution, ScheduledEntryNodes: scheduled}, nil
}

func (service *ExecutionService) persistSourceDispatch(
	ctx context.Context,
	execution model.Execution,
	nodeID string,
	dispatch SourceDispatch,
) error {
	state, changed, err := service.executions.MarkNodeQueued(
		ctx,
		execution.CompanyID,
		execution.ID,
		nodeID,
		dispatch.Output,
	)
	if err != nil {
		return err
	}
	if !changed {
		return repository.ErrStateTransition
	}
	job := model.NodeJob{
		CompanyID: execution.CompanyID, WorkflowID: execution.WorkflowID,
		ExecutionID: execution.ID, NodeID: nodeID, NodeExecutionID: state.ID,
		Attempt: state.Attempt, CorrelationID: execution.CorrelationID,
		Origin: execution.Origin, Payload: dispatch.Output,
	}
	started, err := service.executions.MarkNodeRunning(ctx, job)
	if err != nil {
		return err
	}
	if !started {
		return repository.ErrStateTransition
	}
	return service.executions.SaveNodeSuccess(ctx, job, dispatch.Output, dispatch.Routing)
}

func (service *ExecutionService) resumeSourceDispatch(
	ctx context.Context,
	execution model.Execution,
	nodeID string,
	dispatch SourceDispatch,
) error {
	states, err := service.executions.ListAllNodes(ctx, execution.CompanyID, execution.ID)
	if err != nil {
		return err
	}
	var state model.NodeExecution
	found := false
	for _, candidate := range states {
		if candidate.NodeID == nodeID {
			state = candidate
			found = true
			break
		}
	}
	if !found {
		return repository.ErrStateTransition
	}
	if state.Status == model.NodeSucceeded {
		return nil
	}
	if state.Status == model.NodePending || state.Status == model.NodeReady {
		return service.persistSourceDispatch(ctx, execution, nodeID, dispatch)
	}
	job := model.NodeJob{
		CompanyID: execution.CompanyID, WorkflowID: execution.WorkflowID,
		ExecutionID: execution.ID, NodeID: nodeID, NodeExecutionID: state.ID,
		Attempt: state.Attempt, CorrelationID: execution.CorrelationID,
		Origin: execution.Origin, Payload: dispatch.Output,
	}
	if state.Status == model.NodeQueued {
		started, err := service.executions.MarkNodeRunning(ctx, job)
		if err != nil {
			return err
		}
		if !started {
			return repository.ErrStateTransition
		}
	} else if state.Status != model.NodeRunning {
		return repository.ErrStateTransition
	}
	return service.executions.SaveNodeSuccess(ctx, job, dispatch.Output, dispatch.Routing)
}

func expandRequiredInputs(
	definition workflow.Workflow,
	inScope map[string]bool,
	eventRootNodeID string,
	registry *plugin.NodeRegistry,
) error {
	for changed := true; changed; {
		changed = false
		for _, edge := range definition.Edges {
			if !inScope[edge.TargetNodeID] || inScope[edge.SourceNodeID] {
				continue
			}
			target, exists := findWorkflowNode(definition, edge.TargetNodeID)
			if exists && registry != nil {
				registration, registered := registry.Get(target.Type)
				if registered && registration.TriggerInputMode == plugin.TriggerInputAvailable {
					continue
				}
			}
			inScope[edge.SourceNodeID] = true
			changed = true
		}
	}
	for _, root := range workflow.Roots(definition) {
		if root.ID == eventRootNodeID || !inScope[root.ID] {
			continue
		}
		if registry == nil || !registry.CanStartFrom(
			root.Type,
			string(model.ExecutionOriginManualDirect),
		) {
			return fmt.Errorf(
				"%w: required root %s cannot run as a passive dependency",
				ErrInvalidExecutionOrigin,
				root.ID,
			)
		}
	}
	return nil
}

func (service *ExecutionService) ExecuteDataArrivalFromSnapshot(
	ctx context.Context,
	snapshot workflow.WorkflowSnapshot,
	sourceNodeID string,
	payload map[string]any,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (ExecutionOutcome, error) {
	return service.ExecuteTriggerFromSnapshot(
		ctx,
		snapshot,
		sourceNodeID,
		plugin.DataArrivalStartInput(payload),
		model.ExecutionOriginDataArrival,
		correlationID,
		idempotencyKey,
		fingerprint,
	)
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
	startInput any,
	source model.ExecutionOrigin,
) error {
	acceptsManualEntryInput := false
	for _, node := range definition.Nodes {
		if len(workflow.Predecessors(definition, node.ID)) != 0 {
			continue
		}
		if !service.scheduler.registry.CanStartFrom(
			node.Type, string(source),
		) {
			return ErrInvalidExecutionOrigin
		}
		if source == model.ExecutionOriginManualDirect && hasStartInput(startInput) {
			registration, exists := service.scheduler.registry.Get(node.Type)
			if exists && plugin.CanReceiveEntryInput(registration) {
				acceptsManualEntryInput = true
			}
		}
	}
	if source == model.ExecutionOriginManualDirect &&
		hasStartInput(startInput) && !acceptsManualEntryInput {
		return ErrStartInputNotAccepted
	}
	return nil
}

func hasStartInput(startInput any) bool {
	if startInput == nil {
		return false
	}
	if values, ok := startInput.(map[string]any); ok {
		return len(values) > 0
	}
	return true
}
