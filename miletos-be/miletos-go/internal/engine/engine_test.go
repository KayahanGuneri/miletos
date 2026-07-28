package engine

import (
	context "context"
	errors "errors"
	fmt "fmt"
	graph "miletos-go/internal/engine/graph"
	core "miletos-go/internal/engine/nodes/core"
	plugin "miletos-go/internal/engine/plugin"
	runtime "miletos-go/internal/engine/runtime"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	transport "miletos-go/internal/infra/kafka"
	sharedclock "miletos-go/internal/shared/clock"
	reflect "reflect"
	strings "strings"
	sync "sync"
	atomic "sync/atomic"
	testing "testing"
	time "time"
)

type resultConsumerRunnerStub struct {
	runCalled bool
	closed    bool
}

func (stub *resultConsumerRunnerStub) Run(ctx context.Context, handler transport.DeliveryHandler) error {
	stub.runCalled = true
	if ctx == nil || handler == nil {
		return context.Canceled
	}
	return nil
}

func (stub *resultConsumerRunnerStub) Close() { stub.closed = true }

func TestAsyncResultConsumerRequiresDependencies(t *testing.T) {
	if _, err := NewAsyncResultConsumer(nil, nil); err == nil {
		t.Fatal("NewAsyncResultConsumer() accepted nil dependencies")
	}
}

func TestAsyncResultConsumerDelegatesRunAndClose(t *testing.T) {
	stub := &resultConsumerRunnerStub{}
	consumer := &AsyncResultConsumer{consumer: stub, processor: &AsyncResultProcessor{}}
	if err := consumer.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	consumer.Close()
	if !stub.runCalled || !stub.closed {
		t.Fatalf("delegation run=%v close=%v", stub.runCalled, stub.closed)
	}
}

func TestClockFuncReturnsProvidedTime(
	t *testing.T,
) {
	expected := time.Date(
		2026,
		time.July,
		17,
		9,
		30,
		0,
		0,
		time.UTC,
	)

	callCount := 0

	clock := sharedclock.From(
		func() time.Time {
			callCount++
			return expected
		},
	)

	actual := clock.Now()

	if !actual.Equal(expected) {
		t.Fatalf(
			"Now() = %v, want %v",
			actual,
			expected,
		)
	}

	if callCount != 1 {
		t.Fatalf(
			"clock function call count = %d, want 1",
			callCount,
		)
	}
}

func TestNilClockFuncReturnsZeroTime(
	t *testing.T,
) {
	var clock sharedclock.Function

	if actual := clock.Now(); !actual.IsZero() {
		t.Fatalf(
			"nil ClockFunc Now() = %v, want zero time",
			actual,
		)
	}
}

func TestSystemClockReturnsCurrentUTCTime(
	t *testing.T,
) {
	before := time.Now().UTC()

	actual := sharedclock.System().Now()

	after := time.Now().UTC()

	if actual.Location() != time.UTC {
		t.Fatalf(
			"Now() location = %v, want UTC",
			actual.Location(),
		)
	}

	if actual.Before(before) {
		t.Fatalf(
			"Now() = %v, before lower bound %v",
			actual,
			before,
		)
	}

	if actual.After(after) {
		t.Fatalf(
			"Now() = %v, after upper bound %v",
			actual,
			after,
		)
	}
}

