package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/queue"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	pluginstate "miletos-go/internal/features/workflow-runtime/plugin-state"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

type NodeProcessor struct {
	workflows       *workflow.WorkflowRepository
	executions      *repository.ExecutionRepository
	registry        *plugin.NodeRegistry
	scheduler       *Scheduler
	queue           queue.Queue
	topic           string
	maximumAttempts int
	retryDelay      time.Duration
	state           *pluginstate.Repository
	emitter         *PluginEmitter
	workflowsInfra  *WorkflowInfrastructure
	database        plugin.DatabaseInfrastructure
	secrets         plugin.SecretsInfrastructure
}

func (processor *NodeProcessor) SetWorkflowInfrastructure(
	infrastructure *WorkflowInfrastructure,
) {
	processor.workflowsInfra = infrastructure
}

func (processor *NodeProcessor) SetDatabase(database plugin.DatabaseInfrastructure) {
	processor.database = database
}

func (processor *NodeProcessor) SetSecrets(secrets plugin.SecretsInfrastructure) {
	processor.secrets = secrets
}

type classifiedNodeError struct {
	category       model.FailureCategory
	code           model.FailureCode
	message        string
	retryable      bool
	decisionReason model.RetryDecisionReason
}

type PersistedNodeFailure struct {
	Cause error
}

type nodeRunResult struct {
	output  any
	routing model.NodeRoutingOutcome
}

func (failure *PersistedNodeFailure) Error() string {
	return "node execution failed and the outcome was persisted"
}

func (failure *PersistedNodeFailure) Unwrap() error {
	return failure.Cause
}

var errNodeOnRunPanicked = errors.New("node handler panicked")
var ErrStaleNodeJob = errors.New("stale node job")
var ErrPermanentNodeJob = errors.New("permanent node job failure")

func NewNodeProcessor(
	workflows *workflow.WorkflowRepository,
	executions *repository.ExecutionRepository,
	registry *plugin.NodeRegistry,
	scheduler *Scheduler,
	nodeQueue queue.Queue,
	topic string,
	maximumAttempts int,
	retryDelay time.Duration,
	state *pluginstate.Repository,
	emitter *PluginEmitter,
) (*NodeProcessor, error) {
	if maximumAttempts < 1 {
		return nil, fmt.Errorf("retry maximum attempts must be positive")
	}
	if maximumAttempts > 32767 {
		return nil, fmt.Errorf("retry maximum attempts cannot exceed 32767")
	}
	if retryDelay <= 0 {
		return nil, fmt.Errorf("retry delay must be positive")
	}
	return &NodeProcessor{
		workflows: workflows, executions: executions, registry: registry,
		scheduler: scheduler, queue: nodeQueue, topic: topic,
		maximumAttempts: maximumAttempts, retryDelay: retryDelay,
		state: state, emitter: emitter,
	}, nil
}

func (processor *NodeProcessor) Run(ctx context.Context) error {
	if processor.queue == nil {
		return fmt.Errorf("node queue is unavailable")
	}
	return processor.queue.Consume(ctx, processor.topic, processor.consume)
}

func (processor *NodeProcessor) consume(
	ctx context.Context,
	payload []byte,
) queue.RecordResult {
	var job model.NodeJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return queue.RecordResult{
			Disposition: queue.RecordDeadLetter,
			Code:        "MALFORMED_NODE_JOB",
			Err:         err,
		}
	}
	if job.CompanyID == "" || job.WorkflowID == "" || job.ExecutionID == "" ||
		job.NodeID == "" || job.NodeExecutionID == "" || job.Attempt < 1 {
		return queue.RecordResult{
			Disposition: queue.RecordDeadLetter,
			Code:        "INVALID_NODE_JOB",
		}
	}
	err := processor.Process(ctx, job)
	switch {
	case err == nil:
		return queue.RecordResult{Disposition: queue.RecordHandled}
	case errors.Is(err, repository.ErrNotFound):
		return queue.RecordResult{
			Disposition: queue.RecordDeadLetter,
			Code:        "ORPHAN_NODE_JOB",
			Err:         err,
		}
	case errors.Is(err, repository.ErrStateTransition):
		return queue.RecordResult{
			Disposition: queue.RecordHandled,
			Code:        "STALE_NODE_JOB",
			Err:         err,
		}
	case errors.Is(err, ErrStaleNodeJob):
		return queue.RecordResult{
			Disposition: queue.RecordHandled,
			Code:        "STALE_NODE_JOB",
			Err:         err,
		}
	case errors.Is(err, ErrPermanentNodeJob):
		return queue.RecordResult{
			Disposition: queue.RecordDeadLetter,
			Code:        "PERMANENT_NODE_JOB",
			Err:         err,
		}
	default:
		return queue.RecordResult{
			Disposition: queue.RecordRetry,
			Code:        string(model.FailureCodeJobNotDurable),
			Err:         err,
		}
	}
}

