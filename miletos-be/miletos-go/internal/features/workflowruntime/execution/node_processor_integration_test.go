//go:build integration

package execution_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	executionfeature "miletos-go/internal/features/workflowruntime/execution"
	model "miletos-go/internal/features/workflowruntime/execution/model"
	repository "miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/plugin"
	workflowfeature "miletos-go/internal/features/workflowruntime/workflow"
)

func TestNodeProcessorIsolatesPanicAndContinuesIndependentBranchIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	const (
		panicSecret  = "sensitive panic value that must not be exposed"
		configSecret = "sensitive node configuration"
		inputSecret  = "sensitive node input"
	)
	workflow := workflowfeature.Workflow{
		ID: "panic-branches", CompanyID: "company-1", Name: "Panic Branches", Revision: 1,
		Nodes: []workflowfeature.WorkflowNode{
			{
				ID: "panic", Type: "panic-node", Version: "v1",
				Configuration: map[string]any{"secret": configSecret},
			},
			{ID: "dependent", Type: "success-node", Version: "v1"},
			{ID: "independent", Type: "success-node", Version: "v1"},
		},
		Edges: []workflowfeature.Edge{{
			ID: "panic-dependent", SourceNodeID: "panic", SourceOutputPort: "output",
			TargetNodeID: "dependent", TargetInputPort: "input",
		}},
	}
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(engineDatabaseClient(t))
	nodeQueue := &recordingQueue{}
	scheduler := executionfeature.NewScheduler(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, nodeQueue, "commands",
	)
	registry := plugin.NewNodeRegistry()
	var panicCalls atomic.Int32
	if err := registry.DefineNode("panic-node", func(
		context.Context, map[string]any, any,
	) (any, error) {
		panicCalls.Add(1)
		panic(panicSecret)
	}); err != nil {
		t.Fatalf("DefineNode(panic-node) error = %v", err)
	}
	if err := registry.DefineNode("success-node", func(
		_ context.Context, _ map[string]any, input any,
	) (any, error) {
		return input, nil
	}); err != nil {
		t.Fatalf("DefineNode(success-node) error = %v", err)
	}
	processor := executionfeature.NewNodeProcessor(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, registry, scheduler,
		nodeQueue, "commands", 1, time.Millisecond,
	)
	if scheduled, err := scheduler.Activate(
		context.Background(), execution, map[string]any{"secret": inputSecret},
	); err != nil || scheduled != 2 {
		t.Fatalf("Activate() = (%d, %v)", scheduled, err)
	}
	var panicJob, independentJob model.NodeJob
	for _, job := range nodeQueue.snapshot() {
		switch job.NodeID {
		case "panic":
			panicJob = job
		case "independent":
			independentJob = job
		}
	}

	if err := processor.Process(context.Background(), panicJob); err != nil {
		t.Fatalf("panic Process() error = %v", err)
	}
	if err := processor.Process(context.Background(), panicJob); err != nil {
		t.Fatalf("duplicate panic Process() error = %v", err)
	}
	if panicCalls.Load() != 1 {
		t.Fatalf("panic handler calls = %d, want 1", panicCalls.Load())
	}
	failed, err := executions.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "panic",
	)
	if err != nil || failed.Status != model.NodeFailed || failed.Attempt != panicJob.Attempt ||
		failed.CompanyID != workflow.CompanyID || failed.ExecutionID != execution.ID ||
		failed.NodeID != panicJob.NodeID || failed.ID != panicJob.NodeExecutionID {
		t.Fatalf("failed panic node = (%#v, %v)", failed, err)
	}
	assertSafePanicText(t, fmt.Sprint(failed.Failure), panicSecret, configSecret, inputSecret)
	midway, err := executions.FindByID(
		context.Background(), workflow.CompanyID, execution.ID,
	)
	if err != nil || midway.Status != model.ExecutionRunning {
		t.Fatalf("execution before independent branch = (%#v, %v)", midway, err)
	}
	dependent, err := executions.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "dependent",
	)
	if err != nil || dependent.Status != model.NodePending {
		t.Fatalf("dependent before finalization = (%#v, %v)", dependent, err)
	}

	if err := processor.Process(context.Background(), independentJob); err != nil {
		t.Fatalf("independent Process() error = %v", err)
	}
	completed, err := executions.FindByID(
		context.Background(), workflow.CompanyID, execution.ID,
	)
	if err != nil || completed.Status != model.ExecutionFailed ||
		completed.WorkflowID != workflow.ID ||
		completed.CompanyID != workflow.CompanyID ||
		completed.CorrelationID != execution.CorrelationID {
		t.Fatalf("completed execution = (%#v, %v)", completed, err)
	}
	dependent, err = executions.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "dependent",
	)
	if err != nil || dependent.Status != model.NodeSkipped {
		t.Fatalf("dependent after finalization = (%#v, %v)", dependent, err)
	}
	if jobs := nodeQueue.snapshot(); len(jobs) != 2 {
		t.Fatalf("unexpected jobs after panic exhaustion = %#v", jobs)
	}

	errorsPage, err := executions.ListErrors(
		context.Background(), workflow.CompanyID, execution.ID, "", 100,
	)
	if err != nil || len(errorsPage.Items) != 1 {
		t.Fatalf("ListErrors() = (%#v, %v)", errorsPage, err)
	}
	persistedError := errorsPage.Items[0]
	if persistedError.ExecutionID != execution.ID ||
		persistedError.NodeExecutionID != panicJob.NodeExecutionID ||
		persistedError.Retryable {
		t.Fatalf("persisted panic error = %#v", persistedError)
	}
	assertSafePanicText(t, fmt.Sprint(persistedError), panicSecret, configSecret, inputSecret)
	logs, err := executions.ListLogs(
		context.Background(), workflow.CompanyID, execution.ID, "", 100,
	)
	if err != nil || len(logs.Items) == 0 {
		t.Fatalf("ListLogs() = (%#v, %v)", logs, err)
	}
	events, err := executions.ListEvents(
		context.Background(), workflow.CompanyID, execution.ID, "", 100,
	)
	if err != nil || len(events.Items) == 0 {
		t.Fatalf("ListEvents() = (%#v, %v)", events, err)
	}
	foundFailureEvent := false
	for _, event := range events.Items {
		if event.Type == "NODE_FAILED" && event.NodeExecutionID == panicJob.NodeExecutionID {
			foundFailureEvent = true
			if event.CorrelationID != execution.CorrelationID {
				t.Fatalf("failure event correlation = %q", event.CorrelationID)
			}
		}
	}
	if !foundFailureEvent {
		t.Fatalf("NODE_FAILED event absent: %#v", events.Items)
	}
	var attemptStatus string
	var attemptNumber int
	if err := pool.QueryRow(context.Background(), `
		SELECT attempt_status, attempt
		FROM workflow_runtime.node_execution_attempts
		WHERE workflow_execution_id = $1 AND node_execution_id = $2`,
		execution.ID, panicJob.NodeExecutionID,
	).Scan(&attemptStatus, &attemptNumber); err != nil {
		t.Fatalf("load panic attempt: %v", err)
	}
	if attemptStatus != model.NodeFailed || attemptNumber != panicJob.Attempt {
		t.Fatalf("panic attempt = (%s, %d)", attemptStatus, attemptNumber)
	}
}