func TestSyncRunnerCancelsRealCoreDelayNodeEndToEnd(
	t *testing.T,
) {
	const expectedDelay = 2 * time.Second

	signalContext :=
		newNodeExecutionSignalContext(
			false,
		)

	waitCallCount := 0

	var observedDelay time.Duration

	waiter := core.WaiterFunc(
		func(
			ctx context.Context,
			duration time.Duration,
		) error {
			if ctx == nil {
				t.Fatal(
					"core delay waiter received a nil context",
				)
			}

			if err := ctx.Err(); err != nil {
				t.Fatalf(
					"delay context was already completed before waiter invocation: %v",
					err,
				)
			}

			waitCallCount++
			observedDelay = duration

			signalContext.Fail(
				context.Canceled,
			)

			if err := ctx.Err(); err != nil {
				return err
			}

			return context.Canceled
		},
	)

	runner, request :=
		mustRealCoreSyncRunner(
			t,
			waiter,
			&nodeExecutionTestClock{
				next: time.Date(
					2026,
					time.July,
					28,
					10,
					0,
					0,
					0,
					time.UTC,
				),
			},
			"workflow-core-cancellation-e2e",
			`{"value":{"message":"cancel-core-delay"}}`,
			`{"delay":"2s"}`,
		)

	result, err := runner.Run(
		signalContext,
		request,
	)
	if err != nil {
		t.Fatalf(
			"SyncRunner.Run() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"real-core cancellation SyncRunResult is invalid",
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"real-core cancellation result is not terminal",
		)
	}

	if !result.IsCancelled() {
		t.Fatalf(
			"sync-run status = %q, want CANCELLED",
			result.Status(),
		)
	}

	if result.Status() !=
		execution.WorkflowExecutionStatusCancelled {
		t.Fatalf(
			"workflow status = %q, want %q",
			result.Status(),
			execution.WorkflowExecutionStatusCancelled,
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"cancelled real-core workflow is marked as rejected",
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"cancelled real-core workflow is marked as succeeded",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"cancelled real-core workflow is marked as failed",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"cancelled real-core workflow is marked as timed out",
		)
	}

	validationReport :=
		result.ValidationReport()

	if !validationReport.IsValid() {
		t.Fatalf(
			"real-core cancellation preflight contains %d issues",
			validationReport.Len(),
		)
	}

	if validationReport.Len() != 0 {
		t.Fatalf(
			"validation issue count = %d, want 0",
			validationReport.Len(),
		)
	}

	if !result.HasExecutionResult() {
		t.Fatal(
			"cancelled real-core result contains no ExecutionResult",
		)
	}

	executionResult, exists :=
		result.ExecutionResult()

	if !exists {
		t.Fatal(
			"ExecutionResult() exists = false",
		)
	}

	if !executionResult.IsTerminal() {
		t.Fatal(
			"real-core cancellation ExecutionResult is not terminal",
		)
	}

	if !executionResult.IsCancelled() {
		t.Fatalf(
			"execution-result status = %q, want CANCELLED",
			executionResult.Status(),
		)
	}

	if executionResult.IsStalled() {
		t.Fatal(
			"real-core cancellation is marked as stalled",
		)
	}

	if executionResult.Status() !=
		result.Status() {
		t.Fatalf(
			"execution-result status = %q, sync-run status = %q",
			executionResult.Status(),
			result.Status(),
		)
	}

	if executionResult.WorkflowExecution().ID() !=
		result.WorkflowExecution().ID() {
		t.Fatalf(
			"execution-result workflow ID = %q, sync-run workflow ID = %q",
			executionResult.WorkflowExecution().ID(),
			result.WorkflowExecution().ID(),
		)
	}

	expectedExecutedOrder := []workflow.NodeID{
		workflow.NodeID("static-input"),
		workflow.NodeID("pass-through"),
		workflow.NodeID("delay"),
	}

	if !reflect.DeepEqual(
		executionResult.ExecutedNodeOrder(),
		expectedExecutedOrder,
	) {
		t.Fatalf(
			"executed node order = %#v, want %#v",
			executionResult.ExecutedNodeOrder(),
			expectedExecutedOrder,
		)
	}

	expectedNodeOrder := []workflow.NodeID{
		workflow.NodeID("static-input"),
		workflow.NodeID("pass-through"),
		workflow.NodeID("delay"),
		workflow.NodeID("terminal"),
	}

	if !reflect.DeepEqual(
		executionResult.NodeExecutionOrder(),
		expectedNodeOrder,
	) {
		t.Fatalf(
			"node execution order = %#v, want %#v",
			executionResult.NodeExecutionOrder(),
			expectedNodeOrder,
		)
	}

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("static-input"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("pass-through"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("delay"),
		execution.NodeExecutionStatusCancelled,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusCancelled,
	)

	requireSyncRunnerStartedAndFinishedNode(
		t,
		executionResult,
		workflow.NodeID("static-input"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireSyncRunnerStartedAndFinishedNode(
		t,
		executionResult,
		workflow.NodeID("pass-through"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireSyncRunnerStartedAndFinishedNode(
		t,
		executionResult,
		workflow.NodeID("delay"),
		execution.NodeExecutionStatusCancelled,
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		executionResult,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusCancelled,
	)

	if waitCallCount != 1 {
		t.Fatalf(
			"delay waiter call count = %d, want 1",
			waitCallCount,
		)
	}

	if observedDelay != expectedDelay {
		t.Fatalf(
			"delay waiter duration = %v, want %v",
			observedDelay,
			expectedDelay,
		)
	}

	workflowFailure, exists :=
		executionResult.WorkflowFailure()

	if !exists {
		t.Fatal(
			"cancelled real-core execution contains no workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryCanceled {
		t.Fatalf(
			"workflow failure category = %q, want CANCELED",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowCanceled {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowCanceled,
		)
	}

	if actual :=
		workflowFailure.Details()["contextError"]; actual != context.Canceled.Error() {
		t.Fatalf(
			"contextError detail = %q, want %q",
			actual,
			context.Canceled.Error(),
		)
	}

	if failures :=
		executionResult.NodeFailures(); failures != nil {
		t.Fatalf(
			"cancelled real-core node failures = %#v, want nil",
			failures,
		)
	}

	if outputs :=
		executionResult.TerminalOutputs(); outputs != nil {
		t.Fatalf(
			"cancelled real-core terminal outputs = %#v, want nil",
			outputs,
		)
	}

	if _, exists, err :=
		executionResult.TerminalOutput(
			workflow.NodeID("terminal"),
		); err != nil {
		t.Fatalf(
			"TerminalOutput(terminal) returned an unexpected error: %v",
			err,
		)
	} else if exists {
		t.Fatal(
			"cancelled real-core execution contains a terminal output",
		)
	}
}

func mustRealCoreSyncRunner(
	t *testing.T,
	waiter core.Waiter,
	clock sharedclock.Clock,
	workflowID string,
	staticConfiguration string,
	delayConfiguration string,
) (
	SyncRunner,
	ExecutionRequest,
) {
	t.Helper()

	limits := mustValidationRuntimeLimits(
		t,
		64,
		1,
		4096,
	)

	descriptors, err :=
		core.CoreDescriptors()
	if err != nil {
		t.Fatalf(
			"core.CoreDescriptors() returned an unexpected error: %v",
			err,
		)
	}

	pluginRegistry, err :=
		plugin.NewRegistry(
			descriptors,
		)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	registrations, err :=
		core.ExecutorRegistrations(
			limits,
			waiter,
		)
	if err != nil {
		t.Fatalf(
			"core.ExecutorRegistrations() returned an unexpected error: %v",
			err,
		)
	}

	executorRegistry, err :=
		runtime.NewExecutorRegistry(
			registrations,
		)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	dependencies, err :=
		NewEngineDependencies(
			pluginRegistry,
			executorRegistry,
			limits,
			clock,
		)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	staticInput := mustValidationNode(
		t,
		"static-input",
		core.StaticInputPluginType.String(),
		core.CorePluginVersion.String(),
		staticConfiguration,
	)

	passThrough := mustValidationNode(
		t,
		"pass-through",
		core.PassThroughPluginType.String(),
		core.CorePluginVersion.String(),
		`{}`,
	)

	delay := mustValidationNode(
		t,
		"delay",
		core.DelayPluginType.String(),
		core.CorePluginVersion.String(),
		delayConfiguration,
	)

	terminal := mustValidationNode(
		t,
		"terminal",
		core.TerminalPluginType.String(),
		core.CorePluginVersion.String(),
		`{}`,
	)

	staticToPass := mustValidationEdge(
		t,
		"edge-static-pass",
		"static-input",
		core.OutputPortName,
		"pass-through",
		core.InputPortName,
	)

	passToDelay := mustValidationEdge(
		t,
		"edge-pass-delay",
		"pass-through",
		core.OutputPortName,
		"delay",
		core.InputPortName,
	)

	delayToTerminal := mustValidationEdge(
		t,
		"edge-delay-terminal",
		"delay",
		core.OutputPortName,
		"terminal",
		core.InputPortName,
	)

	definition := mustValidationDefinition(
		t,
		workflowID,
		[]workflow.NodeDefinition{
			staticInput,
			passThrough,
			delay,
			terminal,
		},
		[]workflow.EdgeDefinition{
			staticToPass,
			passToDelay,
			delayToTerminal,
		},
	)

	request :=
		mustValidationExecutionRequest(
			t,
			definition,
		)

	runner, err :=
		NewSyncRunner(
			dependencies,
		)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	return runner, request
}

func TestSyncRunnerExecutesRealCoreNodeChainEndToEnd(
	t *testing.T,
) {
	const expectedPayload = `{"message":"miletos-core-e2e","count":4}`

	const expectedDelay = 250 * time.Millisecond

	/*
		Core descriptor queue/cache defaults:

		queue capacity = 64
		cache capacity = 1

		Runtime limits bu production descriptor değerlerini
		destekleyecek şekilde hazırlanır.
	*/
	limits := mustValidationRuntimeLimits(
		t,
		64,
		1,
		4096,
	)

	descriptors, err :=
		core.CoreDescriptors()
	if err != nil {
		t.Fatalf(
			"core.CoreDescriptors() returned an unexpected error: %v",
			err,
		)
	}

	if len(descriptors) != 4 {
		t.Fatalf(
			"core descriptor count = %d, want 4",
			len(descriptors),
		)
	}

	pluginRegistry, err :=
		plugin.NewRegistry(
			descriptors,
		)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	waitDurations := make(
		[]time.Duration,
		0,
		1,
	)

	waiter := core.WaiterFunc(
		func(
			ctx context.Context,
			duration time.Duration,
		) error {
			if ctx == nil {
				t.Fatal(
					"core delay waiter received a nil context",
				)
			}

			if err := ctx.Err(); err != nil {
				t.Fatalf(
					"core delay waiter received a completed context: %v",
					err,
				)
			}

			if duration <= 0 {
				t.Fatalf(
					"core delay waiter duration = %v, want positive duration",
					duration,
				)
			}

			waitDurations = append(
				waitDurations,
				duration,
			)

			return nil
		},
	)

	registrations, err :=
		core.ExecutorRegistrations(
			limits,
			waiter,
		)
	if err != nil {
		t.Fatalf(
			"core.ExecutorRegistrations() returned an unexpected error: %v",
			err,
		)
	}

	if len(registrations) != 4 {
		t.Fatalf(
			"core executor registration count = %d, want 4",
			len(registrations),
		)
	}

	executorRegistry, err :=
		runtime.NewExecutorRegistry(
			registrations,
		)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	if executorRegistry.Len() != 4 {
		t.Fatalf(
			"executor registry length = %d, want 4",
			executorRegistry.Len(),
		)
	}

	dependencies, err :=
		NewEngineDependencies(
			pluginRegistry,
			executorRegistry,
			limits,
			&nodeExecutionTestClock{
				next: time.Date(
					2026,
					time.July,
					27,
					10,
					0,
					0,
					0,
					time.UTC,
				),
			},
		)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	staticInput := mustValidationNode(
		t,
		"static-input",
		core.StaticInputPluginType.String(),
		core.CorePluginVersion.String(),
		`{"value":{"message":"miletos-core-e2e","count":4}}`,
	)

	passThrough := mustValidationNode(
		t,
		"pass-through",
		core.PassThroughPluginType.String(),
		core.CorePluginVersion.String(),
		`{}`,
	)

	delay := mustValidationNode(
		t,
		"delay",
		core.DelayPluginType.String(),
		core.CorePluginVersion.String(),
		`{"delay":"250ms"}`,
	)

	terminal := mustValidationNode(
		t,
		"terminal",
		core.TerminalPluginType.String(),
		core.CorePluginVersion.String(),
		`{}`,
	)

	staticToPass := mustValidationEdge(
		t,
		"edge-static-pass",
		"static-input",
		core.OutputPortName,
		"pass-through",
		core.InputPortName,
	)

	passToDelay := mustValidationEdge(
		t,
		"edge-pass-delay",
		"pass-through",
		core.OutputPortName,
		"delay",
		core.InputPortName,
	)

	delayToTerminal := mustValidationEdge(
		t,
		"edge-delay-terminal",
		"delay",
		core.OutputPortName,
		"terminal",
		core.InputPortName,
	)

	definition := mustValidationDefinition(
		t,
		"workflow-core-sync-e2e",
		[]workflow.NodeDefinition{
			staticInput,
			passThrough,
			delay,
			terminal,
		},
		[]workflow.EdgeDefinition{
			staticToPass,
			passToDelay,
			delayToTerminal,
		},
	)

	request :=
		mustValidationExecutionRequest(
			t,
			definition,
		)

	runner, err :=
		NewSyncRunner(
			dependencies,
		)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	if !runner.IsValid() {
		t.Fatal(
			"NewSyncRunner() returned an invalid runner",
		)
	}

	result, err := runner.Run(
		context.Background(),
		request,
	)
	if err != nil {
		t.Fatalf(
			"SyncRunner.Run() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"real-core SyncRunResult is invalid",
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"real-core SyncRunResult is not terminal",
		)
	}

	if !result.IsSucceeded() {
		t.Fatalf(
			"real-core workflow status = %q, want SUCCEEDED",
			result.Status(),
		)
	}

	if result.Status() !=
		execution.WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"workflow status = %q, want %q",
			result.Status(),
			execution.WorkflowExecutionStatusSucceeded,
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"real-core execution is marked as rejected",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"real-core execution is marked as failed",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"real-core execution is marked as cancelled",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"real-core execution is marked as timed out",
		)
	}

	validationReport :=
		result.ValidationReport()

	if !validationReport.IsValid() {
		t.Fatalf(
			"real-core preflight report contains %d issues",
			validationReport.Len(),
		)
	}

	if validationReport.Len() != 0 {
		t.Fatalf(
			"real-core validation issue count = %d, want 0",
			validationReport.Len(),
		)
	}

	if !result.HasExecutionResult() {
		t.Fatal(
			"real-core SyncRunResult contains no ExecutionResult",
		)
	}

	executionResult, exists :=
		result.ExecutionResult()

	if !exists {
		t.Fatal(
			"ExecutionResult() exists = false",
		)
	}

	if !executionResult.IsTerminal() {
		t.Fatal(
			"real-core ExecutionResult is not terminal",
		)
	}

	if !executionResult.IsSucceeded() {
		t.Fatalf(
			"execution-result status = %q, want SUCCEEDED",
			executionResult.Status(),
		)
	}

	if executionResult.IsStalled() {
		t.Fatal(
			"real-core execution is marked as stalled",
		)
	}

	if executionResult.WorkflowExecution().ID() !=
		result.WorkflowExecution().ID() {
		t.Fatalf(
			"execution-result workflow ID = %q, sync-run workflow ID = %q",
			executionResult.WorkflowExecution().ID(),
			result.WorkflowExecution().ID(),
		)
	}

	if executionResult.Status() !=
		result.Status() {
		t.Fatalf(
			"execution-result status = %q, sync-run status = %q",
			executionResult.Status(),
			result.Status(),
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("static-input"),
		workflow.NodeID("pass-through"),
		workflow.NodeID("delay"),
		workflow.NodeID("terminal"),
	}

	if !reflect.DeepEqual(
		executionResult.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"executed node order = %#v, want %#v",
			executionResult.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		executionResult.NodeExecutionOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"node execution order = %#v, want %#v",
			executionResult.NodeExecutionOrder(),
			expectedOrder,
		)
	}

	for _, nodeID := range expectedOrder {
		requireExecutionResultNodeStatus(
			t,
			executionResult,
			nodeID,
			execution.NodeExecutionStatusSucceeded,
		)

		requireSyncRunnerStartedAndFinishedNode(
			t,
			executionResult,
			nodeID,
			execution.NodeExecutionStatusSucceeded,
		)
	}

	if len(waitDurations) != 1 {
		t.Fatalf(
			"delay waiter call count = %d, want 1",
			len(waitDurations),
		)
	}

	if waitDurations[0] != expectedDelay {
		t.Fatalf(
			"delay waiter duration = %v, want %v",
			waitDurations[0],
			expectedDelay,
		)
	}

	terminalOutputs :=
		executionResult.TerminalOutputs()

	if len(terminalOutputs) != 1 {
		t.Fatalf(
			"terminal output count = %d, want 1",
			len(terminalOutputs),
		)
	}

	terminalOutput, exists, err :=
		executionResult.TerminalOutput(
			workflow.NodeID("terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"real-core terminal output does not exist",
		)
	}

	inlineData, exists :=
		terminalOutput.InlineData()

	if !exists {
		t.Fatal(
			"real-core terminal output contains no inline data",
		)
	}

	if actual := string(inlineData); actual != expectedPayload {
		t.Fatalf(
			"terminal payload = %q, want %q",
			actual,
			expectedPayload,
		)
	}

	if failures :=
		executionResult.NodeFailures(); failures != nil {
		t.Fatalf(
			"real-core node failures = %#v, want nil",
			failures,
		)
	}

	if _, exists :=
		executionResult.WorkflowFailure(); exists {
		t.Fatal(
			"real-core successful workflow contains a workflow failure",
		)
	}
}

type repeatedCoreRunContextKey string

const repeatedCoreRunIDKey repeatedCoreRunContextKey = "core-repeat-run-id"

func TestSyncRunnerCreatesIsolatedDeterministicStateAcrossRepeatedRealCoreRuns(
	t *testing.T,
) {
	const expectedPayload = `{"message":"repeatable-core-run","sequence":2}`

	const expectedDelay = 25 * time.Millisecond

	waitDurations := make(
		[]time.Duration,
		0,
		2,
	)

	waitContextValues := make(
		[]string,
		0,
		2,
	)

	waiter := core.WaiterFunc(
		func(
			ctx context.Context,
			duration time.Duration,
		) error {
			if ctx == nil {
				t.Fatal(
					"core delay waiter received a nil context",
				)
			}

			if err := ctx.Err(); err != nil {
				t.Fatalf(
					"core delay waiter received a completed context: %v",
					err,
				)
			}

			runID, exists :=
				ctx.Value(
					repeatedCoreRunIDKey,
				).(string)

			if !exists || runID == "" {
				t.Fatal(
					"core delay waiter received no run identifier",
				)
			}

			waitDurations = append(
				waitDurations,
				duration,
			)

			waitContextValues = append(
				waitContextValues,
				runID,
			)

			return nil
		},
	)

	runner, request :=
		mustRealCoreSyncRunner(
			t,
			waiter,
			&nodeExecutionTestClock{
				next: time.Date(
					2026,
					time.July,
					29,
					10,
					0,
					0,
					0,
					time.UTC,
				),
			},
			"workflow-core-repeatability-e2e",
			`{"value":{"message":"repeatable-core-run","sequence":2}}`,
			`{"delay":"25ms"}`,
		)

	firstContext := context.WithValue(
		context.Background(),
		repeatedCoreRunIDKey,
		"run-1",
	)

	firstResult, err := runner.Run(
		firstContext,
		request,
	)
	if err != nil {
		t.Fatalf(
			"first SyncRunner.Run() returned an unexpected error: %v",
			err,
		)
	}

	secondContext := context.WithValue(
		context.Background(),
		repeatedCoreRunIDKey,
		"run-2",
	)

	secondResult, err := runner.Run(
		secondContext,
		request,
	)
	if err != nil {
		t.Fatalf(
			"second SyncRunner.Run() returned an unexpected error: %v",
			err,
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("static-input"),
		workflow.NodeID("pass-through"),
		workflow.NodeID("delay"),
		workflow.NodeID("terminal"),
	}

	firstExecution :=
		requireSuccessfulRepeatedRealCoreRun(
			t,
			firstResult,
			request.WorkflowExecutionID(),
			expectedOrder,
			expectedPayload,
		)

	secondExecution :=
		requireSuccessfulRepeatedRealCoreRun(
			t,
			secondResult,
			request.WorkflowExecutionID(),
			expectedOrder,
			expectedPayload,
		)

	if len(waitDurations) != 2 {
		t.Fatalf(
			"delay waiter call count = %d, want 2",
			len(waitDurations),
		)
	}

	for index, duration := range waitDurations {
		if duration != expectedDelay {
			t.Fatalf(
				"delay waiter duration %d = %v, want %v",
				index,
				duration,
				expectedDelay,
			)
		}
	}

	expectedContextValues := []string{
		"run-1",
		"run-2",
	}

	if !reflect.DeepEqual(
		waitContextValues,
		expectedContextValues,
	) {
		t.Fatalf(
			"waiter context values = %#v, want %#v",
			waitContextValues,
			expectedContextValues,
		)
	}

	if !reflect.DeepEqual(
		firstExecution.ExecutedNodeOrder(),
		secondExecution.ExecutedNodeOrder(),
	) {
		t.Fatalf(
			"repeated execution orders differ: first=%#v second=%#v",
			firstExecution.ExecutedNodeOrder(),
			secondExecution.ExecutedNodeOrder(),
		)
	}

	if !reflect.DeepEqual(
		firstExecution.NodeExecutionOrder(),
		secondExecution.NodeExecutionOrder(),
	) {
		t.Fatalf(
			"repeated node orders differ: first=%#v second=%#v",
			firstExecution.NodeExecutionOrder(),
			secondExecution.NodeExecutionOrder(),
		)
	}

	/*
		İlk result accessor'larından alınan slice, map ve byte
		kopyaları değiştirilir. Saklanan ilk sonuç ile ikinci
		çalıştırmanın sonucu değişmemelidir.
	*/
	mutatedExecutedOrder :=
		firstExecution.ExecutedNodeOrder()

	mutatedExecutedOrder[0] =
		workflow.NodeID("changed")

	mutatedNodeOrder :=
		firstExecution.NodeExecutionOrder()

	mutatedNodeOrder[0] =
		workflow.NodeID("changed")

	mutatedOutputs :=
		firstExecution.TerminalOutputs()

	delete(
		mutatedOutputs,
		workflow.NodeID("terminal"),
	)

	mutatedPayload, exists, err :=
		firstExecution.TerminalOutput(
			workflow.NodeID("terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"first execution terminal output does not exist",
		)
	}

	mutatedInlineData, exists :=
		mutatedPayload.InlineData()

	if !exists {
		t.Fatal(
			"first execution terminal output contains no inline data",
		)
	}

	if len(mutatedInlineData) == 0 {
		t.Fatal(
			"first execution terminal inline data is empty",
		)
	}

	mutatedInlineData[0] = 'X'

	/*
		Accessor kopyaları değiştirildikten sonra iki sonuç da
		yeniden doğrulanır.
	*/
	requireSuccessfulRepeatedRealCoreRun(
		t,
		firstResult,
		request.WorkflowExecutionID(),
		expectedOrder,
		expectedPayload,
	)

	requireSuccessfulRepeatedRealCoreRun(
		t,
		secondResult,
		request.WorkflowExecutionID(),
		expectedOrder,
		expectedPayload,
	)
}

func requireSuccessfulRepeatedRealCoreRun(
	t *testing.T,
	result SyncRunResult,
	expectedExecutionID execution.WorkflowExecutionID,
	expectedOrder []workflow.NodeID,
	expectedPayload string,
) ExecutionResult {
	t.Helper()

	if !result.IsValid() {
		t.Fatal(
			"repeated real-core SyncRunResult is invalid",
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"repeated real-core SyncRunResult is not terminal",
		)
	}

	if !result.IsSucceeded() {
		t.Fatalf(
			"sync-run status = %q, want SUCCEEDED",
			result.Status(),
		)
	}

	if result.Status() !=
		execution.WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"workflow status = %q, want %q",
			result.Status(),
			execution.WorkflowExecutionStatusSucceeded,
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"repeated real-core result is marked as rejected",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"repeated real-core result is marked as failed",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"repeated real-core result is marked as cancelled",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"repeated real-core result is marked as timed out",
		)
	}

	if result.WorkflowExecution().ID() !=
		expectedExecutionID {
		t.Fatalf(
			"sync-run workflow ID = %q, want %q",
			result.WorkflowExecution().ID(),
			expectedExecutionID,
		)
	}

	validationReport :=
		result.ValidationReport()

	if !validationReport.IsValid() {
		t.Fatalf(
			"preflight report contains %d issues",
			validationReport.Len(),
		)
	}

	if validationReport.Len() != 0 {
		t.Fatalf(
			"validation issue count = %d, want 0",
			validationReport.Len(),
		)
	}

	if !result.HasExecutionResult() {
		t.Fatal(
			"repeated real-core result contains no ExecutionResult",
		)
	}

	executionResult, exists :=
		result.ExecutionResult()

	if !exists {
		t.Fatal(
			"ExecutionResult() exists = false",
		)
	}

	if !executionResult.IsTerminal() {
		t.Fatal(
			"repeated real-core ExecutionResult is not terminal",
		)
	}

	if !executionResult.IsSucceeded() {
		t.Fatalf(
			"execution-result status = %q, want SUCCEEDED",
			executionResult.Status(),
		)
	}

	if executionResult.IsStalled() {
		t.Fatal(
			"repeated real-core execution is marked as stalled",
		)
	}

	if executionResult.WorkflowExecution().ID() !=
		expectedExecutionID {
		t.Fatalf(
			"execution-result workflow ID = %q, want %q",
			executionResult.WorkflowExecution().ID(),
			expectedExecutionID,
		)
	}

	if executionResult.Status() !=
		result.Status() {
		t.Fatalf(
			"execution-result status = %q, sync-run status = %q",
			executionResult.Status(),
			result.Status(),
		)
	}

	if !reflect.DeepEqual(
		executionResult.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"executed node order = %#v, want %#v",
			executionResult.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		executionResult.NodeExecutionOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"node execution order = %#v, want %#v",
			executionResult.NodeExecutionOrder(),
			expectedOrder,
		)
	}

	nodeExecutions :=
		executionResult.NodeExecutions()

	if len(nodeExecutions) !=
		len(expectedOrder) {
		t.Fatalf(
			"node execution count = %d, want %d",
			len(nodeExecutions),
			len(expectedOrder),
		)
	}

	for index, nodeID := range expectedOrder {
		if nodeExecutions[index].NodeID() != nodeID {
			t.Fatalf(
				"node execution %d ID = %q, want %q",
				index,
				nodeExecutions[index].NodeID(),
				nodeID,
			)
		}

		requireExecutionResultNodeStatus(
			t,
			executionResult,
			nodeID,
			execution.NodeExecutionStatusSucceeded,
		)

		requireSyncRunnerStartedAndFinishedNode(
			t,
			executionResult,
			nodeID,
			execution.NodeExecutionStatusSucceeded,
		)
	}

	terminalOutputs :=
		executionResult.TerminalOutputs()

	if len(terminalOutputs) != 1 {
		t.Fatalf(
			"terminal output count = %d, want 1",
			len(terminalOutputs),
		)
	}

	terminalOutput, exists, err :=
		executionResult.TerminalOutput(
			workflow.NodeID("terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"terminal output does not exist",
		)
	}

	inlineData, exists :=
		terminalOutput.InlineData()

	if !exists {
		t.Fatal(
			"terminal output contains no inline data",
		)
	}

	if actual := string(inlineData); actual != expectedPayload {
		t.Fatalf(
			"terminal payload = %q, want %q",
			actual,
			expectedPayload,
		)
	}

	if failures :=
		executionResult.NodeFailures(); failures != nil {
		t.Fatalf(
			"node failures = %#v, want nil",
			failures,
		)
	}

	if _, exists :=
		executionResult.WorkflowFailure(); exists {
		t.Fatal(
			"successful repeated execution contains a workflow failure",
		)
	}

	return executionResult
}

type lifecycleRecorderTestStub struct{}

var _ LifecycleRecorder = (*lifecycleRecorderTestStub)(nil)

func (recorder *lifecycleRecorderTestStub) IsValid() bool {
	return recorder != nil
}

func (
	*lifecycleRecorderTestStub,
) RecordWorkflowCreation(
	context.Context,
	WorkflowCreationObservation,
) error {
	return nil
}

func (
	*lifecycleRecorderTestStub,
) RecordWorkflowTransition(
	context.Context,
	WorkflowTransitionObservation,
) error {
	return nil
}

func (
	*lifecycleRecorderTestStub,
) RecordNodeExecutionsCreation(
	context.Context,
	NodeExecutionsCreationObservation,
) error {
	return nil
}

func (
	*lifecycleRecorderTestStub,
) RecordNodeTransition(
	context.Context,
	NodeTransitionObservation,
) error {
	return nil
}

func TestEngineDependenciesUsesNoopLifecycleRecorderByDefault(
	t *testing.T,
) {
	dependencies := newLifecycleRecorderTestDependencies(t)

	recorder := dependencies.LifecycleRecorder()
	if recorder == nil {
		t.Fatal(
			"default lifecycle recorder is nil",
		)
	}

	if _, ok := recorder.(NoopLifecycleRecorder); !ok {
		t.Fatalf(
			"default lifecycle recorder type = %T, want NoopLifecycleRecorder",
			recorder,
		)
	}

	if !dependencies.IsValid() {
		t.Fatal(
			"dependencies with default lifecycle recorder are invalid",
		)
	}
}

func TestEngineDependenciesWithLifecycleRecorder(
	t *testing.T,
) {
	dependencies := newLifecycleRecorderTestDependencies(t)
	recorder := &lifecycleRecorderTestStub{}

	updated, err := dependencies.WithLifecycleRecorder(
		recorder,
	)
	if err != nil {
		t.Fatalf(
			"WithLifecycleRecorder() returned an unexpected error: %v",
			err,
		)
	}

	if updated.LifecycleRecorder() != recorder {
		t.Fatalf(
			"updated lifecycle recorder = %T, want supplied recorder",
			updated.LifecycleRecorder(),
		)
	}

	if _, ok :=
		dependencies.LifecycleRecorder().(NoopLifecycleRecorder); !ok {
		t.Fatal(
			"WithLifecycleRecorder() mutated the original dependencies",
		)
	}

	if !updated.IsValid() {
		t.Fatal(
			"updated dependencies are invalid",
		)
	}
}

func TestEngineDependenciesRejectsNilLifecycleRecorder(
	t *testing.T,
) {
	dependencies := newLifecycleRecorderTestDependencies(t)

	_, err := dependencies.WithLifecycleRecorder(nil)
	if err == nil {
		t.Fatal(
			"WithLifecycleRecorder() accepted nil recorder",
		)
	}
}

func newLifecycleRecorderTestDependencies(
	t *testing.T,
) EngineDependencies {
	t.Helper()

	pluginRegistry, err := plugin.NewRegistry(nil)
	if err != nil {
		t.Fatalf(
			"NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	executorRegistry, err :=
		runtime.NewExecutorRegistry(nil)
	if err != nil {
		t.Fatalf(
			"NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	runtimeLimits, err := runtime.NewRuntimeLimits(
		64,
		8,
		1024,
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeLimits() returned an unexpected error: %v",
			err,
		)
	}

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		runtimeLimits,
		sharedclock.System(),
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	return dependencies
}

func TestNewEngineDependenciesStoresDependencies(
	t *testing.T,
) {
	pluginRegistry := mustEnginePluginRegistry(t)
	executorRegistry := mustEngineExecutorRegistry(t)
	limits := mustEngineRuntimeLimits(t)

	expectedTime := time.Date(
		2026,
		time.July,
		17,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	clock := sharedclock.From(func() time.Time {
		return expectedTime
	})

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		clock,
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	if !dependencies.IsValid() {
		t.Fatal(
			"IsValid() = false for valid dependencies",
		)
	}

	if actual := dependencies.PluginRegistry().Len(); actual != 0 {
		t.Fatalf(
			"plugin registry length = %d, want 0",
			actual,
		)
	}

	if actual := dependencies.ExecutorRegistry().Len(); actual != 0 {
		t.Fatalf(
			"executor registry length = %d, want 0",
			actual,
		)
	}

	actualLimits := dependencies.RuntimeLimits()

	if actual := actualLimits.MaximumQueueCapacity(); actual != 64 {
		t.Fatalf(
			"MaximumQueueCapacity() = %d, want 64",
			actual,
		)
	}

	if actual := actualLimits.MaximumCacheCapacity(); actual != 8 {
		t.Fatalf(
			"MaximumCacheCapacity() = %d, want 8",
			actual,
		)
	}

	if actual := actualLimits.MaximumInlinePayloadBytes(); actual != 4096 {
		t.Fatalf(
			"MaximumInlinePayloadBytes() = %d, want 4096",
			actual,
		)
	}

	actualClock := dependencies.Clock()

	if actual := actualClock.Now(); !actual.Equal(expectedTime) {
		t.Fatalf(
			"Clock().Now() = %v, want %v",
			actual,
			expectedTime,
		)
	}
}

func TestNewEngineDependenciesAllowsEmptyRegistries(
	t *testing.T,
) {
	dependencies, err := NewEngineDependencies(
		mustEnginePluginRegistry(t),
		mustEngineExecutorRegistry(t),
		mustEngineRuntimeLimits(t),
		sharedclock.System(),
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	if !dependencies.PluginRegistry().IsEmpty() {
		t.Fatal(
			"plugin registry IsEmpty() = false",
		)
	}

	if !dependencies.ExecutorRegistry().IsEmpty() {
		t.Fatal(
			"executor registry IsEmpty() = false",
		)
	}

	if !dependencies.IsValid() {
		t.Fatal(
			"dependencies with empty registries IsValid() = false",
		)
	}
}

func TestNewEngineDependenciesRejectsInvalidRuntimeLimits(
	t *testing.T,
) {
	_, err := NewEngineDependencies(
		mustEnginePluginRegistry(t),
		mustEngineExecutorRegistry(t),
		runtime.RuntimeLimits{},
		sharedclock.System(),
	)

	requireEngineValidationField(
		t,
		err,
		"runtimeLimits",
	)
}

func TestNewEngineDependenciesRejectsNilClockValues(
	t *testing.T,
) {
	pluginRegistry := mustEnginePluginRegistry(t)
	executorRegistry := mustEngineExecutorRegistry(t)
	limits := mustEngineRuntimeLimits(t)

	t.Run("nil interface", func(t *testing.T) {
		var clock sharedclock.Clock

		_, err := NewEngineDependencies(
			pluginRegistry,
			executorRegistry,
			limits,
			clock,
		)

		requireEngineValidationField(
			t,
			err,
			"clock",
		)
	})

	t.Run("typed nil function", func(t *testing.T) {
		var clock sharedclock.Clock

		_, err := NewEngineDependencies(
			pluginRegistry,
			executorRegistry,
			limits,
			clock,
		)

		requireEngineValidationField(
			t,
			err,
			"clock",
		)
	})

}

func TestZeroAndMalformedEngineDependenciesAreInvalid(
	t *testing.T,
) {
	var zeroDependencies EngineDependencies

	if zeroDependencies.IsValid() {
		t.Fatal(
			"zero EngineDependencies IsValid() = true",
		)
	}

	malformedLimits := EngineDependencies{
		pluginRegistry:   mustEnginePluginRegistry(t),
		executorRegistry: mustEngineExecutorRegistry(t),
		runtimeLimits:    runtime.RuntimeLimits{},
		clock:            sharedclock.System(),
		initialized:      true,
	}

	if malformedLimits.IsValid() {
		t.Fatal(
			"dependencies with invalid limits IsValid() = true",
		)
	}

	var typedNilClock sharedclock.Clock

	malformedClock := EngineDependencies{
		pluginRegistry:   mustEnginePluginRegistry(t),
		executorRegistry: mustEngineExecutorRegistry(t),
		runtimeLimits:    mustEngineRuntimeLimits(t),
		clock:            typedNilClock,
		initialized:      true,
	}

	if malformedClock.IsValid() {
		t.Fatal(
			"dependencies with typed-nil clock IsValid() = true",
		)
	}
}

func TestZeroEngineDependencyAccessorsAreSafe(
	t *testing.T,
) {
	var dependencies EngineDependencies

	if actual := dependencies.PluginRegistry().Len(); actual != 0 {
		t.Fatalf(
			"zero PluginRegistry().Len() = %d, want 0",
			actual,
		)
	}

	if actual := dependencies.ExecutorRegistry().Len(); actual != 0 {
		t.Fatalf(
			"zero ExecutorRegistry().Len() = %d, want 0",
			actual,
		)
	}

	if dependencies.RuntimeLimits().IsValid() {
		t.Fatal(
			"zero RuntimeLimits() unexpectedly valid",
		)
	}

	if dependencies.Clock() != nil {
		t.Fatal(
			"zero Clock() must be invalid",
		)
	}
}

func mustEnginePluginRegistry(
	t *testing.T,
) plugin.Registry {
	t.Helper()

	registry, err := plugin.NewRegistry(nil)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry(nil) returned an unexpected error: %v",
			err,
		)
	}

	return registry
}

func mustEngineExecutorRegistry(
	t *testing.T,
) runtime.ExecutorRegistry {
	t.Helper()

	registry, err := runtime.NewExecutorRegistry(nil)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry(nil) returned an unexpected error: %v",
			err,
		)
	}

	return registry
}

func mustEngineRuntimeLimits(
	t *testing.T,
) runtime.RuntimeLimits {
	t.Helper()

	limits, err := runtime.NewRuntimeLimits(
		64,
		8,
		4096,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewRuntimeLimits() returned an unexpected error: %v",
			err,
		)
	}

	return limits
}

func TestValidationErrorFormatsSupportedCombinations(
	t *testing.T,
) {
	tests := map[string]struct {
		err      *ValidationError
		expected string
	}{
		"field and reason": {
			err: &ValidationError{
				Field:  "correlationID",
				Reason: "must not be empty",
			},
			expected: "correlationID: must not be empty",
		},
		"reason only": {
			err: &ValidationError{
				Reason: "invalid execution request",
			},
			expected: "engine validation failed: invalid execution request",
		},
		"field only": {
			err: &ValidationError{
				Field: "runtimeLimits",
			},
			expected: "engine validation failed for runtimeLimits",
		},
		"empty": {
			err:      &ValidationError{},
			expected: "engine validation failed",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if actual := test.err.Error(); actual != test.expected {
				t.Fatalf(
					"Error() = %q, want %q",
					actual,
					test.expected,
				)
			}
		})
	}
}

func TestNilValidationErrorUsesFallbackMessage(
	t *testing.T,
) {
	var validationError *ValidationError

	if actual := validationError.Error(); actual !=
		"engine validation failed" {
		t.Fatalf(
			"nil Error() = %q, want %q",
			actual,
			"engine validation failed",
		)
	}
}

func TestNormalizeRequiredStringTrimsValue(
	t *testing.T,
) {
	actual, err := normalizeRequiredString(
		"correlationID",
		" correlation-1 ",
	)
	if err != nil {
		t.Fatalf(
			"normalizeRequiredString() returned an unexpected error: %v",
			err,
		)
	}

	if actual != "correlation-1" {
		t.Fatalf(
			"normalized value = %q, want %q",
			actual,
			"correlation-1",
		)
	}
}

func TestNormalizeRequiredStringRejectsBlankValue(
	t *testing.T,
) {
	_, err := normalizeRequiredString(
		"correlationID",
		"   ",
	)

	requireEngineValidationField(
		t,
		err,
		"correlationID",
	)
}

func requireEngineValidationField(
	t *testing.T,
	err error,
	expectedField string,
) *ValidationError {
	t.Helper()

	if err == nil {
		t.Fatalf(
			"expected validation error for field %q, got nil",
			expectedField,
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field != expectedField {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			expectedField,
		)
	}

	return validationError
}

func TestNewAsyncExecutionRequestPreservesMode(t *testing.T) {
	definition := mustEngineWorkflowDefinition(t)
	request, err := NewAsyncExecutionRequest("execution-async", definition, "correlation-async", nil)
	if err != nil {
		t.Fatalf("NewAsyncExecutionRequest() error = %v", err)
	}
	if request.Mode() != execution.ExecutionModeAsync {
		t.Fatalf("Mode() = %q, want ASYNC", request.Mode())
	}
	if !request.IsValid() {
		t.Fatal("async request is invalid")
	}
}

func TestNewExecutionRequestStoresNormalizedValues(
	t *testing.T,
) {
	definition := mustEngineWorkflowDefinition(t)

	initialVariables := map[string]runtime.RuntimeValue{
		" currency ": mustEngineRuntimeValue(
			t,
			`"TRY"`,
		),
		"attempt": mustEngineRuntimeValue(
			t,
			`1`,
		),
	}

	request, err := NewExecutionRequest(
		execution.WorkflowExecutionID(
			" workflow-execution-1 ",
		),
		definition,
		" correlation-1 ",
		initialVariables,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	if !request.IsValid() {
		t.Fatal(
			"IsValid() = false for a valid request",
		)
	}

	if actual := request.WorkflowExecutionID().String(); actual !=
		"workflow-execution-1" {
		t.Fatalf(
			"WorkflowExecutionID() = %q, want %q",
			actual,
			"workflow-execution-1",
		)
	}

	if actual := request.CorrelationID(); actual !=
		"correlation-1" {
		t.Fatalf(
			"CorrelationID() = %q, want %q",
			actual,
			"correlation-1",
		)
	}

	actualDefinition := request.Definition()

	if actual := actualDefinition.ID().String(); actual !=
		"workflow-1" {
		t.Fatalf(
			"definition ID = %q, want %q",
			actual,
			"workflow-1",
		)
	}

	if actual := actualDefinition.CompanyID().String(); actual !=
		"company-1" {
		t.Fatalf(
			"definition company ID = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := actualDefinition.Name(); actual !=
		"Request Workflow" {
		t.Fatalf(
			"definition name = %q, want %q",
			actual,
			"Request Workflow",
		)
	}

	if actual := actualDefinition.Revision(); actual != 3 {
		t.Fatalf(
			"definition revision = %d, want 3",
			actual,
		)
	}

	if actual := actualDefinition.Metadata().String(); actual !=
		`{"purpose":"request-test"}` {
		t.Fatalf(
			"definition metadata = %q",
			actual,
		)
	}

	actualVariables := request.InitialVariables()

	if len(actualVariables) != 2 {
		t.Fatalf(
			"initial variable count = %d, want 2",
			len(actualVariables),
		)
	}

	if actual := actualVariables["currency"].String(); actual !=
		`"TRY"` {
		t.Fatalf(
			"currency = %q, want %q",
			actual,
			`"TRY"`,
		)
	}

	if actual := actualVariables["attempt"].String(); actual !=
		`1` {
		t.Fatalf(
			"attempt = %q, want %q",
			actual,
			`1`,
		)
	}
}

func TestExecutionRequestCopiesInitialVariables(
	t *testing.T,
) {
	originalValue := mustEngineRuntimeValue(
		t,
		`{"code":"original"}`,
	)

	sourceVariables := map[string]runtime.RuntimeValue{
		"state": originalValue,
	}

	request, err := NewExecutionRequest(
		execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		mustEngineWorkflowDefinition(t),
		"correlation-1",
		sourceVariables,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	sourceVariables["state"] =
		mustEngineRuntimeValue(
			t,
			`{"code":"changed"}`,
		)

	sourceVariables["added"] =
		mustEngineRuntimeValue(
			t,
			`true`,
		)

	firstSnapshot := request.InitialVariables()

	if len(firstSnapshot) != 1 {
		t.Fatalf(
			"snapshot variable count = %d, want 1",
			len(firstSnapshot),
		)
	}

	if actual := firstSnapshot["state"].String(); actual !=
		`{"code":"original"}` {
		t.Fatalf(
			"stored state = %q, want original value",
			actual,
		)
	}

	firstSnapshot["state"] =
		mustEngineRuntimeValue(
			t,
			`{"code":"returned-map-change"}`,
		)

	firstSnapshot["new-key"] =
		mustEngineRuntimeValue(
			t,
			`null`,
		)

	valueBytes := request.
		InitialVariables()["state"].
		Bytes()

	valueBytes[0] = '['

	secondSnapshot := request.InitialVariables()

	if len(secondSnapshot) != 1 {
		t.Fatalf(
			"second snapshot variable count = %d, want 1",
			len(secondSnapshot),
		)
	}

	if actual := secondSnapshot["state"].String(); actual !=
		`{"code":"original"}` {
		t.Fatalf(
			"request state changed through source or returned data: got %q",
			actual,
		)
	}
}

func TestExecutionRequestDefinitionMetadataIsIndependent(
	t *testing.T,
) {
	request, err := NewExecutionRequest(
		execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		mustEngineWorkflowDefinition(t),
		"correlation-1",
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	firstMetadata := request.
		Definition().
		Metadata().
		Bytes()

	firstMetadata[0] = '['

	if actual := request.
		Definition().
		Metadata().
		String(); actual !=
		`{"purpose":"request-test"}` {
		t.Fatalf(
			"definition metadata changed through returned bytes: got %q",
			actual,
		)
	}
}

func TestNewExecutionRequestAllowsEmptyInitialVariables(
	t *testing.T,
) {
	request, err := NewExecutionRequest(
		execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		mustEngineWorkflowDefinition(t),
		"correlation-1",
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	if variables := request.InitialVariables(); variables != nil {
		t.Fatalf(
			"InitialVariables() = %#v, want nil",
			variables,
		)
	}

	if !request.IsValid() {
		t.Fatal(
			"request with empty initial variables IsValid() = false",
		)
	}
}

func TestNewExecutionRequestRejectsInvalidRequiredValues(
	t *testing.T,
) {
	validDefinition := mustEngineWorkflowDefinition(t)

	t.Run("blank workflow execution ID", func(t *testing.T) {
		_, err := NewExecutionRequest(
			execution.WorkflowExecutionID(" "),
			validDefinition,
			"correlation-1",
			nil,
		)

		requireEngineValidationField(
			t,
			err,
			"workflowExecutionID",
		)
	})

	t.Run("invalid workflow definition", func(t *testing.T) {
		_, err := NewExecutionRequest(
			execution.WorkflowExecutionID(
				"workflow-execution-1",
			),
			workflow.WorkflowDefinition{},
			"correlation-1",
			nil,
		)

		requireEngineValidationField(
			t,
			err,
			"definition",
		)
	})

	t.Run("blank correlation ID", func(t *testing.T) {
		_, err := NewExecutionRequest(
			execution.WorkflowExecutionID(
				"workflow-execution-1",
			),
			validDefinition,
			"   ",
			nil,
		)

		requireEngineValidationField(
			t,
			err,
			"correlationID",
		)
	})
}

func TestNewExecutionRequestRejectsInvalidInitialVariables(
	t *testing.T,
) {
	definition := mustEngineWorkflowDefinition(t)

	t.Run("protected key", func(t *testing.T) {
		_, err := NewExecutionRequest(
			execution.WorkflowExecutionID(
				"workflow-execution-1",
			),
			definition,
			"correlation-1",
			map[string]runtime.RuntimeValue{
				runtime.ProtectedKeyCompanyID: mustEngineRuntimeValue(
					t,
					`"changed-company"`,
				),
			},
		)

		requireEngineValidationField(
			t,
			err,
			"initialVariables",
		)
	})

	t.Run("invalid value", func(t *testing.T) {
		_, err := NewExecutionRequest(
			execution.WorkflowExecutionID(
				"workflow-execution-1",
			),
			definition,
			"correlation-1",
			map[string]runtime.RuntimeValue{
				"state": {},
			},
		)

		requireEngineValidationField(
			t,
			err,
			"initialVariables",
		)
	})
}

func TestZeroAndMalformedExecutionRequestsAreInvalid(
	t *testing.T,
) {
	var zeroRequest ExecutionRequest

	if zeroRequest.IsValid() {
		t.Fatal(
			"zero ExecutionRequest IsValid() = true",
		)
	}

	malformedRequest := ExecutionRequest{
		workflowExecutionID: execution.WorkflowExecutionID(
			"workflow-execution-1",
		),
		definition:       mustEngineWorkflowDefinition(t),
		correlationID:    " ",
		initialVariables: runtime.ContextChanges{},
		initialized:      true,
	}

	if malformedRequest.IsValid() {
		t.Fatal(
			"malformed ExecutionRequest IsValid() = true",
		)
	}
}

func mustEngineWorkflowDefinition(
	t *testing.T,
) workflow.WorkflowDefinition {
	t.Helper()

	definition, err := workflow.NewWorkflowDefinition(
		workflow.WorkflowID(
			" workflow-1 ",
		),
		workflow.CompanyID(
			" company-1 ",
		),
		" Request Workflow ",
		3,
		nil,
		nil,
		[]byte(
			`{"purpose":"request-test"}`,
		),
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewWorkflowDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return definition
}

func mustEngineRuntimeValue(
	t *testing.T,
	value string,
) runtime.RuntimeValue {
	t.Helper()

	runtimeValue, err := runtime.NewRuntimeValue(
		[]byte(value),
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewRuntimeValue(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return runtimeValue
}

func TestNewSyncExecutionServiceProvidesSyncOnlyCapability(
	t *testing.T,
) {
	fixture :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	syncRunner, err :=
		NewSyncRunner(
			fixture.dependencies,
		)

	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	service, err :=
		NewSyncExecutionService(
			syncRunner,
		)

	if err != nil {
		t.Fatalf(
			"NewSyncExecutionService() returned an unexpected error: %v",
			err,
		)
	}

	if !service.IsValid() {
		t.Fatal(
			"sync-only execution service is invalid",
		)
	}

	if service.AsyncEnabled() {
		t.Fatal(
			"sync-only execution service reports async capability",
		)
	}

	asyncRequest, err :=
		NewAsyncExecutionRequest(
			fixture.request.
				WorkflowExecutionID(),

			fixture.request.
				Definition(),

			fixture.request.
				CorrelationID(),

			fixture.request.
				InitialVariables(),
		)

	if err != nil {
		t.Fatalf(
			"NewAsyncExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err =
		service.Run(
			context.Background(),
			asyncRequest,
		)

	if !errors.Is(
		err,
		ErrAsyncExecutionUnavailable,
	) {
		t.Fatalf(
			"Run() error = %v, want %v",
			err,
			ErrAsyncExecutionUnavailable,
		)
	}
}

func TestNewSyncExecutionServiceRejectsInvalidRunner(
	t *testing.T,
) {
	service, err :=
		NewSyncExecutionService(
			SyncRunner{},
		)

	if err == nil {
		t.Fatal(
			"expected invalid sync runner error",
		)
	}

	if service.IsValid() {
		t.Fatal(
			"invalid constructor result produced a valid service",
		)
	}
}

func TestZeroExecutionServiceIsInvalid(
	t *testing.T,
) {
	var service ExecutionService

	if service.IsValid() {
		t.Fatal(
			"zero execution service is valid",
		)
	}

	if service.AsyncEnabled() {
		t.Fatal(
			"zero execution service enables async execution",
		)
	}
}

func TestPrepareNodeInputBuildsEmptyInputForRoot(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	input, err := prepareNodeInput(
		prepared,
		workflow.NodeID("source"),
	)
	if err != nil {
		t.Fatalf(
			"prepareNodeInput(source) returned an unexpected error: %v",
			err,
		)
	}

	if !input.IsValid() {
		t.Fatal(
			"root NodeInput IsValid() = false",
		)
	}

	if !input.IsEmpty() {
		t.Fatal(
			"root NodeInput IsEmpty() = false",
		)
	}

	if actual := input.PortCount(); actual != 0 {
		t.Fatalf(
			"root PortCount() = %d, want 0",
			actual,
		)
	}

	if actual := input.TotalPayloadCount(); actual != 0 {
		t.Fatalf(
			"root TotalPayloadCount() = %d, want 0",
			actual,
		)
	}

	if input.Ports() != nil {
		t.Fatalf(
			"root Ports() = %#v, want nil",
			input.Ports(),
		)
	}
}

func TestPrepareNodeInputConsumesSingleIncomingPayload(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-source-pass",
		"source-payload",
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 1 {
		t.Fatalf(
			"queue length before preparation = %d, want 1",
			actual,
		)
	}

	input, err := prepareNodeInput(
		prepared,
		workflow.NodeID("pass"),
	)
	if err != nil {
		t.Fatalf(
			"prepareNodeInput(pass) returned an unexpected error: %v",
			err,
		)
	}

	if !input.IsValid() {
		t.Fatal(
			"prepared NodeInput IsValid() = false",
		)
	}

	if input.IsEmpty() {
		t.Fatal(
			"prepared NodeInput IsEmpty() = true",
		)
	}

	if actual := input.PortCount(); actual != 1 {
		t.Fatalf(
			"PortCount() = %d, want 1",
			actual,
		)
	}

	if actual := input.TotalPayloadCount(); actual != 1 {
		t.Fatalf(
			"TotalPayloadCount() = %d, want 1",
			actual,
		)
	}

	expectedPorts := []string{
		"input",
	}

	if actual := input.Ports(); !reflect.DeepEqual(
		actual,
		expectedPorts,
	) {
		t.Fatalf(
			"Ports() = %#v, want %#v",
			actual,
			expectedPorts,
		)
	}

	payloads := requirePreparedInputPayloads(
		t,
		input,
		"input",
	)

	if len(payloads) != 1 {
		t.Fatalf(
			"input payload count = %d, want 1",
			len(payloads),
		)
	}

	if actual := preparedInputPayloadText(
		t,
		payloads[0],
	); actual != "source-payload" {
		t.Fatalf(
			"input payload = %q, want %q",
			actual,
			"source-payload",
		)
	}

	if actual := payloads[0].Metadata()["edge"]; actual !=
		"edge-source-pass" {
		t.Fatalf(
			"payload edge metadata = %q, want %q",
			actual,
			"edge-source-pass",
		)
	}

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after preparation = %d, want 0",
			actual,
		)
	}

	_, err = prepareNodeInput(
		prepared,
		workflow.NodeID("pass"),
	)
	if err == nil {
		t.Fatal(
			"second prepareNodeInput() returned nil error for an empty queue",
		)
	}

	if !strings.Contains(
		err.Error(),
		"contains no payload",
	) {
		t.Fatalf(
			"second preparation error = %q, want empty-queue information",
			err,
		)
	}

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after second preparation = %d, want 0",
			actual,
		)
	}
}

func TestPrepareNodeInputGroupsFanInPayloadsDeterministically(
	t *testing.T,
) {
	prepared := mustSchedulerFanInPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source-a"),
		execution.NodeExecutionStatusSucceeded,
	)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source-b"),
		execution.NodeExecutionStatusSucceeded,
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-b",
		"payload-b",
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-a",
		"payload-a",
	)

	input, err := prepareNodeInput(
		prepared,
		workflow.NodeID("target"),
	)
	if err != nil {
		t.Fatalf(
			"prepareNodeInput(target) returned an unexpected error: %v",
			err,
		)
	}

	if actual := input.PortCount(); actual != 1 {
		t.Fatalf(
			"PortCount() = %d, want 1",
			actual,
		)
	}

	if actual := input.TotalPayloadCount(); actual != 2 {
		t.Fatalf(
			"TotalPayloadCount() = %d, want 2",
			actual,
		)
	}

	payloads := requirePreparedInputPayloads(
		t,
		input,
		"input",
	)

	if len(payloads) != 2 {
		t.Fatalf(
			"input payload count = %d, want 2",
			len(payloads),
		)
	}

	actualPayloads := []string{
		preparedInputPayloadText(
			t,
			payloads[0],
		),
		preparedInputPayloadText(
			t,
			payloads[1],
		),
	}

	expectedPayloads := []string{
		"payload-a",
		"payload-b",
	}

	if !reflect.DeepEqual(
		actualPayloads,
		expectedPayloads,
	) {
		t.Fatalf(
			"payload order = %#v, want %#v",
			actualPayloads,
			expectedPayloads,
		)
	}

	if actual := payloads[0].Metadata()["edge"]; actual !=
		"edge-a" {
		t.Fatalf(
			"first payload edge metadata = %q, want %q",
			actual,
			"edge-a",
		)
	}

	if actual := payloads[1].Metadata()["edge"]; actual !=
		"edge-b" {
		t.Fatalf(
			"second payload edge metadata = %q, want %q",
			actual,
			"edge-b",
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-a",
	).Len(); actual != 0 {
		t.Fatalf(
			"edge-a queue length = %d, want 0",
			actual,
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-b",
	).Len(); actual != 0 {
		t.Fatalf(
			"edge-b queue length = %d, want 0",
			actual,
		)
	}

	exposedBytes, exists := payloads[0].InlineData()
	if !exists {
		t.Fatal(
			"first input payload does not contain inline data",
		)
	}

	exposedBytes[0] = 'X'

	exposedMetadata := payloads[0].Metadata()
	exposedMetadata["edge"] = "changed"

	secondPayloads := requirePreparedInputPayloads(
		t,
		input,
		"input",
	)

	if actual := preparedInputPayloadText(
		t,
		secondPayloads[0],
	); actual != "payload-a" {
		t.Fatalf(
			"NodeInput payload changed through accessor data: got %q",
			actual,
		)
	}

	if actual := secondPayloads[0].Metadata()["edge"]; actual !=
		"edge-a" {
		t.Fatalf(
			"NodeInput metadata changed through accessor map: got %q",
			actual,
		)
	}
}

func TestPrepareNodeInputGroupsDistinctPortsDeterministically(
	t *testing.T,
) {
	prepared := mustInputMultiPortPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("left-source"),
		execution.NodeExecutionStatusSucceeded,
	)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("right-source"),
		execution.NodeExecutionStatusSucceeded,
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-right",
		"right-value",
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-left",
		"left-value",
	)

	input, err := prepareNodeInput(
		prepared,
		workflow.NodeID("target"),
	)
	if err != nil {
		t.Fatalf(
			"prepareNodeInput(target) returned an unexpected error: %v",
			err,
		)
	}

	expectedPorts := []string{
		"left",
		"right",
	}

	if actual := input.Ports(); !reflect.DeepEqual(
		actual,
		expectedPorts,
	) {
		t.Fatalf(
			"Ports() = %#v, want %#v",
			actual,
			expectedPorts,
		)
	}

	if actual := input.PortCount(); actual != 2 {
		t.Fatalf(
			"PortCount() = %d, want 2",
			actual,
		)
	}

	if actual := input.TotalPayloadCount(); actual != 2 {
		t.Fatalf(
			"TotalPayloadCount() = %d, want 2",
			actual,
		)
	}

	leftPayloads := requirePreparedInputPayloads(
		t,
		input,
		"left",
	)

	rightPayloads := requirePreparedInputPayloads(
		t,
		input,
		"right",
	)

	if len(leftPayloads) != 1 {
		t.Fatalf(
			"left payload count = %d, want 1",
			len(leftPayloads),
		)
	}

	if len(rightPayloads) != 1 {
		t.Fatalf(
			"right payload count = %d, want 1",
			len(rightPayloads),
		)
	}

	if actual := preparedInputPayloadText(
		t,
		leftPayloads[0],
	); actual != "left-value" {
		t.Fatalf(
			"left payload = %q, want %q",
			actual,
			"left-value",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		rightPayloads[0],
	); actual != "right-value" {
		t.Fatalf(
			"right payload = %q, want %q",
			actual,
			"right-value",
		)
	}
}

func TestPrepareNodeInputDoesNotPartiallyConsumeOnValidationFailure(
	t *testing.T,
) {
	prepared := mustSchedulerFanInPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source-a"),
		execution.NodeExecutionStatusSucceeded,
	)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source-b"),
		execution.NodeExecutionStatusSucceeded,
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-a",
		"payload-a",
	)

	edgeAQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-a",
	)

	edgeBQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-b",
	)

	_, err := prepareNodeInput(
		prepared,
		workflow.NodeID("target"),
	)
	if err == nil {
		t.Fatal(
			"prepareNodeInput() returned nil error while one required queue was empty",
		)
	}

	if !strings.Contains(
		err.Error(),
		"edge-b contains no payload",
	) {
		t.Fatalf(
			"error = %q, want missing edge-b payload information",
			err,
		)
	}

	if actual := edgeAQueue.Len(); actual != 1 {
		t.Fatalf(
			"edge-a queue length after failed preparation = %d, want 1",
			actual,
		)
	}

	if actual := edgeBQueue.Len(); actual != 0 {
		t.Fatalf(
			"edge-b queue length after failed preparation = %d, want 0",
			actual,
		)
	}

	payload, exists := edgeAQueue.Peek()
	if !exists {
		t.Fatal(
			"edge-a payload was consumed during failed preparation",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		payload,
	); actual != "payload-a" {
		t.Fatalf(
			"remaining edge-a payload = %q, want %q",
			actual,
			"payload-a",
		)
	}
}

func TestPrepareNodeInputRejectsAmbiguousQueueWithoutConsumption(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-source-pass",
		"first",
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-source-pass",
		"second",
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	_, err := prepareNodeInput(
		prepared,
		workflow.NodeID("pass"),
	)
	if err == nil {
		t.Fatal(
			"prepareNodeInput() returned nil error for an ambiguous queue",
		)
	}

	if !strings.Contains(
		err.Error(),
		"contains 2 payloads",
	) {
		t.Fatalf(
			"error = %q, want queue ambiguity information",
			err,
		)
	}

	if actual := queue.Len(); actual != 2 {
		t.Fatalf(
			"queue length after rejected preparation = %d, want 2",
			actual,
		)
	}

	first, exists := queue.Pop()
	if !exists {
		t.Fatal(
			"first queued payload does not exist",
		)
	}

	second, exists := queue.Pop()
	if !exists {
		t.Fatal(
			"second queued payload does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		first,
	); actual != "first" {
		t.Fatalf(
			"first FIFO payload = %q, want %q",
			actual,
			"first",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		second,
	); actual != "second" {
		t.Fatalf(
			"second FIFO payload = %q, want %q",
			actual,
			"second",
		)
	}
}

func TestPrepareNodeInputRejectsInvalidPreparedState(
	t *testing.T,
) {
	t.Run("nil prepared execution", func(t *testing.T) {
		_, err := prepareNodeInput(
			nil,
			workflow.NodeID("source"),
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution",
		)
	})

	t.Run("missing execution context", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		prepared.executionContext = nil

		_, err := prepareNodeInput(
			prepared,
			workflow.NodeID("source"),
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.executionContext",
		)
	})

	t.Run("unknown node", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		_, err := prepareNodeInput(
			prepared,
			workflow.NodeID("unknown"),
		)

		requireEngineValidationField(
			t,
			err,
			"nodeID",
		)
	})
}

func mustInputMultiPortPreparedExecution(
	t *testing.T,
) *preparedExecution {
	t.Helper()

	const sourcePluginType workflow.PluginType = "test.input-source"

	const targetPluginType workflow.PluginType = "test.input-target"

	const version workflow.PluginVersion = "v1"

	sourceDescriptor := mustValidationDescriptor(
		t,
		sourcePluginType,
		version,
		plugin.DistributionDistributable,
		nil,
		[]string{"output"},
		plugin.NewExactEdgeConstraint(0),
		plugin.NewUnlimitedEdgeConstraint(1),
	)

	targetDescriptor := mustValidationDescriptor(
		t,
		targetPluginType,
		version,
		plugin.DistributionDistributable,
		[]string{
			"left",
			"right",
		},
		nil,
		plugin.NewExactEdgeConstraint(2),
		plugin.NewExactEdgeConstraint(0),
	)

	pluginRegistry, err := plugin.NewRegistry(
		[]plugin.Descriptor{
			targetDescriptor,
			sourceDescriptor,
		},
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			return runtime.NewNodeSuccessResult(
				nil,
				runtime.ContextChanges{},
			)
		},
	)

	executorRegistry, err := runtime.NewExecutorRegistry(
		[]runtime.ExecutorRegistration{
			mustValidationExecutorRegistration(
				t,
				sourceDescriptor.Identity(),
				executor,
			),
			mustValidationExecutorRegistration(
				t,
				targetDescriptor.Identity(),
				executor,
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	limits := mustValidationRuntimeLimits(
		t,
		8,
		8,
		4096,
	)

	clock := &preparationSequenceClock{
		times: preparationTestTimes(),
	}

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		clock,
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	leftSource := mustValidationNode(
		t,
		"left-source",
		sourcePluginType.String(),
		version.String(),
		`{}`,
	)

	rightSource := mustValidationNode(
		t,
		"right-source",
		sourcePluginType.String(),
		version.String(),
		`{}`,
	)

	target := mustValidationNode(
		t,
		"target",
		targetPluginType.String(),
		version.String(),
		`{}`,
	)

	leftEdge := mustValidationEdge(
		t,
		"edge-left",
		"left-source",
		"output",
		"target",
		"left",
	)

	rightEdge := mustValidationEdge(
		t,
		"edge-right",
		"right-source",
		"output",
		"target",
		"right",
	)

	definition := mustValidationDefinition(
		t,
		"workflow-input-multi-port",
		[]workflow.NodeDefinition{
			target,
			rightSource,
			leftSource,
		},
		[]workflow.EdgeDefinition{
			rightEdge,
			leftEdge,
		},
	)

	preparation, err := prepareExecution(
		context.Background(),
		mustValidationExecutionRequest(
			t,
			definition,
		),
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if preparation.prepared == nil {
		t.Fatal(
			"multi-port workflow was not prepared",
		)
	}

	return preparation.prepared
}

func requirePreparedInputPayloads(
	t *testing.T,
	input runtime.NodeInput,
	port string,
) []runtime.Payload {
	t.Helper()

	payloads, exists, err := input.Payloads(port)
	if err != nil {
		t.Fatalf(
			"Payloads(%q) returned an unexpected error: %v",
			port,
			err,
		)
	}

	if !exists {
		t.Fatalf(
			"Payloads(%q) exists = false",
			port,
		)
	}

	return payloads
}

func preparedInputPayloadText(
	t *testing.T,
	payload runtime.Payload,
) string {
	t.Helper()

	data, exists := payload.InlineData()
	if !exists {
		t.Fatal(
			"payload does not contain inline data",
		)
	}

	return string(data)
}

func TestNoopLifecycleRecorderImplementsContract(
	t *testing.T,
) {
	var implementation LifecycleRecorder = NoopLifecycleRecorder{}

	if implementation == nil {
		t.Fatal(
			"NoopLifecycleRecorder does not implement LifecycleRecorder",
		)
	}
}

func TestNoopLifecycleRecorderAcceptsBackgroundContext(
	t *testing.T,
) {
	recorder := NoopLifecycleRecorder{}
	ctx := context.Background()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "workflow creation",
			run: func() error {
				return recorder.RecordWorkflowCreation(
					ctx,
					WorkflowCreationObservation{},
				)
			},
		},
		{
			name: "workflow transition",
			run: func() error {
				return recorder.RecordWorkflowTransition(
					ctx,
					WorkflowTransitionObservation{},
				)
			},
		},
		{
			name: "node executions creation",
			run: func() error {
				return recorder.RecordNodeExecutionsCreation(
					ctx,
					NodeExecutionsCreationObservation{},
				)
			},
		},
		{
			name: "node transition",
			run: func() error {
				return recorder.RecordNodeTransition(
					ctx,
					NodeTransitionObservation{},
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err != nil {
				t.Fatalf(
					"no-op lifecycle operation returned an error: %v",
					err,
				)
			}
		})
	}
}

func TestNoopLifecycleRecorderRejectsNilContext(
	t *testing.T,
) {
	recorder := NoopLifecycleRecorder{}

	err := recorder.RecordWorkflowCreation(
		nil,
		WorkflowCreationObservation{},
	)
	if err == nil {
		t.Fatal(
			"RecordWorkflowCreation() accepted nil context",
		)
	}
}

func TestNoopLifecycleRecorderPreservesCancelledContext(
	t *testing.T,
) {
	recorder := NoopLifecycleRecorder{}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)
	cancel()

	err := recorder.RecordNodeTransition(
		ctx,
		NodeTransitionObservation{},
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"RecordNodeTransition() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestExecuteReadyNodeMarksCancelledWhenContextEndsDuringExecution(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	signalContext := newNodeExecutionSignalContext(
		false,
	)

	replacePreparedExecutionContextForNodeTest(
		t,
		prepared,
		signalContext,
	)

	successResult, err := runtime.NewNodeSuccessResult(
		nil,
		runtime.ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			if nodeContext.Err() != nil {
				t.Fatalf(
					"node context already contains an error before cancellation: %v",
					nodeContext.Err(),
				)
			}

			signalContext.Fail(
				context.Canceled,
			)

			return successResult, nil
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("source"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("source"),
	)
	if err == nil {
		t.Fatal(
			"executeReadyNode() returned nil error after context cancellation",
		)
	}

	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"error = %v, want wrapped context.Canceled",
			err,
		)
	}

	if result.IsValid() {
		t.Fatal(
			"cancelled execution returned a valid NodeResult",
		)
	}

	if executorCalls != 1 {
		t.Fatalf(
			"executor call count = %d, want 1",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusCancelled,
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after cancellation = %d, want 0",
			actual,
		)
	}
}

func TestExecuteReadyNodeMarksTimedOutWhenDeadlineEndsDuringExecution(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	signalContext := newNodeExecutionSignalContext(
		true,
	)

	replacePreparedExecutionContextForNodeTest(
		t,
		prepared,
		signalContext,
	)

	successResult, err := runtime.NewNodeSuccessResult(
		nil,
		runtime.ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			deadline, exists := nodeContext.Deadline()

			if !exists {
				t.Fatal(
					"node context does not expose a deadline",
				)
			}

			if deadline.IsZero() {
				t.Fatal(
					"node context deadline is zero",
				)
			}

			signalContext.Fail(
				context.DeadlineExceeded,
			)

			return successResult, nil
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("source"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("source"),
	)
	if err == nil {
		t.Fatal(
			"executeReadyNode() returned nil error after deadline expiration",
		)
	}

	if !errors.Is(
		err,
		context.DeadlineExceeded,
	) {
		t.Fatalf(
			"error = %v, want wrapped context.DeadlineExceeded",
			err,
		)
	}

	if result.IsValid() {
		t.Fatal(
			"timed-out execution returned a valid NodeResult",
		)
	}

	if executorCalls != 1 {
		t.Fatalf(
			"executor call count = %d, want 1",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusTimedOut,
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after timeout = %d, want 0",
			actual,
		)
	}
}

func TestExecuteReadyNodeRejectsContextCancelledBeforeExecution(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	signalContext := newNodeExecutionSignalContext(
		false,
	)

	signalContext.Fail(
		context.Canceled,
	)

	replacePreparedExecutionContextForNodeTest(
		t,
		prepared,
		signalContext,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("source"),
	)
	if err == nil {
		t.Fatal(
			"executeReadyNode() returned nil error for a pre-cancelled context",
		)
	}

	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"error = %v, want wrapped context.Canceled",
			err,
		)
	}

	if result.IsValid() {
		t.Fatal(
			"pre-cancelled execution returned a valid NodeResult",
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusPending,
	)
}

func TestExecuteReadyNodeRejectsInvalidPreparedState(
	t *testing.T,
) {
	t.Run("nil prepared execution", func(t *testing.T) {
		_, err := executeReadyNode(
			nil,
			workflow.NodeID("source"),
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution",
		)
	})

	t.Run("missing execution context", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		prepared.executionContext = nil

		_, err := executeReadyNode(
			prepared,
			workflow.NodeID("source"),
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.executionContext",
		)
	})

	t.Run("missing workflow execution", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		prepared.workflowExecution = nil

		_, err := executeReadyNode(
			prepared,
			workflow.NodeID("source"),
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.workflowExecution",
		)
	})

	t.Run("invalid dependencies", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		prepared.dependencies = EngineDependencies{}

		_, err := executeReadyNode(
			prepared,
			workflow.NodeID("source"),
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.dependencies",
		)
	})

	t.Run("unknown node", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		_, err := executeReadyNode(
			prepared,
			workflow.NodeID("unknown"),
		)

		requireEngineValidationField(
			t,
			err,
			"nodeID",
		)
	})

	t.Run("missing node execution", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		delete(
			prepared.nodeExecutionsByNode,
			workflow.NodeID("source"),
		)

		_, err := executeReadyNode(
			prepared,
			workflow.NodeID("source"),
		)
		if err == nil {
			t.Fatal(
				"executeReadyNode() returned nil error for a missing node execution",
			)
		}

		if !strings.Contains(
			err.Error(),
			"node execution",
		) {
			t.Fatalf(
				"error = %q, want node-execution information",
				err,
			)
		}
	})

	t.Run("waiting node", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		_, err := executeReadyNode(
			prepared,
			workflow.NodeID("pass"),
		)

		requireEngineValidationField(
			t,
			err,
			"nodeExecution.readiness",
		)

		requireNodeExecutionStatus(
			t,
			prepared,
			workflow.NodeID("pass"),
			execution.NodeExecutionStatusPending,
		)
	})

	t.Run("missing descriptor", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		delete(
			prepared.plan.descriptorsByNode,
			workflow.NodeID("source"),
		)

		_, err := executeReadyNode(
			prepared,
			workflow.NodeID("source"),
		)
		if err == nil {
			t.Fatal(
				"executeReadyNode() returned nil error for a missing descriptor",
			)
		}

		if !strings.Contains(
			err.Error(),
			"descriptor",
		) {
			t.Fatalf(
				"error = %q, want descriptor information",
				err,
			)
		}

		requireNodeExecutionStatus(
			t,
			prepared,
			workflow.NodeID("source"),
			execution.NodeExecutionStatusPending,
		)
	})

	t.Run("missing executor", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		emptyRegistry, err :=
			runtime.NewExecutorRegistry(nil)

		if err != nil {
			t.Fatalf(
				"runtime.NewExecutorRegistry(nil) returned an unexpected error: %v",
				err,
			)
		}

		prepared.dependencies.executorRegistry =
			emptyRegistry

		_, err = executeReadyNode(
			prepared,
			workflow.NodeID("source"),
		)
		if err == nil {
			t.Fatal(
				"executeReadyNode() returned nil error for a missing executor",
			)
		}

		if !strings.Contains(
			err.Error(),
			"executor",
		) {
			t.Fatalf(
				"error = %q, want executor information",
				err,
			)
		}

		requireNodeExecutionStatus(
			t,
			prepared,
			workflow.NodeID("source"),
			execution.NodeExecutionStatusPending,
		)
	})
}