func (processor *NodeProcessor) Process(ctx context.Context, job model.NodeJob) error {
	snapshot, err := processor.workflows.FindByExecutionID(ctx, job.CompanyID, job.ExecutionID)
	if err != nil {
		return fmt.Errorf("load workflow for node job: %w", err)
	}
	node, exists := findWorkflowNode(snapshot.Workflow, job.NodeID)
	if !exists {
		return fmt.Errorf("%w: node is absent from workflow", ErrPermanentNodeJob)
	}
	if err := processor.executions.MarkExecutionRunning(
		ctx, job.CompanyID, job.ExecutionID,
	); err != nil {
		return err
	}
	state, err := processor.executions.FindNode(
		ctx, job.CompanyID, job.ExecutionID, job.NodeID,
	)
	if err != nil {
		return err
	}
	if isTerminalNodeStatus(state.Status) {
		return processor.scheduler.CompleteNode(ctx, job)
	}
	if state.Status == model.NodeRunning {
		return nil
	}
	if job.NodeExecutionID != state.ID || job.Attempt != state.Attempt {
		return fmt.Errorf(
			"%w: node attempt no longer matches persisted state",
			ErrStaleNodeJob,
		)
	}
	job.NodeExecutionID = state.ID
	job.Attempt = state.Attempt
	if state.Status == model.NodeRetryPending {
		if err := waitUntil(ctx, state.NextAttemptAt); err != nil {
			return err
		}
		job, err = processor.executions.PrepareRetry(ctx, job)
		if err != nil {
			return err
		}
	}
	_, err = processor.processAttempts(ctx, job, snapshot.Workflow, node, false)
	if err != nil {
		var persistedFailure *PersistedNodeFailure
		if !errors.As(err, &persistedFailure) {
			return err
		}
	}
	if err := processor.scheduler.CompleteNode(ctx, job); err != nil {
		return err
	}
	return nil
}

func (processor *NodeProcessor) ProcessSync(
	ctx context.Context,
	job model.NodeJob,
	definition workflow.Workflow,
	node workflow.WorkflowNode,
) (nodeRunResult, error) {
	return processor.processAttempts(ctx, job, definition, node, true)
}

func (processor *NodeProcessor) PersistInterrupted(
	ctx context.Context,
	job model.NodeJob,
) error {
	failedAt := time.Now().UTC()
	return processor.executions.SaveNodeFailure(
		ctx,
		job,
		map[string]any{
			"category":            model.FailureCategoryInternal,
			"code":                model.FailureCodeExecutionInterrupted,
			"message":             "Node execution was interrupted before a durable result.",
			"retryable":           false,
			"attempt":             job.Attempt,
			"retryDecisionKind":   model.RetryKindDoNotRetry,
			"retryDecisionReason": model.RetryReasonNotRetryable,
		},
		false,
		failedAt,
		time.Time{},
		processor.maximumAttempts,
		processor.retryDelay,
		"",
	)
}