func TestNodeProcessorPanicRetryThenExhaustionIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := workflowfeature.Workflow{
		ID: "panic-retry", CompanyID: "company-1", Name: "Panic Retry", Revision: 1,
		Nodes: []workflowfeature.WorkflowNode{{ID: "panic", Type: "panic-node", Version: "v1"}},
	}
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(engineDatabaseClient(t))
	nodeQueue := &recordingQueue{}
	scheduler := executionfeature.NewScheduler(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, nodeQueue, "commands",
	)
	registry := plugin.NewNodeRegistry()
	var calls atomic.Int32
	if err := registry.DefineNode("panic-node", func(
		context.Context, map[string]any, any,
	) (any, error) {
		calls.Add(1)
		panic("retry panic secret")
	}); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}
	retryDelay := time.Millisecond
	processor := executionfeature.NewNodeProcessor(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, registry, scheduler,
		nodeQueue, "commands", 2, retryDelay,
	)
	if _, err := scheduler.Activate(context.Background(), execution, nil); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	firstJob := nodeQueue.snapshot()[0]

	if err := processor.Process(context.Background(), firstJob); err != nil {
		t.Fatalf("first panic Process() error = %v", err)
	}

	jobs := nodeQueue.snapshot()
	if len(jobs) != 2 || jobs[1].Attempt != 2 ||
		jobs[1].NodeExecutionID != firstJob.NodeExecutionID ||
		jobs[1].CorrelationID != execution.CorrelationID {
		t.Fatalf("panic retry jobs = %#v", jobs)
	}
	var finishedAt, updatedAt, nextAttemptAt, errorCreatedAt time.Time
	var retryBackoff int64
	var decisionKind string
	if err := pool.QueryRow(context.Background(), `
		SELECT finished_at, updated_at, next_attempt_at, retry_backoff_ns,
			retry_decision_kind
		FROM workflow_runtime.node_execution_attempts
		WHERE workflow_execution_id = $1 AND node_execution_id = $2 AND attempt = 1`,
		execution.ID, firstJob.NodeExecutionID,
	).Scan(&finishedAt, &updatedAt, &nextAttemptAt, &retryBackoff, &decisionKind); err != nil {
		t.Fatalf("load retry attempt: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT created_at
		FROM workflow_runtime.execution_errors
		WHERE workflow_execution_id = $1 AND node_execution_id = $2
		ORDER BY created_at LIMIT 1`,
		execution.ID, firstJob.NodeExecutionID,
	).Scan(&errorCreatedAt); err != nil {
		t.Fatalf("load retry execution error: %v", err)
	}
	if !finishedAt.Equal(updatedAt) || !finishedAt.Equal(errorCreatedAt) ||
		!nextAttemptAt.Equal(finishedAt.Add(retryDelay)) ||
		retryBackoff != retryDelay.Nanoseconds() ||
		decisionKind != "RETRY" {
		t.Fatalf(
			"retry timestamps/decision = finished:%v updated:%v error:%v next:%v backoff:%d kind:%s",
			finishedAt, updatedAt, errorCreatedAt, nextAttemptAt, retryBackoff, decisionKind,
		)
	}

	if err := processor.Process(context.Background(), jobs[1]); err != nil {
		t.Fatalf("exhausting panic Process() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("panic handler calls = %d, want 2", calls.Load())
	}
	if jobsAfter := nodeQueue.snapshot(); len(jobsAfter) != 2 {
		t.Fatalf("retry exhaustion queued another job: %#v", jobsAfter)
	}
	failed, err := executions.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "panic",
	)
	if err != nil || failed.Status != model.NodeFailed || failed.Attempt != 2 {
		t.Fatalf("exhausted panic node = (%#v, %v)", failed, err)
	}
	completed, err := executions.FindByID(
		context.Background(), workflow.CompanyID, execution.ID,
	)
	if err != nil || completed.Status != model.ExecutionFailed {
		t.Fatalf("exhausted panic execution = (%#v, %v)", completed, err)
	}
	var exhaustedKind string
	var exhaustedNext *time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT retry_decision_kind, next_attempt_at
		FROM workflow_runtime.node_execution_attempts
		WHERE workflow_execution_id = $1 AND node_execution_id = $2 AND attempt = 2`,
		execution.ID, firstJob.NodeExecutionID,
	).Scan(&exhaustedKind, &exhaustedNext); err != nil {
		t.Fatalf("load exhausted attempt: %v", err)
	}
	if exhaustedKind != "EXHAUSTED" || exhaustedNext != nil {
		t.Fatalf("exhausted decision = (%s, %v)", exhaustedKind, exhaustedNext)
	}
}