func TestIncomingEdgeIDsForNodeUsesStableOrder(
	t *testing.T,
) {
	prepared := mustSchedulerFanInPreparedExecution(t)

	actual := incomingEdgeIDsForNode(
		prepared,
		workflow.NodeID("target"),
	)

	expected := []workflow.EdgeID{
		workflow.EdgeID("edge-a"),
		workflow.EdgeID("edge-b"),
	}

	if !reflect.DeepEqual(
		actual,
		expected,
	) {
		t.Fatalf(
			"incoming edge ID order = %#v, want %#v",
			actual,
			expected,
		)
	}

	actual[0] = workflow.EdgeID("changed")

	secondSnapshot := incomingEdgeIDsForNode(
		prepared,
		workflow.NodeID("target"),
	)

	if secondSnapshot[0].String() != "edge-a" {
		t.Fatalf(
			"incoming edge order changed through returned slice: %#v",
			secondSnapshot,
		)
	}

	if actual := incomingEdgeIDsForNode(
		prepared,
		workflow.NodeID("source-a"),
	); actual != nil {
		t.Fatalf(
			"root incoming edge IDs = %#v, want nil",
			actual,
		)
	}

	if actual := incomingEdgeIDsForNode(
		nil,
		workflow.NodeID("target"),
	); actual != nil {
		t.Fatalf(
			"nil prepared incoming edge IDs = %#v, want nil",
			actual,
		)
	}
}

func TestTransitionRunningNodeRejectsNonRunningExecution(
	t *testing.T,
) {
	prepared :=
		mustSchedulerLinearPreparedExecution(t)

	nodeID := workflow.NodeID("source")

	nodeExecution :=
		prepared.nodeExecutionsByNode[nodeID]

	if nodeExecution == nil {
		t.Fatal(
			"source node execution is nil",
		)
	}

	err := transitionRunningNode(
		prepared,
		nodeID,
		nodeExecution,
		execution.NodeExecutionStatusFailed,
		nodeTransitionObservationDetails{},
	)

	requireEngineValidationField(
		t,
		err,
		"nodeExecution.status",
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		nodeID,
		execution.NodeExecutionStatusPending,
	)
}

func TestTransitionRunningNodeRejectsUnsupportedTarget(
	t *testing.T,
) {
	prepared :=
		mustSchedulerLinearPreparedExecution(t)

	nodeID := workflow.NodeID("source")

	transitionSchedulerNode(
		t,
		prepared,
		nodeID,
		execution.NodeExecutionStatusRunning,
	)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				18,
				12,
				0,
				0,
				0,
				time.UTC,
			),
		}

	nodeExecution :=
		prepared.nodeExecutionsByNode[nodeID]

	if nodeExecution == nil {
		t.Fatal(
			"source node execution is nil",
		)
	}

	err := transitionRunningNode(
		prepared,
		nodeID,
		nodeExecution,
		execution.NodeExecutionStatusSkipped,
		nodeTransitionObservationDetails{},
	)

	requireEngineValidationField(
		t,
		err,
		"targetStatus",
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		nodeID,
		execution.NodeExecutionStatusRunning,
	)
}

func replacePreparedExecutionContextForNodeTest(
	t *testing.T,
	prepared *preparedExecution,
	parent context.Context,
) {
	t.Helper()

	if prepared == nil {
		t.Fatal(
			"prepared execution is nil",
		)
	}

	if parent == nil {
		t.Fatal(
			"replacement parent context is nil",
		)
	}

	if prepared.workflowExecution == nil {
		t.Fatal(
			"prepared workflow execution is nil",
		)
	}

	if prepared.executionContext == nil {
		t.Fatal(
			"prepared execution context is nil",
		)
	}

	edgeRuntimes := []*runtime.EdgeRuntime{
		routingEdgeRuntime(
			t,
			prepared,
			"edge-source-pass",
		),
		routingEdgeRuntime(
			t,
			prepared,
			"edge-pass-terminal",
		),
	}

	replacement, err := runtime.NewExecutionContext(
		parent,
		*prepared.workflowExecution,
		prepared.request.CorrelationID(),
		edgeRuntimes,
		prepared.executionContext.VariablesSnapshot(),
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	prepared.executionContext = replacement
}

type nodeExecutionSignalContext struct {
	done chan struct{}

	once sync.Once

	mu  sync.RWMutex
	err error

	deadline    time.Time
	hasDeadline bool
}

func newNodeExecutionSignalContext(
	hasDeadline bool,
) *nodeExecutionSignalContext {
	deadline := time.Time{}

	if hasDeadline {
		deadline = time.Date(
			2026,
			time.July,
			18,
			13,
			0,
			0,
			0,
			time.UTC,
		)
	}

	return &nodeExecutionSignalContext{
		done: make(chan struct{}),

		deadline:    deadline,
		hasDeadline: hasDeadline,
	}
}

func (signal *nodeExecutionSignalContext) Deadline() (
	time.Time,
	bool,
) {
	if signal == nil {
		return time.Time{}, false
	}

	return signal.deadline,
		signal.hasDeadline
}

func (signal *nodeExecutionSignalContext) Done() <-chan struct{} {
	if signal == nil {
		return nil
	}

	return signal.done
}

func (signal *nodeExecutionSignalContext) Err() error {
	if signal == nil {
		return nil
	}

	signal.mu.RLock()
	defer signal.mu.RUnlock()

	return signal.err
}

func (signal *nodeExecutionSignalContext) Value(
	key any,
) any {
	return nil
}

func (signal *nodeExecutionSignalContext) Fail(
	err error,
) {
	if signal == nil {
		return
	}

	signal.once.Do(
		func() {
			signal.mu.Lock()
			signal.err = err
			signal.mu.Unlock()

			close(signal.done)
		},
	)
}

func TestExecuteReadyNodeCompletesSuccessfulRootExecution(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	outputPayload := mustRoutingPayload(
		t,
		"source-output",
		map[string]string{
			"producer": "source",
		},
	)

	contextValue := mustNodeExecutionRuntimeValue(
		t,
		`"completed"`,
	)

	contextChanges, err := runtime.NewContextChanges(
		map[string]runtime.RuntimeValue{
			"stage": contextValue,
		},
		nil,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewContextChanges() returned an unexpected error: %v",
			err,
		)
	}

	expectedResult, err := runtime.NewNodeSuccessResult(
		map[string][]runtime.Payload{
			"output": {
				outputPayload,
			},
		},
		contextChanges,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			if actual := nodeContext.NodeID().String(); actual != "source" {
				t.Fatalf(
					"executor node ID = %q, want %q",
					actual,
					"source",
				)
			}

			if actual := nodeContext.WorkflowExecutionID(); actual != prepared.workflowExecution.ID() {
				t.Fatalf(
					"executor workflow execution ID = %q, want %q",
					actual,
					prepared.workflowExecution.ID(),
				)
			}

			if !input.IsValid() {
				t.Fatal(
					"executor received an invalid NodeInput",
				)
			}

			if !input.IsEmpty() {
				t.Fatal(
					"root executor received a non-empty NodeInput",
				)
			}

			if actual := input.TotalPayloadCount(); actual != 0 {
				t.Fatalf(
					"root input payload count = %d, want 0",
					actual,
				)
			}

			if configuration.String() == "" {
				t.Fatal(
					"executor received an empty node configuration",
				)
			}

			return expectedResult, nil
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("source"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("source"),
	)
	if err != nil {
		t.Fatalf(
			"executeReadyNode(source) returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"executeReadyNode() returned an invalid result",
		)
	}

	if !result.IsSuccess() {
		t.Fatalf(
			"result status = %q, want SUCCEEDED",
			result.Status(),
		)
	}

	if executorCalls != 1 {
		t.Fatalf(
			"executor call count = %d, want 1",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireCompletedNodeExecutionTimestamps(
		t,
		prepared,
		workflow.NodeID("source"),
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 1 {
		t.Fatalf(
			"source output queue length = %d, want 1",
			actual,
		)
	}

	routedPayload, exists := queue.Peek()
	if !exists {
		t.Fatal(
			"source output queue contains no payload",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		routedPayload,
	); actual != "source-output" {
		t.Fatalf(
			"routed payload = %q, want %q",
			actual,
			"source-output",
		)
	}

	if actual := routedPayload.Metadata()["producer"]; actual != "source" {
		t.Fatalf(
			"routed payload producer = %q, want %q",
			actual,
			"source",
		)
	}

	value, exists, err := prepared.executionContext.Variable(
		"stage",
	)
	if err != nil {
		t.Fatalf(
			"executionContext.Variable(stage) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"context variable stage does not exist",
		)
	}

	if actual := value.String(); actual != `"completed"` {
		t.Fatalf(
			"context variable stage = %q, want %q",
			actual,
			`"completed"`,
		)
	}
}

func TestExecuteReadyNodeConsumesChildInputAndRoutesOutput(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-source-pass",
		"incoming-value",
	)

	expectedResult, err := runtime.NewNodeSuccessResult(
		map[string][]runtime.Payload{
			"output": {
				mustRoutingPayload(
					t,
					"pass-output",
					nil,
				),
			},
		},
		runtime.ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			if actual := nodeContext.NodeID().String(); actual != "pass" {
				t.Fatalf(
					"executor node ID = %q, want %q",
					actual,
					"pass",
				)
			}

			incomingEdges := nodeContext.IncomingEdges()

			if len(incomingEdges) != 1 {
				t.Fatalf(
					"authorized incoming-edge count = %d, want 1",
					len(incomingEdges),
				)
			}

			if actual := incomingEdges[0].ID().String(); actual != "edge-source-pass" {
				t.Fatalf(
					"incoming edge ID = %q, want %q",
					actual,
					"edge-source-pass",
				)
			}

			payloads, exists, err := input.Payloads(
				"input",
			)
			if err != nil {
				t.Fatalf(
					"input.Payloads(input) returned an unexpected error: %v",
					err,
				)
			}

			if !exists {
				t.Fatal(
					"input port does not exist",
				)
			}

			if len(payloads) != 1 {
				t.Fatalf(
					"input payload count = %d, want 1",
					len(payloads),
				)
			}

			if actual := preparedInputPayloadText(
				t,
				payloads[0],
			); actual != "incoming-value" {
				t.Fatalf(
					"input payload = %q, want %q",
					actual,
					"incoming-value",
				)
			}

			if configuration.String() == "" {
				t.Fatal(
					"executor received an empty node configuration",
				)
			}

			return expectedResult, nil
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("pass"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("pass"),
	)
	if err != nil {
		t.Fatalf(
			"executeReadyNode(pass) returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsSuccess() {
		t.Fatalf(
			"result status = %q, want SUCCEEDED",
			result.Status(),
		)
	}

	if executorCalls != 1 {
		t.Fatalf(
			"executor call count = %d, want 1",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusSucceeded,
	)

	incomingQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := incomingQueue.Len(); actual != 0 {
		t.Fatalf(
			"incoming queue length after execution = %d, want 0",
			actual,
		)
	}

	outgoingQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-pass-terminal",
	)

	if actual := outgoingQueue.Len(); actual != 1 {
		t.Fatalf(
			"outgoing queue length after execution = %d, want 1",
			actual,
		)
	}

	payload, exists := outgoingQueue.Peek()
	if !exists {
		t.Fatal(
			"outgoing queue contains no payload",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		payload,
	); actual != "pass-output" {
		t.Fatalf(
			"outgoing payload = %q, want %q",
			actual,
			"pass-output",
		)
	}
}

func TestExecuteReadyNodeReturnsTerminalOutput(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusSucceeded,
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-pass-terminal",
		"terminal-input",
	)

	terminalPayload := mustRoutingPayload(
		t,
		"workflow-result",
		map[string]string{
			"terminal": "true",
		},
	)

	expectedResult, err :=
		runtime.NewTerminalNodeSuccessResult(
			terminalPayload,
			runtime.ContextChanges{},
		)
	if err != nil {
		t.Fatalf(
			"runtime.NewTerminalNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			if actual := nodeContext.NodeID().String(); actual != "terminal" {
				t.Fatalf(
					"executor node ID = %q, want %q",
					actual,
					"terminal",
				)
			}

			payloads, exists, err := input.Payloads(
				"input",
			)
			if err != nil {
				t.Fatalf(
					"input.Payloads(input) returned an unexpected error: %v",
					err,
				)
			}

			if !exists || len(payloads) != 1 {
				t.Fatalf(
					"terminal input payload count = %d, exists = %t, want 1",
					len(payloads),
					exists,
				)
			}

			if actual := preparedInputPayloadText(
				t,
				payloads[0],
			); actual != "terminal-input" {
				t.Fatalf(
					"terminal input = %q, want %q",
					actual,
					"terminal-input",
				)
			}

			return expectedResult, nil
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("terminal"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("terminal"),
	)
	if err != nil {
		t.Fatalf(
			"executeReadyNode(terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsSuccess() {
		t.Fatalf(
			"terminal result status = %q, want SUCCEEDED",
			result.Status(),
		)
	}

	if !result.HasTerminalOutput() {
		t.Fatal(
			"terminal result does not contain terminal output",
		)
	}

	if result.HasRoutedOutputs() {
		t.Fatal(
			"terminal result unexpectedly contains routed outputs",
		)
	}

	actualTerminalPayload, exists :=
		result.TerminalOutput()

	if !exists {
		t.Fatal(
			"TerminalOutput() exists = false",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		actualTerminalPayload,
	); actual != "workflow-result" {
		t.Fatalf(
			"terminal output = %q, want %q",
			actual,
			"workflow-result",
		)
	}

	if actual := actualTerminalPayload.Metadata()["terminal"]; actual != "true" {
		t.Fatalf(
			"terminal metadata = %q, want %q",
			actual,
			"true",
		)
	}

	if executorCalls != 1 {
		t.Fatalf(
			"executor call count = %d, want 1",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusSucceeded,
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-pass-terminal",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"terminal incoming queue length = %d, want 0",
			actual,
		)
	}
}

func TestExecuteReadyNodeHandlesControlledFailure(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryExecution,
		"TEST_CONTROLLED_FAILURE",
		"Controlled node failure",
		false,
		map[string]string{
			"node": "source",
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewRuntimeFailure() returned an unexpected error: %v",
			err,
		)
	}

	expectedResult, err :=
		runtime.NewNodeFailureResult(
			failure,
		)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeFailureResult() returned an unexpected error: %v",
			err,
		)
	}

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			return expectedResult, nil
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("source"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("source"),
	)
	if err != nil {
		t.Fatalf(
			"executeReadyNode() returned an error for a controlled failure: %v",
			err,
		)
	}

	if !result.IsFailure() {
		t.Fatalf(
			"result status = %q, want FAILED",
			result.Status(),
		)
	}

	returnedFailure, exists := result.Failure()
	if !exists {
		t.Fatal(
			"controlled failure result contains no RuntimeFailure",
		)
	}

	if !returnedFailure.IsValid() {
		t.Fatal(
			"controlled RuntimeFailure is invalid",
		)
	}

	if executorCalls != 1 {
		t.Fatalf(
			"executor call count = %d, want 1",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusFailed,
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after controlled failure = %d, want 0",
			actual,
		)
	}
}

func TestExecuteReadyNodeHandlesTechnicalExecutorError(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	executorFailure := errors.New(
		"executor dependency unavailable",
	)

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			return runtime.NodeResult{},
				executorFailure
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("source"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("source"),
	)
	if err == nil {
		t.Fatal(
			"executeReadyNode() returned nil error for a technical executor error",
		)
	}

	if !errors.Is(
		err,
		executorFailure,
	) {
		t.Fatalf(
			"error = %v, want wrapped executor failure",
			err,
		)
	}

	if result.IsValid() {
		t.Fatal(
			"technical error returned a valid NodeResult",
		)
	}

	if executorCalls != 1 {
		t.Fatalf(
			"executor call count = %d, want 1",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusFailed,
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after technical failure = %d, want 0",
			actual,
		)
	}
}

func TestExecuteReadyNodeRejectsInvalidExecutorResult(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			return runtime.NodeResult{}, nil
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("source"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("source"),
	)
	if err == nil {
		t.Fatal(
			"executeReadyNode() returned nil error for an invalid NodeResult",
		)
	}

	if !strings.Contains(
		err.Error(),
		"invalid result",
	) {
		t.Fatalf(
			"error = %q, want invalid-result information",
			err,
		)
	}

	if result.IsValid() {
		t.Fatal(
			"invalid executor result unexpectedly became valid",
		)
	}

	if executorCalls != 1 {
		t.Fatalf(
			"executor call count = %d, want 1",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusFailed,
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after invalid result = %d, want 0",
			actual,
		)
	}
}

func TestExecuteReadyNodeRejectsAlreadyExecutedNode(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			return runtime.NewNodeSuccessResult(
				nil,
				runtime.ContextChanges{},
			)
		},
	)

	installNodeExecutionTestExecutor(
		t,
		prepared,
		workflow.NodeID("source"),
		executor,
	)

	result, err := executeReadyNode(
		prepared,
		workflow.NodeID("source"),
	)

	requireEngineValidationField(
		t,
		err,
		"nodeExecution.status",
	)

	if result.IsValid() {
		t.Fatal(
			"already-executed node returned a valid result",
		)
	}

	if executorCalls != 0 {
		t.Fatalf(
			"executor call count = %d, want 0",
			executorCalls,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)
}

func installNodeExecutionTestExecutor(
	t *testing.T,
	prepared *preparedExecution,
	nodeID workflow.NodeID,
	executor runtime.NodeExecutor,
) {
	t.Helper()

	if prepared == nil {
		t.Fatal(
			"prepared execution is nil",
		)
	}

	descriptor, exists := prepared.plan.descriptorsByNode[nodeID]
	if !exists {
		t.Fatalf(
			"plugin descriptor for node %q is unavailable",
			nodeID,
		)
	}

	registration, err := runtime.NewExecutorRegistration(
		descriptor.Identity(),
		executor,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistration() returned an unexpected error: %v",
			err,
		)
	}

	registry, err := runtime.NewExecutorRegistry(
		[]runtime.ExecutorRegistration{
			registration,
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	prepared.dependencies.executorRegistry =
		registry

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				18,
				10,
				0,
				0,
				0,
				time.UTC,
			),
		}
}

func mustNodeExecutionRuntimeValue(
	t *testing.T,
	value string,
) runtime.RuntimeValue {
	t.Helper()

	runtimeValue, err := runtime.NewRuntimeValue(
		[]byte(value),
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewRuntimeValue(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return runtimeValue
}

func requireNodeExecutionStatus(
	t *testing.T,
	prepared *preparedExecution,
	nodeID workflow.NodeID,
	expected execution.NodeExecutionStatus,
) {
	t.Helper()

	if prepared == nil {
		t.Fatal(
			"prepared execution is nil",
		)
	}

	nodeExecution, exists := prepared.nodeExecutionsByNode[nodeID]

	if !exists || nodeExecution == nil {
		t.Fatalf(
			"node execution for node %q is unavailable",
			nodeID,
		)
	}

	if actual := nodeExecution.Status(); actual != expected {
		t.Fatalf(
			"node %q status = %q, want %q",
			nodeID,
			actual,
			expected,
		)
	}
}

func requireCompletedNodeExecutionTimestamps(
	t *testing.T,
	prepared *preparedExecution,
	nodeID workflow.NodeID,
) {
	t.Helper()

	nodeExecution, exists := prepared.nodeExecutionsByNode[nodeID]

	if !exists || nodeExecution == nil {
		t.Fatalf(
			"node execution for node %q is unavailable",
			nodeID,
		)
	}

	startedAt, started := nodeExecution.StartedAt()
	if !started || startedAt.IsZero() {
		t.Fatalf(
			"node %q has no startedAt timestamp",
			nodeID,
		)
	}

	finishedAt, finished := nodeExecution.FinishedAt()
	if !finished || finishedAt.IsZero() {
		t.Fatalf(
			"node %q has no finishedAt timestamp",
			nodeID,
		)
	}

	if finishedAt.Before(startedAt) {
		t.Fatalf(
			"node %q finishedAt %v is before startedAt %v",
			nodeID,
			finishedAt,
			startedAt,
		)
	}
}

type nodeExecutionTestClock struct {
	next time.Time
}

func (clock *nodeExecutionTestClock) IsValid() bool {
	return clock != nil
}

func (clock *nodeExecutionTestClock) Now() time.Time {
	if clock == nil ||
		clock.next.IsZero() {
		return time.Time{}
	}

	current := clock.next

	clock.next = clock.next.Add(
		time.Minute,
	)

	return current
}

type nodeLifecycleRecorderSpy struct {
	nodeTransitions []NodeTransitionObservation
	nodeError       error
}

var _ LifecycleRecorder = (*nodeLifecycleRecorderSpy)(nil)

func (recorder *nodeLifecycleRecorderSpy) IsValid() bool {
	return recorder != nil
}

func (
	*nodeLifecycleRecorderSpy,
) RecordWorkflowCreation(
	context.Context,
	WorkflowCreationObservation,
) error {
	return nil
}

func (
	*nodeLifecycleRecorderSpy,
) RecordWorkflowTransition(
	context.Context,
	WorkflowTransitionObservation,
) error {
	return nil
}

func (
	*nodeLifecycleRecorderSpy,
) RecordNodeExecutionsCreation(
	context.Context,
	NodeExecutionsCreationObservation,
) error {
	return nil
}

func (
	recorder *nodeLifecycleRecorderSpy,
) RecordNodeTransition(
	_ context.Context,
	observation NodeTransitionObservation,
) error {
	recorder.nodeTransitions = append(
		recorder.nodeTransitions,
		observation,
	)

	return recorder.nodeError
}

func TestApplyRecordedNodeTransitionCommitsAfterRecorderSuccess(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		2,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder :=
		newPreparedNodeLifecycleTestExecution(
			t,
			baseTime,
		)

	nodeID := workflow.NodeID("source")

	nodeExecution :=
		prepared.nodeExecutionsByNode[nodeID]
	if nodeExecution == nil {
		t.Fatal(
			"source node execution is nil",
		)
	}

	transitionAt :=
		baseTime.Add(10 * time.Minute)

	err := applyRecordedNodeTransition(
		prepared,
		nodeID,
		nodeExecution,
		transitionAt,
		func(candidate *execution.NodeExecution) error {
			return candidate.MarkReady(
				transitionAt,
			)
		},
		nodeTransitionObservationDetails{},
	)
	if err != nil {
		t.Fatalf(
			"applyRecordedNodeTransition() returned an error: %v",
			err,
		)
	}

	if nodeExecution.Status() !=
		execution.NodeExecutionStatusReady {
		t.Fatalf(
			"node status = %q, want READY",
			nodeExecution.Status(),
		)
	}

	if len(recorder.nodeTransitions) != 1 {
		t.Fatalf(
			"node transition count = %d, want 1",
			len(recorder.nodeTransitions),
		)
	}

	observation :=
		recorder.nodeTransitions[0]

	if observation.Before.Status() !=
		execution.NodeExecutionStatusPending {
		t.Fatalf(
			"previous status = %q, want PENDING",
			observation.Before.Status(),
		)
	}

	if observation.After.Status() !=
		execution.NodeExecutionStatusReady {
		t.Fatalf(
			"target status = %q, want READY",
			observation.After.Status(),
		)
	}

	if !observation.TransitionAt.Equal(
		transitionAt,
	) {
		t.Fatalf(
			"transition time = %v, want %v",
			observation.TransitionAt,
			transitionAt,
		)
	}

	if observation.Definition.ID() != nodeID {
		t.Fatalf(
			"definition node ID = %q, want %q",
			observation.Definition.ID(),
			nodeID,
		)
	}
}

func TestApplyRecordedNodeTransitionDoesNotMutateWhenRecorderFails(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		3,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder :=
		newPreparedNodeLifecycleTestExecution(
			t,
			baseTime,
		)

	sentinel := errors.New(
		"controlled node persistence failure",
	)

	recorder.nodeError = sentinel

	nodeID := workflow.NodeID("source")

	nodeExecution :=
		prepared.nodeExecutionsByNode[nodeID]
	if nodeExecution == nil {
		t.Fatal(
			"source node execution is nil",
		)
	}

	transitionAt :=
		baseTime.Add(10 * time.Minute)

	err := applyRecordedNodeTransition(
		prepared,
		nodeID,
		nodeExecution,
		transitionAt,
		func(candidate *execution.NodeExecution) error {
			return candidate.MarkReady(
				transitionAt,
			)
		},
		nodeTransitionObservationDetails{},
	)

	if !errors.Is(err, sentinel) {
		t.Fatalf(
			"applyRecordedNodeTransition() error = %v, want sentinel",
			err,
		)
	}

	if nodeExecution.Status() !=
		execution.NodeExecutionStatusPending {
		t.Fatalf(
			"node status after recorder failure = %q, want PENDING",
			nodeExecution.Status(),
		)
	}

	if len(recorder.nodeTransitions) != 1 {
		t.Fatalf(
			"node transition attempt count = %d, want 1",
			len(recorder.nodeTransitions),
		)
	}
}

func TestTransitionRunningNodeRecordsControlledFailure(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		4,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder :=
		newPreparedNodeLifecycleTestExecution(
			t,
			baseTime,
		)

	nodeID := workflow.NodeID("source")

	nodeExecution :=
		prepared.nodeExecutionsByNode[nodeID]
	if nodeExecution == nil {
		t.Fatal(
			"source node execution is nil",
		)
	}

	readyAt :=
		baseTime.Add(10 * time.Minute)

	if err := applyRecordedNodeTransition(
		prepared,
		nodeID,
		nodeExecution,
		readyAt,
		func(candidate *execution.NodeExecution) error {
			return candidate.MarkReady(
				readyAt,
			)
		},
		nodeTransitionObservationDetails{},
	); err != nil {
		t.Fatalf(
			"mark node ready: %v",
			err,
		)
	}

	startedAt :=
		baseTime.Add(11 * time.Minute)

	if err := applyRecordedNodeTransition(
		prepared,
		nodeID,
		nodeExecution,
		startedAt,
		func(candidate *execution.NodeExecution) error {
			return candidate.Start(
				startedAt,
			)
		},
		nodeTransitionObservationDetails{},
	); err != nil {
		t.Fatalf(
			"start node: %v",
			err,
		)
	}

	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryExecution,
		"CONTROLLED_NODE_FAILURE",
		"Node execution failed",
		false,
		map[string]string{
			"nodeID": nodeID.String(),
		},
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeFailure() returned an error: %v",
			err,
		)
	}

	result, err := runtime.NewNodeFailureResult(
		failure,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeFailureResult() returned an error: %v",
			err,
		)
	}

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime.Add(
				12 * time.Minute,
			),
		}

	if err := transitionRunningNode(
		prepared,
		nodeID,
		nodeExecution,
		execution.NodeExecutionStatusFailed,
		nodeTransitionObservationDetails{
			result:     result,
			hasResult:  true,
			failure:    failure,
			hasFailure: true,
		},
	); err != nil {
		t.Fatalf(
			"transitionRunningNode() returned an error: %v",
			err,
		)
	}

	if nodeExecution.Status() !=
		execution.NodeExecutionStatusFailed {
		t.Fatalf(
			"node status = %q, want FAILED",
			nodeExecution.Status(),
		)
	}

	if len(recorder.nodeTransitions) != 3 {
		t.Fatalf(
			"node transition count = %d, want 3",
			len(recorder.nodeTransitions),
		)
	}

	terminalObservation :=
		recorder.nodeTransitions[2]

	if terminalObservation.Before.Status() !=
		execution.NodeExecutionStatusRunning {
		t.Fatalf(
			"terminal previous status = %q, want RUNNING",
			terminalObservation.Before.Status(),
		)
	}

	if terminalObservation.After.Status() !=
		execution.NodeExecutionStatusFailed {
		t.Fatalf(
			"terminal target status = %q, want FAILED",
			terminalObservation.After.Status(),
		)
	}

	if !terminalObservation.HasResult {
		t.Fatal(
			"controlled failure observation has no NodeResult",
		)
	}

	if !terminalObservation.HasFailure {
		t.Fatal(
			"controlled failure observation has no RuntimeFailure",
		)
	}

	if terminalObservation.Failure.Code() !=
		"CONTROLLED_NODE_FAILURE" {
		t.Fatalf(
			"failure code = %q",
			terminalObservation.Failure.Code(),
		)
	}
}

func newPreparedNodeLifecycleTestExecution(
	t *testing.T,
	baseTime time.Time,
) (
	*preparedExecution,
	*nodeLifecycleRecorderSpy,
) {
	t.Helper()

	limits := mustValidationRuntimeLimits(
		t,
		64,
		1,
		4096,
	)

	dependencies :=
		mustValidationCoreDependencies(
			t,
			limits,
			true,
		)

	dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime,
		}

	recorder :=
		&nodeLifecycleRecorderSpy{}

	updatedDependencies, err :=
		dependencies.WithLifecycleRecorder(
			recorder,
		)
	if err != nil {
		t.Fatalf(
			"WithLifecycleRecorder() returned an error: %v",
			err,
		)
	}

	request :=
		mustValidationExecutionRequest(
			t,
			mustValidationLinearWorkflow(
				t,
				"source",
				"pass",
				"terminal",
			),
		)

	preparation, err := prepareExecution(
		context.Background(),
		request,
		updatedDependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an error: %v",
			err,
		)
	}

	if !preparation.IsPrepared() ||
		preparation.prepared == nil {
		t.Fatal(
			"test workflow was not prepared",
		)
	}

	recorder.nodeTransitions = nil

	return preparation.prepared,
		recorder
}

const (
	preparationOperationWorkflowCreation = "WORKFLOW_CREATION"

	preparationOperationWorkflowTransition = "WORKFLOW_TRANSITION"

	preparationOperationNodeExecutionsCreation = "NODE_EXECUTIONS_CREATION"
)

type preparationLifecycleRecorder struct {
	operations []string

	workflowCreations   []WorkflowCreationObservation
	workflowTransitions []WorkflowTransitionObservation
	nodeCreations       []NodeExecutionsCreationObservation

	failAt  int
	failErr error
}

var _ LifecycleRecorder = (*preparationLifecycleRecorder)(nil)

func (recorder *preparationLifecycleRecorder) IsValid() bool {
	return recorder != nil
}

func (
	recorder *preparationLifecycleRecorder,
) RecordWorkflowCreation(
	_ context.Context,
	observation WorkflowCreationObservation,
) error {
	recorder.workflowCreations = append(
		recorder.workflowCreations,
		observation,
	)

	return recorder.recordOperation(
		preparationOperationWorkflowCreation,
	)
}

func (
	recorder *preparationLifecycleRecorder,
) RecordWorkflowTransition(
	_ context.Context,
	observation WorkflowTransitionObservation,
) error {
	recorder.workflowTransitions = append(
		recorder.workflowTransitions,
		observation,
	)

	return recorder.recordOperation(
		preparationOperationWorkflowTransition,
	)
}

func (
	recorder *preparationLifecycleRecorder,
) RecordNodeExecutionsCreation(
	_ context.Context,
	observation NodeExecutionsCreationObservation,
) error {
	recorder.nodeCreations = append(
		recorder.nodeCreations,
		observation,
	)

	return recorder.recordOperation(
		preparationOperationNodeExecutionsCreation,
	)
}

func (
	recorder *preparationLifecycleRecorder,
) RecordNodeTransition(
	_ context.Context,
	_ NodeTransitionObservation,
) error {
	return recorder.recordOperation(
		"NODE_TRANSITION",
	)
}

func (
	recorder *preparationLifecycleRecorder,
) recordOperation(
	operation string,
) error {
	recorder.operations = append(
		recorder.operations,
		operation,
	)

	if recorder.failAt > 0 &&
		len(recorder.operations) ==
			recorder.failAt {
		if recorder.failErr != nil {
			return recorder.failErr
		}

		return errors.New(
			"controlled lifecycle recorder failure",
		)
	}

	return nil
}

func TestPrepareExecutionRecordsValidLifecycleInOrder(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		17,
		23,
		0,
		0,
		0,
		time.UTC,
	)

	limits := mustValidationRuntimeLimits(
		t,
		64,
		1,
		4096,
	)

	dependencies :=
		mustValidationCoreDependencies(
			t,
			limits,
			true,
		)

	dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime,
		}

	recorder :=
		&preparationLifecycleRecorder{}

	dependencies, err :=
		dependencies.WithLifecycleRecorder(
			recorder,
		)
	if err != nil {
		t.Fatalf(
			"WithLifecycleRecorder() returned an error: %v",
			err,
		)
	}

	definition :=
		mustValidationLinearWorkflow(
			t,
			"source",
			"pass",
			"terminal",
		)

	request :=
		mustValidationExecutionRequest(
			t,
			definition,
		)

	preparation, err := prepareExecution(
		context.Background(),
		request,
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an error: %v",
			err,
		)
	}

	if !preparation.IsPrepared() {
		t.Fatal(
			"valid workflow was not prepared",
		)
	}

	expectedOperations := []string{
		preparationOperationWorkflowCreation,
		preparationOperationWorkflowTransition,
		preparationOperationNodeExecutionsCreation,
		preparationOperationWorkflowTransition,
	}

	if !reflect.DeepEqual(
		recorder.operations,
		expectedOperations,
	) {
		t.Fatalf(
			"lifecycle operations = %#v, want %#v",
			recorder.operations,
			expectedOperations,
		)
	}

	if len(recorder.workflowCreations) != 1 {
		t.Fatalf(
			"workflow creation count = %d, want 1",
			len(recorder.workflowCreations),
		)
	}

	creation :=
		recorder.workflowCreations[0]

	if creation.WorkflowExecution.Status() !=
		execution.WorkflowExecutionStatusCreated {
		t.Fatalf(
			"created workflow status = %q, want CREATED",
			creation.WorkflowExecution.Status(),
		)
	}

	if !creation.WorkflowExecution.
		CreatedAt().
		Equal(baseTime) {
		t.Fatalf(
			"workflow creation time = %v, want %v",
			creation.WorkflowExecution.CreatedAt(),
			baseTime,
		)
	}

	if len(recorder.workflowTransitions) != 2 {
		t.Fatalf(
			"workflow transition count = %d, want 2",
			len(recorder.workflowTransitions),
		)
	}

	validationTransition :=
		recorder.workflowTransitions[0]

	if validationTransition.Before.Status() !=
		execution.WorkflowExecutionStatusCreated {
		t.Fatalf(
			"validation previous status = %q, want CREATED",
			validationTransition.Before.Status(),
		)
	}

	if validationTransition.After.Status() !=
		execution.WorkflowExecutionStatusValidating {
		t.Fatalf(
			"validation target status = %q, want VALIDATING",
			validationTransition.After.Status(),
		)
	}

	if !validationTransition.
		TransitionAt.
		Equal(
			baseTime.Add(time.Minute),
		) {
		t.Fatalf(
			"validation transition time = %v",
			validationTransition.TransitionAt,
		)
	}

	if validationTransition.HasFailure {
		t.Fatal(
			"VALIDATING transition unexpectedly contains a failure",
		)
	}

	if len(recorder.nodeCreations) != 1 {
		t.Fatalf(
			"node creation count = %d, want 1",
			len(recorder.nodeCreations),
		)
	}

	nodeCreation :=
		recorder.nodeCreations[0]

	if nodeCreation.WorkflowExecution.Status() !=
		execution.WorkflowExecutionStatusValidating {
		t.Fatalf(
			"node creation workflow status = %q, want VALIDATING",
			nodeCreation.WorkflowExecution.Status(),
		)
	}

	expectedNodeIDs := []workflow.NodeID{
		workflow.NodeID("source"),
		workflow.NodeID("pass"),
		workflow.NodeID("terminal"),
	}

	if len(nodeCreation.Items) !=
		len(expectedNodeIDs) {
		t.Fatalf(
			"node creation item count = %d, want %d",
			len(nodeCreation.Items),
			len(expectedNodeIDs),
		)
	}

	for index, expectedNodeID := range expectedNodeIDs {
		item := nodeCreation.Items[index]

		if item.Definition.ID() !=
			expectedNodeID {
			t.Fatalf(
				"node item[%d] definition ID = %q, want %q",
				index,
				item.Definition.ID(),
				expectedNodeID,
			)
		}

		if item.Execution.NodeID() !=
			expectedNodeID {
			t.Fatalf(
				"node item[%d] execution node ID = %q, want %q",
				index,
				item.Execution.NodeID(),
				expectedNodeID,
			)
		}

		if item.Execution.Status() !=
			execution.NodeExecutionStatusPending {
			t.Fatalf(
				"node item[%d] status = %q, want PENDING",
				index,
				item.Execution.Status(),
			)
		}

		if !item.Execution.
			CreatedAt().
			Equal(
				baseTime.Add(
					2 * time.Minute,
				),
			) {
			t.Fatalf(
				"node item[%d] creation time = %v",
				index,
				item.Execution.CreatedAt(),
			)
		}
	}

	startTransition :=
		recorder.workflowTransitions[1]

	if startTransition.Before.Status() !=
		execution.WorkflowExecutionStatusValidating {
		t.Fatalf(
			"start previous status = %q, want VALIDATING",
			startTransition.Before.Status(),
		)
	}

	if startTransition.After.Status() !=
		execution.WorkflowExecutionStatusRunning {
		t.Fatalf(
			"start target status = %q, want RUNNING",
			startTransition.After.Status(),
		)
	}

	if !startTransition.
		TransitionAt.
		Equal(
			baseTime.Add(
				3 * time.Minute,
			),
		) {
		t.Fatalf(
			"start transition time = %v",
			startTransition.TransitionAt,
		)
	}

	if startTransition.HasFailure {
		t.Fatal(
			"RUNNING transition unexpectedly contains a failure",
		)
	}
}

func TestPrepareExecutionRecordsValidationRejection(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		0,
		0,
		0,
		0,
		time.UTC,
	)

	limits := mustValidationRuntimeLimits(
		t,
		64,
		1,
		4096,
	)

	dependencies :=
		mustValidationCoreDependencies(
			t,
			limits,
			false,
		)

	dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime,
		}

	recorder :=
		&preparationLifecycleRecorder{}

	dependencies, err :=
		dependencies.WithLifecycleRecorder(
			recorder,
		)
	if err != nil {
		t.Fatalf(
			"WithLifecycleRecorder() returned an error: %v",
			err,
		)
	}

	request :=
		mustValidationExecutionRequest(
			t,
			mustValidationLinearWorkflow(
				t,
				"source",
				"pass",
				"terminal",
			),
		)

	preparation, err := prepareExecution(
		context.Background(),
		request,
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an error: %v",
			err,
		)
	}

	if !preparation.IsRejected() {
		t.Fatal(
			"invalid workflow was not rejected",
		)
	}

	expectedOperations := []string{
		preparationOperationWorkflowCreation,
		preparationOperationWorkflowTransition,
		preparationOperationWorkflowTransition,
	}

	if !reflect.DeepEqual(
		recorder.operations,
		expectedOperations,
	) {
		t.Fatalf(
			"lifecycle operations = %#v, want %#v",
			recorder.operations,
			expectedOperations,
		)
	}

	if len(recorder.nodeCreations) != 0 {
		t.Fatalf(
			"node creation count = %d, want 0",
			len(recorder.nodeCreations),
		)
	}

	if len(recorder.workflowTransitions) != 2 {
		t.Fatalf(
			"workflow transition count = %d, want 2",
			len(recorder.workflowTransitions),
		)
	}

	rejection :=
		recorder.workflowTransitions[1]

	if rejection.Before.Status() !=
		execution.WorkflowExecutionStatusValidating {
		t.Fatalf(
			"rejection previous status = %q, want VALIDATING",
			rejection.Before.Status(),
		)
	}

	if rejection.After.Status() !=
		execution.WorkflowExecutionStatusRejected {
		t.Fatalf(
			"rejection target status = %q, want REJECTED",
			rejection.After.Status(),
		)
	}

	if !rejection.
		TransitionAt.
		Equal(
			baseTime.Add(
				2 * time.Minute,
			),
		) {
		t.Fatalf(
			"rejection transition time = %v",
			rejection.TransitionAt,
		)
	}

	if !rejection.HasFailure {
		t.Fatal(
			"REJECTED transition does not contain a structured failure",
		)
	}

	if rejection.Failure.Category() !=
		runtime.FailureCategoryValidation {
		t.Fatalf(
			"rejection failure category = %q, want VALIDATION",
			rejection.Failure.Category(),
		)
	}

	if rejection.Failure.Code() !=
		failureCodeWorkflowValidationRejected {
		t.Fatalf(
			"rejection failure code = %q, want %q",
			rejection.Failure.Code(),
			failureCodeWorkflowValidationRejected,
		)
	}

	if rejection.Failure.Message() !=
		"Workflow validation failed" {
		t.Fatalf(
			"rejection safe message = %q",
			rejection.Failure.Message(),
		)
	}

	if rejection.Failure.Retryable() {
		t.Fatal(
			"validation rejection is unexpectedly retryable",
		)
	}

	if rejection.Failure.
		Details()["issueCount"] == "" {
		t.Fatal(
			"validation rejection does not contain issueCount",
		)
	}
}

