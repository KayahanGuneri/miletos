package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/features/workflowruntime/execution/model"
	"miletos-go/internal/features/workflowruntime/execution/queue"
	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/plugin"
	"miletos-go/internal/features/workflowruntime/workflow"
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
}

type RetryableError interface {
	error
	Retryable() bool
}

type classifiedNodeError struct {
	category       string
	code           string
	message        string
	retryable      bool
	decisionReason string
}

type PersistedNodeFailure struct {
	Cause error
}

func (failure *PersistedNodeFailure) Error() string {
	return "node execution failed and the outcome was persisted"
}

func (failure *PersistedNodeFailure) Unwrap() error {
	return failure.Cause
}

var errNodeHandlerPanicked = errors.New("node handler panicked")
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
) *NodeProcessor {
	return &NodeProcessor{
		workflows: workflows, executions: executions, registry: registry,
		scheduler: scheduler, queue: nodeQueue, topic: topic,
		maximumAttempts: maximumAttempts, retryDelay: retryDelay,
	}
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
			Code:        "NODE_JOB_NOT_DURABLE",
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
	_, err = processor.processAttempts(ctx, job, node, false)
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
	node workflow.WorkflowNode,
) (any, error) {
	return processor.processAttempts(ctx, job, node, true)
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
			"category":            "INTERNAL",
			"code":                "NODE_EXECUTION_INTERRUPTED",
			"message":             "Node execution was interrupted before a durable result.",
			"retryable":           false,
			"attempt":             job.Attempt,
			"retryDecisionKind":   "DO_NOT_RETRY",
			"retryDecisionReason": "FAILURE_NOT_RETRYABLE",
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
	node workflow.WorkflowNode,
	inlineRetry bool,
) (any, error) {
	for {
		started, err := processor.executions.MarkNodeRunning(ctx, job)
		if err != nil {
			return nil, err
		}
		if !started {
			state, stateErr := processor.executions.FindNode(
				ctx, job.CompanyID, job.ExecutionID, job.NodeID,
			)
			if stateErr != nil {
				return nil, stateErr
			}
			if state.Status == model.NodeSucceeded || state.Status == model.NodeRunning {
				return state.Output, nil
			}
			if isTerminalNodeStatus(state.Status) {
				return nil, &PersistedNodeFailure{
					Cause: fmt.Errorf("node execution is terminal"),
				}
			}
			return nil, fmt.Errorf(
				"%w: node %q cannot start from status %s",
				repository.ErrStateTransition, job.NodeID, state.Status,
			)
		}
		handler, exists := processor.registry.GetVersion(node.Type, node.Version)
		var output any
		var executionError error
		if !exists {
			executionError = &plugin.NodeError{
				Category: "VALIDATION",
				Code:     "NODE_HANDLER_NOT_FOUND",
				Message:  "The configured node handler is unavailable.",
			}
		} else if node.Type == "core.http-trigger" &&
			job.Origin != model.ExecutionOriginHTTPWebhook {
			executionError = &plugin.NodeError{
				Category: "VALIDATION",
				Code:     "HTTP_TRIGGER_ORIGIN_INVALID",
				Message:  "HTTP trigger nodes require a trusted webhook execution origin.",
			}
		} else {
			output, executionError = executeNodeHandler(ctx, handler, node.Configuration, job.Payload)
		}
		if executionError == nil {
			if err := processor.executions.SaveNodeSuccess(ctx, job, output); err != nil {
				return nil, err
			}
			return output, nil
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
			return nil, err
		}
		if !decision.Retry {
			return nil, &PersistedNodeFailure{Cause: executionError}
		}
		if !inlineRetry {
			return nil, nil
		}
		if err := waitUntil(ctx, &nextAttempt); err != nil {
			return nil, err
		}
		job, err = processor.executions.PrepareRetry(ctx, job)
		if err != nil {
			return nil, err
		}
	}
}

func executeNodeHandler(
	ctx context.Context,
	handler plugin.NodeHandler,
	configuration map[string]any,
	input any,
) (output any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errNodeHandlerPanicked
		}
	}()
	return handler(ctx, configuration, input)
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
		category: "EXECUTION", code: "NODE_EXECUTION_FAILED",
		message: "Node execution failed.", retryable: false,
	}
	if !handlerExists {
		classification.category = "VALIDATION"
		classification.code = "NODE_HANDLER_NOT_FOUND"
		classification.message = "The configured node handler is unavailable."
		classification.retryable = false
		classification.decisionReason = "CATEGORY_NOT_RETRYABLE"
		return classification
	}
	if errors.Is(err, errNodeHandlerPanicked) {
		classification.category = "INTERNAL"
		classification.code = "NODE_HANDLER_PANICKED"
		classification.message = "The node handler failed unexpectedly."
		classification.retryable = false
		classification.decisionReason = "FAILURE_NOT_RETRYABLE"
		return classification
	}
	if errors.Is(err, context.Canceled) {
		classification.category = "CANCELED"
		classification.code = "NODE_EXECUTION_CANCELLED"
		classification.message = "Node execution was cancelled."
		classification.retryable = false
		classification.decisionReason = "CATEGORY_NOT_RETRYABLE"
		return classification
	}
	if errors.Is(err, context.DeadlineExceeded) {
		classification.category = "TIMEOUT"
		classification.code = "NODE_EXECUTION_TIMEOUT"
		classification.message = "Node execution exceeded its deadline."
		classification.retryable = false
		classification.decisionReason = "DEADLINE_WOULD_BE_EXCEEDED"
	}
	var typed *plugin.NodeError
	if errors.As(err, &typed) {
		classification.category = typed.Category
		classification.code = typed.Code
		classification.message = typed.Message
		classification.retryable = typed.CanRetry
		if !typed.CanRetry {
			classification.decisionReason = "FAILURE_NOT_RETRYABLE"
		}
		return classification
	}
	var marked RetryableError
	if errors.As(err, &marked) {
		classification.retryable = marked.Retryable()
		if !classification.retryable {
			classification.code = "NODE_FAILURE_NOT_RETRYABLE"
			classification.decisionReason = "FAILURE_NOT_RETRYABLE"
		}
	}
	return classification
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
) (string, string) {
	if retry {
		return "RETRY", "RETRY_SCHEDULED"
	}
	if attempt >= maximumAttempts && classification.retryable {
		return "EXHAUSTED", "MAX_ATTEMPTS_REACHED"
	}
	if errors.Is(ctx.Err(), context.Canceled) || !classification.retryable {
		reason := classification.decisionReason
		if reason == "" {
			reason = "FAILURE_NOT_RETRYABLE"
		}
		return "DO_NOT_RETRY", reason
	}
	if !deadline.IsZero() && now.Add(delay).After(deadline) {
		return "DO_NOT_RETRY", "DEADLINE_WOULD_BE_EXCEEDED"
	}
	return "DO_NOT_RETRY", "FAILURE_NOT_RETRYABLE"
}

func failureSummary(
	classification classifiedNodeError,
	retryable bool,
	attempt int,
	decisionKind string,
	decisionReason string,
) map[string]any {
	code := classification.code
	if decisionKind == "EXHAUSTED" {
		code = "NODE_RETRY_EXHAUSTED"
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

func isTerminalNodeStatus(status string) bool {
	switch status {
	case model.NodeSucceeded, model.NodeFailed, model.NodeSkipped,
		model.NodeCancelled, model.NodeTimedOut:
		return true
	default:
		return false
	}
}