func TestNodeProcessorSyncPanicReturnsSafeErrorIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := workflowfeature.Workflow{
		ID: "panic-sync", CompanyID: "company-1", Name: "Panic Sync", Revision: 1,
		Nodes: []workflowfeature.WorkflowNode{{
			ID: "panic", Type: "panic-node", Version: "v1",
			Configuration: map[string]any{"secret": "config secret"},
		}},
	}
	execution := seedEngineExecution(t, pool, workflow, "SYNC")
	executions := repository.NewExecutionRepository(engineDatabaseClient(t))
	node, changed, err := executions.MarkNodeQueued(
		context.Background(), workflow.CompanyID, execution.ID, "panic",
	)
	if err != nil || !changed {
		t.Fatalf("MarkNodeQueued() = (%#v, %v, %v)", node, changed, err)
	}
	job := model.NodeJob{
		CompanyID: workflow.CompanyID, WorkflowID: workflow.ID, ExecutionID: execution.ID,
		NodeID: "panic", NodeExecutionID: node.ID, Attempt: node.Attempt,
		CorrelationID: execution.CorrelationID, Payload: "input secret",
	}
	registry := plugin.NewNodeRegistry()
	if err := registry.DefineNode("panic-node", func(
		context.Context, map[string]any, any,
	) (any, error) {
		panic("panic secret")
	}); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}
	processor := executionfeature.NewNodeProcessor(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, registry, nil,
		nil, "", 1, time.Millisecond,
	)

	_, executionError := processor.ProcessSync(context.Background(), job, workflow.Nodes[0])

	if executionError == nil {
		t.Fatal("ProcessSync() error = nil")
	}
	assertSafePanicText(
		t, executionError.Error(), "panic secret", "config secret", "input secret",
	)
}