func TestPrepareExecutionStopsWhenLifecyclePersistenceFails(
	t *testing.T,
) {
	sentinel := errors.New(
		"controlled preparation persistence failure",
	)

	limits := mustValidationRuntimeLimits(
		t,
		64,
		1,
		4096,
	)

	dependencies :=
		mustValidationCoreDependencies(
			t,
			limits,
			true,
		)

	dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				18,
				1,
				0,
				0,
				0,
				time.UTC,
			),
		}

	recorder :=
		&preparationLifecycleRecorder{
			failAt:  2,
			failErr: sentinel,
		}

	dependencies, err :=
		dependencies.WithLifecycleRecorder(
			recorder,
		)
	if err != nil {
		t.Fatalf(
			"WithLifecycleRecorder() returned an error: %v",
			err,
		)
	}

	request :=
		mustValidationExecutionRequest(
			t,
			mustValidationLinearWorkflow(
				t,
				"source",
				"pass",
				"terminal",
			),
		)

	preparation, err := prepareExecution(
		context.Background(),
		request,
		dependencies,
	)

	if !errors.Is(err, sentinel) {
		t.Fatalf(
			"prepareExecution() error = %v, want sentinel error",
			err,
		)
	}

	if preparation.IsPrepared() {
		t.Fatal(
			"preparation succeeded after lifecycle persistence failure",
		)
	}

	expectedOperations := []string{
		preparationOperationWorkflowCreation,
		preparationOperationWorkflowTransition,
	}

	if !reflect.DeepEqual(
		recorder.operations,
		expectedOperations,
	) {
		t.Fatalf(
			"lifecycle operations = %#v, want %#v",
			recorder.operations,
			expectedOperations,
		)
	}

	if len(recorder.nodeCreations) != 0 {
		t.Fatalf(
			"node creation count after failure = %d, want 0",
			len(recorder.nodeCreations),
		)
	}
}

func TestPrepareExecutionBuildsRunningExecutionState(
	t *testing.T,
) {
	times := preparationTestTimes()

	clock := &preparationSequenceClock{
		times: times,
	}

	var executorCalls atomic.Int64

	limits := mustValidationRuntimeLimits(
		t,
		64,
		8,
		4096,
	)

	dependencies := mustPreparationDependencies(
		t,
		limits,
		clock,
		&executorCalls,
	)

	definition := mustValidationLinearWorkflow(
		t,
		"source",
		"pass",
		"terminal",
	)

	sourceVariables := map[string]runtime.RuntimeValue{
		" state ": mustEngineRuntimeValue(
			t,
			`{"step":"initial"}`,
		),
	}

	request, err := NewExecutionRequest(
		execution.WorkflowExecutionID(
			" workflow-execution-preparation ",
		),
		definition,
		" correlation-preparation ",
		sourceVariables,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	sourceVariables["state"] = mustEngineRuntimeValue(
		t,
		`{"step":"changed"}`,
	)

	type parentKey string

	parent := context.WithValue(
		context.Background(),
		parentKey("request"),
		"parent-value",
	)

	preparation, err := prepareExecution(
		parent,
		request,
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if !preparation.IsPrepared() {
		t.Fatal(
			"IsPrepared() = false for a valid workflow",
		)
	}

	if preparation.IsRejected() {
		t.Fatal(
			"IsRejected() = true for a valid workflow",
		)
	}

	if !preparation.ValidationReport().IsValid() {
		t.Fatalf(
			"validation report contains issues: structural=%v plugin=%v engine=%v",
			preparation.ValidationReport().StructuralIssues(),
			preparation.ValidationReport().PluginIssues(),
			preparation.ValidationReport().EngineIssues(),
		)
	}

	workflowExecution := preparation.WorkflowExecution()

	if actual := workflowExecution.Status(); actual !=
		execution.WorkflowExecutionStatusRunning {
		t.Fatalf(
			"workflow status = %q, want %q",
			actual,
			execution.WorkflowExecutionStatusRunning,
		)
	}

	if actual := workflowExecution.ID().String(); actual !=
		"workflow-execution-preparation" {
		t.Fatalf(
			"workflow execution ID = %q",
			actual,
		)
	}

	if actual := workflowExecution.CompanyID().String(); actual !=
		"company-1" {
		t.Fatalf(
			"company ID = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := workflowExecution.WorkflowID().String(); actual !=
		"workflow-linear" {
		t.Fatalf(
			"workflow ID = %q, want %q",
			actual,
			"workflow-linear",
		)
	}

	if actual := workflowExecution.WorkflowRevision(); actual != 1 {
		t.Fatalf(
			"workflow revision = %d, want 1",
			actual,
		)
	}

	if actual := workflowExecution.Mode(); actual !=
		execution.ExecutionModeSync {
		t.Fatalf(
			"execution mode = %q, want %q",
			actual,
			execution.ExecutionModeSync,
		)
	}

	if !workflowExecution.CreatedAt().Equal(times[0]) {
		t.Fatalf(
			"CreatedAt() = %v, want %v",
			workflowExecution.CreatedAt(),
			times[0],
		)
	}

	startedAt, started := workflowExecution.StartedAt()
	if !started {
		t.Fatal(
			"workflow StartedAt() does not exist",
		)
	}

	if !startedAt.Equal(times[3]) {
		t.Fatalf(
			"StartedAt() = %v, want %v",
			startedAt,
			times[3],
		)
	}

	if _, finished := workflowExecution.FinishedAt(); finished {
		t.Fatal(
			"running workflow contains FinishedAt()",
		)
	}

	prepared := preparation.prepared

	if prepared == nil {
		t.Fatal(
			"prepared execution is nil",
		)
	}

	if prepared.workflowExecution == nil {
		t.Fatal(
			"prepared workflow execution is nil",
		)
	}

	if prepared.executionContext == nil {
		t.Fatal(
			"prepared execution context is nil",
		)
	}

	expectedNodeOrder := []string{
		"source",
		"pass",
		"terminal",
	}

	actualNodeOrder := validationNodeIDStrings(
		prepared.nodeExecutionOrder,
	)

	if !reflect.DeepEqual(
		actualNodeOrder,
		expectedNodeOrder,
	) {
		t.Fatalf(
			"node execution order = %#v, want %#v",
			actualNodeOrder,
			expectedNodeOrder,
		)
	}

	if actual := len(
		prepared.nodeExecutionsByNode,
	); actual != 3 {
		t.Fatalf(
			"node execution count = %d, want 3",
			actual,
		)
	}

	for _, nodeID := range prepared.nodeExecutionOrder {
		nodeExecution, exists :=
			prepared.nodeExecutionsByNode[nodeID]

		if !exists {
			t.Fatalf(
				"node execution %q does not exist",
				nodeID,
			)
		}

		if nodeExecution == nil {
			t.Fatalf(
				"node execution %q is nil",
				nodeID,
			)
		}

		expectedExecutionID :=
			"workflow-execution-preparation/node/" +
				nodeID.String()

		if actual := nodeExecution.ID().String(); actual !=
			expectedExecutionID {
			t.Fatalf(
				"node execution ID for %q = %q, want %q",
				nodeID,
				actual,
				expectedExecutionID,
			)
		}

		if actual := nodeExecution.
			WorkflowExecutionID().
			String(); actual !=
			"workflow-execution-preparation" {
			t.Fatalf(
				"workflow execution ID for node %q = %q",
				nodeID,
				actual,
			)
		}

		if actual := nodeExecution.Status(); actual !=
			execution.NodeExecutionStatusPending {
			t.Fatalf(
				"node %q status = %q, want %q",
				nodeID,
				actual,
				execution.NodeExecutionStatusPending,
			)
		}

		if !nodeExecution.CreatedAt().Equal(times[2]) {
			t.Fatalf(
				"node %q CreatedAt() = %v, want %v",
				nodeID,
				nodeExecution.CreatedAt(),
				times[2],
			)
		}

		if _, exists := nodeExecution.StartedAt(); exists {
			t.Fatalf(
				"pending node %q contains StartedAt()",
				nodeID,
			)
		}

		if _, exists := nodeExecution.FinishedAt(); exists {
			t.Fatalf(
				"pending node %q contains FinishedAt()",
				nodeID,
			)
		}
	}

	executionContext := prepared.executionContext

	if executionContext.Context() != parent {
		t.Fatal(
			"execution context contains a different parent context",
		)
	}

	if actual := executionContext.Context().Value(
		parentKey("request"),
	); actual != "parent-value" {
		t.Fatalf(
			"parent context value = %#v, want %q",
			actual,
			"parent-value",
		)
	}

	if actual := executionContext.
		WorkflowExecutionID().
		String(); actual !=
		"workflow-execution-preparation" {
		t.Fatalf(
			"context workflow execution ID = %q",
			actual,
		)
	}

	if actual := executionContext.CompanyID().String(); actual !=
		"company-1" {
		t.Fatalf(
			"context company ID = %q",
			actual,
		)
	}

	if actual := executionContext.WorkflowID().String(); actual !=
		"workflow-linear" {
		t.Fatalf(
			"context workflow ID = %q",
			actual,
		)
	}

	if actual := executionContext.Mode(); actual !=
		execution.ExecutionModeSync {
		t.Fatalf(
			"context execution mode = %q",
			actual,
		)
	}

	if actual := executionContext.CorrelationID(); actual !=
		"correlation-preparation" {
		t.Fatalf(
			"context correlation ID = %q",
			actual,
		)
	}

	if !executionContext.StartedAt().Equal(times[3]) {
		t.Fatalf(
			"context StartedAt() = %v, want %v",
			executionContext.StartedAt(),
			times[3],
		)
	}

	variables := executionContext.VariablesSnapshot()

	if len(variables) != 1 {
		t.Fatalf(
			"variable count = %d, want 1",
			len(variables),
		)
	}

	if actual := variables["state"].String(); actual !=
		`{"step":"initial"}` {
		t.Fatalf(
			"state variable = %q, want original value",
			actual,
		)
	}

	variables["state"] = mustEngineRuntimeValue(
		t,
		`{"step":"mutated-snapshot"}`,
	)

	secondVariables :=
		executionContext.VariablesSnapshot()

	if actual := secondVariables["state"].String(); actual !=
		`{"step":"initial"}` {
		t.Fatalf(
			"context variable changed through snapshot: got %q",
			actual,
		)
	}

	assertPreparedEdges(
		t,
		executionContext.Edges(),
	)

	if actual := executorCalls.Load(); actual != 0 {
		t.Fatalf(
			"executor call count = %d, want 0",
			actual,
		)
	}

	if actual := clock.CallCount(); actual != 4 {
		t.Fatalf(
			"clock call count = %d, want 4",
			actual,
		)
	}
}

func TestPrepareExecutionRejectsInvalidWorkflowWithoutRuntimePreparation(
	t *testing.T,
) {
	times := preparationTestTimes()[:3]

	clock := &preparationSequenceClock{
		times: times,
	}

	var executorCalls atomic.Int64

	limits := mustValidationRuntimeLimits(
		t,
		64,
		8,
		4096,
	)

	dependencies := mustPreparationDependencies(
		t,
		limits,
		clock,
		&executorCalls,
	)

	definition, err := workflow.NewWorkflowDefinition(
		workflow.WorkflowID(
			"workflow-empty",
		),
		workflow.CompanyID(
			"company-1",
		),
		"Empty Workflow",
		1,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewWorkflowDefinition() returned an unexpected error: %v",
			err,
		)
	}

	request, err := NewExecutionRequest(
		execution.WorkflowExecutionID(
			"workflow-execution-rejected",
		),
		definition,
		"correlation-rejected",
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	preparation, err := prepareExecution(
		context.Background(),
		request,
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if preparation.IsPrepared() {
		t.Fatal(
			"IsPrepared() = true for an invalid workflow",
		)
	}

	if !preparation.IsRejected() {
		t.Fatal(
			"IsRejected() = false for an invalid workflow",
		)
	}

	if preparation.prepared != nil {
		t.Fatal(
			"rejected preparation contains runtime state",
		)
	}

	report := preparation.ValidationReport()

	if report.IsValid() {
		t.Fatal(
			"rejected workflow contains a valid validation report",
		)
	}

	if len(report.StructuralIssues()) == 0 {
		t.Fatal(
			"rejected workflow contains no structural issues",
		)
	}

	if report.PluginIssues() != nil {
		t.Fatalf(
			"plugin issues = %#v, want nil",
			report.PluginIssues(),
		)
	}

	if report.EngineIssues() != nil {
		t.Fatalf(
			"engine issues = %#v, want nil",
			report.EngineIssues(),
		)
	}

	workflowExecution := preparation.WorkflowExecution()

	if actual := workflowExecution.Status(); actual !=
		execution.WorkflowExecutionStatusRejected {
		t.Fatalf(
			"workflow status = %q, want %q",
			actual,
			execution.WorkflowExecutionStatusRejected,
		)
	}

	if !workflowExecution.CreatedAt().Equal(times[0]) {
		t.Fatalf(
			"CreatedAt() = %v, want %v",
			workflowExecution.CreatedAt(),
			times[0],
		)
	}

	if _, started := workflowExecution.StartedAt(); started {
		t.Fatal(
			"rejected workflow contains StartedAt()",
		)
	}

	finishedAt, finished := workflowExecution.FinishedAt()
	if !finished {
		t.Fatal(
			"rejected workflow contains no FinishedAt()",
		)
	}

	if !finishedAt.Equal(times[2]) {
		t.Fatalf(
			"FinishedAt() = %v, want %v",
			finishedAt,
			times[2],
		)
	}

	if actual := executorCalls.Load(); actual != 0 {
		t.Fatalf(
			"executor call count = %d, want 0",
			actual,
		)
	}

	if actual := clock.CallCount(); actual != 3 {
		t.Fatalf(
			"clock call count = %d, want 3",
			actual,
		)
	}
}

func TestPreparedExecutionPropagatesParentCancellation(
	t *testing.T,
) {
	times := preparationTestTimes()

	clock := &preparationSequenceClock{
		times: times,
	}

	var executorCalls atomic.Int64

	dependencies := mustPreparationDependencies(
		t,
		mustValidationRuntimeLimits(
			t,
			64,
			8,
			4096,
		),
		clock,
		&executorCalls,
	)

	parent, cancel := context.WithCancel(
		context.Background(),
	)

	preparation, err := prepareExecution(
		parent,
		mustValidationExecutionRequest(
			t,
			mustValidationLinearWorkflow(
				t,
				"source",
				"pass",
				"terminal",
			),
		),
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if preparation.prepared == nil ||
		preparation.prepared.executionContext == nil {
		t.Fatal(
			"execution context was not prepared",
		)
	}

	cancel()

	executionContext :=
		preparation.prepared.executionContext

	if !errors.Is(
		executionContext.Err(),
		context.Canceled,
	) {
		t.Fatalf(
			"execution context error = %v, want context.Canceled",
			executionContext.Err(),
		)
	}

	select {
	case <-executionContext.Done():
	default:
		t.Fatal(
			"execution context Done() was not closed",
		)
	}

	if actual := executorCalls.Load(); actual != 0 {
		t.Fatalf(
			"executor call count = %d, want 0",
			actual,
		)
	}
}

func TestPrepareExecutionRejectsInvalidContracts(
	t *testing.T,
) {
	validRequest := mustValidationExecutionRequest(
		t,
		mustValidationLinearWorkflow(
			t,
			"source",
			"pass",
			"terminal",
		),
	)

	t.Run("nil context", func(t *testing.T) {
		clock := &preparationSequenceClock{
			times: preparationTestTimes(),
		}

		var executorCalls atomic.Int64

		dependencies := mustPreparationDependencies(
			t,
			mustValidationRuntimeLimits(
				t,
				64,
				8,
				4096,
			),
			clock,
			&executorCalls,
		)

		_, err := prepareExecution(
			nil,
			validRequest,
			dependencies,
		)

		requireEngineValidationField(
			t,
			err,
			"context",
		)

		if clock.CallCount() != 0 {
			t.Fatal(
				"clock was called for a nil context",
			)
		}
	})

	t.Run("invalid request", func(t *testing.T) {
		clock := &preparationSequenceClock{
			times: preparationTestTimes(),
		}

		var executorCalls atomic.Int64

		dependencies := mustPreparationDependencies(
			t,
			mustValidationRuntimeLimits(
				t,
				64,
				8,
				4096,
			),
			clock,
			&executorCalls,
		)

		_, err := prepareExecution(
			context.Background(),
			ExecutionRequest{},
			dependencies,
		)

		requireEngineValidationField(
			t,
			err,
			"request",
		)

		if clock.CallCount() != 0 {
			t.Fatal(
				"clock was called for an invalid request",
			)
		}
	})

	t.Run("invalid dependencies", func(t *testing.T) {
		_, err := prepareExecution(
			context.Background(),
			validRequest,
			EngineDependencies{},
		)

		requireEngineValidationField(
			t,
			err,
			"dependencies",
		)
	})
}

func TestPrepareExecutionRejectsZeroClockValues(
	t *testing.T,
) {
	request := mustValidationExecutionRequest(
		t,
		mustValidationLinearWorkflow(
			t,
			"source",
			"pass",
			"terminal",
		),
	)

	t.Run("workflow creation time", func(t *testing.T) {
		clock := &preparationSequenceClock{
			times: []time.Time{
				{},
			},
		}

		var executorCalls atomic.Int64

		dependencies := mustPreparationDependencies(
			t,
			mustValidationRuntimeLimits(
				t,
				64,
				8,
				4096,
			),
			clock,
			&executorCalls,
		)

		_, err := prepareExecution(
			context.Background(),
			request,
			dependencies,
		)

		requireEngineValidationField(
			t,
			err,
			"workflowExecution.createdAt",
		)
	})

	t.Run("node execution creation time", func(t *testing.T) {
		times := preparationTestTimes()

		clock := &preparationSequenceClock{
			times: []time.Time{
				times[0],
				times[1],
				{},
			},
		}

		var executorCalls atomic.Int64

		dependencies := mustPreparationDependencies(
			t,
			mustValidationRuntimeLimits(
				t,
				64,
				8,
				4096,
			),
			clock,
			&executorCalls,
		)

		_, err := prepareExecution(
			context.Background(),
			request,
			dependencies,
		)

		requireEngineValidationField(
			t,
			err,
			"nodeExecutions.createdAt",
		)

		if actual := executorCalls.Load(); actual != 0 {
			t.Fatalf(
				"executor call count = %d, want 0",
				actual,
			)
		}
	})
}

func TestExecutionTimeNormalizesUTCAndRejectsInvalidClock(
	t *testing.T,
) {
	t.Run("normalizes UTC", func(t *testing.T) {
		location := time.FixedZone(
			"test-zone",
			3*60*60,
		)

		localTime := time.Date(
			2026,
			time.July,
			17,
			15,
			0,
			0,
			0,
			location,
		)

		actual, err := executionTime(
			sharedclock.From(
				func() time.Time {
					return localTime
				},
			),
			"executionTime",
		)
		if err != nil {
			t.Fatalf(
				"executionTime() returned an unexpected error: %v",
				err,
			)
		}

		if actual.Location() != time.UTC {
			t.Fatalf(
				"execution time location = %v, want UTC",
				actual.Location(),
			)
		}

		if !actual.Equal(localTime) {
			t.Fatalf(
				"execution time = %v, want instant %v",
				actual,
				localTime,
			)
		}
	})

	t.Run("nil clock", func(t *testing.T) {
		var clock sharedclock.Clock

		_, err := executionTime(
			clock,
			"executionTime",
		)

		requireEngineValidationField(
			t,
			err,
			"clock",
		)
	})

	t.Run("typed nil clock", func(t *testing.T) {
		var clock sharedclock.Clock

		_, err := executionTime(
			clock,
			"executionTime",
		)

		requireEngineValidationField(
			t,
			err,
			"clock",
		)
	})

	t.Run("zero time", func(t *testing.T) {
		_, err := executionTime(
			sharedclock.From(
				func() time.Time {
					return time.Time{}
				},
			),
			"executionTime",
		)

		requireEngineValidationField(
			t,
			err,
			"executionTime",
		)
	})
}

func assertPreparedEdges(
	t *testing.T,
	edges []*runtime.EdgeRuntime,
) {
	t.Helper()

	if len(edges) != 2 {
		t.Fatalf(
			"prepared edge count = %d, want 2",
			len(edges),
		)
	}

	expected := []struct {
		id         string
		sourceNode string
		sourcePort string
		targetNode string
		targetPort string
	}{
		{
			id:         "edge-pass-terminal",
			sourceNode: "pass",
			sourcePort: core.OutputPortName,
			targetNode: "terminal",
			targetPort: core.InputPortName,
		},
		{
			id:         "edge-source-pass",
			sourceNode: "source",
			sourcePort: core.OutputPortName,
			targetNode: "pass",
			targetPort: core.InputPortName,
		},
	}

	for index, expectation := range expected {
		edge := edges[index]

		if edge == nil {
			t.Fatalf(
				"edge %d is nil",
				index,
			)
		}

		if actual := edge.ID().String(); actual !=
			expectation.id {
			t.Fatalf(
				"edge %d ID = %q, want %q",
				index,
				actual,
				expectation.id,
			)
		}

		if actual := edge.SourceNodeID().String(); actual !=
			expectation.sourceNode {
			t.Fatalf(
				"edge %q source node = %q, want %q",
				expectation.id,
				actual,
				expectation.sourceNode,
			)
		}

		if actual := edge.SourceOutputPort(); actual !=
			expectation.sourcePort {
			t.Fatalf(
				"edge %q source port = %q, want %q",
				expectation.id,
				actual,
				expectation.sourcePort,
			)
		}

		if actual := edge.TargetNodeID().String(); actual !=
			expectation.targetNode {
			t.Fatalf(
				"edge %q target node = %q, want %q",
				expectation.id,
				actual,
				expectation.targetNode,
			)
		}

		if actual := edge.TargetInputPort(); actual !=
			expectation.targetPort {
			t.Fatalf(
				"edge %q target port = %q, want %q",
				expectation.id,
				actual,
				expectation.targetPort,
			)
		}

		if edge.Queue() == nil {
			t.Fatalf(
				"edge %q queue is nil",
				expectation.id,
			)
		}

		if edge.Cache() == nil {
			t.Fatalf(
				"edge %q cache is nil",
				expectation.id,
			)
		}

		if actual := edge.Queue().Len(); actual != 0 {
			t.Fatalf(
				"edge %q queue length = %d, want 0",
				expectation.id,
				actual,
			)
		}

		if actual := edge.Cache().Len(); actual != 0 {
			t.Fatalf(
				"edge %q cache length = %d, want 0",
				expectation.id,
				actual,
			)
		}
	}
}

func mustPreparationDependencies(
	t *testing.T,
	limits runtime.RuntimeLimits,
	clock sharedclock.Clock,
	executorCalls *atomic.Int64,
) EngineDependencies {
	t.Helper()

	descriptors, err := core.CoreDescriptors()
	if err != nil {
		t.Fatalf(
			"core.CoreDescriptors() returned an unexpected error: %v",
			err,
		)
	}

	pluginRegistry, err := plugin.NewRegistry(
		descriptors,
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls.Add(1)

			return runtime.NewNodeSuccessResult(
				nil,
				runtime.ContextChanges{},
			)
		},
	)

	registrations := make(
		[]runtime.ExecutorRegistration,
		0,
		len(descriptors),
	)

	for _, descriptor := range descriptors {
		registration, err :=
			runtime.NewExecutorRegistration(
				descriptor.Identity(),
				executor,
			)
		if err != nil {
			t.Fatalf(
				"runtime.NewExecutorRegistration() returned an unexpected error: %v",
				err,
			)
		}

		registrations = append(
			registrations,
			registration,
		)
	}

	executorRegistry, err :=
		runtime.NewExecutorRegistry(
			registrations,
		)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		clock,
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	return dependencies
}

func preparationTestTimes() []time.Time {
	base := time.Date(
		2026,
		time.July,
		17,
		13,
		0,
		0,
		0,
		time.UTC,
	)

	return []time.Time{
		base,
		base.Add(time.Minute),
		base.Add(2 * time.Minute),
		base.Add(3 * time.Minute),
	}
}

type preparationSequenceClock struct {
	times []time.Time
	index int
}

func (clock *preparationSequenceClock) IsValid() bool {
	return clock != nil
}

func (clock *preparationSequenceClock) Now() time.Time {
	if clock == nil ||
		clock.index >= len(clock.times) {
		return time.Time{}
	}

	current := clock.times[clock.index]
	clock.index++

	return current
}

func (clock *preparationSequenceClock) CallCount() int {
	if clock == nil {
		return 0
	}

	return clock.index
}

func TestAssessNodeReadinessHandlesRootWaitingReadyAndIneligible(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	rootAssessment, err := assessNodeReadiness(
		prepared,
		workflow.NodeID("source"),
	)
	if err != nil {
		t.Fatalf(
			"assessNodeReadiness(source) returned an unexpected error: %v",
			err,
		)
	}

	if !rootAssessment.IsReady() {
		t.Fatalf(
			"root readiness state = %q, want READY",
			rootAssessment.state,
		)
	}

	waitingAssessment, err := assessNodeReadiness(
		prepared,
		workflow.NodeID("pass"),
	)
	if err != nil {
		t.Fatalf(
			"assessNodeReadiness(pass) returned an unexpected error: %v",
			err,
		)
	}

	if !waitingAssessment.IsWaiting() {
		t.Fatalf(
			"pass readiness state = %q, want WAITING",
			waitingAssessment.state,
		)
	}

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	waitingWithoutPayload, err := assessNodeReadiness(
		prepared,
		workflow.NodeID("pass"),
	)
	if err != nil {
		t.Fatalf(
			"assessNodeReadiness(pass) without payload returned an unexpected error: %v",
			err,
		)
	}

	if !waitingWithoutPayload.IsWaiting() {
		t.Fatalf(
			"pass readiness state without payload = %q, want WAITING",
			waitingWithoutPayload.state,
		)
	}

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-source-pass",
		"payload",
	)

	readyAssessment, err := assessNodeReadiness(
		prepared,
		workflow.NodeID("pass"),
	)
	if err != nil {
		t.Fatalf(
			"assessNodeReadiness(pass) returned an unexpected error: %v",
			err,
		)
	}

	if !readyAssessment.IsReady() {
		t.Fatalf(
			"pass readiness state = %q, want READY",
			readyAssessment.state,
		)
	}

	passExecution := prepared.nodeExecutionsByNode[workflow.NodeID("pass")]

	if passExecution == nil {
		t.Fatal(
			"pass node execution is nil",
		)
	}

	if err := passExecution.MarkReady(
		schedulerTransitionTime(0),
	); err != nil {
		t.Fatalf(
			"pass MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	ineligibleAssessment, err := assessNodeReadiness(
		prepared,
		workflow.NodeID("pass"),
	)
	if err != nil {
		t.Fatalf(
			"assessNodeReadiness(ready pass) returned an unexpected error: %v",
			err,
		)
	}

	if !ineligibleAssessment.IsIneligible() {
		t.Fatalf(
			"ready node assessment state = %q, want INELIGIBLE",
			ineligibleAssessment.state,
		)
	}
}

func TestAssessNodeReadinessReportsBlockedUpstreamStates(
	t *testing.T,
) {
	tests := []struct {
		name   string
		status execution.NodeExecutionStatus
	}{
		{
			name:   "failed",
			status: execution.NodeExecutionStatusFailed,
		},
		{
			name:   "skipped",
			status: execution.NodeExecutionStatusSkipped,
		},
		{
			name:   "cancelled",
			status: execution.NodeExecutionStatusCancelled,
		},
		{
			name:   "timed out",
			status: execution.NodeExecutionStatusTimedOut,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prepared := mustSchedulerLinearPreparedExecution(t)

			transitionSchedulerNode(
				t,
				prepared,
				workflow.NodeID("source"),
				test.status,
			)

			assessment, err := assessNodeReadiness(
				prepared,
				workflow.NodeID("pass"),
			)
			if err != nil {
				t.Fatalf(
					"assessNodeReadiness() returned an unexpected error: %v",
					err,
				)
			}

			if !assessment.IsBlocked() {
				t.Fatalf(
					"readiness state = %q, want BLOCKED",
					assessment.state,
				)
			}

			actualBlockingNodes := assessment.BlockingUpstreamNodes()

			expectedBlockingNodes := []workflow.NodeID{
				workflow.NodeID("source"),
			}

			if !reflect.DeepEqual(
				actualBlockingNodes,
				expectedBlockingNodes,
			) {
				t.Fatalf(
					"blocking nodes = %#v, want %#v",
					actualBlockingNodes,
					expectedBlockingNodes,
				)
			}

			actualBlockingNodes[0] = workflow.NodeID("changed")

			secondSnapshot := assessment.BlockingUpstreamNodes()

			if secondSnapshot[0].String() != "source" {
				t.Fatalf(
					"blocking node changed through returned slice: got %q",
					secondSnapshot[0],
				)
			}
		})
	}
}

func TestAssessNodeReadinessRejectsQueueAmbiguity(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-source-pass",
		"first",
	)

	pushSchedulerEdgePayload(
		t,
		prepared,
		"edge-source-pass",
		"second",
	)

	_, err := assessNodeReadiness(
		prepared,
		workflow.NodeID("pass"),
	)
	if err == nil {
		t.Fatal(
			"assessNodeReadiness() returned nil error for an ambiguous queue",
		)
	}

	if !strings.Contains(
		err.Error(),
		"contains 2 payloads",
	) {
		t.Fatalf(
			"error = %q, want queue ambiguity information",
			err,
		)
	}

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 2 {
		t.Fatalf(
			"queue length after readiness failure = %d, want 2",
			actual,
		)
	}
}

func TestNextReadyNodeUsesTopologicalPriority(
	t *testing.T,
) {
	prepared := mustSchedulerFanInPreparedExecution(t)

	firstNodeID, exists, err := nextReadyNode(prepared)
	if err != nil {
		t.Fatalf(
			"nextReadyNode() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"nextReadyNode() exists = false",
		)
	}

	if actual := firstNodeID.String(); actual != "source-a" {
		t.Fatalf(
			"first ready node = %q, want %q",
			actual,
			"source-a",
		)
	}

	firstExecution := prepared.nodeExecutionsByNode[firstNodeID]

	if firstExecution == nil {
		t.Fatal(
			"first node execution is nil",
		)
	}

	if err := firstExecution.MarkReady(
		schedulerTransitionTime(0),
	); err != nil {
		t.Fatalf(
			"first node MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	secondNodeID, exists, err := nextReadyNode(prepared)
	if err != nil {
		t.Fatalf(
			"second nextReadyNode() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"second nextReadyNode() exists = false",
		)
	}

	if actual := secondNodeID.String(); actual != "source-b" {
		t.Fatalf(
			"second ready node = %q, want %q",
			actual,
			"source-b",
		)
	}
}

func TestAssessNodeReadinessRejectsInvalidPreparedState(
	t *testing.T,
) {
	t.Run("nil prepared execution", func(t *testing.T) {
		_, err := assessNodeReadiness(
			nil,
			workflow.NodeID("source"),
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution",
		)
	})

	t.Run("missing execution context", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		prepared.executionContext = nil

		_, err := assessNodeReadiness(
			prepared,
			workflow.NodeID("source"),
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.executionContext",
		)
	})

	t.Run("unknown node", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		_, err := assessNodeReadiness(
			prepared,
			workflow.NodeID("unknown"),
		)

		requireEngineValidationField(
			t,
			err,
			"nodeID",
		)
	})

	t.Run("missing node execution", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		delete(
			prepared.nodeExecutionsByNode,
			workflow.NodeID("source"),
		)

		_, err := assessNodeReadiness(
			prepared,
			workflow.NodeID("source"),
		)

		if err == nil {
			t.Fatal(
				"assessNodeReadiness() returned nil error for a missing node execution",
			)
		}

		if !strings.Contains(
			err.Error(),
			"node execution",
		) {
			t.Fatalf(
				"error = %q, want node-execution information",
				err,
			)
		}
	})
}

func TestNextReadyNodeRejectsNilPreparedExecution(
	t *testing.T,
) {
	_, _, err := nextReadyNode(nil)

	requireEngineValidationField(
		t,
		err,
		"preparedExecution",
	)
}

func mustSchedulerLinearPreparedExecution(
	t *testing.T,
) *preparedExecution {
	t.Helper()

	clock := &preparationSequenceClock{
		times: preparationTestTimes(),
	}

	var executorCalls atomic.Int64

	dependencies := mustPreparationDependencies(
		t,
		mustValidationRuntimeLimits(
			t,
			64,
			8,
			4096,
		),
		clock,
		&executorCalls,
	)

	request := mustValidationExecutionRequest(
		t,
		mustValidationLinearWorkflow(
			t,
			"source",
			"pass",
			"terminal",
		),
	)

	preparation, err := prepareExecution(
		context.Background(),
		request,
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if preparation.prepared == nil {
		t.Fatal(
			"prepareExecution() returned no prepared execution",
		)
	}

	return preparation.prepared
}

func mustSchedulerFanInPreparedExecution(
	t *testing.T,
) *preparedExecution {
	t.Helper()

	const sourcePluginType workflow.PluginType = "test.scheduler-source"

	const targetPluginType workflow.PluginType = "test.scheduler-target"

	const version workflow.PluginVersion = "v1"

	sourceDescriptor := mustValidationDescriptor(
		t,
		sourcePluginType,
		version,
		plugin.DistributionDistributable,
		nil,
		[]string{"output"},
		plugin.NewExactEdgeConstraint(0),
		plugin.NewUnlimitedEdgeConstraint(1),
	)

	targetDescriptor := mustValidationDescriptor(
		t,
		targetPluginType,
		version,
		plugin.DistributionDistributable,
		[]string{"input"},
		nil,
		plugin.NewExactEdgeConstraint(2),
		plugin.NewExactEdgeConstraint(0),
	)

	pluginRegistry, err := plugin.NewRegistry(
		[]plugin.Descriptor{
			targetDescriptor,
			sourceDescriptor,
		},
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			return runtime.NewNodeSuccessResult(
				nil,
				runtime.ContextChanges{},
			)
		},
	)

	executorRegistry, err := runtime.NewExecutorRegistry(
		[]runtime.ExecutorRegistration{
			mustValidationExecutorRegistration(
				t,
				sourceDescriptor.Identity(),
				executor,
			),
			mustValidationExecutorRegistration(
				t,
				targetDescriptor.Identity(),
				executor,
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	clock := &preparationSequenceClock{
		times: preparationTestTimes(),
	}

	limits := mustValidationRuntimeLimits(
		t,
		8,
		8,
		4096,
	)

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		clock,
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	sourceA := mustValidationNode(
		t,
		"source-a",
		sourcePluginType.String(),
		version.String(),
		`{}`,
	)

	sourceB := mustValidationNode(
		t,
		"source-b",
		sourcePluginType.String(),
		version.String(),
		`{}`,
	)

	target := mustValidationNode(
		t,
		"target",
		targetPluginType.String(),
		version.String(),
		`{}`,
	)

	edgeA := mustValidationEdge(
		t,
		"edge-a",
		"source-a",
		"output",
		"target",
		"input",
	)

	edgeB := mustValidationEdge(
		t,
		"edge-b",
		"source-b",
		"output",
		"target",
		"input",
	)

	definition := mustValidationDefinition(
		t,
		"workflow-scheduler-fan-in",
		[]workflow.NodeDefinition{
			target,
			sourceB,
			sourceA,
		},
		[]workflow.EdgeDefinition{
			edgeB,
			edgeA,
		},
	)

	preparation, err := prepareExecution(
		context.Background(),
		mustValidationExecutionRequest(
			t,
			definition,
		),
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if preparation.prepared == nil {
		t.Fatal(
			"fan-in workflow was not prepared",
		)
	}

	return preparation.prepared
}

func transitionSchedulerNode(
	t *testing.T,
	prepared *preparedExecution,
	nodeID workflow.NodeID,
	target execution.NodeExecutionStatus,
) {
	t.Helper()

	nodeExecution, exists := prepared.nodeExecutionsByNode[nodeID]

	if !exists || nodeExecution == nil {
		t.Fatalf(
			"node execution %q is unavailable",
			nodeID,
		)
	}

	firstAt := schedulerTransitionTime(0)
	secondAt := schedulerTransitionTime(1)
	thirdAt := schedulerTransitionTime(2)

	var err error

	switch target {
	case execution.NodeExecutionStatusPending:
		return

	case execution.NodeExecutionStatusReady:
		err = nodeExecution.MarkReady(firstAt)

	case execution.NodeExecutionStatusRunning:
		if err = nodeExecution.MarkReady(firstAt); err == nil {
			err = nodeExecution.Start(secondAt)
		}

	case execution.NodeExecutionStatusSucceeded:
		if err = nodeExecution.MarkReady(firstAt); err == nil {
			err = nodeExecution.Start(secondAt)
		}

		if err == nil {
			err = nodeExecution.Succeed(thirdAt)
		}

	case execution.NodeExecutionStatusFailed:
		if err = nodeExecution.MarkReady(firstAt); err == nil {
			err = nodeExecution.Start(secondAt)
		}

		if err == nil {
			err = nodeExecution.Fail(thirdAt)
		}

	case execution.NodeExecutionStatusSkipped:
		err = nodeExecution.Skip(firstAt)

	case execution.NodeExecutionStatusCancelled:
		err = nodeExecution.Cancel(firstAt)

	case execution.NodeExecutionStatusTimedOut:
		if err = nodeExecution.MarkReady(firstAt); err == nil {
			err = nodeExecution.Timeout(secondAt)
		}

	default:
		t.Fatalf(
			"unsupported scheduler test status %q",
			target,
		)
	}

	if err != nil {
		t.Fatalf(
			"transition node %q to %q returned an error: %v",
			nodeID,
			target,
			err,
		)
	}
}

func pushSchedulerEdgePayload(
	t *testing.T,
	prepared *preparedExecution,
	edgeID string,
	value string,
) {
	t.Helper()

	queue := schedulerEdgeQueue(
		t,
		prepared,
		edgeID,
	)

	payload, err := runtime.NewInlinePayload(
		runtime.ContentTypeTextPlain,
		[]byte(value),
		map[string]string{
			"edge": edgeID,
		},
		4096,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	if _, _, err := queue.Push(payload); err != nil {
		t.Fatalf(
			"queue.Push() returned an unexpected error: %v",
			err,
		)
	}
}

func schedulerEdgeQueue(
	t *testing.T,
	prepared *preparedExecution,
	edgeID string,
) *runtime.EdgeQueue {
	t.Helper()

	if prepared == nil ||
		prepared.executionContext == nil {
		t.Fatal(
			"prepared execution context is unavailable",
		)
	}

	edgeRuntime, exists, err := prepared.executionContext.Edge(
		workflow.EdgeID(edgeID),
	)
	if err != nil {
		t.Fatalf(
			"executionContext.Edge(%q) returned an unexpected error: %v",
			edgeID,
			err,
		)
	}

	if !exists || edgeRuntime == nil {
		t.Fatalf(
			"runtime edge %q is unavailable",
			edgeID,
		)
	}

	if edgeRuntime.Queue() == nil {
		t.Fatalf(
			"runtime edge %q queue is nil",
			edgeID,
		)
	}

	return edgeRuntime.Queue()
}

func schedulerTransitionTime(
	offset int,
) time.Time {
	return time.Date(
		2026,
		time.July,
		17,
		15,
		offset,
		0,
		0,
		time.UTC,
	)
}

func TestRouteNodeOutputsRoutesSingleEdge(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	result := mustRoutingSuccessResult(
		t,
		map[string][]runtime.Payload{
			"output": {
				mustRoutingPayload(
					t,
					"single-output",
					map[string]string{
						"source": "source",
					},
				),
			},
		},
	)

	err := routeNodeOutputs(
		prepared,
		workflow.NodeID("source"),
		result,
	)
	if err != nil {
		t.Fatalf(
			"routeNodeOutputs() returned an unexpected error: %v",
			err,
		)
	}

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 1 {
		t.Fatalf(
			"routed queue length = %d, want 1",
			actual,
		)
	}

	payload, exists := queue.Peek()
	if !exists {
		t.Fatal(
			"routed queue contains no payload",
		)
	}

	actualPayload := preparedInputPayloadText(
		t,
		payload,
	)

	if actualPayload != "single-output" {
		t.Fatalf(
			"routed payload = %q, want %q",
			actualPayload,
			"single-output",
		)
	}

	actualSource := payload.Metadata()["source"]

	if actualSource != "source" {
		t.Fatalf(
			"routed metadata source = %q, want %q",
			actualSource,
			"source",
		)
	}
}

func TestBuildOutputRoutingOperationsUsesStableFanOutOrder(
	t *testing.T,
) {
	prepared := mustRoutingFanOutPreparedExecution(t)

	result := mustRoutingSuccessResult(
		t,
		map[string][]runtime.Payload{
			"output": {
				mustRoutingPayload(
					t,
					"fan-out",
					nil,
				),
			},
		},
	)

	operations, err := buildOutputRoutingOperations(
		prepared,
		workflow.NodeID("source"),
		result,
	)
	if err != nil {
		t.Fatalf(
			"buildOutputRoutingOperations() returned an unexpected error: %v",
			err,
		)
	}

	if len(operations) != 2 {
		t.Fatalf(
			"routing operation count = %d, want 2",
			len(operations),
		)
	}

	actualEdgeIDs := []string{
		operations[0].edgeID.String(),
		operations[1].edgeID.String(),
	}

	expectedEdgeIDs := []string{
		"edge-a",
		"edge-b",
	}

	if !reflect.DeepEqual(
		actualEdgeIDs,
		expectedEdgeIDs,
	) {
		t.Fatalf(
			"routing operation order = %#v, want %#v",
			actualEdgeIDs,
			expectedEdgeIDs,
		)
	}

	for _, operation := range operations {
		if operation.sourcePort != "output" {
			t.Fatalf(
				"operation source port = %q, want %q",
				operation.sourcePort,
				"output",
			)
		}

		if operation.edgeRuntime == nil {
			t.Fatalf(
				"operation for edge %q has nil runtime edge",
				operation.edgeID,
			)
		}
	}

	edgeAQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-a",
	)

	if edgeAQueue.Len() != 0 {
		t.Fatal(
			"building routing operations mutated edge-a queue",
		)
	}

	edgeBQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-b",
	)

	if edgeBQueue.Len() != 0 {
		t.Fatal(
			"building routing operations mutated edge-b queue",
		)
	}
}

func TestRouteNodeOutputsFansOutToEveryMatchingEdge(
	t *testing.T,
) {
	prepared := mustRoutingFanOutPreparedExecution(t)

	result := mustRoutingSuccessResult(
		t,
		map[string][]runtime.Payload{
			"output": {
				mustRoutingPayload(
					t,
					"fan-out-value",
					map[string]string{
						"route": "fan-out",
					},
				),
			},
		},
	)

	err := routeNodeOutputs(
		prepared,
		workflow.NodeID("source"),
		result,
	)
	if err != nil {
		t.Fatalf(
			"routeNodeOutputs() returned an unexpected error: %v",
			err,
		)
	}

	queueA := schedulerEdgeQueue(
		t,
		prepared,
		"edge-a",
	)

	queueB := schedulerEdgeQueue(
		t,
		prepared,
		"edge-b",
	)

	if queueA.Len() != 1 {
		t.Fatalf(
			"edge-a queue length = %d, want 1",
			queueA.Len(),
		)
	}

	if queueB.Len() != 1 {
		t.Fatalf(
			"edge-b queue length = %d, want 1",
			queueB.Len(),
		)
	}

	payloadA, exists := queueA.Pop()
	if !exists {
		t.Fatal(
			"edge-a queue contains no payload",
		)
	}

	payloadB, exists := queueB.Pop()
	if !exists {
		t.Fatal(
			"edge-b queue contains no payload",
		)
	}

	actualPayloadA := preparedInputPayloadText(
		t,
		payloadA,
	)

	if actualPayloadA != "fan-out-value" {
		t.Fatalf(
			"edge-a payload = %q, want %q",
			actualPayloadA,
			"fan-out-value",
		)
	}

	actualPayloadB := preparedInputPayloadText(
		t,
		payloadB,
	)

	if actualPayloadB != "fan-out-value" {
		t.Fatalf(
			"edge-b payload = %q, want %q",
			actualPayloadB,
			"fan-out-value",
		)
	}

	exposedData, exists := payloadA.InlineData()
	if !exists {
		t.Fatal(
			"edge-a payload contains no inline data",
		)
	}

	exposedData[0] = 'X'

	exposedMetadata := payloadA.Metadata()
	exposedMetadata["route"] = "changed"

	actualPayloadBAfterMutation := preparedInputPayloadText(
		t,
		payloadB,
	)

	if actualPayloadBAfterMutation != "fan-out-value" {
		t.Fatalf(
			"edge-b payload changed through edge-a accessor: got %q",
			actualPayloadBAfterMutation,
		)
	}

	actualRouteB := payloadB.Metadata()["route"]

	if actualRouteB != "fan-out" {
		t.Fatalf(
			"edge-b metadata changed through edge-a accessor: got %q",
			actualRouteB,
		)
	}
}

func TestRouteNodeOutputsAllowsDeclaredPortWithoutMatchingEdge(
	t *testing.T,
) {
	prepared := mustRoutingFanOutPreparedExecution(t)

	result := mustRoutingSuccessResult(
		t,
		map[string][]runtime.Payload{
			"optional": {
				mustRoutingPayload(
					t,
					"unused-value",
					nil,
				),
			},
		},
	)

	operations, err := buildOutputRoutingOperations(
		prepared,
		workflow.NodeID("source"),
		result,
	)
	if err != nil {
		t.Fatalf(
			"buildOutputRoutingOperations() returned an unexpected error: %v",
			err,
		)
	}

	if actual := len(operations); actual != 0 {
		t.Fatalf(
			"routing operation count = %d, want 0",
			actual,
		)
	}

	err = routeNodeOutputs(
		prepared,
		workflow.NodeID("source"),
		result,
	)
	if err != nil {
		t.Fatalf(
			"routeNodeOutputs() returned an unexpected error: %v",
			err,
		)
	}

	edgeAQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-a",
	)

	if edgeAQueue.Len() != 0 {
		t.Fatal(
			"optional output mutated edge-a queue",
		)
	}

	edgeBQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-b",
	)

	if edgeBQueue.Len() != 0 {
		t.Fatal(
			"optional output mutated edge-b queue",
		)
	}
}

func TestRouteNodeOutputsTreatsTerminalOutputAsNoOp(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	result := mustRoutingTerminalResult(
		t,
		mustRoutingPayload(
			t,
			"terminal-value",
			nil,
		),
	)

	err := routeNodeOutputs(
		prepared,
		workflow.NodeID("terminal"),
		result,
	)
	if err != nil {
		t.Fatalf(
			"routeNodeOutputs(terminal) returned an unexpected error: %v",
			err,
		)
	}

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-pass-terminal",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"incoming terminal queue length = %d, want 0",
			actual,
		)
	}
}

func TestRouteNodeOutputsRejectsUnknownPortWithoutMutation(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	result := mustRoutingSuccessResult(
		t,
		map[string][]runtime.Payload{
			"unknown": {
				mustRoutingPayload(
					t,
					"invalid-output",
					nil,
				),
			},
		},
	)

	err := routeNodeOutputs(
		prepared,
		workflow.NodeID("source"),
		result,
	)
	if err == nil {
		t.Fatal(
			"routeNodeOutputs() returned nil error for an undeclared port",
		)
	}

	if !strings.Contains(
		err.Error(),
		"undeclared output port",
	) {
		t.Fatalf(
			"error = %q, want undeclared-port information",
			err,
		)
	}

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after rejected routing = %d, want 0",
			actual,
		)
	}
}