func (processor *NodeProcessor) processAttempts(
	ctx context.Context,
	job model.NodeJob,
	definition workflow.Workflow,
	node workflow.WorkflowNode,
	inlineRetry bool,
) (nodeRunResult, error) {
	for {
		started, err := processor.executions.MarkNodeRunning(ctx, job)
		if err != nil {
			return nodeRunResult{}, err
		}
		if !started {
			state, stateErr := processor.executions.FindNode(
				ctx, job.CompanyID, job.ExecutionID, job.NodeID,
			)
			if stateErr != nil {
				return nodeRunResult{}, stateErr
			}
			if state.Status == model.NodeSucceeded {
				decoded, decodeErr := decodeNodeExecutionOutcome(state, processor.registry)
				if decodeErr != nil {
					return nodeRunResult{}, decodeErr
				}
				return nodeRunResult{
					output: decoded.OutputPayload, routing: decoded.Routing,
				}, nil
			}
			if state.Status == model.NodeRunning {
				return nodeRunResult{}, nil
			}
			if isTerminalNodeStatus(state.Status) {
				return nodeRunResult{}, &PersistedNodeFailure{
					Cause: fmt.Errorf("node execution is terminal"),
				}
			}
			return nodeRunResult{}, fmt.Errorf(
				"%w: node %q cannot start from status %s",
				repository.ErrStateTransition, job.NodeID, state.Status,
			)
		}
		registration, exists := processor.registry.Get(node.Type)
		var output any
		var routing model.NodeRoutingOutcome
		var executionError error
		if !exists || registration.Handler == nil {
			executionError = &plugin.NodeError{
				Category: string(model.FailureCategoryValidation),
				Code:     string(model.FailureCodeHandlerNotFound),
				Message:  "The configured node handler is unavailable.",
			}
		} else {
			access := newNodeAccess(definition, node.ID, registration.RoutingMode)
			lifecycles := plugin.NewLifecycles()
			storage := pluginstate.NewStorage(ctx, processor.state, pluginstate.Scope{
				CompanyID:        job.CompanyID,
				WorkflowID:       job.WorkflowID,
				WorkflowRevision: definition.Revision,
				NodeID:           job.NodeID,
			})
			infrastructure := plugin.Infrastructure{}
			if processor.emitter != nil {
				infrastructure.Emitter = processor.emitter.ForContext(ctx, registration.Key)
			}
			if processor.workflowsInfra != nil {
				infrastructure.Workflow = processor.workflowsInfra.ForContext(
					ctx, job.CompanyID, job.WorkflowID,
				)
			}
			if processor.database != nil {
				infrastructure.Database = processor.database
			}
			if processor.secrets != nil {
				infrastructure.Secrets = processor.secrets
			}
			nodeContext := plugin.NewContext(plugin.ContextOptions{
				Runtime:        ctx,
				Configuration:  node.Configuration,
				CompanyID:      job.CompanyID,
				Lifecycles:     lifecycles,
				Storage:        storage,
				Access:         access,
				Infrastructure: infrastructure,
				Payload:        job.Payload,
			})
			output, executionError = executeNodeOnRun(
				registration.Handler, lifecycles, nodeContext,
			)
			if executionError == nil {
				routing = access.outcome()
			} else {
				access.discard()
			}
		}
		if executionError == nil {
			if err := processor.executions.SaveNodeSuccess(
				ctx, job, output, routing,
			); err != nil {
				return nodeRunResult{}, err
			}
			return nodeRunResult{output: output, routing: routing}, nil
		}
		failedAt := time.Now().UTC()
		classification := classifyNodeError(executionError, exists)
		deadline, _ := ctx.Deadline()
		decision := DecideRetry(
			ctx, job.Attempt, processor.maximumAttempts, classification.retryable,
			processor.retryDelay, failedAt, deadline,
		)
		decisionKind, decisionReason := retryDecision(
			ctx, job.Attempt, processor.maximumAttempts, classification,
			failedAt, deadline, processor.retryDelay, decision.Retry,
		)
		nextAttempt := time.Time{}
		if decision.Retry {
			nextAttempt = failedAt.Add(decision.Delay)
		}
		retryDestination := ""
		if !inlineRetry {
			retryDestination = processor.topic
		}
		if err := processor.executions.SaveNodeFailure(
			ctx, job, failureSummary(
				classification, classification.retryable,
				job.Attempt, decisionKind, decisionReason,
			), decision.Retry,
			failedAt, nextAttempt, processor.maximumAttempts, processor.retryDelay,
			retryDestination,
		); err != nil {
			return nodeRunResult{}, err
		}
		if !decision.Retry {
			return nodeRunResult{}, &PersistedNodeFailure{Cause: executionError}
		}
		if !inlineRetry {
			return nodeRunResult{}, nil
		}
		if err := waitUntil(ctx, &nextAttempt); err != nil {
			return nodeRunResult{}, err
		}
		job, err = processor.executions.PrepareRetry(ctx, job)
		if err != nil {
			return nodeRunResult{}, err
		}
	}
}

func executeNodeOnRun(
	handler plugin.NodeHandler,
	lifecycles *plugin.LifecycleCallbacks,
	nodeContext *plugin.Context,
) (output any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errNodeOnRunPanicked
		}
	}()
	if err := handler(nodeContext); err != nil {
		return nil, err
	}
	return lifecycles.InvokeRun()
}

func findWorkflowNode(definition workflow.Workflow, nodeID string) (workflow.WorkflowNode, bool) {
	for _, node := range definition.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return workflow.WorkflowNode{}, false
}