func assertSafePanicText(t *testing.T, text string, secrets ...string) {
	t.Helper()
	if !strings.Contains(text, "node handler panicked") {
		t.Fatalf("panic text = %q, want safe panic phrase", text)
	}
	for _, secret := range secrets {
		if strings.Contains(text, secret) {
			t.Fatalf("panic text exposed %q: %q", secret, text)
		}
	}
	for _, stackMarker := range []string{"goroutine ", ".go:", "runtime/debug.Stack"} {
		if strings.Contains(text, stackMarker) {
			t.Fatalf("panic text exposed stack marker %q: %q", stackMarker, text)
		}
	}
}

func TestNodeProcessorExecutesAndContinuesIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(engineDatabaseClient(t))
	nodeQueue := &recordingQueue{}
	scheduler := executionfeature.NewScheduler(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, nodeQueue, "commands",
	)
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	processor := executionfeature.NewNodeProcessor(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, registry, scheduler,
		nodeQueue, "commands", 1, 0,
	)
	if _, err := scheduler.Activate(context.Background(), execution, nil); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	rootJob := nodeQueue.snapshot()[0]

	if err := processor.Process(context.Background(), rootJob); err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	root, err := executions.FindNode(
		context.Background(), workflow.CompanyID, execution.ID, "root",
	)
	if err != nil || root.Status != model.NodeSucceeded || root.Output["value"] != "payload" {
		t.Fatalf("root node = (%#v, %v)", root, err)
	}
	jobs := nodeQueue.snapshot()
	if len(jobs) != 2 || jobs[1].NodeID != "terminal" {
		t.Fatalf("continued jobs = %#v", jobs)
	}
}

