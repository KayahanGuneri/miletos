package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/engine/modules"
	"miletos-go/internal/model"
	"miletos-go/internal/queue"
	"miletos-go/internal/repository"
)

type NodeProcessor struct {
	workflows       *repository.WorkflowRepository
	executions      *repository.ExecutionRepository
	registry        *NodeRegistry
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
	category   string
	code       string
	message    string
	retryable  bool
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

func NewNodeProcessor(
	workflows *repository.WorkflowRepository,
	executions *repository.ExecutionRepository,
	registry *NodeRegistry,
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

func (processor *NodeProcessor) consume(ctx context.Context, payload []byte) error {
	var job model.NodeJob
	if err := json.Unmarshal(payload, &job); err != nil {
		return fmt.Errorf("decode node job: %w", err)
	}
	return processor.Process(ctx, job)
}

func (processor *NodeProcessor) Process(ctx context.Context, job model.NodeJob) error {
	snapshot, err := processor.workflows.FindByExecutionID(ctx, job.CompanyID, job.ExecutionID)
	if err != nil {
		return fmt.Errorf("load workflow for node job: %w", err)
	}
	node, exists := findWorkflowNode(snapshot.Workflow, job.NodeID)
	if !exists {
		return fmt.Errorf("node %q is absent from workflow", job.NodeID)
	}
	if err := processor.executions.MarkExecutionRunning(ctx, job.ExecutionID); err != nil {
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
	_, err = processor.processAttempts(ctx, job, node)
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
	node model.Node,
) (any, error) {
	return processor.processAttempts(ctx, job, node)
}

func (processor *NodeProcessor) processAttempts(
	ctx context.Context,
	job model.NodeJob,
	node model.Node,
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
			executionError = fmt.Errorf("node handler is not registered")
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
		decision := modules.DecideRetry(
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
		if err := processor.executions.SaveNodeFailure(
			ctx, job, failureSummary(
				classification, decision.Retry, job.Attempt, decisionKind, decisionReason,
			), decision.Retry,
			failedAt, nextAttempt, processor.maximumAttempts, processor.retryDelay,
		); err != nil {
			return nil, err
		}
		if !decision.Retry {
			return nil, &PersistedNodeFailure{Cause: executionError}
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
	handler NodeHandler,
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

func findWorkflowNode(workflow model.Workflow, nodeID string) (model.Node, bool) {
	for _, node := range workflow.Nodes {
		if node.ID == nodeID {
			return node, true
		}
	}
	return model.Node{}, false
}

func classifyNodeError(err error, handlerExists bool) classifiedNodeError {
	classification := classifiedNodeError{
		category: "EXECUTION", code: "NODE_EXECUTION_FAILED",
		message: "Node execution failed.", retryable: true,
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
		classification.retryable = true
		classification.decisionReason = "DEADLINE_WOULD_BE_EXCEEDED"
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
		"category":  classification.category,
		"code":      code,
		"message":   classification.message,
		"retryable": retryable,
		"attempt": attempt,
		"retryDecisionKind": decisionKind,
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