func classifyNodeError(err error, handlerExists bool) classifiedNodeError {
	classification := classifiedNodeError{
		category: model.FailureCategoryExecution, code: model.FailureCodeExecutionFailed,
		message: "Node execution failed.", retryable: false,
	}
	if !handlerExists {
		classification.category = model.FailureCategoryValidation
		classification.code = model.FailureCodeHandlerNotFound
		classification.message = "The configured node handler is unavailable."
		classification.retryable = false
		classification.decisionReason = model.RetryReasonCategory
		return classification
	}
	if errors.Is(err, errNodeOnRunPanicked) {
		classification.category = model.FailureCategoryInternal
		classification.code = model.FailureCodeHandlerPanicked
		classification.message = "The node handler failed unexpectedly."
		classification.retryable = false
		classification.decisionReason = model.RetryReasonNotRetryable
		return classification
	}
	if errors.Is(err, context.Canceled) {
		classification.category = model.FailureCategoryCanceled
		classification.code = model.FailureCodeExecutionCancelled
		classification.message = "Node execution was cancelled."
		classification.retryable = false
		classification.decisionReason = model.RetryReasonCategory
		return classification
	}
	if errors.Is(err, context.DeadlineExceeded) {
		classification.category = model.FailureCategoryTimeout
		classification.code = model.FailureCodeExecutionTimeout
		classification.message = "Node execution exceeded its deadline."
		classification.retryable = false
		classification.decisionReason = model.RetryReasonDeadline
	}
	var typed *plugin.NodeError
	if errors.As(err, &typed) {
		if category, valid := parseFailureCategory(typed.Category); valid {
			classification.category = category
		}
		if code := normalizeFailureCode(typed.Code); code != "" {
			classification.code = code
		}
		classification.message = typed.Message
		classification.retryable = typed.CanRetry
		if !typed.CanRetry {
			classification.decisionReason = model.RetryReasonNotRetryable
		}
		return classification
	}
	return classification
}

func parseFailureCategory(raw string) (model.FailureCategory, bool) {
	category := model.FailureCategory(strings.TrimSpace(raw))
	switch category {
	case model.FailureCategoryValidation,
		model.FailureCategoryExecution,
		model.FailureCategoryInternal,
		model.FailureCategoryCanceled,
		model.FailureCategoryTimeout:
		return category, true
	default:
		return "", false
	}
}

func normalizeFailureCode(raw string) model.FailureCode {
	return model.FailureCode(strings.TrimSpace(raw))
}

func retryDecision(
	ctx context.Context,
	attempt int,
	maximumAttempts int,
	classification classifiedNodeError,
	now time.Time,
	deadline time.Time,
	delay time.Duration,
	retry bool,
) (model.RetryDecisionKind, model.RetryDecisionReason) {
	if retry {
		return model.RetryKindRetry, model.RetryReasonScheduled
	}
	if attempt >= maximumAttempts && classification.retryable {
		return model.RetryKindExhausted, model.RetryReasonMaxAttempts
	}
	if errors.Is(ctx.Err(), context.Canceled) || !classification.retryable {
		reason := classification.decisionReason
		if reason == "" {
			reason = model.RetryReasonNotRetryable
		}
		return model.RetryKindDoNotRetry, reason
	}
	if !deadline.IsZero() && now.Add(delay).After(deadline) {
		return model.RetryKindDoNotRetry, model.RetryReasonDeadline
	}
	return model.RetryKindDoNotRetry, model.RetryReasonNotRetryable
}

func failureSummary(
	classification classifiedNodeError,
	retryable bool,
	attempt int,
	decisionKind model.RetryDecisionKind,
	decisionReason model.RetryDecisionReason,
) map[string]any {
	code := classification.code
	if decisionKind == model.RetryKindExhausted {
		code = model.FailureCodeRetryExhausted
	}
	return map[string]any{
		"category":            classification.category,
		"code":                code,
		"message":             classification.message,
		"retryable":           retryable,
		"attempt":             attempt,
		"retryDecisionKind":   decisionKind,
		"retryDecisionReason": decisionReason,
	}
}

func waitUntil(ctx context.Context, when *time.Time) error {
	if when == nil {
		return fmt.Errorf("%w: retry has no scheduled time", repository.ErrStateTransition)
	}
	delay := time.Until(*when)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isTerminalNodeStatus(status model.NodeStatus) bool {
	switch status {
	case model.NodeSucceeded, model.NodeFailed, model.NodeSkipped,
		model.NodeCancelled, model.NodeTimedOut:
		return true
	default:
		return false
	}
}