func TestNodeProcessorUnknownHandlerDoesNotStallIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := workflowfeature.Workflow{
		ID: "unknown-workflow", CompanyID: "company-1", Name: "Unknown", Revision: 1,
		Nodes: []workflowfeature.WorkflowNode{{ID: "unknown", Type: "unknown.node", Version: "v1"}},
	}
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(engineDatabaseClient(t))
	nodeQueue := &recordingQueue{}
	scheduler := executionfeature.NewScheduler(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, nodeQueue, "commands",
	)
	processor := executionfeature.NewNodeProcessor(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, plugin.NewNodeRegistry(), scheduler,
		nodeQueue, "commands", 1, 0,
	)
	if _, err := scheduler.Activate(context.Background(), execution, nil); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	if err := processor.Process(context.Background(), nodeQueue.snapshot()[0]); err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	completed, err := executions.FindByID(context.Background(), workflow.CompanyID, execution.ID)
	if err != nil || completed.Status != model.ExecutionFailed || completed.FinishedAt == nil {
		t.Fatalf("completed execution = (%#v, %v)", completed, err)
	}
}

func TestNodeProcessorRetriesThenSucceedsAndIgnoresDuplicateDeliveryIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := workflowfeature.Workflow{
		ID: "retry-workflow", CompanyID: "company-1", Name: "Retry", Revision: 1,
		Nodes: []workflowfeature.WorkflowNode{{ID: "retry", Type: "test.retry", Version: "v1"}},
	}
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(engineDatabaseClient(t))
	nodeQueue := &recordingQueue{}
	scheduler := executionfeature.NewScheduler(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, nodeQueue, "commands",
	)
	registry := plugin.NewNodeRegistry()
	var calls atomic.Int32
	if err := registry.DefineNode("test.retry", func(
		context.Context, map[string]any, any,
	) (any, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("retry me")
		}
		return "success", nil
	}); err != nil {
		t.Fatalf("DefineNode() error = %v", err)
	}
	processor := executionfeature.NewNodeProcessor(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, registry, scheduler,
		nodeQueue, "commands", 2, time.Millisecond,
	)
	if _, err := scheduler.Activate(context.Background(), execution, nil); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	firstJob := nodeQueue.snapshot()[0]
	if err := processor.Process(context.Background(), firstJob); err != nil {
		t.Fatalf("first Process() error = %v", err)
	}
	jobs := nodeQueue.snapshot()
	if len(jobs) != 2 || jobs[1].Attempt != 2 ||
		jobs[1].CorrelationID != execution.CorrelationID {
		t.Fatalf("retry jobs = %#v", jobs)
	}
	if err := processor.Process(context.Background(), jobs[1]); err != nil {
		t.Fatalf("retry Process() error = %v", err)
	}
	if err := processor.Process(context.Background(), jobs[1]); err != nil {
		t.Fatalf("duplicate Process() error = %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2", calls.Load())
	}
	completed, err := executions.FindByID(context.Background(), workflow.CompanyID, execution.ID)
	if err != nil || completed.Status != model.ExecutionSucceeded {
		t.Fatalf("completed execution = (%#v, %v)", completed, err)
	}
}

func TestNodeProcessorRejectsCompanyMismatchIntegration(t *testing.T) {
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()
	execution := seedEngineExecution(t, pool, workflow, "ASYNC")
	executions := repository.NewExecutionRepository(engineDatabaseClient(t))
	nodeQueue := &recordingQueue{}
	scheduler := executionfeature.NewScheduler(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, nodeQueue, "commands",
	)
	processor := executionfeature.NewNodeProcessor(
		workflowfeature.NewWorkflowRepository(engineDatabaseClient(t)), executions, plugin.NewNodeRegistry(), scheduler,
		nodeQueue, "commands", 1, 0,
	)
	if _, err := scheduler.Activate(context.Background(), execution, nil); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	job := nodeQueue.snapshot()[0]
	job.CompanyID = "another-company"

	if err := processor.Process(context.Background(), job); err == nil {
		t.Fatal("company-mismatched Process() error = nil")
	}
}