func TestRouteNodeOutputsRejectsMultiplePayloadsWithoutMutation(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	result := mustRoutingSuccessResult(
		t,
		map[string][]runtime.Payload{
			"output": {
				mustRoutingPayload(
					t,
					"first",
					nil,
				),
				mustRoutingPayload(
					t,
					"second",
					nil,
				),
			},
		},
	)

	err := routeNodeOutputs(
		prepared,
		workflow.NodeID("source"),
		result,
	)
	if err == nil {
		t.Fatal(
			"routeNodeOutputs() returned nil error for multiple output payloads",
		)
	}

	if !strings.Contains(
		err.Error(),
		"contains 2 payloads",
	) {
		t.Fatalf(
			"error = %q, want single-shot payload information",
			err,
		)
	}

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after rejected routing = %d, want 0",
			actual,
		)
	}
}

func TestRouteNodeOutputsRejectsFailureResult(
	t *testing.T,
) {
	prepared := mustSchedulerLinearPreparedExecution(t)

	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryExecution,
		"TEST_EXECUTION_FAILED",
		"Test execution failed",
		false,
		map[string]string{
			"source": "routing-test",
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewRuntimeFailure() returned an unexpected error: %v",
			err,
		)
	}

	result, err := runtime.NewNodeFailureResult(
		failure,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeFailureResult() returned an unexpected error: %v",
			err,
		)
	}

	err = routeNodeOutputs(
		prepared,
		workflow.NodeID("source"),
		result,
	)

	requireEngineValidationField(
		t,
		err,
		"nodeResult",
	)

	queue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := queue.Len(); actual != 0 {
		t.Fatalf(
			"queue length after failure result = %d, want 0",
			actual,
		)
	}
}

func TestRouteNodeOutputsDoesNotPartiallyRouteWhenRuntimeEdgeIsMissing(
	t *testing.T,
) {
	prepared := mustRoutingFanOutPreparedExecution(t)

	edgeA := routingEdgeRuntime(
		t,
		prepared,
		"edge-a",
	)

	replacementContext, err := runtime.NewExecutionContext(
		context.Background(),
		*prepared.workflowExecution,
		prepared.request.CorrelationID(),
		[]*runtime.EdgeRuntime{
			edgeA,
		},
		prepared.request.InitialVariables(),
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	prepared.executionContext = replacementContext

	result := mustRoutingSuccessResult(
		t,
		map[string][]runtime.Payload{
			"output": {
				mustRoutingPayload(
					t,
					"fan-out",
					nil,
				),
			},
		},
	)

	err = routeNodeOutputs(
		prepared,
		workflow.NodeID("source"),
		result,
	)
	if err == nil {
		t.Fatal(
			"routeNodeOutputs() returned nil error for a missing runtime edge",
		)
	}

	if !strings.Contains(
		err.Error(),
		"edge-b",
	) {
		t.Fatalf(
			"error = %q, want missing edge-b information",
			err,
		)
	}

	if actual := edgeA.Queue().Len(); actual != 0 {
		t.Fatalf(
			"edge-a queue was partially mutated: length=%d",
			actual,
		)
	}
}

func TestBuildOutputRoutingOperationsRejectsInvalidState(
	t *testing.T,
) {
	validResult := mustRoutingSuccessResult(
		t,
		map[string][]runtime.Payload{
			"output": {
				mustRoutingPayload(
					t,
					"value",
					nil,
				),
			},
		},
	)

	t.Run("nil prepared execution", func(t *testing.T) {
		_, err := buildOutputRoutingOperations(
			nil,
			workflow.NodeID("source"),
			validResult,
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution",
		)
	})

	t.Run("missing execution context", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)
		prepared.executionContext = nil

		_, err := buildOutputRoutingOperations(
			prepared,
			workflow.NodeID("source"),
			validResult,
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.executionContext",
		)
	})

	t.Run("unknown node", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		_, err := buildOutputRoutingOperations(
			prepared,
			workflow.NodeID("unknown"),
			validResult,
		)

		requireEngineValidationField(
			t,
			err,
			"nodeID",
		)
	})

	t.Run("missing descriptor", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		delete(
			prepared.plan.descriptorsByNode,
			workflow.NodeID("source"),
		)

		_, err := buildOutputRoutingOperations(
			prepared,
			workflow.NodeID("source"),
			validResult,
		)
		if err == nil {
			t.Fatal(
				"buildOutputRoutingOperations() returned nil error for a missing descriptor",
			)
		}

		if !strings.Contains(
			err.Error(),
			"descriptor",
		) {
			t.Fatalf(
				"error = %q, want descriptor information",
				err,
			)
		}
	})

	t.Run("invalid node result", func(t *testing.T) {
		prepared := mustSchedulerLinearPreparedExecution(t)

		_, err := buildOutputRoutingOperations(
			prepared,
			workflow.NodeID("source"),
			runtime.NodeResult{},
		)

		requireEngineValidationField(
			t,
			err,
			"nodeResult",
		)
	})
}

func mustRoutingFanOutPreparedExecution(
	t *testing.T,
) *preparedExecution {
	t.Helper()

	const sourcePluginType workflow.PluginType = "test.routing-source"

	const targetPluginType workflow.PluginType = "test.routing-target"

	const version workflow.PluginVersion = "v1"

	sourceDescriptor := mustValidationDescriptor(
		t,
		sourcePluginType,
		version,
		plugin.DistributionDistributable,
		nil,
		[]string{
			"optional",
			"output",
		},
		plugin.NewExactEdgeConstraint(0),
		plugin.NewExactEdgeConstraint(2),
	)

	targetDescriptor := mustValidationDescriptor(
		t,
		targetPluginType,
		version,
		plugin.DistributionDistributable,
		[]string{
			"input",
		},
		nil,
		plugin.NewExactEdgeConstraint(1),
		plugin.NewExactEdgeConstraint(0),
	)

	pluginRegistry, err := plugin.NewRegistry(
		[]plugin.Descriptor{
			targetDescriptor,
			sourceDescriptor,
		},
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			return runtime.NewNodeSuccessResult(
				nil,
				runtime.ContextChanges{},
			)
		},
	)

	executorRegistry, err := runtime.NewExecutorRegistry(
		[]runtime.ExecutorRegistration{
			mustValidationExecutorRegistration(
				t,
				sourceDescriptor.Identity(),
				executor,
			),
			mustValidationExecutorRegistration(
				t,
				targetDescriptor.Identity(),
				executor,
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	limits := mustValidationRuntimeLimits(
		t,
		1,
		1,
		4096,
	)

	clock := &preparationSequenceClock{
		times: preparationTestTimes(),
	}

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		clock,
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	source := mustValidationNode(
		t,
		"source",
		sourcePluginType.String(),
		version.String(),
		`{}`,
	)

	targetA := mustValidationNode(
		t,
		"target-a",
		targetPluginType.String(),
		version.String(),
		`{}`,
	)

	targetB := mustValidationNode(
		t,
		"target-b",
		targetPluginType.String(),
		version.String(),
		`{}`,
	)

	edgeA := mustValidationEdge(
		t,
		"edge-a",
		"source",
		"output",
		"target-a",
		"input",
	)

	edgeB := mustValidationEdge(
		t,
		"edge-b",
		"source",
		"output",
		"target-b",
		"input",
	)

	definition := mustValidationDefinition(
		t,
		"workflow-routing-fan-out",
		[]workflow.NodeDefinition{
			targetB,
			source,
			targetA,
		},
		[]workflow.EdgeDefinition{
			edgeB,
			edgeA,
		},
	)

	preparation, err := prepareExecution(
		context.Background(),
		mustValidationExecutionRequest(
			t,
			definition,
		),
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if preparation.prepared == nil {
		t.Fatal(
			"fan-out workflow was not prepared",
		)
	}

	return preparation.prepared
}

func mustRoutingPayload(
	t *testing.T,
	value string,
	metadata map[string]string,
) runtime.Payload {
	t.Helper()

	payload, err := runtime.NewInlinePayload(
		runtime.ContentTypeTextPlain,
		[]byte(value),
		metadata,
		4096,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	return payload
}

func mustRoutingSuccessResult(
	t *testing.T,
	outputs map[string][]runtime.Payload,
) runtime.NodeResult {
	t.Helper()

	result, err := runtime.NewNodeSuccessResult(
		outputs,
		runtime.ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	return result
}

func mustRoutingTerminalResult(
	t *testing.T,
	payload runtime.Payload,
) runtime.NodeResult {
	t.Helper()

	result, err := runtime.NewTerminalNodeSuccessResult(
		payload,
		runtime.ContextChanges{},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewTerminalNodeSuccessResult() returned an unexpected error: %v",
			err,
		)
	}

	return result
}

func routingEdgeRuntime(
	t *testing.T,
	prepared *preparedExecution,
	edgeID string,
) *runtime.EdgeRuntime {
	t.Helper()

	if prepared == nil ||
		prepared.executionContext == nil {
		t.Fatal(
			"prepared execution context is unavailable",
		)
	}

	edgeRuntime, exists, err := prepared.executionContext.Edge(
		workflow.EdgeID(edgeID),
	)
	if err != nil {
		t.Fatalf(
			"executionContext.Edge(%q) returned an unexpected error: %v",
			edgeID,
			err,
		)
	}

	if !exists || edgeRuntime == nil {
		t.Fatalf(
			"runtime edge %q is unavailable",
			edgeID,
		)
	}

	return edgeRuntime
}

func TestRunPreparedExecutionContinuesIndependentBranchAfterControlledFailure(
	t *testing.T,
) {
	prepared :=
		mustSchedulerBranchFailurePreparedExecution(
			t,
		)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				21,
				10,
				0,
				0,
				0,
				time.UTC,
			),
		}

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	actualExecutorOrder := make(
		[]workflow.NodeID,
		0,
		4,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			actualExecutorOrder = append(
				actualExecutorOrder,
				nodeID,
			)

			switch nodeID {
			case workflow.NodeID("a-source"):
				if !input.IsValid() {
					t.Fatal(
						"source received invalid input",
					)
				}

				if !input.IsEmpty() {
					t.Fatal(
						"source received non-empty input",
					)
				}

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"source-value",
								map[string]string{
									"node": "a-source",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("b-failed"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				failure, err :=
					runtime.NewRuntimeFailure(
						runtime.FailureCategoryExecution,
						"TEST_BRANCH_FAILURE",
						"Controlled branch failure",
						false,
						map[string]string{
							"nodeID": nodeID.String(),
						},
					)
				if err != nil {
					t.Fatalf(
						"runtime.NewRuntimeFailure() returned an unexpected error: %v",
						err,
					)
				}

				return runtime.NewNodeFailureResult(
					failure,
				)

			case workflow.NodeID("c-descendant"):
				t.Fatal(
					"blocked descendant executor must not be called",
				)

				return runtime.NodeResult{},
					fmt.Errorf(
						"blocked descendant was executed",
					)

			case workflow.NodeID("d-healthy"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"healthy-value",
								map[string]string{
									"node": "d-healthy",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("e-terminal"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"healthy-value",
				)

				return mustRoutingTerminalResult(
					t,
					mustRoutingPayload(
						t,
						"healthy-result",
						map[string]string{
							"node": "e-terminal",
						},
					),
				), nil

			default:
				return runtime.NodeResult{},
					fmt.Errorf(
						"unexpected scheduler node %s",
						nodeID,
					)
			}
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		prepared,
		executor,
	)

	result, err :=
		runPreparedExecution(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"runPreparedExecution() returned an unexpected technical error: %v",
			err,
		)
	}

	expectedExecutedOrder := []workflow.NodeID{
		workflow.NodeID("a-source"),
		workflow.NodeID("b-failed"),
		workflow.NodeID("d-healthy"),
		workflow.NodeID("e-terminal"),
	}

	if !reflect.DeepEqual(
		actualExecutorOrder,
		expectedExecutedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			actualExecutorOrder,
			expectedExecutedOrder,
		)
	}

	if !reflect.DeepEqual(
		result.ExecutedNodeOrder(),
		expectedExecutedOrder,
	) {
		t.Fatalf(
			"recorded executed-node order = %#v, want %#v",
			result.ExecutedNodeOrder(),
			expectedExecutedOrder,
		)
	}

	expectedNodeOrder := []workflow.NodeID{
		workflow.NodeID("a-source"),
		workflow.NodeID("b-failed"),
		workflow.NodeID("c-descendant"),
		workflow.NodeID("d-healthy"),
		workflow.NodeID("e-terminal"),
	}

	if !reflect.DeepEqual(
		result.NodeExecutionOrder(),
		expectedNodeOrder,
	) {
		t.Fatalf(
			"node execution order = %#v, want %#v",
			result.NodeExecutionOrder(),
			expectedNodeOrder,
		)
	}

	if executorCalls[workflow.NodeID("a-source")] != 1 {
		t.Fatalf(
			"a-source executor call count = %d, want 1",
			executorCalls[workflow.NodeID("a-source")],
		)
	}

	if executorCalls[workflow.NodeID("b-failed")] != 1 {
		t.Fatalf(
			"b-failed executor call count = %d, want 1",
			executorCalls[workflow.NodeID("b-failed")],
		)
	}

	if executorCalls[workflow.NodeID("c-descendant")] != 0 {
		t.Fatalf(
			"c-descendant executor call count = %d, want 0",
			executorCalls[workflow.NodeID("c-descendant")],
		)
	}

	if executorCalls[workflow.NodeID("d-healthy")] != 1 {
		t.Fatalf(
			"d-healthy executor call count = %d, want 1",
			executorCalls[workflow.NodeID("d-healthy")],
		)
	}

	if executorCalls[workflow.NodeID("e-terminal")] != 1 {
		t.Fatalf(
			"e-terminal executor call count = %d, want 1",
			executorCalls[workflow.NodeID("e-terminal")],
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"branch-failure workflow result is not terminal",
		)
	}

	if !result.IsFailed() {
		t.Fatalf(
			"workflow status = %q, want FAILED",
			result.Status(),
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"failed workflow is marked as succeeded",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"failed workflow is marked as cancelled",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"failed workflow is marked as timed out",
		)
	}

	if result.IsStalled() {
		t.Fatal(
			"controlled branch failure is marked as stalled",
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("a-source"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("b-failed"),
		execution.NodeExecutionStatusFailed,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("c-descendant"),
		execution.NodeExecutionStatusSkipped,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("d-healthy"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("e-terminal"),
		execution.NodeExecutionStatusSucceeded,
	)

	descendantExecution, exists, err :=
		result.NodeExecution(
			workflow.NodeID("c-descendant"),
		)
	if err != nil {
		t.Fatalf(
			"NodeExecution(c-descendant) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"c-descendant node execution does not exist",
		)
	}

	if descendantExecution.Status() !=
		execution.NodeExecutionStatusSkipped {
		t.Fatalf(
			"c-descendant status = %q, want SKIPPED",
			descendantExecution.Status(),
		)
	}

	if _, started :=
		descendantExecution.StartedAt(); started {
		t.Fatal(
			"skipped descendant unexpectedly contains startedAt",
		)
	}

	failedNodeFailure, exists, err :=
		result.NodeFailure(
			workflow.NodeID("b-failed"),
		)
	if err != nil {
		t.Fatalf(
			"NodeFailure(b-failed) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"b-failed RuntimeFailure does not exist",
		)
	}

	if failedNodeFailure.Category() !=
		runtime.FailureCategoryExecution {
		t.Fatalf(
			"b-failed failure category = %q, want EXECUTION",
			failedNodeFailure.Category(),
		)
	}

	if failedNodeFailure.Code() !=
		"TEST_BRANCH_FAILURE" {
		t.Fatalf(
			"b-failed failure code = %q, want %q",
			failedNodeFailure.Code(),
			"TEST_BRANCH_FAILURE",
		)
	}

	if actual :=
		failedNodeFailure.Details()["nodeID"]; actual != "b-failed" {
		t.Fatalf(
			"b-failed failure nodeID = %q, want %q",
			actual,
			"b-failed",
		)
	}

	nodeFailures :=
		result.NodeFailures()

	if len(nodeFailures) != 1 {
		t.Fatalf(
			"node failure count = %d, want 1",
			len(nodeFailures),
		)
	}

	if _, exists :=
		nodeFailures[workflow.NodeID("b-failed")]; !exists {
		t.Fatal(
			"node failure map does not contain b-failed",
		)
	}

	workflowFailure, exists :=
		result.WorkflowFailure()

	if !exists {
		t.Fatal(
			"failed workflow does not contain a workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryExecution {
		t.Fatalf(
			"workflow failure category = %q, want EXECUTION",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowNodeFailure {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowNodeFailure,
		)
	}

	failureDetails :=
		workflowFailure.Details()

	if actual :=
		failureDetails["failedNodeCount"]; actual != "1" {
		t.Fatalf(
			"failedNodeCount = %q, want %q",
			actual,
			"1",
		)
	}

	if actual :=
		failureDetails["skippedNodeCount"]; actual != "1" {
		t.Fatalf(
			"skippedNodeCount = %q, want %q",
			actual,
			"1",
		)
	}

	if actual :=
		failureDetails["recordedFailureCount"]; actual != "1" {
		t.Fatalf(
			"recordedFailureCount = %q, want %q",
			actual,
			"1",
		)
	}

	terminalPayload, exists, err :=
		result.TerminalOutput(
			workflow.NodeID("e-terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(e-terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"healthy branch terminal output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		terminalPayload,
	); actual != "healthy-result" {
		t.Fatalf(
			"healthy branch terminal output = %q, want %q",
			actual,
			"healthy-result",
		)
	}

	if actual :=
		terminalPayload.Metadata()["node"]; actual != "e-terminal" {
		t.Fatalf(
			"terminal output node metadata = %q, want %q",
			actual,
			"e-terminal",
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-failed",
	).Len(); actual != 0 {
		t.Fatalf(
			"edge-source-failed queue length = %d, want 0",
			actual,
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-healthy",
	).Len(); actual != 0 {
		t.Fatalf(
			"edge-source-healthy queue length = %d, want 0",
			actual,
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-failed-descendant",
	).Len(); actual != 0 {
		t.Fatalf(
			"edge-failed-descendant queue length = %d, want 0",
			actual,
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-healthy-terminal",
	).Len(); actual != 0 {
		t.Fatalf(
			"edge-healthy-terminal queue length = %d, want 0",
			actual,
		)
	}
}

func mustSchedulerBranchFailurePreparedExecution(
	t *testing.T,
) *preparedExecution {
	t.Helper()

	const sourcePluginType workflow.PluginType = "test.scheduler-branch-source"

	const branchPluginType workflow.PluginType = "test.scheduler-branch-node"

	const sinkPluginType workflow.PluginType = "test.scheduler-branch-sink"

	const version workflow.PluginVersion = "v1"

	sourceDescriptor := mustValidationDescriptor(
		t,
		sourcePluginType,
		version,
		plugin.DistributionDistributable,
		nil,
		[]string{
			"output",
		},
		plugin.NewExactEdgeConstraint(0),
		plugin.NewExactEdgeConstraint(2),
	)

	branchDescriptor := mustValidationDescriptor(
		t,
		branchPluginType,
		version,
		plugin.DistributionDistributable,
		[]string{
			"input",
		},
		[]string{
			"output",
		},
		plugin.NewExactEdgeConstraint(1),
		plugin.NewExactEdgeConstraint(1),
	)

	sinkDescriptor := mustValidationDescriptor(
		t,
		sinkPluginType,
		version,
		plugin.DistributionDistributable,
		[]string{
			"input",
		},
		nil,
		plugin.NewExactEdgeConstraint(1),
		plugin.NewExactEdgeConstraint(0),
	)

	pluginRegistry, err := plugin.NewRegistry(
		[]plugin.Descriptor{
			sinkDescriptor,
			sourceDescriptor,
			branchDescriptor,
		},
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	placeholderExecutor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			return runtime.NewNodeSuccessResult(
				nil,
				runtime.ContextChanges{},
			)
		},
	)

	executorRegistry, err := runtime.NewExecutorRegistry(
		[]runtime.ExecutorRegistration{
			mustValidationExecutorRegistration(
				t,
				sourceDescriptor.Identity(),
				placeholderExecutor,
			),
			mustValidationExecutorRegistration(
				t,
				branchDescriptor.Identity(),
				placeholderExecutor,
			),
			mustValidationExecutorRegistration(
				t,
				sinkDescriptor.Identity(),
				placeholderExecutor,
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	limits := mustValidationRuntimeLimits(
		t,
		1,
		1,
		4096,
	)

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		&preparationSequenceClock{
			times: preparationTestTimes(),
		},
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	source := mustValidationNode(
		t,
		"a-source",
		sourcePluginType.String(),
		version.String(),
		`{}`,
	)

	failed := mustValidationNode(
		t,
		"b-failed",
		branchPluginType.String(),
		version.String(),
		`{}`,
	)

	descendant := mustValidationNode(
		t,
		"c-descendant",
		sinkPluginType.String(),
		version.String(),
		`{}`,
	)

	healthy := mustValidationNode(
		t,
		"d-healthy",
		branchPluginType.String(),
		version.String(),
		`{}`,
	)

	terminal := mustValidationNode(
		t,
		"e-terminal",
		sinkPluginType.String(),
		version.String(),
		`{}`,
	)

	edgeSourceFailed := mustValidationEdge(
		t,
		"edge-source-failed",
		"a-source",
		"output",
		"b-failed",
		"input",
	)

	edgeFailedDescendant := mustValidationEdge(
		t,
		"edge-failed-descendant",
		"b-failed",
		"output",
		"c-descendant",
		"input",
	)

	edgeSourceHealthy := mustValidationEdge(
		t,
		"edge-source-healthy",
		"a-source",
		"output",
		"d-healthy",
		"input",
	)

	edgeHealthyTerminal := mustValidationEdge(
		t,
		"edge-healthy-terminal",
		"d-healthy",
		"output",
		"e-terminal",
		"input",
	)

	definition := mustValidationDefinition(
		t,
		"workflow-scheduler-branch-failure",
		[]workflow.NodeDefinition{
			source,
			failed,
			descendant,
			healthy,
			terminal,
		},
		[]workflow.EdgeDefinition{
			edgeSourceFailed,
			edgeFailedDescendant,
			edgeSourceHealthy,
			edgeHealthyTerminal,
		},
	)

	preparation, err := prepareExecution(
		context.Background(),
		mustValidationExecutionRequest(
			t,
			definition,
		),
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if !preparation.IsPrepared() {
		t.Fatalf(
			"branch workflow was not prepared; validation issues = %d",
			preparation.ValidationReport().Len(),
		)
	}

	if preparation.prepared == nil {
		t.Fatal(
			"prepared branch workflow is nil",
		)
	}

	return preparation.prepared
}

func TestRunPreparedExecutionCancelsRunningAndRemainingNodes(
	t *testing.T,
) {
	prepared :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	signalContext :=
		newNodeExecutionSignalContext(
			false,
		)

	replacePreparedExecutionContextForNodeTest(
		t,
		prepared,
		signalContext,
	)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				23,
				10,
				0,
				0,
				0,
				time.UTC,
			),
		}

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executorOrder := make(
		[]workflow.NodeID,
		0,
		1,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			if nodeID != workflow.NodeID("source") {
				t.Fatalf(
					"executor was called for node %q after cancellation",
					nodeID,
				)
			}

			if !input.IsValid() {
				t.Fatal(
					"source received invalid input",
				)
			}

			if !input.IsEmpty() {
				t.Fatal(
					"source received non-empty input",
				)
			}

			if nodeContext.Err() != nil {
				t.Fatalf(
					"node context already contained an error before cancellation: %v",
					nodeContext.Err(),
				)
			}

			signalContext.Fail(
				context.Canceled,
			)

			return mustRoutingSuccessResult(
				t,
				map[string][]runtime.Payload{
					"output": {
						mustRoutingPayload(
							t,
							"must-not-be-routed",
							map[string]string{
								"node": "source",
							},
						),
					},
				},
			), nil
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		prepared,
		executor,
	)

	result, err :=
		runPreparedExecution(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"runPreparedExecution() returned an unexpected error: %v",
			err,
		)
	}

	expectedExecutedOrder := []workflow.NodeID{
		workflow.NodeID("source"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedExecutedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedExecutedOrder,
		)
	}

	if !reflect.DeepEqual(
		result.ExecutedNodeOrder(),
		expectedExecutedOrder,
	) {
		t.Fatalf(
			"recorded executed-node order = %#v, want %#v",
			result.ExecutedNodeOrder(),
			expectedExecutedOrder,
		)
	}

	if executorCalls[workflow.NodeID("source")] != 1 {
		t.Fatalf(
			"source executor call count = %d, want 1",
			executorCalls[workflow.NodeID("source")],
		)
	}

	if executorCalls[workflow.NodeID("pass")] != 0 {
		t.Fatalf(
			"pass executor call count = %d, want 0",
			executorCalls[workflow.NodeID("pass")],
		)
	}

	if executorCalls[workflow.NodeID("terminal")] != 0 {
		t.Fatalf(
			"terminal executor call count = %d, want 0",
			executorCalls[workflow.NodeID("terminal")],
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"cancelled workflow result is not terminal",
		)
	}

	if !result.IsCancelled() {
		t.Fatalf(
			"workflow status = %q, want CANCELLED",
			result.Status(),
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"cancelled workflow is marked as succeeded",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"cancelled workflow is marked as failed",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"cancelled workflow is marked as timed out",
		)
	}

	if result.IsStalled() {
		t.Fatal(
			"cancelled workflow is marked as stalled",
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusCancelled,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusCancelled,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusCancelled,
	)

	requireCompletedNodeExecutionTimestamps(
		t,
		prepared,
		workflow.NodeID("source"),
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		result,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusCancelled,
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		result,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusCancelled,
	)

	workflowFailure, exists :=
		result.WorkflowFailure()

	if !exists {
		t.Fatal(
			"cancelled workflow does not contain a workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryCanceled {
		t.Fatalf(
			"workflow failure category = %q, want CANCELED",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowCanceled {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowCanceled,
		)
	}

	if actual :=
		workflowFailure.Details()["contextError"]; actual != context.Canceled.Error() {
		t.Fatalf(
			"context error detail = %q, want %q",
			actual,
			context.Canceled.Error(),
		)
	}

	if failures := result.NodeFailures(); failures != nil {
		t.Fatalf(
			"cancelled workflow node failures = %#v, want nil",
			failures,
		)
	}

	if outputs := result.TerminalOutputs(); outputs != nil {
		t.Fatalf(
			"cancelled workflow terminal outputs = %#v, want nil",
			outputs,
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	).Len(); actual != 0 {
		t.Fatalf(
			"source-pass queue length = %d, want 0",
			actual,
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-pass-terminal",
	).Len(); actual != 0 {
		t.Fatalf(
			"pass-terminal queue length = %d, want 0",
			actual,
		)
	}
}

func TestRunPreparedExecutionTimesOutRunningAndRemainingNodes(
	t *testing.T,
) {
	prepared :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	signalContext :=
		newNodeExecutionSignalContext(
			true,
		)

	replacePreparedExecutionContextForNodeTest(
		t,
		prepared,
		signalContext,
	)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				23,
				12,
				0,
				0,
				0,
				time.UTC,
			),
		}

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executorOrder := make(
		[]workflow.NodeID,
		0,
		1,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			if nodeID != workflow.NodeID("source") {
				t.Fatalf(
					"executor was called for node %q after timeout",
					nodeID,
				)
			}

			deadline, exists :=
				nodeContext.Deadline()

			if !exists {
				t.Fatal(
					"node context does not expose a deadline",
				)
			}

			if deadline.IsZero() {
				t.Fatal(
					"node context deadline is zero",
				)
			}

			signalContext.Fail(
				context.DeadlineExceeded,
			)

			return mustRoutingSuccessResult(
				t,
				map[string][]runtime.Payload{
					"output": {
						mustRoutingPayload(
							t,
							"must-not-be-routed",
							nil,
						),
					},
				},
			), nil
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		prepared,
		executor,
	)

	result, err :=
		runPreparedExecution(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"runPreparedExecution() returned an unexpected error: %v",
			err,
		)
	}

	expectedExecutedOrder := []workflow.NodeID{
		workflow.NodeID("source"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedExecutedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedExecutedOrder,
		)
	}

	if !reflect.DeepEqual(
		result.ExecutedNodeOrder(),
		expectedExecutedOrder,
	) {
		t.Fatalf(
			"recorded executed-node order = %#v, want %#v",
			result.ExecutedNodeOrder(),
			expectedExecutedOrder,
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"timed-out workflow result is not terminal",
		)
	}

	if !result.IsTimedOut() {
		t.Fatalf(
			"workflow status = %q, want TIMED_OUT",
			result.Status(),
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"timed-out workflow is marked as succeeded",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"timed-out workflow is marked as failed",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"timed-out workflow is marked as cancelled",
		)
	}

	if result.IsStalled() {
		t.Fatal(
			"timed-out workflow is marked as stalled",
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireCompletedNodeExecutionTimestamps(
		t,
		prepared,
		workflow.NodeID("source"),
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		result,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		result,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusTimedOut,
	)

	workflowFailure, exists :=
		result.WorkflowFailure()

	if !exists {
		t.Fatal(
			"timed-out workflow does not contain a workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryTimeout {
		t.Fatalf(
			"workflow failure category = %q, want TIMEOUT",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowTimedOut {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowTimedOut,
		)
	}

	if actual :=
		workflowFailure.Details()["contextError"]; actual != context.DeadlineExceeded.Error() {
		t.Fatalf(
			"context error detail = %q, want %q",
			actual,
			context.DeadlineExceeded.Error(),
		)
	}

	if failures := result.NodeFailures(); failures != nil {
		t.Fatalf(
			"timed-out workflow node failures = %#v, want nil",
			failures,
		)
	}

	if outputs := result.TerminalOutputs(); outputs != nil {
		t.Fatalf(
			"timed-out workflow terminal outputs = %#v, want nil",
			outputs,
		)
	}
}

func TestRunPreparedExecutionDetectsStalledWorkflow(
	t *testing.T,
) {
	prepared :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				23,
				14,
				0,
				0,
				0,
				time.UTC,
			),
		}

	/*
		Source node succeeded gibi işaretleniyor fakat source output
		queue'suna payload yazılmıyor.

		Pass node:
		- upstream SUCCEEDED
		- incoming queue empty
		- WAITING

		Terminal node da pass tamamlanmadığı için WAITING olur.
	*/
	transitionSchedulerNode(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	executorCalls := 0

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			executorCalls++

			return runtime.NodeResult{},
				errors.New(
					"stalled workflow executor must not be called",
				)
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		prepared,
		executor,
	)

	result, err :=
		runPreparedExecution(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"runPreparedExecution() returned an unexpected error for stalled execution: %v",
			err,
		)
	}

	if executorCalls != 0 {
		t.Fatalf(
			"executor call count = %d, want 0",
			executorCalls,
		)
	}

	if order := result.ExecutedNodeOrder(); order != nil {
		t.Fatalf(
			"executed node order = %#v, want nil",
			order,
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"stalled workflow result is not terminal",
		)
	}

	if !result.IsFailed() {
		t.Fatalf(
			"workflow status = %q, want FAILED",
			result.Status(),
		)
	}

	if !result.IsStalled() {
		t.Fatal(
			"stalled workflow is not marked as stalled",
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"stalled workflow is marked as succeeded",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"stalled workflow is marked as cancelled",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"stalled workflow is marked as timed out",
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusSkipped,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusSkipped,
	)

	workflowFailure, exists :=
		result.WorkflowFailure()

	if !exists {
		t.Fatal(
			"stalled workflow does not contain a workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryInternal {
		t.Fatalf(
			"stalled workflow category = %q, want INTERNAL",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowStalled {
		t.Fatalf(
			"stalled workflow code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowStalled,
		)
	}

	details :=
		workflowFailure.Details()

	if actual :=
		details["pendingNodeCount"]; actual != "2" {
		t.Fatalf(
			"pendingNodeCount = %q, want %q",
			actual,
			"2",
		)
	}

	if actual :=
		details["readyNodeCount"]; actual != "0" {
		t.Fatalf(
			"readyNodeCount = %q, want %q",
			actual,
			"0",
		)
	}

	if actual :=
		details["queuedNodeCount"]; actual != "0" {
		t.Fatalf(
			"queuedNodeCount = %q, want %q",
			actual,
			"0",
		)
	}

	if actual :=
		details["runningNodeCount"]; actual != "0" {
		t.Fatalf(
			"runningNodeCount = %q, want %q",
			actual,
			"0",
		)
	}

	if failures := result.NodeFailures(); failures != nil {
		t.Fatalf(
			"stalled workflow node failures = %#v, want nil",
			failures,
		)
	}

	if outputs := result.TerminalOutputs(); outputs != nil {
		t.Fatalf(
			"stalled workflow terminal outputs = %#v, want nil",
			outputs,
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	).Len(); actual != 0 {
		t.Fatalf(
			"source-pass queue length = %d, want 0",
			actual,
		)
	}
}

func requireSchedulerNodeFinishedWithoutStart(
	t *testing.T,
	result ExecutionResult,
	nodeID workflow.NodeID,
	expectedStatus execution.NodeExecutionStatus,
) {
	t.Helper()

	nodeExecution, exists, err :=
		result.NodeExecution(
			nodeID,
		)
	if err != nil {
		t.Fatalf(
			"NodeExecution(%s) returned an unexpected error: %v",
			nodeID,
			err,
		)
	}

	if !exists {
		t.Fatalf(
			"node execution %q does not exist",
			nodeID,
		)
	}

	if nodeExecution.Status() != expectedStatus {
		t.Fatalf(
			"node %q status = %q, want %q",
			nodeID,
			nodeExecution.Status(),
			expectedStatus,
		)
	}

	if _, started :=
		nodeExecution.StartedAt(); started {
		t.Fatalf(
			"node %q unexpectedly contains startedAt",
			nodeID,
		)
	}

	finishedAt, finished :=
		nodeExecution.FinishedAt()

	if !finished || finishedAt.IsZero() {
		t.Fatalf(
			"node %q does not contain finishedAt",
			nodeID,
		)
	}
}

func TestRunPreparedExecutionCompletesLinearWorkflowDeterministically(
	t *testing.T,
) {
	prepared, result, actualExecutorOrder :=
		mustRunSuccessfulLinearScheduler(
			t,
		)

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("source"),
		workflow.NodeID("pass"),
		workflow.NodeID("terminal"),
	}

	if !reflect.DeepEqual(
		actualExecutorOrder,
		expectedOrder,
	) {
		t.Fatalf(
			"executor call order = %#v, want %#v",
			actualExecutorOrder,
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		result.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"executed node order = %#v, want %#v",
			result.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		result.NodeExecutionOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"node execution order = %#v, want %#v",
			result.NodeExecutionOrder(),
			expectedOrder,
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"execution result is not terminal",
		)
	}

	if !result.IsSucceeded() {
		t.Fatalf(
			"execution result status = %q, want SUCCEEDED",
			result.Status(),
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"successful execution result is marked as failed",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"successful execution result is marked as cancelled",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"successful execution result is marked as timed out",
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"successful execution result is marked as rejected",
		)
	}

	if result.IsStalled() {
		t.Fatal(
			"successful execution result is marked as stalled",
		)
	}

	if actual := result.Status(); actual !=
		execution.WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"workflow status = %q, want %q",
			actual,
			execution.WorkflowExecutionStatusSucceeded,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusSucceeded,
	)

	nodeExecutions := result.NodeExecutions()

	if len(nodeExecutions) != 3 {
		t.Fatalf(
			"node execution count = %d, want 3",
			len(nodeExecutions),
		)
	}

	for index, nodeExecution := range nodeExecutions {
		if nodeExecution.NodeID() != expectedOrder[index] {
			t.Fatalf(
				"node execution %d node ID = %q, want %q",
				index,
				nodeExecution.NodeID(),
				expectedOrder[index],
			)
		}

		if nodeExecution.Status() !=
			execution.NodeExecutionStatusSucceeded {
			t.Fatalf(
				"node %q status = %q, want SUCCEEDED",
				nodeExecution.NodeID(),
				nodeExecution.Status(),
			)
		}

		startedAt, started :=
			nodeExecution.StartedAt()

		if !started || startedAt.IsZero() {
			t.Fatalf(
				"node %q has no startedAt timestamp",
				nodeExecution.NodeID(),
			)
		}

		finishedAt, finished :=
			nodeExecution.FinishedAt()

		if !finished || finishedAt.IsZero() {
			t.Fatalf(
				"node %q has no finishedAt timestamp",
				nodeExecution.NodeID(),
			)
		}

		if finishedAt.Before(startedAt) {
			t.Fatalf(
				"node %q finishedAt %v is before startedAt %v",
				nodeExecution.NodeID(),
				finishedAt,
				startedAt,
			)
		}
	}

	workflowExecution :=
		result.WorkflowExecution()

	finishedAt, finished :=
		workflowExecution.FinishedAt()

	if !finished || finishedAt.IsZero() {
		t.Fatal(
			"workflow execution has no finishedAt timestamp",
		)
	}

	terminalPayload, exists, err :=
		result.TerminalOutput(
			workflow.NodeID("terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"terminal output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		terminalPayload,
	); actual != "workflow-result" {
		t.Fatalf(
			"terminal output = %q, want %q",
			actual,
			"workflow-result",
		)
	}

	if actual := terminalPayload.Metadata()["node"]; actual != "terminal" {
		t.Fatalf(
			"terminal output metadata node = %q, want %q",
			actual,
			"terminal",
		)
	}

	terminalOutputs :=
		result.TerminalOutputs()

	if len(terminalOutputs) != 1 {
		t.Fatalf(
			"terminal output count = %d, want 1",
			len(terminalOutputs),
		)
	}

	if _, exists :=
		terminalOutputs[workflow.NodeID("terminal")]; !exists {
		t.Fatal(
			"terminal output map does not contain terminal node",
		)
	}

	if failures := result.NodeFailures(); failures != nil {
		t.Fatalf(
			"node failures = %#v, want nil",
			failures,
		)
	}

	if _, exists := result.WorkflowFailure(); exists {
		t.Fatal(
			"successful workflow unexpectedly contains a workflow failure",
		)
	}

	sourceQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-pass",
	)

	if actual := sourceQueue.Len(); actual != 0 {
		t.Fatalf(
			"source-pass queue length = %d, want 0",
			actual,
		)
	}

	terminalQueue := schedulerEdgeQueue(
		t,
		prepared,
		"edge-pass-terminal",
	)

	if actual := terminalQueue.Len(); actual != 0 {
		t.Fatalf(
			"pass-terminal queue length = %d, want 0",
			actual,
		)
	}
}

func TestExecutionResultReturnsDefensiveSnapshots(
	t *testing.T,
) {
	_, result, _ :=
		mustRunSuccessfulLinearScheduler(
			t,
		)

	executedOrder :=
		result.ExecutedNodeOrder()

	if len(executedOrder) != 3 {
		t.Fatalf(
			"executed node order length = %d, want 3",
			len(executedOrder),
		)
	}

	executedOrder[0] =
		workflow.NodeID("changed")

	secondExecutedOrder :=
		result.ExecutedNodeOrder()

	if secondExecutedOrder[0] !=
		workflow.NodeID("source") {
		t.Fatalf(
			"executed node order changed through returned slice: %#v",
			secondExecutedOrder,
		)
	}

	nodeOrder :=
		result.NodeExecutionOrder()

	nodeOrder[0] =
		workflow.NodeID("changed")

	secondNodeOrder :=
		result.NodeExecutionOrder()

	if secondNodeOrder[0] !=
		workflow.NodeID("source") {
		t.Fatalf(
			"node execution order changed through returned slice: %#v",
			secondNodeOrder,
		)
	}

	terminalOutputs :=
		result.TerminalOutputs()

	delete(
		terminalOutputs,
		workflow.NodeID("terminal"),
	)

	secondTerminalOutputs :=
		result.TerminalOutputs()

	if len(secondTerminalOutputs) != 1 {
		t.Fatalf(
			"terminal outputs changed through returned map: %#v",
			secondTerminalOutputs,
		)
	}

	terminalPayload, exists, err :=
		result.TerminalOutput(
			workflow.NodeID("terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"terminal output does not exist",
		)
	}

	inlineData, exists :=
		terminalPayload.InlineData()

	if !exists {
		t.Fatal(
			"terminal output does not contain inline data",
		)
	}

	inlineData[0] = 'X'

	metadata :=
		terminalPayload.Metadata()

	metadata["node"] =
		"changed"

	secondTerminalPayload, exists, err :=
		result.TerminalOutput(
			workflow.NodeID("terminal"),
		)
	if err != nil {
		t.Fatalf(
			"second TerminalOutput() returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"second terminal output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		secondTerminalPayload,
	); actual != "workflow-result" {
		t.Fatalf(
			"stored terminal output changed through accessor: %q",
			actual,
		)
	}

	if actual := secondTerminalPayload.Metadata()["node"]; actual != "terminal" {
		t.Fatalf(
			"stored terminal metadata changed through accessor: %q",
			actual,
		)
	}
}

func TestRunPreparedExecutionRejectsInvalidPreparedState(
	t *testing.T,
) {
	t.Run("nil prepared execution", func(t *testing.T) {
		_, err := runPreparedExecution(
			nil,
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution",
		)
	})

	t.Run("missing workflow execution", func(t *testing.T) {
		prepared :=
			mustSchedulerLinearPreparedExecution(
				t,
			)

		prepared.workflowExecution = nil

		_, err := runPreparedExecution(
			prepared,
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.workflowExecution",
		)
	})

	t.Run("missing execution context", func(t *testing.T) {
		prepared :=
			mustSchedulerLinearPreparedExecution(
				t,
			)

		prepared.executionContext = nil

		_, err := runPreparedExecution(
			prepared,
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.executionContext",
		)
	})

	t.Run("invalid dependencies", func(t *testing.T) {
		prepared :=
			mustSchedulerLinearPreparedExecution(
				t,
			)

		prepared.dependencies =
			EngineDependencies{}

		_, err := runPreparedExecution(
			prepared,
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.dependencies",
		)
	})

	t.Run("workflow is not running", func(t *testing.T) {
		prepared :=
			mustSchedulerLinearPreparedExecution(
				t,
			)

		prepared.dependencies.clock =
			&nodeExecutionTestClock{
				next: time.Date(
					2026,
					time.July,
					20,
					12,
					0,
					0,
					0,
					time.UTC,
				),
			}

		err := transitionWorkflowExecution(
			prepared,
			execution.WorkflowExecutionStatusSucceeded,
		)
		if err != nil {
			t.Fatalf(
				"transitionWorkflowExecution() returned an unexpected error: %v",
				err,
			)
		}

		_, err = runPreparedExecution(
			prepared,
		)

		requireEngineValidationField(
			t,
			err,
			"preparedExecution.workflowExecution.status",
		)
	})
}

func mustRunSuccessfulLinearScheduler(
	t *testing.T,
) (
	*preparedExecution,
	ExecutionResult,
	[]workflow.NodeID,
) {
	t.Helper()

	prepared :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				20,
				10,
				0,
				0,
				0,
				time.UTC,
			),
		}

	executorOrder := make(
		[]workflow.NodeID,
		0,
		3,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			switch nodeID {
			case workflow.NodeID("source"):
				if !input.IsValid() {
					t.Fatal(
						"source received an invalid input",
					)
				}

				if !input.IsEmpty() {
					t.Fatal(
						"source received a non-empty input",
					)
				}

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"source-value",
								map[string]string{
									"node": "source",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("pass"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"pass-value",
								map[string]string{
									"node": "pass",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("terminal"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"pass-value",
				)

				return mustRoutingTerminalResult(
					t,
					mustRoutingPayload(
						t,
						"workflow-result",
						map[string]string{
							"node": "terminal",
						},
					),
				), nil

			default:
				return runtime.NodeResult{},
					fmt.Errorf(
						"unexpected scheduler test node %s",
						nodeID,
					)
			}
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		prepared,
		executor,
	)

	result, err :=
		runPreparedExecution(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"runPreparedExecution() returned an unexpected error: %v",
			err,
		)
	}

	return prepared,
		result,
		append(
			[]workflow.NodeID(nil),
			executorOrder...,
		)
}

func installSchedulerExecutorForAllNodes(
	t *testing.T,
	prepared *preparedExecution,
	executor runtime.NodeExecutor,
) {
	t.Helper()

	if prepared == nil {
		t.Fatal(
			"prepared execution is nil",
		)
	}

	registrations := make(
		[]runtime.ExecutorRegistration,
		0,
		len(prepared.nodeExecutionOrder),
	)

	registeredIdentities := make(
		map[string]struct{},
	)

	for _, nodeID := range prepared.nodeExecutionOrder {
		descriptor, exists :=
			prepared.plan.descriptorsByNode[nodeID]

		if !exists {
			t.Fatalf(
				"plugin descriptor for node %q is unavailable",
				nodeID,
			)
		}

		identity :=
			descriptor.Identity()

		identityKey :=
			identity.String()

		if _, exists :=
			registeredIdentities[identityKey]; exists {
			continue
		}

		registration, err :=
			runtime.NewExecutorRegistration(
				identity,
				executor,
			)
		if err != nil {
			t.Fatalf(
				"runtime.NewExecutorRegistration(%s) returned an unexpected error: %v",
				identity,
				err,
			)
		}

		registrations = append(
			registrations,
			registration,
		)

		registeredIdentities[identityKey] =
			struct{}{}
	}

	registry, err :=
		runtime.NewExecutorRegistry(
			registrations,
		)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	prepared.dependencies.executorRegistry =
		registry
}

func requireSchedulerInputValue(
	t *testing.T,
	input runtime.NodeInput,
	port string,
	expected string,
) {
	t.Helper()

	if !input.IsValid() {
		t.Fatal(
			"scheduler executor received an invalid NodeInput",
		)
	}

	payloads, exists, err :=
		input.Payloads(
			port,
		)
	if err != nil {
		t.Fatalf(
			"input.Payloads(%q) returned an unexpected error: %v",
			port,
			err,
		)
	}

	if !exists {
		t.Fatalf(
			"input port %q does not exist",
			port,
		)
	}

	if len(payloads) != 1 {
		t.Fatalf(
			"input port %q payload count = %d, want 1",
			port,
			len(payloads),
		)
	}

	if actual := preparedInputPayloadText(
		t,
		payloads[0],
	); actual != expected {
		t.Fatalf(
			"input port %q payload = %q, want %q",
			port,
			actual,
			expected,
		)
	}
}

func TestRunPreparedExecutionContinuesIndependentBranchAfterTechnicalFailure(
	t *testing.T,
) {
	prepared :=
		mustSchedulerBranchFailurePreparedExecution(
			t,
		)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				22,
				10,
				0,
				0,
				0,
				time.UTC,
			),
		}

	technicalFailure := errors.New(
		"test dependency connection failed",
	)

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executorOrder := make(
		[]workflow.NodeID,
		0,
		4,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			switch nodeID {
			case workflow.NodeID("a-source"):
				if !input.IsValid() {
					t.Fatal(
						"source received invalid input",
					)
				}

				if !input.IsEmpty() {
					t.Fatal(
						"source received non-empty input",
					)
				}

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"source-value",
								map[string]string{
									"node": "a-source",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("b-failed"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				return runtime.NodeResult{},
					technicalFailure

			case workflow.NodeID("c-descendant"):
				t.Fatal(
					"blocked descendant executor must not be called",
				)

				return runtime.NodeResult{},
					fmt.Errorf(
						"blocked descendant was executed",
					)

			case workflow.NodeID("d-healthy"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"healthy-value",
								map[string]string{
									"node": "d-healthy",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("e-terminal"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"healthy-value",
				)

				return mustRoutingTerminalResult(
					t,
					mustRoutingPayload(
						t,
						"healthy-result",
						map[string]string{
							"node": "e-terminal",
						},
					),
				), nil

			default:
				return runtime.NodeResult{},
					fmt.Errorf(
						"unexpected scheduler node %s",
						nodeID,
					)
			}
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		prepared,
		executor,
	)

	result, err :=
		runPreparedExecution(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"runPreparedExecution() returned an unexpected scheduler error: %v",
			err,
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("a-source"),
		workflow.NodeID("b-failed"),
		workflow.NodeID("d-healthy"),
		workflow.NodeID("e-terminal"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		result.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"recorded execution order = %#v, want %#v",
			result.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if executorCalls[workflow.NodeID("c-descendant")] != 0 {
		t.Fatalf(
			"c-descendant executor call count = %d, want 0",
			executorCalls[workflow.NodeID("c-descendant")],
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"technical-failure workflow is not terminal",
		)
	}

	if !result.IsFailed() {
		t.Fatalf(
			"workflow status = %q, want FAILED",
			result.Status(),
		)
	}

	if result.IsStalled() {
		t.Fatal(
			"technical node failure is incorrectly marked as stalled",
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("a-source"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("b-failed"),
		execution.NodeExecutionStatusFailed,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("c-descendant"),
		execution.NodeExecutionStatusSkipped,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("d-healthy"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("e-terminal"),
		execution.NodeExecutionStatusSucceeded,
	)

	nodeFailure, exists, err :=
		result.NodeFailure(
			workflow.NodeID("b-failed"),
		)
	if err != nil {
		t.Fatalf(
			"NodeFailure(b-failed) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"technical node RuntimeFailure does not exist",
		)
	}

	if nodeFailure.Category() !=
		runtime.FailureCategoryInternal {
		t.Fatalf(
			"technical node failure category = %q, want INTERNAL",
			nodeFailure.Category(),
		)
	}

	if nodeFailure.Code() !=
		failureCodeNodeExecutionError {
		t.Fatalf(
			"technical node failure code = %q, want %q",
			nodeFailure.Code(),
			failureCodeNodeExecutionError,
		)
	}

	failureDetails :=
		nodeFailure.Details()

	if actual :=
		failureDetails["nodeID"]; actual != "b-failed" {
		t.Fatalf(
			"technical failure nodeID = %q, want %q",
			actual,
			"b-failed",
		)
	}

	if actual :=
		failureDetails["error"]; actual == "" {
		t.Fatal(
			"technical failure does not contain an error detail",
		)
	}

	workflowFailure, exists :=
		result.WorkflowFailure()

	if !exists {
		t.Fatal(
			"failed workflow does not contain a workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryExecution {
		t.Fatalf(
			"workflow failure category = %q, want EXECUTION",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowNodeFailure {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowNodeFailure,
		)
	}

	terminalOutput, exists, err :=
		result.TerminalOutput(
			workflow.NodeID("e-terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(e-terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"healthy independent branch terminal output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		terminalOutput,
	); actual != "healthy-result" {
		t.Fatalf(
			"healthy terminal output = %q, want %q",
			actual,
			"healthy-result",
		)
	}
}

func TestRunPreparedExecutionCollectsMultipleTerminalOutputs(
	t *testing.T,
) {
	prepared :=
		mustSchedulerMultipleTerminalPreparedExecution(
			t,
		)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				22,
				12,
				0,
				0,
				0,
				time.UTC,
			),
		}

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executorOrder := make(
		[]workflow.NodeID,
		0,
		3,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			switch nodeID {
			case workflow.NodeID("source"):
				if !input.IsValid() {
					t.Fatal(
						"source received invalid input",
					)
				}

				if !input.IsEmpty() {
					t.Fatal(
						"source received non-empty input",
					)
				}

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"fan-out-value",
								map[string]string{
									"node": "source",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("terminal-a"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"fan-out-value",
				)

				return mustRoutingTerminalResult(
					t,
					mustRoutingPayload(
						t,
						"terminal-a-result",
						map[string]string{
							"node": "terminal-a",
						},
					),
				), nil

			case workflow.NodeID("terminal-b"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"fan-out-value",
				)

				return mustRoutingTerminalResult(
					t,
					mustRoutingPayload(
						t,
						"terminal-b-result",
						map[string]string{
							"node": "terminal-b",
						},
					),
				), nil

			default:
				return runtime.NodeResult{},
					fmt.Errorf(
						"unexpected scheduler node %s",
						nodeID,
					)
			}
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		prepared,
		executor,
	)

	result, err :=
		runPreparedExecution(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"runPreparedExecution() returned an unexpected error: %v",
			err,
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("source"),
		workflow.NodeID("terminal-a"),
		workflow.NodeID("terminal-b"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		result.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"recorded execution order = %#v, want %#v",
			result.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if !result.IsSucceeded() {
		t.Fatalf(
			"workflow status = %q, want SUCCEEDED",
			result.Status(),
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"multiple-terminal workflow is marked as failed",
		)
	}

	if result.IsStalled() {
		t.Fatal(
			"multiple-terminal workflow is marked as stalled",
		)
	}

	for _, nodeID := range expectedOrder {
		if executorCalls[nodeID] != 1 {
			t.Fatalf(
				"node %q executor call count = %d, want 1",
				nodeID,
				executorCalls[nodeID],
			)
		}

		requireNodeExecutionStatus(
			t,
			prepared,
			nodeID,
			execution.NodeExecutionStatusSucceeded,
		)
	}

	terminalOutputs :=
		result.TerminalOutputs()

	if len(terminalOutputs) != 2 {
		t.Fatalf(
			"terminal output count = %d, want 2",
			len(terminalOutputs),
		)
	}

	terminalA, exists, err :=
		result.TerminalOutput(
			workflow.NodeID("terminal-a"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(terminal-a) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"terminal-a output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		terminalA,
	); actual != "terminal-a-result" {
		t.Fatalf(
			"terminal-a output = %q, want %q",
			actual,
			"terminal-a-result",
		)
	}

	if actual :=
		terminalA.Metadata()["node"]; actual != "terminal-a" {
		t.Fatalf(
			"terminal-a metadata node = %q, want %q",
			actual,
			"terminal-a",
		)
	}

	terminalB, exists, err :=
		result.TerminalOutput(
			workflow.NodeID("terminal-b"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(terminal-b) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"terminal-b output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		terminalB,
	); actual != "terminal-b-result" {
		t.Fatalf(
			"terminal-b output = %q, want %q",
			actual,
			"terminal-b-result",
		)
	}

	if actual :=
		terminalB.Metadata()["node"]; actual != "terminal-b" {
		t.Fatalf(
			"terminal-b metadata node = %q, want %q",
			actual,
			"terminal-b",
		)
	}

	if failures := result.NodeFailures(); failures != nil {
		t.Fatalf(
			"node failures = %#v, want nil",
			failures,
		)
	}

	if _, exists := result.WorkflowFailure(); exists {
		t.Fatal(
			"successful multiple-terminal workflow contains a workflow failure",
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-terminal-a",
	).Len(); actual != 0 {
		t.Fatalf(
			"terminal-a edge queue length = %d, want 0",
			actual,
		)
	}

	if actual := schedulerEdgeQueue(
		t,
		prepared,
		"edge-source-terminal-b",
	).Len(); actual != 0 {
		t.Fatalf(
			"terminal-b edge queue length = %d, want 0",
			actual,
		)
	}
}

func mustSchedulerMultipleTerminalPreparedExecution(
	t *testing.T,
) *preparedExecution {
	t.Helper()

	const sourcePluginType workflow.PluginType = "test.scheduler-multiple-terminal-source"

	const terminalPluginType workflow.PluginType = "test.scheduler-multiple-terminal-sink"

	const version workflow.PluginVersion = "v1"

	sourceDescriptor := mustValidationDescriptor(
		t,
		sourcePluginType,
		version,
		plugin.DistributionDistributable,
		nil,
		[]string{
			"output",
		},
		plugin.NewExactEdgeConstraint(0),
		plugin.NewExactEdgeConstraint(2),
	)

	terminalDescriptor := mustValidationDescriptor(
		t,
		terminalPluginType,
		version,
		plugin.DistributionDistributable,
		[]string{
			"input",
		},
		nil,
		plugin.NewExactEdgeConstraint(1),
		plugin.NewExactEdgeConstraint(0),
	)

	pluginRegistry, err := plugin.NewRegistry(
		[]plugin.Descriptor{
			terminalDescriptor,
			sourceDescriptor,
		},
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	placeholderExecutor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			return runtime.NewNodeSuccessResult(
				nil,
				runtime.ContextChanges{},
			)
		},
	)

	executorRegistry, err := runtime.NewExecutorRegistry(
		[]runtime.ExecutorRegistration{
			mustValidationExecutorRegistration(
				t,
				sourceDescriptor.Identity(),
				placeholderExecutor,
			),
			mustValidationExecutorRegistration(
				t,
				terminalDescriptor.Identity(),
				placeholderExecutor,
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	limits := mustValidationRuntimeLimits(
		t,
		1,
		1,
		4096,
	)

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		&preparationSequenceClock{
			times: preparationTestTimes(),
		},
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	source := mustValidationNode(
		t,
		"source",
		sourcePluginType.String(),
		version.String(),
		`{}`,
	)

	terminalA := mustValidationNode(
		t,
		"terminal-a",
		terminalPluginType.String(),
		version.String(),
		`{}`,
	)

	terminalB := mustValidationNode(
		t,
		"terminal-b",
		terminalPluginType.String(),
		version.String(),
		`{}`,
	)

	edgeA := mustValidationEdge(
		t,
		"edge-source-terminal-a",
		"source",
		"output",
		"terminal-a",
		"input",
	)

	edgeB := mustValidationEdge(
		t,
		"edge-source-terminal-b",
		"source",
		"output",
		"terminal-b",
		"input",
	)

	definition := mustValidationDefinition(
		t,
		"workflow-scheduler-multiple-terminal",
		[]workflow.NodeDefinition{
			source,
			terminalA,
			terminalB,
		},
		[]workflow.EdgeDefinition{
			edgeA,
			edgeB,
		},
	)

	preparation, err := prepareExecution(
		context.Background(),
		mustValidationExecutionRequest(
			t,
			definition,
		),
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"prepareExecution() returned an unexpected error: %v",
			err,
		)
	}

	if !preparation.IsPrepared() {
		t.Fatalf(
			"multiple-terminal workflow was not prepared; validation issues = %d",
			preparation.ValidationReport().Len(),
		)
	}

	if preparation.prepared == nil {
		t.Fatal(
			"prepared multiple-terminal workflow is nil",
		)
	}

	return preparation.prepared
}

type schedulerTerminalizationRecorder struct {
	nodeTransitions []NodeTransitionObservation

	failAt  int
	failErr error
}

var _ LifecycleRecorder = (*schedulerTerminalizationRecorder)(nil)

func (recorder *schedulerTerminalizationRecorder) IsValid() bool {
	return recorder != nil
}

func (
	*schedulerTerminalizationRecorder,
) RecordWorkflowCreation(
	ctx context.Context,
	_ WorkflowCreationObservation,
) error {
	return schedulerTerminalizationContextError(
		ctx,
	)
}

func (
	*schedulerTerminalizationRecorder,
) RecordWorkflowTransition(
	ctx context.Context,
	_ WorkflowTransitionObservation,
) error {
	return schedulerTerminalizationContextError(
		ctx,
	)
}

func (
	*schedulerTerminalizationRecorder,
) RecordNodeExecutionsCreation(
	ctx context.Context,
	_ NodeExecutionsCreationObservation,
) error {
	return schedulerTerminalizationContextError(
		ctx,
	)
}

func (
	recorder *schedulerTerminalizationRecorder,
) RecordNodeTransition(
	ctx context.Context,
	observation NodeTransitionObservation,
) error {
	if err :=
		schedulerTerminalizationContextError(
			ctx,
		); err != nil {
		return err
	}

	recorder.nodeTransitions = append(
		recorder.nodeTransitions,
		observation,
	)

	if recorder.failAt > 0 &&
		len(recorder.nodeTransitions) ==
			recorder.failAt {
		if recorder.failErr != nil {
			return recorder.failErr
		}

		return errors.New(
			"controlled scheduler terminalization persistence failure",
		)
	}

	return nil
}

func schedulerTerminalizationContextError(
	ctx context.Context,
) error {
	if ctx == nil {
		return errors.New(
			"scheduler terminalization context must not be nil",
		)
	}

	return ctx.Err()
}

func TestSkipBlockedPendingNodesRecordsSkippedTransitions(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		5,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedSchedulerTerminalizationTestExecution(
			t,
			baseTime,
		)
	defer cancel()

	sourceID :=
		workflow.NodeID("source")

	sourceExecution :=
		prepared.nodeExecutionsByNode[sourceID]
	if sourceExecution == nil {
		t.Fatal(
			"source node execution is nil",
		)
	}

	if err := sourceExecution.MarkReady(
		baseTime.Add(10 * time.Minute),
	); err != nil {
		t.Fatalf(
			"mark source ready: %v",
			err,
		)
	}

	if err := sourceExecution.Start(
		baseTime.Add(11 * time.Minute),
	); err != nil {
		t.Fatalf(
			"start source: %v",
			err,
		)
	}

	if err := sourceExecution.Fail(
		baseTime.Add(12 * time.Minute),
	); err != nil {
		t.Fatalf(
			"fail source: %v",
			err,
		)
	}

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime.Add(
				20 * time.Minute,
			),
		}

	skippedCount, err :=
		skipBlockedPendingNodes(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"skipBlockedPendingNodes() returned an error: %v",
			err,
		)
	}

	if skippedCount != 2 {
		t.Fatalf(
			"skipped count = %d, want 2",
			skippedCount,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusSkipped,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusSkipped,
	)

	if len(recorder.nodeTransitions) != 2 {
		t.Fatalf(
			"recorded transition count = %d, want 2",
			len(recorder.nodeTransitions),
		)
	}

	for index, observation := range recorder.nodeTransitions {
		if observation.Before.Status() !=
			execution.NodeExecutionStatusPending {
			t.Fatalf(
				"transition[%d] previous status = %q, want PENDING",
				index,
				observation.Before.Status(),
			)
		}

		if observation.After.Status() !=
			execution.NodeExecutionStatusSkipped {
			t.Fatalf(
				"transition[%d] target status = %q, want SKIPPED",
				index,
				observation.After.Status(),
			)
		}

		if observation.HasFailure {
			t.Fatalf(
				"transition[%d] unexpectedly contains failure",
				index,
			)
		}
	}
}

func TestTerminalizeRemainingNodesForCancellationRecordsAllNodes(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		6,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedSchedulerTerminalizationTestExecution(
			t,
			baseTime,
		)

	cancel()

	err :=
		terminalizeRemainingNodesForContext(
			prepared,
			context.Canceled,
		)
	if err != nil {
		t.Fatalf(
			"terminalizeRemainingNodesForContext() returned an error: %v",
			err,
		)
	}

	expectedNodeIDs := []workflow.NodeID{
		workflow.NodeID("source"),
		workflow.NodeID("pass"),
		workflow.NodeID("terminal"),
	}

	if len(recorder.nodeTransitions) !=
		len(expectedNodeIDs) {
		t.Fatalf(
			"recorded transition count = %d, want %d",
			len(recorder.nodeTransitions),
			len(expectedNodeIDs),
		)
	}

	for index, nodeID := range expectedNodeIDs {
		requireNodeExecutionStatus(
			t,
			prepared,
			nodeID,
			execution.NodeExecutionStatusCancelled,
		)

		observation :=
			recorder.nodeTransitions[index]

		if observation.Definition.ID() !=
			nodeID {
			t.Fatalf(
				"transition[%d] node ID = %q, want %q",
				index,
				observation.Definition.ID(),
				nodeID,
			)
		}

		if observation.After.Status() !=
			execution.NodeExecutionStatusCancelled {
			t.Fatalf(
				"transition[%d] target status = %q, want CANCELLED",
				index,
				observation.After.Status(),
			)
		}

		if !observation.HasFailure {
			t.Fatalf(
				"transition[%d] does not contain cancellation failure",
				index,
			)
		}

		if observation.Failure.Category() !=
			runtime.FailureCategoryCanceled {
			t.Fatalf(
				"transition[%d] category = %q, want CANCELED",
				index,
				observation.Failure.Category(),
			)
		}
	}
}

func TestTerminalizeRemainingNodesForTimeoutRecordsReadyAndTimedOut(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		7,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedSchedulerTerminalizationTestExecution(
			t,
			baseTime,
		)

	cancel()

	err :=
		terminalizeRemainingNodesForContext(
			prepared,
			context.DeadlineExceeded,
		)
	if err != nil {
		t.Fatalf(
			"terminalizeRemainingNodesForContext() returned an error: %v",
			err,
		)
	}

	expectedNodeIDs := []workflow.NodeID{
		workflow.NodeID("source"),
		workflow.NodeID("pass"),
		workflow.NodeID("terminal"),
	}

	expectedTransitionCount :=
		len(expectedNodeIDs) * 2

	if len(recorder.nodeTransitions) !=
		expectedTransitionCount {
		t.Fatalf(
			"recorded transition count = %d, want %d",
			len(recorder.nodeTransitions),
			expectedTransitionCount,
		)
	}

	for index, nodeID := range expectedNodeIDs {
		requireNodeExecutionStatus(
			t,
			prepared,
			nodeID,
			execution.NodeExecutionStatusTimedOut,
		)

		readyObservation :=
			recorder.nodeTransitions[index*2]

		timeoutObservation :=
			recorder.nodeTransitions[index*2+1]

		if readyObservation.Before.Status() !=
			execution.NodeExecutionStatusPending {
			t.Fatalf(
				"node %s READY previous status = %q, want PENDING",
				nodeID,
				readyObservation.Before.Status(),
			)
		}

		if readyObservation.After.Status() !=
			execution.NodeExecutionStatusReady {
			t.Fatalf(
				"node %s first target status = %q, want READY",
				nodeID,
				readyObservation.After.Status(),
			)
		}

		if readyObservation.HasFailure {
			t.Fatalf(
				"node %s READY transition unexpectedly contains failure",
				nodeID,
			)
		}

		if timeoutObservation.Before.Status() !=
			execution.NodeExecutionStatusReady {
			t.Fatalf(
				"node %s timeout previous status = %q, want READY",
				nodeID,
				timeoutObservation.Before.Status(),
			)
		}

		if timeoutObservation.After.Status() !=
			execution.NodeExecutionStatusTimedOut {
			t.Fatalf(
				"node %s timeout target status = %q, want TIMED_OUT",
				nodeID,
				timeoutObservation.After.Status(),
			)
		}

		if !timeoutObservation.HasFailure {
			t.Fatalf(
				"node %s timeout transition does not contain failure",
				nodeID,
			)
		}

		if timeoutObservation.Failure.Category() !=
			runtime.FailureCategoryTimeout {
			t.Fatalf(
				"node %s timeout category = %q, want TIMEOUT",
				nodeID,
				timeoutObservation.Failure.Category(),
			)
		}
	}
}

func TestTerminalizeRemainingNodesForFailureRecordsSkippedAndFailed(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		8,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedSchedulerTerminalizationTestExecution(
			t,
			baseTime,
		)
	defer cancel()

	sourceID :=
		workflow.NodeID("source")

	sourceExecution :=
		prepared.nodeExecutionsByNode[sourceID]
	if sourceExecution == nil {
		t.Fatal(
			"source node execution is nil",
		)
	}

	if err := sourceExecution.MarkReady(
		baseTime.Add(10 * time.Minute),
	); err != nil {
		t.Fatalf(
			"mark source ready: %v",
			err,
		)
	}

	if err := sourceExecution.Start(
		baseTime.Add(11 * time.Minute),
	); err != nil {
		t.Fatalf(
			"start source: %v",
			err,
		)
	}

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime.Add(
				20 * time.Minute,
			),
		}

	err :=
		terminalizeRemainingNodesForFailure(
			prepared,
		)
	if err != nil {
		t.Fatalf(
			"terminalizeRemainingNodesForFailure() returned an error: %v",
			err,
		)
	}

	requireNodeExecutionStatus(
		t,
		prepared,
		sourceID,
		execution.NodeExecutionStatusFailed,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusSkipped,
	)

	requireNodeExecutionStatus(
		t,
		prepared,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusSkipped,
	)

	if len(recorder.nodeTransitions) != 3 {
		t.Fatalf(
			"recorded transition count = %d, want 3",
			len(recorder.nodeTransitions),
		)
	}

	sourceObservation :=
		recorder.nodeTransitions[0]

	if sourceObservation.Before.Status() !=
		execution.NodeExecutionStatusRunning {
		t.Fatalf(
			"source previous status = %q, want RUNNING",
			sourceObservation.Before.Status(),
		)
	}

	if sourceObservation.After.Status() !=
		execution.NodeExecutionStatusFailed {
		t.Fatalf(
			"source target status = %q, want FAILED",
			sourceObservation.After.Status(),
		)
	}

	if !sourceObservation.HasFailure {
		t.Fatal(
			"failed source transition does not contain failure",
		)
	}

	if sourceObservation.Failure.Category() !=
		runtime.FailureCategoryInternal {
		t.Fatalf(
			"source failure category = %q, want INTERNAL",
			sourceObservation.Failure.Category(),
		)
	}

	if sourceObservation.Failure.Code() !=
		failureCodeNodeSchedulerTerminalized {
		t.Fatalf(
			"source failure code = %q, want %q",
			sourceObservation.Failure.Code(),
			failureCodeNodeSchedulerTerminalized,
		)
	}

	for index := 1; index < len(recorder.nodeTransitions); index++ {
		observation :=
			recorder.nodeTransitions[index]

		if observation.After.Status() !=
			execution.NodeExecutionStatusSkipped {
			t.Fatalf(
				"transition[%d] target status = %q, want SKIPPED",
				index,
				observation.After.Status(),
			)
		}

		if observation.HasFailure {
			t.Fatalf(
				"transition[%d] unexpectedly contains failure",
				index,
			)
		}
	}
}

func TestTerminalizeRemainingNodesDoesNotMutateWhenPersistenceFails(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		9,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedSchedulerTerminalizationTestExecution(
			t,
			baseTime,
		)
	defer cancel()

	sentinel := errors.New(
		"controlled terminalization persistence failure",
	)

	recorder.failAt = 1
	recorder.failErr = sentinel

	err :=
		terminalizeRemainingNodesForFailure(
			prepared,
		)

	if !errors.Is(err, sentinel) {
		t.Fatalf(
			"terminalizeRemainingNodesForFailure() error = %v, want sentinel",
			err,
		)
	}

	expectedNodeIDs := []workflow.NodeID{
		workflow.NodeID("source"),
		workflow.NodeID("pass"),
		workflow.NodeID("terminal"),
	}

	for _, nodeID := range expectedNodeIDs {
		requireNodeExecutionStatus(
			t,
			prepared,
			nodeID,
			execution.NodeExecutionStatusPending,
		)
	}

	if len(recorder.nodeTransitions) != 1 {
		t.Fatalf(
			"recorded transition attempt count = %d, want 1",
			len(recorder.nodeTransitions),
		)
	}
}

func newPreparedSchedulerTerminalizationTestExecution(
	t *testing.T,
	baseTime time.Time,
) (
	*preparedExecution,
	*schedulerTerminalizationRecorder,
	context.CancelFunc,
) {
	t.Helper()

	limits :=
		mustValidationRuntimeLimits(
			t,
			64,
			1,
			4096,
		)

	dependencies :=
		mustValidationCoreDependencies(
			t,
			limits,
			true,
		)

	dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime,
		}

	recorder :=
		&schedulerTerminalizationRecorder{}

	updatedDependencies, err :=
		dependencies.WithLifecycleRecorder(
			recorder,
		)
	if err != nil {
		t.Fatalf(
			"WithLifecycleRecorder() returned an error: %v",
			err,
		)
	}

	parent, cancel :=
		context.WithCancel(
			context.Background(),
		)

	request :=
		mustValidationExecutionRequest(
			t,
			mustValidationLinearWorkflow(
				t,
				"source",
				"pass",
				"terminal",
			),
		)

	preparation, err :=
		prepareExecution(
			parent,
			request,
			updatedDependencies,
		)
	if err != nil {
		cancel()

		t.Fatalf(
			"prepareExecution() returned an error: %v",
			err,
		)
	}

	if !preparation.IsPrepared() ||
		preparation.prepared == nil {
		cancel()

		t.Fatal(
			"test workflow was not prepared",
		)
	}

	recorder.nodeTransitions = nil

	return preparation.prepared,
		recorder,
		cancel
}

func TestSyncRunnerReturnsCancelledExecutionWhenParentContextIsCancelled(
	t *testing.T,
) {
	fixture :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	fixture.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				26,
				10,
				0,
				0,
				0,
				time.UTC,
			),
		}

	signalContext :=
		newNodeExecutionSignalContext(
			false,
		)

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executorOrder := make(
		[]workflow.NodeID,
		0,
		1,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			if nodeID !=
				workflow.NodeID("source") {
				t.Fatalf(
					"executor was called for node %q after cancellation",
					nodeID,
				)
			}

			if !input.IsValid() {
				t.Fatal(
					"source received an invalid input",
				)
			}

			if !input.IsEmpty() {
				t.Fatal(
					"source received a non-empty input",
				)
			}

			if nodeContext.Err() != nil {
				t.Fatalf(
					"node context already contained an error before cancellation: %v",
					nodeContext.Err(),
				)
			}

			signalContext.Fail(
				context.Canceled,
			)

			return mustRoutingSuccessResult(
				t,
				map[string][]runtime.Payload{
					"output": {
						mustRoutingPayload(
							t,
							"must-not-be-routed",
							map[string]string{
								"node": "source",
							},
						),
					},
				},
			), nil
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		fixture,
		executor,
	)

	runner, err := NewSyncRunner(
		fixture.dependencies,
	)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	result, err := runner.Run(
		signalContext,
		fixture.request,
	)
	if err != nil {
		t.Fatalf(
			"SyncRunner.Run() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"cancelled SyncRunResult is invalid",
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"cancelled SyncRunResult is not terminal",
		)
	}

	if !result.IsCancelled() {
		t.Fatalf(
			"sync-run status = %q, want CANCELLED",
			result.Status(),
		)
	}

	if result.Status() !=
		execution.WorkflowExecutionStatusCancelled {
		t.Fatalf(
			"workflow status = %q, want %q",
			result.Status(),
			execution.WorkflowExecutionStatusCancelled,
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"cancelled execution is marked as rejected",
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"cancelled execution is marked as succeeded",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"cancelled execution is marked as failed",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"cancelled execution is marked as timed out",
		)
	}

	if !result.ValidationReport().IsValid() {
		t.Fatal(
			"cancelled execution contains an invalid preflight report",
		)
	}

	if !result.HasExecutionResult() {
		t.Fatal(
			"cancelled SyncRunResult contains no ExecutionResult",
		)
	}

	executionResult, exists :=
		result.ExecutionResult()

	if !exists {
		t.Fatal(
			"ExecutionResult() exists = false",
		)
	}

	if !executionResult.IsTerminal() {
		t.Fatal(
			"cancelled ExecutionResult is not terminal",
		)
	}

	if !executionResult.IsCancelled() {
		t.Fatalf(
			"execution-result status = %q, want CANCELLED",
			executionResult.Status(),
		)
	}

	if executionResult.IsStalled() {
		t.Fatal(
			"cancelled execution is marked as stalled",
		)
	}

	if executionResult.Status() !=
		result.Status() {
		t.Fatalf(
			"execution-result status = %q, sync-run status = %q",
			executionResult.Status(),
			result.Status(),
		)
	}

	if executionResult.WorkflowExecution().ID() !=
		result.WorkflowExecution().ID() {
		t.Fatalf(
			"execution-result workflow ID = %q, sync-run workflow ID = %q",
			executionResult.WorkflowExecution().ID(),
			result.WorkflowExecution().ID(),
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("source"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		executionResult.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"recorded execution order = %#v, want %#v",
			executionResult.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if executorCalls[workflow.NodeID("source")] != 1 {
		t.Fatalf(
			"source executor call count = %d, want 1",
			executorCalls[workflow.NodeID("source")],
		)
	}

	if executorCalls[workflow.NodeID("pass")] != 0 {
		t.Fatalf(
			"pass executor call count = %d, want 0",
			executorCalls[workflow.NodeID("pass")],
		)
	}

	if executorCalls[workflow.NodeID("terminal")] != 0 {
		t.Fatalf(
			"terminal executor call count = %d, want 0",
			executorCalls[workflow.NodeID("terminal")],
		)
	}

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusCancelled,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusCancelled,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusCancelled,
	)

	requireSyncRunnerStartedAndFinishedNode(
		t,
		executionResult,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusCancelled,
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		executionResult,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusCancelled,
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		executionResult,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusCancelled,
	)

	workflowFailure, exists :=
		executionResult.WorkflowFailure()

	if !exists {
		t.Fatal(
			"cancelled execution contains no workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryCanceled {
		t.Fatalf(
			"workflow failure category = %q, want CANCELED",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowCanceled {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowCanceled,
		)
	}

	if actual :=
		workflowFailure.Details()["contextError"]; actual != context.Canceled.Error() {
		t.Fatalf(
			"contextError detail = %q, want %q",
			actual,
			context.Canceled.Error(),
		)
	}

	if failures :=
		executionResult.NodeFailures(); failures != nil {
		t.Fatalf(
			"cancelled execution node failures = %#v, want nil",
			failures,
		)
	}

	if outputs :=
		executionResult.TerminalOutputs(); outputs != nil {
		t.Fatalf(
			"cancelled execution terminal outputs = %#v, want nil",
			outputs,
		)
	}
}

func TestSyncRunnerReturnsTimedOutExecutionWhenParentDeadlineExpires(
	t *testing.T,
) {
	fixture :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	fixture.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				26,
				12,
				0,
				0,
				0,
				time.UTC,
			),
		}

	signalContext :=
		newNodeExecutionSignalContext(
			true,
		)

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executorOrder := make(
		[]workflow.NodeID,
		0,
		1,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			if nodeID !=
				workflow.NodeID("source") {
				t.Fatalf(
					"executor was called for node %q after timeout",
					nodeID,
				)
			}

			deadline, exists :=
				nodeContext.Deadline()

			if !exists {
				t.Fatal(
					"node context does not expose a deadline",
				)
			}

			if deadline.IsZero() {
				t.Fatal(
					"node context deadline is zero",
				)
			}

			if !input.IsValid() {
				t.Fatal(
					"source received an invalid input",
				)
			}

			if !input.IsEmpty() {
				t.Fatal(
					"source received a non-empty input",
				)
			}

			signalContext.Fail(
				context.DeadlineExceeded,
			)

			return mustRoutingSuccessResult(
				t,
				map[string][]runtime.Payload{
					"output": {
						mustRoutingPayload(
							t,
							"must-not-be-routed",
							map[string]string{
								"node": "source",
							},
						),
					},
				},
			), nil
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		fixture,
		executor,
	)

	runner, err := NewSyncRunner(
		fixture.dependencies,
	)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	result, err := runner.Run(
		signalContext,
		fixture.request,
	)
	if err != nil {
		t.Fatalf(
			"SyncRunner.Run() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"timed-out SyncRunResult is invalid",
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"timed-out SyncRunResult is not terminal",
		)
	}

	if !result.IsTimedOut() {
		t.Fatalf(
			"sync-run status = %q, want TIMED_OUT",
			result.Status(),
		)
	}

	if result.Status() !=
		execution.WorkflowExecutionStatusTimedOut {
		t.Fatalf(
			"workflow status = %q, want %q",
			result.Status(),
			execution.WorkflowExecutionStatusTimedOut,
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"timed-out execution is marked as rejected",
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"timed-out execution is marked as succeeded",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"timed-out execution is marked as failed",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"timed-out execution is marked as cancelled",
		)
	}

	if !result.ValidationReport().IsValid() {
		t.Fatal(
			"timed-out execution contains an invalid preflight report",
		)
	}

	if !result.HasExecutionResult() {
		t.Fatal(
			"timed-out SyncRunResult contains no ExecutionResult",
		)
	}

	executionResult, exists :=
		result.ExecutionResult()

	if !exists {
		t.Fatal(
			"ExecutionResult() exists = false",
		)
	}

	if !executionResult.IsTerminal() {
		t.Fatal(
			"timed-out ExecutionResult is not terminal",
		)
	}

	if !executionResult.IsTimedOut() {
		t.Fatalf(
			"execution-result status = %q, want TIMED_OUT",
			executionResult.Status(),
		)
	}

	if executionResult.IsStalled() {
		t.Fatal(
			"timed-out execution is marked as stalled",
		)
	}

	if executionResult.Status() !=
		result.Status() {
		t.Fatalf(
			"execution-result status = %q, sync-run status = %q",
			executionResult.Status(),
			result.Status(),
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("source"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		executionResult.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"recorded execution order = %#v, want %#v",
			executionResult.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if executorCalls[workflow.NodeID("source")] != 1 {
		t.Fatalf(
			"source executor call count = %d, want 1",
			executorCalls[workflow.NodeID("source")],
		)
	}

	if executorCalls[workflow.NodeID("pass")] != 0 {
		t.Fatalf(
			"pass executor call count = %d, want 0",
			executorCalls[workflow.NodeID("pass")],
		)
	}

	if executorCalls[workflow.NodeID("terminal")] != 0 {
		t.Fatalf(
			"terminal executor call count = %d, want 0",
			executorCalls[workflow.NodeID("terminal")],
		)
	}

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireSyncRunnerStartedAndFinishedNode(
		t,
		executionResult,
		workflow.NodeID("source"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		executionResult,
		workflow.NodeID("pass"),
		execution.NodeExecutionStatusTimedOut,
	)

	requireSchedulerNodeFinishedWithoutStart(
		t,
		executionResult,
		workflow.NodeID("terminal"),
		execution.NodeExecutionStatusTimedOut,
	)

	workflowFailure, exists :=
		executionResult.WorkflowFailure()

	if !exists {
		t.Fatal(
			"timed-out execution contains no workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryTimeout {
		t.Fatalf(
			"workflow failure category = %q, want TIMEOUT",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowTimedOut {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowTimedOut,
		)
	}

	if actual :=
		workflowFailure.Details()["contextError"]; actual != context.DeadlineExceeded.Error() {
		t.Fatalf(
			"contextError detail = %q, want %q",
			actual,
			context.DeadlineExceeded.Error(),
		)
	}

	if failures :=
		executionResult.NodeFailures(); failures != nil {
		t.Fatalf(
			"timed-out execution node failures = %#v, want nil",
			failures,
		)
	}

	if outputs :=
		executionResult.TerminalOutputs(); outputs != nil {
		t.Fatalf(
			"timed-out execution terminal outputs = %#v, want nil",
			outputs,
		)
	}
}

func requireSyncRunnerStartedAndFinishedNode(
	t *testing.T,
	result ExecutionResult,
	nodeID workflow.NodeID,
	expectedStatus execution.NodeExecutionStatus,
) {
	t.Helper()

	nodeExecution, exists, err :=
		result.NodeExecution(
			nodeID,
		)
	if err != nil {
		t.Fatalf(
			"NodeExecution(%s) returned an unexpected error: %v",
			nodeID,
			err,
		)
	}

	if !exists {
		t.Fatalf(
			"node execution %q does not exist",
			nodeID,
		)
	}

	if nodeExecution.Status() !=
		expectedStatus {
		t.Fatalf(
			"node %q status = %q, want %q",
			nodeID,
			nodeExecution.Status(),
			expectedStatus,
		)
	}

	startedAt, started :=
		nodeExecution.StartedAt()

	if !started || startedAt.IsZero() {
		t.Fatalf(
			"node %q does not contain startedAt",
			nodeID,
		)
	}

	finishedAt, finished :=
		nodeExecution.FinishedAt()

	if !finished || finishedAt.IsZero() {
		t.Fatalf(
			"node %q does not contain finishedAt",
			nodeID,
		)
	}

	if finishedAt.Before(startedAt) {
		t.Fatalf(
			"node %q finishedAt %v is before startedAt %v",
			nodeID,
			finishedAt,
			startedAt,
		)
	}
}

func TestSyncRunnerReturnsFailedExecutionForControlledNodeFailure(
	t *testing.T,
) {
	fixture :=
		mustSchedulerBranchFailurePreparedExecution(
			t,
		)

	fixture.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				25,
				10,
				0,
				0,
				0,
				time.UTC,
			),
		}

	executorOrder := make(
		[]workflow.NodeID,
		0,
		4,
	)

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			switch nodeID {
			case workflow.NodeID("a-source"):
				if !input.IsValid() {
					t.Fatal(
						"source received an invalid input",
					)
				}

				if !input.IsEmpty() {
					t.Fatal(
						"source received a non-empty input",
					)
				}

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"source-value",
								map[string]string{
									"node": "a-source",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("b-failed"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				failure, err :=
					runtime.NewRuntimeFailure(
						runtime.FailureCategoryExecution,
						"SYNC_CONTROLLED_FAILURE",
						"Controlled failure through SyncRunner",
						false,
						map[string]string{
							"nodeID": nodeID.String(),
						},
					)
				if err != nil {
					t.Fatalf(
						"runtime.NewRuntimeFailure() returned an unexpected error: %v",
						err,
					)
				}

				return runtime.NewNodeFailureResult(
					failure,
				)

			case workflow.NodeID("c-descendant"):
				t.Fatal(
					"blocked descendant executor must not be called",
				)

				return runtime.NodeResult{},
					fmt.Errorf(
						"blocked descendant was executed",
					)

			case workflow.NodeID("d-healthy"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"healthy-value",
								map[string]string{
									"node": "d-healthy",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("e-terminal"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"healthy-value",
				)

				return mustRoutingTerminalResult(
					t,
					mustRoutingPayload(
						t,
						"healthy-terminal-result",
						map[string]string{
							"node": "e-terminal",
						},
					),
				), nil

			default:
				return runtime.NodeResult{},
					fmt.Errorf(
						"unexpected SyncRunner node %s",
						nodeID,
					)
			}
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		fixture,
		executor,
	)

	runner, err := NewSyncRunner(
		fixture.dependencies,
	)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	result, err := runner.Run(
		context.Background(),
		fixture.request,
	)
	if err != nil {
		t.Fatalf(
			"SyncRunner.Run() returned an unexpected technical error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"controlled-failure SyncRunResult is invalid",
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"controlled-failure SyncRunResult is not terminal",
		)
	}

	if !result.IsFailed() {
		t.Fatalf(
			"sync-run status = %q, want FAILED",
			result.Status(),
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"runtime failure was incorrectly marked as rejected",
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"controlled-failure result is marked as succeeded",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"controlled-failure result is marked as cancelled",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"controlled-failure result is marked as timed out",
		)
	}

	if !result.ValidationReport().IsValid() {
		t.Fatal(
			"runtime failure contains an invalid preflight report",
		)
	}

	if !result.HasExecutionResult() {
		t.Fatal(
			"runtime failure contains no ExecutionResult",
		)
	}

	executionResult, exists :=
		result.ExecutionResult()

	if !exists {
		t.Fatal(
			"ExecutionResult() exists = false",
		)
	}

	if !executionResult.IsFailed() {
		t.Fatalf(
			"execution-result status = %q, want FAILED",
			executionResult.Status(),
		)
	}

	if executionResult.IsStalled() {
		t.Fatal(
			"controlled node failure is marked as stalled",
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("a-source"),
		workflow.NodeID("b-failed"),
		workflow.NodeID("d-healthy"),
		workflow.NodeID("e-terminal"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		executionResult.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"recorded execution order = %#v, want %#v",
			executionResult.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if executorCalls[workflow.NodeID("c-descendant")] != 0 {
		t.Fatalf(
			"c-descendant executor call count = %d, want 0",
			executorCalls[workflow.NodeID("c-descendant")],
		)
	}

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("a-source"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("b-failed"),
		execution.NodeExecutionStatusFailed,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("c-descendant"),
		execution.NodeExecutionStatusSkipped,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("d-healthy"),
		execution.NodeExecutionStatusSucceeded,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("e-terminal"),
		execution.NodeExecutionStatusSucceeded,
	)

	nodeFailure, exists, err :=
		executionResult.NodeFailure(
			workflow.NodeID("b-failed"),
		)
	if err != nil {
		t.Fatalf(
			"NodeFailure(b-failed) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"controlled node failure does not exist",
		)
	}

	if nodeFailure.Category() !=
		runtime.FailureCategoryExecution {
		t.Fatalf(
			"node failure category = %q, want EXECUTION",
			nodeFailure.Category(),
		)
	}

	if nodeFailure.Code() !=
		"SYNC_CONTROLLED_FAILURE" {
		t.Fatalf(
			"node failure code = %q, want %q",
			nodeFailure.Code(),
			"SYNC_CONTROLLED_FAILURE",
		)
	}

	if actual :=
		nodeFailure.Details()["nodeID"]; actual != "b-failed" {
		t.Fatalf(
			"node failure nodeID = %q, want %q",
			actual,
			"b-failed",
		)
	}

	workflowFailure, exists :=
		executionResult.WorkflowFailure()

	if !exists {
		t.Fatal(
			"failed execution contains no workflow failure",
		)
	}

	if workflowFailure.Category() !=
		runtime.FailureCategoryExecution {
		t.Fatalf(
			"workflow failure category = %q, want EXECUTION",
			workflowFailure.Category(),
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowNodeFailure {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowNodeFailure,
		)
	}

	terminalOutput, exists, err :=
		executionResult.TerminalOutput(
			workflow.NodeID("e-terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(e-terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"healthy terminal output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		terminalOutput,
	); actual != "healthy-terminal-result" {
		t.Fatalf(
			"healthy terminal output = %q, want %q",
			actual,
			"healthy-terminal-result",
		)
	}
}

func TestSyncRunnerConvertsTechnicalExecutorErrorToNodeFailure(
	t *testing.T,
) {
	fixture :=
		mustSchedulerBranchFailurePreparedExecution(
			t,
		)

	fixture.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				25,
				12,
				0,
				0,
				0,
				time.UTC,
			),
		}

	technicalError := errors.New(
		"sync executor dependency unavailable",
	)

	executorOrder := make(
		[]workflow.NodeID,
		0,
		4,
	)

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			switch nodeID {
			case workflow.NodeID("a-source"):
				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"source-value",
								nil,
							),
						},
					},
				), nil

			case workflow.NodeID("b-failed"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				return runtime.NodeResult{},
					technicalError

			case workflow.NodeID("c-descendant"):
				t.Fatal(
					"blocked descendant executor must not be called",
				)

				return runtime.NodeResult{},
					fmt.Errorf(
						"blocked descendant was executed",
					)

			case workflow.NodeID("d-healthy"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"source-value",
				)

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"healthy-value",
								nil,
							),
						},
					},
				), nil

			case workflow.NodeID("e-terminal"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"healthy-value",
				)

				return mustRoutingTerminalResult(
					t,
					mustRoutingPayload(
						t,
						"technical-failure-healthy-result",
						nil,
					),
				), nil

			default:
				return runtime.NodeResult{},
					fmt.Errorf(
						"unexpected SyncRunner node %s",
						nodeID,
					)
			}
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		fixture,
		executor,
	)

	runner, err := NewSyncRunner(
		fixture.dependencies,
	)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	result, err := runner.Run(
		context.Background(),
		fixture.request,
	)
	if err != nil {
		t.Fatalf(
			"SyncRunner.Run() returned an unexpected scheduler error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"technical-failure SyncRunResult is invalid",
		)
	}

	if !result.IsFailed() {
		t.Fatalf(
			"sync-run status = %q, want FAILED",
			result.Status(),
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"technical runtime failure was incorrectly rejected",
		)
	}

	if !result.HasExecutionResult() {
		t.Fatal(
			"technical runtime failure contains no ExecutionResult",
		)
	}

	executionResult, exists :=
		result.ExecutionResult()

	if !exists {
		t.Fatal(
			"ExecutionResult() exists = false",
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("a-source"),
		workflow.NodeID("b-failed"),
		workflow.NodeID("d-healthy"),
		workflow.NodeID("e-terminal"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		executionResult.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"recorded execution order = %#v, want %#v",
			executionResult.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	if executorCalls[workflow.NodeID("c-descendant")] != 0 {
		t.Fatalf(
			"c-descendant executor call count = %d, want 0",
			executorCalls[workflow.NodeID("c-descendant")],
		)
	}

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("b-failed"),
		execution.NodeExecutionStatusFailed,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("c-descendant"),
		execution.NodeExecutionStatusSkipped,
	)

	requireExecutionResultNodeStatus(
		t,
		executionResult,
		workflow.NodeID("e-terminal"),
		execution.NodeExecutionStatusSucceeded,
	)

	nodeFailure, exists, err :=
		executionResult.NodeFailure(
			workflow.NodeID("b-failed"),
		)
	if err != nil {
		t.Fatalf(
			"NodeFailure(b-failed) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"technical node failure does not exist",
		)
	}

	if nodeFailure.Category() !=
		runtime.FailureCategoryInternal {
		t.Fatalf(
			"technical failure category = %q, want INTERNAL",
			nodeFailure.Category(),
		)
	}

	if nodeFailure.Code() !=
		failureCodeNodeExecutionError {
		t.Fatalf(
			"technical failure code = %q, want %q",
			nodeFailure.Code(),
			failureCodeNodeExecutionError,
		)
	}

	failureDetails :=
		nodeFailure.Details()

	if actual :=
		failureDetails["nodeID"]; actual != "b-failed" {
		t.Fatalf(
			"technical failure nodeID = %q, want %q",
			actual,
			"b-failed",
		)
	}

	if actual :=
		failureDetails["error"]; actual == "" {
		t.Fatal(
			"technical failure contains no error detail",
		)
	}

	workflowFailure, exists :=
		executionResult.WorkflowFailure()

	if !exists {
		t.Fatal(
			"technical failure contains no workflow failure",
		)
	}

	if workflowFailure.Code() !=
		failureCodeWorkflowNodeFailure {
		t.Fatalf(
			"workflow failure code = %q, want %q",
			workflowFailure.Code(),
			failureCodeWorkflowNodeFailure,
		)
	}

	terminalOutput, exists, err :=
		executionResult.TerminalOutput(
			workflow.NodeID("e-terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(e-terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"healthy branch terminal output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		terminalOutput,
	); actual != "technical-failure-healthy-result" {
		t.Fatalf(
			"terminal output = %q, want %q",
			actual,
			"technical-failure-healthy-result",
		)
	}
}

func requireExecutionResultNodeStatus(
	t *testing.T,
	result ExecutionResult,
	nodeID workflow.NodeID,
	expected execution.NodeExecutionStatus,
) {
	t.Helper()

	nodeExecution, exists, err :=
		result.NodeExecution(
			nodeID,
		)
	if err != nil {
		t.Fatalf(
			"NodeExecution(%s) returned an unexpected error: %v",
			nodeID,
			err,
		)
	}

	if !exists {
		t.Fatalf(
			"node execution %q does not exist",
			nodeID,
		)
	}

	if nodeExecution.Status() != expected {
		t.Fatalf(
			"node %q status = %q, want %q",
			nodeID,
			nodeExecution.Status(),
			expected,
		)
	}
}

func TestSyncRunnerCompletesSuccessfulWorkflow(
	t *testing.T,
) {
	fixture :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	fixture.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				24,
				10,
				0,
				0,
				0,
				time.UTC,
			),
		}

	executorCalls := make(
		map[workflow.NodeID]int,
	)

	executorOrder := make(
		[]workflow.NodeID,
		0,
		3,
	)

	executor := runtime.NodeExecutorFunc(
		func(
			nodeContext *runtime.NodeExecutionContext,
			input runtime.NodeInput,
			configuration workflow.JSONObject,
		) (runtime.NodeResult, error) {
			if nodeContext == nil {
				t.Fatal(
					"executor received a nil NodeExecutionContext",
				)
			}

			nodeID :=
				nodeContext.NodeID()

			executorCalls[nodeID]++

			executorOrder = append(
				executorOrder,
				nodeID,
			)

			switch nodeID {
			case workflow.NodeID("source"):
				if !input.IsValid() {
					t.Fatal(
						"source received an invalid input",
					)
				}

				if !input.IsEmpty() {
					t.Fatal(
						"source received a non-empty input",
					)
				}

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"sync-source-value",
								map[string]string{
									"node": "source",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("pass"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"sync-source-value",
				)

				return mustRoutingSuccessResult(
					t,
					map[string][]runtime.Payload{
						"output": {
							mustRoutingPayload(
								t,
								"sync-pass-value",
								map[string]string{
									"node": "pass",
								},
							),
						},
					},
				), nil

			case workflow.NodeID("terminal"):
				requireSchedulerInputValue(
					t,
					input,
					"input",
					"sync-pass-value",
				)

				return mustRoutingTerminalResult(
					t,
					mustRoutingPayload(
						t,
						"sync-workflow-result",
						map[string]string{
							"node": "terminal",
						},
					),
				), nil

			default:
				return runtime.NodeResult{},
					fmt.Errorf(
						"unexpected SyncRunner test node %s",
						nodeID,
					)
			}
		},
	)

	installSchedulerExecutorForAllNodes(
		t,
		fixture,
		executor,
	)

	runner, err := NewSyncRunner(
		fixture.dependencies,
	)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	if !runner.IsValid() {
		t.Fatal(
			"NewSyncRunner() returned an invalid runner",
		)
	}

	result, err := runner.Run(
		context.Background(),
		fixture.request,
	)
	if err != nil {
		t.Fatalf(
			"SyncRunner.Run() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"SyncRunner.Run() returned an invalid SyncRunResult",
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"successful SyncRunResult is not terminal",
		)
	}

	if !result.IsSucceeded() {
		t.Fatalf(
			"sync-run status = %q, want SUCCEEDED",
			result.Status(),
		)
	}

	if result.IsRejected() {
		t.Fatal(
			"successful SyncRunResult is marked as rejected",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"successful SyncRunResult is marked as failed",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"successful SyncRunResult is marked as cancelled",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"successful SyncRunResult is marked as timed out",
		)
	}

	if result.Status() !=
		execution.WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"workflow status = %q, want %q",
			result.Status(),
			execution.WorkflowExecutionStatusSucceeded,
		)
	}

	validationReport :=
		result.ValidationReport()

	if !validationReport.IsValid() {
		t.Fatalf(
			"successful run validation report contains %d issues",
			validationReport.Len(),
		)
	}

	if validationReport.Len() != 0 {
		t.Fatalf(
			"successful run validation issue count = %d, want 0",
			validationReport.Len(),
		)
	}

	if !result.HasExecutionResult() {
		t.Fatal(
			"successful SyncRunResult contains no ExecutionResult",
		)
	}

	executionResult, exists :=
		result.ExecutionResult()

	if !exists {
		t.Fatal(
			"ExecutionResult() exists = false",
		)
	}

	if !executionResult.IsSucceeded() {
		t.Fatalf(
			"execution result status = %q, want SUCCEEDED",
			executionResult.Status(),
		)
	}

	if executionResult.WorkflowExecution().ID() !=
		result.WorkflowExecution().ID() {
		t.Fatalf(
			"execution-result workflow ID = %q, sync-run workflow ID = %q",
			executionResult.WorkflowExecution().ID(),
			result.WorkflowExecution().ID(),
		)
	}

	if executionResult.Status() !=
		result.Status() {
		t.Fatalf(
			"execution-result status = %q, sync-run status = %q",
			executionResult.Status(),
			result.Status(),
		)
	}

	expectedOrder := []workflow.NodeID{
		workflow.NodeID("source"),
		workflow.NodeID("pass"),
		workflow.NodeID("terminal"),
	}

	if !reflect.DeepEqual(
		executorOrder,
		expectedOrder,
	) {
		t.Fatalf(
			"executor order = %#v, want %#v",
			executorOrder,
			expectedOrder,
		)
	}

	if !reflect.DeepEqual(
		executionResult.ExecutedNodeOrder(),
		expectedOrder,
	) {
		t.Fatalf(
			"recorded execution order = %#v, want %#v",
			executionResult.ExecutedNodeOrder(),
			expectedOrder,
		)
	}

	for _, nodeID := range expectedOrder {
		if executorCalls[nodeID] != 1 {
			t.Fatalf(
				"node %q executor call count = %d, want 1",
				nodeID,
				executorCalls[nodeID],
			)
		}

		nodeExecution, exists, err :=
			executionResult.NodeExecution(
				nodeID,
			)
		if err != nil {
			t.Fatalf(
				"NodeExecution(%s) returned an unexpected error: %v",
				nodeID,
				err,
			)
		}

		if !exists {
			t.Fatalf(
				"node execution %q does not exist",
				nodeID,
			)
		}

		if nodeExecution.Status() !=
			execution.NodeExecutionStatusSucceeded {
			t.Fatalf(
				"node %q status = %q, want SUCCEEDED",
				nodeID,
				nodeExecution.Status(),
			)
		}
	}

	terminalOutput, exists, err :=
		executionResult.TerminalOutput(
			workflow.NodeID("terminal"),
		)
	if err != nil {
		t.Fatalf(
			"TerminalOutput(terminal) returned an unexpected error: %v",
			err,
		)
	}

	if !exists {
		t.Fatal(
			"terminal output does not exist",
		)
	}

	if actual := preparedInputPayloadText(
		t,
		terminalOutput,
	); actual != "sync-workflow-result" {
		t.Fatalf(
			"terminal output = %q, want %q",
			actual,
			"sync-workflow-result",
		)
	}

	if actual :=
		terminalOutput.Metadata()["node"]; actual != "terminal" {
		t.Fatalf(
			"terminal metadata node = %q, want %q",
			actual,
			"terminal",
		)
	}

	if failures :=
		executionResult.NodeFailures(); failures != nil {
		t.Fatalf(
			"successful execution node failures = %#v, want nil",
			failures,
		)
	}

	if _, exists :=
		executionResult.WorkflowFailure(); exists {
		t.Fatal(
			"successful execution contains a workflow failure",
		)
	}
}

func TestSyncRunnerReturnsRejectedResultWhenExecutorIsMissing(
	t *testing.T,
) {
	fixture :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	emptyExecutorRegistry, err :=
		runtime.NewExecutorRegistry(nil)

	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry(nil) returned an unexpected error: %v",
			err,
		)
	}

	dependencies :=
		fixture.dependencies

	dependencies.executorRegistry =
		emptyExecutorRegistry

	dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				24,
				12,
				0,
				0,
				0,
				time.UTC,
			),
		}

	runner, err := NewSyncRunner(
		dependencies,
	)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	result, err := runner.Run(
		context.Background(),
		fixture.request,
	)
	if err != nil {
		t.Fatalf(
			"SyncRunner.Run() returned an unexpected error for preflight rejection: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"rejected SyncRunResult is invalid",
		)
	}

	if !result.IsTerminal() {
		t.Fatal(
			"rejected SyncRunResult is not terminal",
		)
	}

	if !result.IsRejected() {
		t.Fatalf(
			"sync-run status = %q, want REJECTED",
			result.Status(),
		)
	}

	if result.Status() !=
		execution.WorkflowExecutionStatusRejected {
		t.Fatalf(
			"workflow status = %q, want %q",
			result.Status(),
			execution.WorkflowExecutionStatusRejected,
		)
	}

	if result.IsSucceeded() {
		t.Fatal(
			"rejected result is marked as succeeded",
		)
	}

	if result.IsFailed() {
		t.Fatal(
			"rejected result is marked as failed",
		)
	}

	if result.IsCancelled() {
		t.Fatal(
			"rejected result is marked as cancelled",
		)
	}

	if result.IsTimedOut() {
		t.Fatal(
			"rejected result is marked as timed out",
		)
	}

	if result.HasExecutionResult() {
		t.Fatal(
			"rejected result unexpectedly contains an ExecutionResult",
		)
	}

	if _, exists :=
		result.ExecutionResult(); exists {
		t.Fatal(
			"ExecutionResult() exists = true for rejected workflow",
		)
	}

	validationReport :=
		result.ValidationReport()

	if validationReport.IsValid() {
		t.Fatal(
			"rejected workflow has a valid validation report",
		)
	}

	if validationReport.Len() == 0 {
		t.Fatal(
			"rejected workflow validation report contains no issues",
		)
	}

	if !validationReport.HasEngineCode(
		IssueCodeExecutorNotFound,
	) {
		t.Fatalf(
			"validation report does not contain %q",
			IssueCodeExecutorNotFound,
		)
	}

	engineIssues :=
		validationReport.EngineIssues()

	if len(engineIssues) == 0 {
		t.Fatal(
			"rejected workflow contains no engine issues",
		)
	}

	for _, issue := range engineIssues {
		if issue.Code !=
			IssueCodeExecutorNotFound {
			t.Fatalf(
				"unexpected engine issue code = %q",
				issue.Code,
			)
		}

		if issue.NodeID.String() == "" {
			t.Fatal(
				"executor-not-found issue contains no node ID",
			)
		}

		if issue.PluginIdentity.String() == "" {
			t.Fatal(
				"executor-not-found issue contains no plugin identity",
			)
		}
	}

	workflowExecution :=
		result.WorkflowExecution()

	if workflowExecution.Status() !=
		execution.WorkflowExecutionStatusRejected {
		t.Fatalf(
			"workflow execution status = %q, want REJECTED",
			workflowExecution.Status(),
		)
	}

	if _, started :=
		workflowExecution.StartedAt(); started {
		t.Fatal(
			"rejected workflow unexpectedly contains startedAt",
		)
	}

	finishedAt, finished :=
		workflowExecution.FinishedAt()

	if !finished || finishedAt.IsZero() {
		t.Fatal(
			"rejected workflow does not contain finishedAt",
		)
	}
}

func TestNewSyncRunnerRejectsInvalidDependencies(
	t *testing.T,
) {
	runner, err := NewSyncRunner(
		EngineDependencies{},
	)

	requireEngineValidationField(
		t,
		err,
		"dependencies",
	)

	if runner.IsValid() {
		t.Fatal(
			"NewSyncRunner() returned a valid runner for invalid dependencies",
		)
	}

	var zeroRunner SyncRunner

	if zeroRunner.IsValid() {
		t.Fatal(
			"zero-value SyncRunner is valid",
		)
	}
}

func TestSyncRunnerRejectsInvalidRunArguments(
	t *testing.T,
) {
	fixture :=
		mustSchedulerLinearPreparedExecution(
			t,
		)

	fixture.dependencies.clock =
		&nodeExecutionTestClock{
			next: time.Date(
				2026,
				time.July,
				24,
				14,
				0,
				0,
				0,
				time.UTC,
			),
		}

	runner, err := NewSyncRunner(
		fixture.dependencies,
	)
	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	t.Run("invalid runner", func(t *testing.T) {
		var invalidRunner SyncRunner

		_, err := invalidRunner.Run(
			context.Background(),
			fixture.request,
		)

		requireEngineValidationField(
			t,
			err,
			"syncRunner",
		)
	})

	t.Run("nil context", func(t *testing.T) {
		_, err := runner.Run(
			nil,
			fixture.request,
		)

		requireEngineValidationField(
			t,
			err,
			"context",
		)
	})

	t.Run("invalid request", func(t *testing.T) {
		_, err := runner.Run(
			context.Background(),
			ExecutionRequest{},
		)

		requireEngineValidationField(
			t,
			err,
			"request",
		)
	})
}

func TestPreflightValidationIssueCodeString(
	t *testing.T,
) {
	tests := map[PreflightValidationIssueCode]string{
		IssueCodeTopologicalOrderUnavailable: "TOPOLOGICAL_ORDER_UNAVAILABLE",

		IssueCodeRuntimeEdgeInvalid: "RUNTIME_EDGE_INVALID",

		IssueCodeExecutorNotFound: "EXECUTOR_NOT_FOUND",
	}

	for code, expected := range tests {
		t.Run(expected, func(t *testing.T) {
			if actual := code.String(); actual != expected {
				t.Fatalf(
					"String() = %q, want %q",
					actual,
					expected,
				)
			}
		})
	}
}

func TestNewPreflightValidationReportSortsEngineIssuesDeterministically(
	t *testing.T,
) {
	identity := mustValidationPluginIdentity(
		t,
		"core.pass-through",
		"v1",
	)

	sourceIssues := []PreflightValidationIssue{
		{
			Code:           IssueCodeExecutorNotFound,
			Message:        "executor is missing",
			NodeID:         workflow.NodeID("node-b"),
			PluginIdentity: identity,
			Field:          "executor",
		},
		{
			Code:           IssueCodeExecutorNotFound,
			Message:        "executor is missing",
			NodeID:         workflow.NodeID("node-a"),
			PluginIdentity: identity,
			Field:          "executor",
		},
		{
			Code:    IssueCodeRuntimeEdgeInvalid,
			Message: "edge runtime is invalid",
			NodeID:  workflow.NodeID("node-a"),
			EdgeID:  workflow.EdgeID("edge-b"),
			Field:   "edgeRuntime",
		},
		{
			Code:    IssueCodeTopologicalOrderUnavailable,
			Message: "topological order is unavailable",
			NodeID:  workflow.NodeID("node-a"),
			Field:   "topologicalOrder",
		},
		{
			Code:    IssueCodeRuntimeEdgeInvalid,
			Message: "edge runtime is invalid",
			NodeID:  workflow.NodeID("node-a"),
			EdgeID:  workflow.EdgeID("edge-a"),
			Field:   "edgeRuntime",
		},
	}

	report := newPreflightValidationReport(
		graph.ValidationReport{},
		plugin.WorkflowValidationReport{},
		sourceIssues,
	)

	if report.IsValid() {
		t.Fatal(
			"report IsValid() = true with engine issues",
		)
	}

	if actual := report.Len(); actual != 5 {
		t.Fatalf(
			"Len() = %d, want 5",
			actual,
		)
	}

	actualIssues := report.EngineIssues()

	actual := make(
		[]string,
		len(actualIssues),
	)

	for index, issue := range actualIssues {
		actual[index] =
			issue.NodeID.String() +
				":" +
				issue.Code.String() +
				":" +
				issue.EdgeID.String()
	}

	expected := []string{
		"node-a:TOPOLOGICAL_ORDER_UNAVAILABLE:",
		"node-a:RUNTIME_EDGE_INVALID:edge-a",
		"node-a:RUNTIME_EDGE_INVALID:edge-b",
		"node-a:EXECUTOR_NOT_FOUND:",
		"node-b:EXECUTOR_NOT_FOUND:",
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"engine issue order = %#v, want %#v",
			actual,
			expected,
		)
	}

	sourceIssues[0].Message =
		"changed after report construction"

	stored := report.EngineIssues()

	if stored[len(stored)-1].Message !=
		"executor is missing" {
		t.Fatalf(
			"report changed through constructor source slice: got %q",
			stored[len(stored)-1].Message,
		)
	}
}

func TestPreflightValidationReportEngineIssuesAreIndependent(
	t *testing.T,
) {
	report := newPreflightValidationReport(
		graph.ValidationReport{},
		plugin.WorkflowValidationReport{},
		[]PreflightValidationIssue{
			{
				Code:    IssueCodeRuntimeEdgeInvalid,
				Message: "original message",
				NodeID:  workflow.NodeID("node-a"),
				EdgeID:  workflow.EdgeID("edge-a"),
				Field:   "edgeRuntime",
			},
		},
	)

	first := report.EngineIssues()

	if len(first) != 1 {
		t.Fatalf(
			"first EngineIssues() length = %d, want 1",
			len(first),
		)
	}

	first[0].Message = "changed message"
	first[0].NodeID = workflow.NodeID("changed-node")

	second := report.EngineIssues()

	if second[0].Message != "original message" {
		t.Fatalf(
			"stored message changed through returned slice: got %q",
			second[0].Message,
		)
	}

	if actual := second[0].NodeID.String(); actual !=
		"node-a" {
		t.Fatalf(
			"stored NodeID changed through returned slice: got %q",
			actual,
		)
	}
}

func TestPreflightValidationReportHasEngineCode(
	t *testing.T,
) {
	report := newPreflightValidationReport(
		graph.ValidationReport{},
		plugin.WorkflowValidationReport{},
		[]PreflightValidationIssue{
			{
				Code:    IssueCodeExecutorNotFound,
				Message: "executor is missing",
				NodeID:  workflow.NodeID("node-a"),
			},
		},
	)

	if !report.HasEngineCode(
		IssueCodeExecutorNotFound,
	) {
		t.Fatal(
			"HasEngineCode(EXECUTOR_NOT_FOUND) = false",
		)
	}

	if report.HasEngineCode(
		IssueCodeRuntimeEdgeInvalid,
	) {
		t.Fatal(
			"HasEngineCode(RUNTIME_EDGE_INVALID) = true",
		)
	}
}

func TestZeroPreflightValidationReportIsValid(
	t *testing.T,
) {
	var report PreflightValidationReport

	if !report.IsValid() {
		t.Fatal(
			"zero report IsValid() = false",
		)
	}

	if actual := report.Len(); actual != 0 {
		t.Fatalf(
			"zero report Len() = %d, want 0",
			actual,
		)
	}

	if report.StructuralIssues() != nil {
		t.Fatal(
			"zero StructuralIssues() must return nil",
		)
	}

	if report.PluginIssues() != nil {
		t.Fatal(
			"zero PluginIssues() must return nil",
		)
	}

	if report.EngineIssues() != nil {
		t.Fatal(
			"zero EngineIssues() must return nil",
		)
	}
}

func mustValidationPluginIdentity(
	t *testing.T,
	pluginType string,
	version string,
) plugin.PluginIdentity {
	t.Helper()

	identity, err := plugin.NewPluginIdentity(
		workflow.PluginType(pluginType),
		workflow.PluginVersion(version),
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	return identity
}

func TestValidateExecutionRequestProducesValidatedPlanForLinearWorkflow(t *testing.T) {
	definition := mustValidationLinearWorkflow(t, "source", "pass", "terminal")
	limits := mustValidationRuntimeLimits(t, 64, 8, 4096)
	dependencies := mustValidationCoreDependencies(t, limits, true)
	request := mustValidationExecutionRequest(t, definition)

	plan, report, err := validateExecutionRequest(request, dependencies)
	if err != nil {
		t.Fatalf("validateExecutionRequest() returned an unexpected error: %v", err)
	}

	if !report.IsValid() {
		t.Fatalf(
			"validation report contains issues: structural=%v plugin=%v engine=%v",
			report.StructuralIssues(),
			report.PluginIssues(),
			report.EngineIssues(),
		)
	}

	if report.Len() != 0 {
		t.Fatalf("report Len() = %d, want 0", report.Len())
	}

	if len(plan.graph.Nodes()) != 3 {
		t.Fatalf("validated graph node count = %d, want 3", len(plan.graph.Nodes()))
	}

	if len(plan.graph.Edges()) != 2 {
		t.Fatalf("validated graph edge count = %d, want 2", len(plan.graph.Edges()))
	}

	actualOrder := validationNodeIDStrings(plan.topologicalOrder)
	expectedOrder := []string{"source", "pass", "terminal"}

	if !reflect.DeepEqual(actualOrder, expectedOrder) {
		t.Fatalf("topological order = %#v, want %#v", actualOrder, expectedOrder)
	}

	if len(plan.descriptorsByNode) != 3 {
		t.Fatalf("descriptor map length = %d, want 3", len(plan.descriptorsByNode))
	}

	expectedIdentities := map[string]string{
		"source":   "core.static-input@v1",
		"pass":     "core.pass-through@v1",
		"terminal": "core.terminal@v1",
	}

	for nodeID, expectedIdentity := range expectedIdentities {
		descriptor, exists := plan.descriptorsByNode[workflow.NodeID(nodeID)]
		if !exists {
			t.Fatalf("descriptor for node %q does not exist", nodeID)
		}

		actualIdentity := descriptor.Identity().String()
		if actualIdentity != expectedIdentity {
			t.Fatalf(
				"descriptor identity for %q = %q, want %q",
				nodeID,
				actualIdentity,
				expectedIdentity,
			)
		}
	}

	publicReport, err := ValidateExecutionRequest(request, dependencies)
	if err != nil {
		t.Fatalf("ValidateExecutionRequest() returned an unexpected error: %v", err)
	}

	if !publicReport.IsValid() {
		t.Fatal("public validation report IsValid() = false")
	}

	plan.topologicalOrder[0] = workflow.NodeID("changed")
	plan.descriptorsByNode[workflow.NodeID("source")] = plugin.Descriptor{}

	secondPlan, secondReport, err := validateExecutionRequest(request, dependencies)
	if err != nil {
		t.Fatalf("second validateExecutionRequest() returned an unexpected error: %v", err)
	}

	if !secondReport.IsValid() {
		t.Fatal("second validation report IsValid() = false")
	}

	if secondPlan.topologicalOrder[0].String() != "source" {
		t.Fatalf(
			"new plan changed through previous plan mutation: got %q",
			secondPlan.topologicalOrder[0].String(),
		)
	}

	secondSourceDescriptor := secondPlan.descriptorsByNode[workflow.NodeID("source")]

	if secondSourceDescriptor.Identity().String() != "core.static-input@v1" {
		t.Fatalf(
			"new descriptor plan changed through previous plan mutation: got %q",
			secondSourceDescriptor.Identity().String(),
		)
	}
}

func TestValidateExecutionRequestPreservesStructuralIssues(t *testing.T) {
	definition, err := workflow.NewWorkflowDefinition(
		workflow.WorkflowID("workflow-empty"),
		workflow.CompanyID("company-1"),
		"Empty Workflow",
		1,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("workflow.NewWorkflowDefinition() returned an unexpected error: %v", err)
	}

	limits := mustValidationRuntimeLimits(t, 64, 8, 4096)
	dependencies := mustValidationCoreDependencies(t, limits, true)

	report, err := ValidateExecutionRequest(
		mustValidationExecutionRequest(t, definition),
		dependencies,
	)
	if err != nil {
		t.Fatalf("ValidateExecutionRequest() returned an unexpected error: %v", err)
	}

	if report.IsValid() {
		t.Fatal("structurally invalid workflow produced a valid report")
	}

	if len(report.StructuralIssues()) == 0 {
		t.Fatal("structural issues are empty")
	}

	if report.PluginIssues() != nil {
		t.Fatalf("plugin issues = %#v, want nil", report.PluginIssues())
	}

	if report.EngineIssues() != nil {
		t.Fatalf("engine issues = %#v, want nil", report.EngineIssues())
	}
}

func TestValidateExecutionRequestPreservesPluginIssues(t *testing.T) {
	unknownNode := mustValidationNode(
		t,
		"unknown-node",
		"custom.unknown",
		"v1",
		`{}`,
	)

	definition := mustValidationDefinition(
		t,
		"workflow-unknown-plugin",
		[]workflow.NodeDefinition{unknownNode},
		nil,
	)

	limits := mustValidationRuntimeLimits(t, 64, 8, 4096)
	dependencies := mustValidationCoreDependencies(t, limits, true)

	report, err := ValidateExecutionRequest(
		mustValidationExecutionRequest(t, definition),
		dependencies,
	)
	if err != nil {
		t.Fatalf("ValidateExecutionRequest() returned an unexpected error: %v", err)
	}

	if report.IsValid() {
		t.Fatal("workflow with unknown plugin produced a valid report")
	}

	if len(report.StructuralIssues()) != 0 {
		t.Fatalf(
			"structural issue count = %d, want 0",
			len(report.StructuralIssues()),
		)
	}

	pluginIssues := report.PluginIssues()

	if len(pluginIssues) != 1 {
		t.Fatalf("plugin issue count = %d, want 1", len(pluginIssues))
	}

	if pluginIssues[0].Code != plugin.IssueCodePluginNotFound {
		t.Fatalf(
			"plugin issue code = %q, want %q",
			pluginIssues[0].Code,
			plugin.IssueCodePluginNotFound,
		)
	}

	if pluginIssues[0].NodeID.String() != "unknown-node" {
		t.Fatalf(
			"plugin issue NodeID = %q, want %q",
			pluginIssues[0].NodeID.String(),
			"unknown-node",
		)
	}

	if report.EngineIssues() != nil {
		t.Fatalf("engine issues = %#v, want nil", report.EngineIssues())
	}
}

func TestValidateExecutionRequestRejectsMissingExecutorsDeterministically(
	t *testing.T,
) {
	definition := mustValidationLinearWorkflow(
		t,
		"z-source",
		"a-pass",
		"m-terminal",
	)

	limits := mustValidationRuntimeLimits(t, 64, 8, 4096)
	dependencies := mustValidationCoreDependencies(t, limits, false)

	report, err := ValidateExecutionRequest(
		mustValidationExecutionRequest(t, definition),
		dependencies,
	)
	if err != nil {
		t.Fatalf("ValidateExecutionRequest() returned an unexpected error: %v", err)
	}

	if report.IsValid() {
		t.Fatal("workflow without executors produced a valid report")
	}

	engineIssues := report.EngineIssues()

	if len(engineIssues) != 3 {
		t.Fatalf("engine issue count = %d, want 3", len(engineIssues))
	}

	actualNodeIDs := make([]string, len(engineIssues))

	for index, issue := range engineIssues {
		if issue.Code != IssueCodeExecutorNotFound {
			t.Fatalf(
				"issue %d code = %q, want %q",
				index,
				issue.Code,
				IssueCodeExecutorNotFound,
			)
		}

		actualNodeIDs[index] = issue.NodeID.String()
	}

	expectedNodeIDs := []string{
		"a-pass",
		"m-terminal",
		"z-source",
	}

	if !reflect.DeepEqual(actualNodeIDs, expectedNodeIDs) {
		t.Fatalf(
			"missing-executor node order = %#v, want %#v",
			actualNodeIDs,
			expectedNodeIDs,
		)
	}

	if !report.HasEngineCode(IssueCodeExecutorNotFound) {
		t.Fatal("report does not contain EXECUTOR_NOT_FOUND")
	}
}

func TestValidateExecutionRequestRejectsRuntimeCapacityBelowPluginPolicy(
	t *testing.T,
) {
	limits := mustValidationRuntimeLimits(t, 63, 1, 4096)
	dependencies := mustValidationCoreDependencies(t, limits, true)

	definition := mustValidationLinearWorkflow(
		t,
		"source",
		"pass",
		"terminal",
	)

	report, err := ValidateExecutionRequest(
		mustValidationExecutionRequest(t, definition),
		dependencies,
	)
	if err != nil {
		t.Fatalf("ValidateExecutionRequest() returned an unexpected error: %v", err)
	}

	if report.IsValid() {
		t.Fatal("workflow above runtime queue limit produced a valid report")
	}

	engineIssues := report.EngineIssues()

	if len(engineIssues) != 2 {
		t.Fatalf("runtime edge issue count = %d, want 2", len(engineIssues))
	}

	actualNodes := make([]string, len(engineIssues))

	for index, issue := range engineIssues {
		if issue.Code != IssueCodeRuntimeEdgeInvalid {
			t.Fatalf(
				"issue %d code = %q, want %q",
				index,
				issue.Code,
				IssueCodeRuntimeEdgeInvalid,
			)
		}

		if issue.Field != "edgeRuntime" {
			t.Fatalf(
				"issue %d field = %q, want %q",
				index,
				issue.Field,
				"edgeRuntime",
			)
		}

		actualNodes[index] = issue.NodeID.String()
	}

	expectedNodes := []string{
		"pass",
		"terminal",
	}

	if !reflect.DeepEqual(actualNodes, expectedNodes) {
		t.Fatalf(
			"runtime edge issue nodes = %#v, want %#v",
			actualNodes,
			expectedNodes,
		)
	}

	if !report.HasEngineCode(IssueCodeRuntimeEdgeInvalid) {
		t.Fatal("report does not contain RUNTIME_EDGE_INVALID")
	}
}

func TestValidateExecutionRequestAcceptsLocalOnlyAndDistributableNodesInSyncMode(
	t *testing.T,
) {
	const localSourceType workflow.PluginType = "test.local-source"

	const distributableTerminalType workflow.PluginType = "test.distributable-terminal"

	const version workflow.PluginVersion = "v1"

	localDescriptor := mustValidationDescriptor(
		t,
		localSourceType,
		version,
		plugin.DistributionLocalOnly,
		nil,
		[]string{"output"},
		plugin.NewExactEdgeConstraint(0),
		plugin.NewUnlimitedEdgeConstraint(1),
	)

	distributableDescriptor := mustValidationDescriptor(
		t,
		distributableTerminalType,
		version,
		plugin.DistributionDistributable,
		[]string{"input"},
		nil,
		plugin.NewUnlimitedEdgeConstraint(1),
		plugin.NewExactEdgeConstraint(0),
	)

	pluginRegistry, err := plugin.NewRegistry(
		[]plugin.Descriptor{
			localDescriptor,
			distributableDescriptor,
		},
	)
	if err != nil {
		t.Fatalf("plugin.NewRegistry() returned an unexpected error: %v", err)
	}

	executor := runtime.NodeExecutorFunc(
		func(
			*runtime.NodeExecutionContext,
			runtime.NodeInput,
			workflow.JSONObject,
		) (runtime.NodeResult, error) {
			return runtime.NewNodeSuccessResult(
				nil,
				runtime.ContextChanges{},
			)
		},
	)

	registrations := []runtime.ExecutorRegistration{
		mustValidationExecutorRegistration(
			t,
			localDescriptor.Identity(),
			executor,
		),
		mustValidationExecutorRegistration(
			t,
			distributableDescriptor.Identity(),
			executor,
		),
	}

	executorRegistry, err := runtime.NewExecutorRegistry(registrations)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	limits := mustValidationRuntimeLimits(t, 1, 1, 1024)

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		sharedclock.System(),
	)
	if err != nil {
		t.Fatalf("NewEngineDependencies() returned an unexpected error: %v", err)
	}

	source := mustValidationNode(
		t,
		"local-source",
		localSourceType.String(),
		version.String(),
		`{}`,
	)

	terminal := mustValidationNode(
		t,
		"distributed-terminal",
		distributableTerminalType.String(),
		version.String(),
		`{}`,
	)

	edge := mustValidationEdge(
		t,
		"edge-1",
		"local-source",
		"output",
		"distributed-terminal",
		"input",
	)

	definition := mustValidationDefinition(
		t,
		"workflow-sync-distribution",
		[]workflow.NodeDefinition{
			terminal,
			source,
		},
		[]workflow.EdgeDefinition{
			edge,
		},
	)

	report, err := ValidateExecutionRequest(
		mustValidationExecutionRequest(t, definition),
		dependencies,
	)
	if err != nil {
		t.Fatalf("ValidateExecutionRequest() returned an unexpected error: %v", err)
	}

	if !report.IsValid() {
		t.Fatalf(
			"SYNC distribution validation produced issues: plugin=%v engine=%v",
			report.PluginIssues(),
			report.EngineIssues(),
		)
	}
}

func TestValidateExecutionRequestRejectsInvalidContracts(t *testing.T) {
	validRequest := mustValidationExecutionRequest(
		t,
		mustValidationLinearWorkflow(
			t,
			"source",
			"pass",
			"terminal",
		),
	)

	limits := mustValidationRuntimeLimits(t, 64, 8, 4096)
	validDependencies := mustValidationCoreDependencies(t, limits, true)

	t.Run("invalid request", func(t *testing.T) {
		_, err := ValidateExecutionRequest(
			ExecutionRequest{},
			validDependencies,
		)

		requireEngineValidationField(
			t,
			err,
			"request",
		)
	})

	t.Run("invalid dependencies", func(t *testing.T) {
		_, err := ValidateExecutionRequest(
			validRequest,
			EngineDependencies{},
		)

		requireEngineValidationField(
			t,
			err,
			"dependencies",
		)
	})
}

func TestRuntimeEdgeValidationIssuesReportsMissingTargetDescriptor(
	t *testing.T,
) {
	definition := mustValidationLinearWorkflow(
		t,
		"source",
		"pass",
		"terminal",
	)

	builtGraph := graph.Build(definition)
	limits := mustValidationRuntimeLimits(t, 64, 8, 4096)

	issues := runtimeEdgeValidationIssues(
		builtGraph,
		nil,
		limits,
	)

	if len(issues) != 2 {
		t.Fatalf("issue count = %d, want 2", len(issues))
	}

	for _, issue := range issues {
		if issue.Code != IssueCodeRuntimeEdgeInvalid {
			t.Fatalf(
				"issue code = %q, want %q",
				issue.Code,
				IssueCodeRuntimeEdgeInvalid,
			)
		}

		if issue.Field != "targetDescriptor" {
			t.Fatalf(
				"issue field = %q, want %q",
				issue.Field,
				"targetDescriptor",
			)
		}
	}
}

func TestCloneDescriptorsByNodeCopiesMapMembership(t *testing.T) {
	descriptor, err := core.StaticInputDescriptor()
	if err != nil {
		t.Fatalf(
			"core.StaticInputDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	source := map[workflow.NodeID]plugin.Descriptor{
		workflow.NodeID("source"): descriptor,
	}

	cloned := cloneDescriptorsByNode(source)

	source[workflow.NodeID("source")] = plugin.Descriptor{}
	delete(source, workflow.NodeID("source"))

	stored, exists := cloned[workflow.NodeID("source")]

	if !exists {
		t.Fatal("cloned descriptor membership changed through source map")
	}

	if stored.Identity().String() != "core.static-input@v1" {
		t.Fatalf(
			"cloned descriptor identity = %q",
			stored.Identity().String(),
		)
	}
}

func mustValidationCoreDependencies(
	t *testing.T,
	limits runtime.RuntimeLimits,
	includeExecutors bool,
) EngineDependencies {
	t.Helper()

	descriptors, err := core.CoreDescriptors()
	if err != nil {
		t.Fatalf(
			"core.CoreDescriptors() returned an unexpected error: %v",
			err,
		)
	}

	pluginRegistry, err := plugin.NewRegistry(descriptors)
	if err != nil {
		t.Fatalf("plugin.NewRegistry() returned an unexpected error: %v", err)
	}

	var registrations []runtime.ExecutorRegistration

	if includeExecutors {
		registrations, err = core.ExecutorRegistrations(
			limits,
			core.TimerWaiter{},
		)
		if err != nil {
			t.Fatalf(
				"core.ExecutorRegistrations() returned an unexpected error: %v",
				err,
			)
		}
	}

	executorRegistry, err := runtime.NewExecutorRegistry(registrations)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	dependencies, err := NewEngineDependencies(
		pluginRegistry,
		executorRegistry,
		limits,
		sharedclock.From(
			func() time.Time {
				return time.Date(
					2026,
					time.July,
					17,
					12,
					0,
					0,
					0,
					time.UTC,
				)
			},
		),
	)
	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	return dependencies
}

func mustValidationLinearWorkflow(
	t *testing.T,
	sourceNodeID string,
	passNodeID string,
	terminalNodeID string,
) workflow.WorkflowDefinition {
	t.Helper()

	source := mustValidationNode(
		t,
		sourceNodeID,
		core.StaticInputPluginType.String(),
		core.CorePluginVersion.String(),
		`{"value":{"message":"hello"}}`,
	)

	pass := mustValidationNode(
		t,
		passNodeID,
		core.PassThroughPluginType.String(),
		core.CorePluginVersion.String(),
		`{}`,
	)

	terminal := mustValidationNode(
		t,
		terminalNodeID,
		core.TerminalPluginType.String(),
		core.CorePluginVersion.String(),
		`{}`,
	)

	firstEdge := mustValidationEdge(
		t,
		"edge-source-pass",
		sourceNodeID,
		core.OutputPortName,
		passNodeID,
		core.InputPortName,
	)

	secondEdge := mustValidationEdge(
		t,
		"edge-pass-terminal",
		passNodeID,
		core.OutputPortName,
		terminalNodeID,
		core.InputPortName,
	)

	return mustValidationDefinition(
		t,
		"workflow-linear",
		[]workflow.NodeDefinition{
			terminal,
			source,
			pass,
		},
		[]workflow.EdgeDefinition{
			secondEdge,
			firstEdge,
		},
	)
}

func mustValidationDefinition(
	t *testing.T,
	workflowID string,
	nodes []workflow.NodeDefinition,
	edges []workflow.EdgeDefinition,
) workflow.WorkflowDefinition {
	t.Helper()

	definition, err := workflow.NewWorkflowDefinition(
		workflow.WorkflowID(workflowID),
		workflow.CompanyID("company-1"),
		"Validation Workflow",
		1,
		nodes,
		edges,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewWorkflowDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return definition
}

func mustValidationNode(
	t *testing.T,
	nodeID string,
	pluginType string,
	version string,
	configuration string,
) workflow.NodeDefinition {
	t.Helper()

	node, err := workflow.NewNodeDefinition(
		workflow.NodeID(nodeID),
		workflow.PluginType(pluginType),
		workflow.PluginVersion(version),
		[]byte(configuration),
		nil,
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewNodeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return node
}

func mustValidationEdge(
	t *testing.T,
	edgeID string,
	sourceNodeID string,
	sourcePort string,
	targetNodeID string,
	targetPort string,
) workflow.EdgeDefinition {
	t.Helper()

	edge, err := workflow.NewEdgeDefinition(
		workflow.EdgeID(edgeID),
		workflow.NodeID(sourceNodeID),
		sourcePort,
		workflow.NodeID(targetNodeID),
		targetPort,
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewEdgeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return edge
}

func mustValidationExecutionRequest(
	t *testing.T,
	definition workflow.WorkflowDefinition,
) ExecutionRequest {
	t.Helper()

	request, err := NewExecutionRequest(
		execution.WorkflowExecutionID(
			"workflow-execution-validation",
		),
		definition,
		"correlation-validation",
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	return request
}

func mustValidationRuntimeLimits(
	t *testing.T,
	queueCapacity uint,
	cacheCapacity uint,
	inlinePayloadBytes int,
) runtime.RuntimeLimits {
	t.Helper()

	limits, err := runtime.NewRuntimeLimits(
		queueCapacity,
		cacheCapacity,
		inlinePayloadBytes,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewRuntimeLimits() returned an unexpected error: %v",
			err,
		)
	}

	return limits
}

func mustValidationDescriptor(
	t *testing.T,
	pluginType workflow.PluginType,
	version workflow.PluginVersion,
	distribution plugin.DistributionCapability,
	inputPortNames []string,
	outputPortNames []string,
	inputConstraint plugin.EdgeConstraint,
	outputConstraint plugin.EdgeConstraint,
) plugin.Descriptor {
	t.Helper()

	identity, err := plugin.NewPluginIdentity(
		pluginType,
		version,
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	metadata, err := plugin.NewPluginMetadata(
		pluginType.String(),
		"Validation test descriptor",
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewPluginMetadata() returned an unexpected error: %v",
			err,
		)
	}

	queuePolicy, err := plugin.NewQueuePolicy(
		1,
		plugin.OverflowStrategyDropOldest,
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewQueuePolicy() returned an unexpected error: %v",
			err,
		)
	}

	cachePolicy, err := plugin.NewCachePolicy(
		1,
		plugin.OverflowStrategyDropOldest,
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewCachePolicy() returned an unexpected error: %v",
			err,
		)
	}

	descriptor, err := plugin.NewDescriptor(
		plugin.DescriptorConfig{
			Identity:             identity,
			Metadata:             metadata,
			InputPorts:           mustValidationPorts(t, inputPortNames),
			OutputPorts:          mustValidationPorts(t, outputPortNames),
			InputEdgeConstraint:  inputConstraint,
			OutputEdgeConstraint: outputConstraint,
			QueuePolicy:          queuePolicy,
			CachePolicy:          cachePolicy,
			Distribution:         distribution,
			ConfigurationValidator: func(
				workflow.JSONObject,
			) []plugin.ConfigurationIssue {
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"plugin.NewDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	return descriptor
}

func mustValidationPorts(
	t *testing.T,
	names []string,
) []plugin.Port {
	t.Helper()

	if len(names) == 0 {
		return nil
	}

	ports := make(
		[]plugin.Port,
		0,
		len(names),
	)

	for _, name := range names {
		port, err := plugin.NewPort(
			name,
			name,
			"",
		)
		if err != nil {
			t.Fatalf(
				"plugin.NewPort() returned an unexpected error: %v",
				err,
			)
		}

		ports = append(
			ports,
			port,
		)
	}

	return ports
}

func mustValidationExecutorRegistration(
	t *testing.T,
	identity plugin.PluginIdentity,
	executor runtime.NodeExecutor,
) runtime.ExecutorRegistration {
	t.Helper()

	registration, err := runtime.NewExecutorRegistration(
		identity,
		executor,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistration() returned an unexpected error: %v",
			err,
		)
	}

	return registration
}

func validationNodeIDStrings(
	nodeIDs []workflow.NodeID,
) []string {
	if len(nodeIDs) == 0 {
		return nil
	}

	result := make(
		[]string,
		len(nodeIDs),
	)

	for index, nodeID := range nodeIDs {
		result[index] = nodeID.String()
	}

	return result
}

type workflowLifecycleRecorderSpy struct {
	workflowTransitions []WorkflowTransitionObservation

	transitionError error
}

var _ LifecycleRecorder = (*workflowLifecycleRecorderSpy)(nil)

func (recorder *workflowLifecycleRecorderSpy) IsValid() bool {
	return recorder != nil
}

func (
	*workflowLifecycleRecorderSpy,
) RecordWorkflowCreation(
	ctx context.Context,
	_ WorkflowCreationObservation,
) error {
	return workflowLifecycleContextError(
		ctx,
	)
}

func (
	recorder *workflowLifecycleRecorderSpy,
) RecordWorkflowTransition(
	ctx context.Context,
	observation WorkflowTransitionObservation,
) error {
	if err := workflowLifecycleContextError(
		ctx,
	); err != nil {
		return err
	}

	recorder.workflowTransitions = append(
		recorder.workflowTransitions,
		observation,
	)

	return recorder.transitionError
}

func (
	*workflowLifecycleRecorderSpy,
) RecordNodeExecutionsCreation(
	ctx context.Context,
	_ NodeExecutionsCreationObservation,
) error {
	return workflowLifecycleContextError(
		ctx,
	)
}

func (
	*workflowLifecycleRecorderSpy,
) RecordNodeTransition(
	ctx context.Context,
	_ NodeTransitionObservation,
) error {
	return workflowLifecycleContextError(
		ctx,
	)
}

func workflowLifecycleContextError(
	ctx context.Context,
) error {
	if ctx == nil {
		return errors.New(
			"workflow lifecycle context must not be nil",
		)
	}

	return ctx.Err()
}

func TestTransitionWorkflowExecutionRecordsSuccessfulTerminalState(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		10,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedWorkflowLifecycleTestExecution(
			t,
			baseTime,
		)
	defer cancel()

	finishedAt :=
		baseTime.Add(30 * time.Minute)

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: finishedAt,
		}

	err := transitionWorkflowExecution(
		prepared,
		execution.WorkflowExecutionStatusSucceeded,
	)
	if err != nil {
		t.Fatalf(
			"transitionWorkflowExecution() returned an error: %v",
			err,
		)
	}

	if prepared.workflowExecution.Status() !=
		execution.WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"workflow status = %q, want SUCCEEDED",
			prepared.workflowExecution.Status(),
		)
	}

	actualFinishedAt, exists :=
		prepared.workflowExecution.FinishedAt()

	if !exists ||
		!actualFinishedAt.Equal(finishedAt) {
		t.Fatalf(
			"workflow finished time = %v, exists = %t, want %v",
			actualFinishedAt,
			exists,
			finishedAt,
		)
	}

	if len(recorder.workflowTransitions) != 1 {
		t.Fatalf(
			"workflow transition count = %d, want 1",
			len(recorder.workflowTransitions),
		)
	}

	observation :=
		recorder.workflowTransitions[0]

	if observation.Before.Status() !=
		execution.WorkflowExecutionStatusRunning {
		t.Fatalf(
			"previous status = %q, want RUNNING",
			observation.Before.Status(),
		)
	}

	if observation.After.Status() !=
		execution.WorkflowExecutionStatusSucceeded {
		t.Fatalf(
			"target status = %q, want SUCCEEDED",
			observation.After.Status(),
		)
	}

	if observation.HasFailure {
		t.Fatal(
			"successful workflow transition unexpectedly contains failure",
		)
	}

	if observation.Stalled {
		t.Fatal(
			"successful workflow transition is unexpectedly stalled",
		)
	}

	if !observation.TransitionAt.Equal(
		finishedAt,
	) {
		t.Fatalf(
			"transition time = %v, want %v",
			observation.TransitionAt,
			finishedAt,
		)
	}
}

func TestTransitionWorkflowExecutionRecordsFailureAndStalledState(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		11,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedWorkflowLifecycleTestExecution(
			t,
			baseTime,
		)
	defer cancel()

	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryInternal,
		failureCodeWorkflowStalled,
		"Workflow execution stalled because no node was ready",
		false,
		map[string]string{
			"error": "controlled stalled workflow detail",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeFailure() returned an error: %v",
			err,
		)
	}

	state :=
		newSchedulerExecutionState()

	if err := state.recordWorkflowFailure(
		failure,
		true,
	); err != nil {
		t.Fatalf(
			"recordWorkflowFailure() returned an error: %v",
			err,
		)
	}

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime.Add(
				30 * time.Minute,
			),
		}

	err = transitionWorkflowExecution(
		prepared,
		execution.WorkflowExecutionStatusFailed,
		&state,
	)
	if err != nil {
		t.Fatalf(
			"transitionWorkflowExecution() returned an error: %v",
			err,
		)
	}

	if prepared.workflowExecution.Status() !=
		execution.WorkflowExecutionStatusFailed {
		t.Fatalf(
			"workflow status = %q, want FAILED",
			prepared.workflowExecution.Status(),
		)
	}

	if len(recorder.workflowTransitions) != 1 {
		t.Fatalf(
			"workflow transition count = %d, want 1",
			len(recorder.workflowTransitions),
		)
	}

	observation :=
		recorder.workflowTransitions[0]

	if !observation.HasFailure {
		t.Fatal(
			"failed workflow transition contains no RuntimeFailure",
		)
	}

	if observation.Failure.Code() !=
		failureCodeWorkflowStalled {
		t.Fatalf(
			"failure code = %q, want %q",
			observation.Failure.Code(),
			failureCodeWorkflowStalled,
		)
	}

	if !observation.Stalled {
		t.Fatal(
			"stalled workflow transition does not contain stalled marker",
		)
	}

	if observation.TechnicalDetail !=
		"controlled stalled workflow detail" {
		t.Fatalf(
			"technical detail = %q",
			observation.TechnicalDetail,
		)
	}
}

func TestTransitionWorkflowExecutionDoesNotMutateWhenRecorderFails(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedWorkflowLifecycleTestExecution(
			t,
			baseTime,
		)
	defer cancel()

	sentinel := errors.New(
		"controlled workflow persistence failure",
	)

	recorder.transitionError =
		sentinel

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime.Add(
				30 * time.Minute,
			),
		}

	err := transitionWorkflowExecution(
		prepared,
		execution.WorkflowExecutionStatusSucceeded,
	)

	if !errors.Is(err, sentinel) {
		t.Fatalf(
			"transitionWorkflowExecution() error = %v, want sentinel",
			err,
		)
	}

	if prepared.workflowExecution.Status() !=
		execution.WorkflowExecutionStatusRunning {
		t.Fatalf(
			"workflow status after recorder failure = %q, want RUNNING",
			prepared.workflowExecution.Status(),
		)
	}

	if _, exists :=
		prepared.workflowExecution.FinishedAt(); exists {
		t.Fatal(
			"workflow finished time remained after recorder failure",
		)
	}

	if len(recorder.workflowTransitions) != 1 {
		t.Fatalf(
			"workflow transition attempt count = %d, want 1",
			len(recorder.workflowTransitions),
		)
	}
}

func TestTransitionWorkflowExecutionUsesCleanupContextAfterCancellation(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		13,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, recorder, cancel :=
		newPreparedWorkflowLifecycleTestExecution(
			t,
			baseTime,
		)

	cancel()

	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryCanceled,
		failureCodeWorkflowCanceled,
		"Workflow execution was canceled",
		false,
		map[string]string{
			"contextError": context.Canceled.Error(),
		},
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeFailure() returned an error: %v",
			err,
		)
	}

	state :=
		newSchedulerExecutionState()

	if err := state.recordWorkflowFailure(
		failure,
		false,
	); err != nil {
		t.Fatalf(
			"recordWorkflowFailure() returned an error: %v",
			err,
		)
	}

	prepared.dependencies.clock =
		&nodeExecutionTestClock{
			next: baseTime.Add(
				30 * time.Minute,
			),
		}

	err = transitionWorkflowExecution(
		prepared,
		execution.WorkflowExecutionStatusCancelled,
		&state,
	)
	if err != nil {
		t.Fatalf(
			"transitionWorkflowExecution() returned an error: %v",
			err,
		)
	}

	if prepared.workflowExecution.Status() !=
		execution.WorkflowExecutionStatusCancelled {
		t.Fatalf(
			"workflow status = %q, want CANCELLED",
			prepared.workflowExecution.Status(),
		)
	}

	if len(recorder.workflowTransitions) != 1 {
		t.Fatalf(
			"workflow transition count = %d, want 1",
			len(recorder.workflowTransitions),
		)
	}

	observation :=
		recorder.workflowTransitions[0]

	if !observation.HasFailure {
		t.Fatal(
			"cancelled workflow transition contains no failure",
		)
	}

	if observation.Failure.Category() !=
		runtime.FailureCategoryCanceled {
		t.Fatalf(
			"failure category = %q, want CANCELED",
			observation.Failure.Category(),
		)
	}
}

func TestTransitionWorkflowExecutionRejectsMultipleSchedulerStates(
	t *testing.T,
) {
	baseTime := time.Date(
		2026,
		time.July,
		18,
		14,
		0,
		0,
		0,
		time.UTC,
	)

	prepared, _, cancel :=
		newPreparedWorkflowLifecycleTestExecution(
			t,
			baseTime,
		)
	defer cancel()

	first :=
		newSchedulerExecutionState()

	second :=
		newSchedulerExecutionState()

	err := transitionWorkflowExecution(
		prepared,
		execution.WorkflowExecutionStatusSucceeded,
		&first,
		&second,
	)

	requireEngineValidationField(
		t,
		err,
		"schedulerExecutionStates",
	)

	if prepared.workflowExecution.Status() !=
		execution.WorkflowExecutionStatusRunning {
		t.Fatalf(
			"workflow status = %q, want RUNNING",
			prepared.workflowExecution.Status(),
		)
	}
}

func newPreparedWorkflowLifecycleTestExecution(
	t *testing.T,
	baseTime time.Time,
) (
	*preparedExecution,
	*workflowLifecycleRecorderSpy,
	context.CancelFunc,
) {
	t.Helper()

	prepared, _, cancel :=
		newPreparedSchedulerTerminalizationTestExecution(
			t,
			baseTime,
		)

	recorder :=
		&workflowLifecycleRecorderSpy{}

	updatedDependencies, err :=
		prepared.
			dependencies.
			WithLifecycleRecorder(
				recorder,
			)
	if err != nil {
		cancel()

		t.Fatalf(
			"WithLifecycleRecorder() returned an error: %v",
			err,
		)
	}

	prepared.dependencies =
		updatedDependencies

	return prepared,
		recorder,
		cancel
}
