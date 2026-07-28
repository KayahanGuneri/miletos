package persistence

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"miletos-go/internal/engine"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/ports/persistence"
)

type lifecycleStoreSpy struct {
	createExecutionCommands     []repository.CreateExecutionCommand
	createNodeExecutionCommands []repository.CreateNodeExecutionsCommand
	workflowTransitionCommands  []repository.WorkflowTransitionCommand
	nodeTransitionCommands      []repository.NodeTransitionCommand
	createExecutionError        error
	createNodeExecutionsError   error
	workflowTransitionError     error
	nodeTransitionError         error
}

var _ repository.ExecutionLifecycleStore = (*lifecycleStoreSpy)(nil)

func (store *lifecycleStoreSpy) IsValid() bool {
	return store != nil
}

func (
	store *lifecycleStoreSpy,
) CreateExecution(
	_ context.Context,
	command repository.CreateExecutionCommand,
) error {
	store.createExecutionCommands = append(
		store.createExecutionCommands,
		command,
	)

	return store.createExecutionError
}

func (
	store *lifecycleStoreSpy,
) CreateNodeExecutions(
	_ context.Context,
	command repository.CreateNodeExecutionsCommand,
) error {
	store.createNodeExecutionCommands = append(
		store.createNodeExecutionCommands,
		command,
	)

	return store.createNodeExecutionsError
}

func (
	store *lifecycleStoreSpy,
) ApplyWorkflowTransition(
	_ context.Context,
	command repository.WorkflowTransitionCommand,
) error {
	store.workflowTransitionCommands = append(
		store.workflowTransitionCommands,
		command,
	)

	return store.workflowTransitionError
}

func (
	store *lifecycleStoreSpy,
) ApplyNodeTransition(
	_ context.Context,
	command repository.NodeTransitionCommand,
) error {
	store.nodeTransitionCommands = append(
		store.nodeTransitionCommands,
		command,
	)

	return store.nodeTransitionError
}

func TestNewRecorderRejectsNilLifecycleStore(
	t *testing.T,
) {
	_, err := NewRecorder(nil)
	if err == nil {
		t.Fatal(
			"NewRecorder() accepted nil lifecycle store",
		)
	}
}

func TestRecorderMapsSuccessfulLifecycleToDeterministicCommands(
	t *testing.T,
) {
	fixture := newRecorderLifecycleFixture(t)

	if err := fixture.recorder.RecordWorkflowCreation(
		context.Background(),
		engine.WorkflowCreationObservation{
			Request:           fixture.request,
			WorkflowExecution: fixture.workflowCreated,
		},
	); err != nil {
		t.Fatalf(
			"RecordWorkflowCreation() returned an error: %v",
			err,
		)
	}

	workflowValidating := fixture.workflowCreated
	if err := workflowValidating.StartValidation(
		fixture.baseTime.Add(time.Minute),
	); err != nil {
		t.Fatalf(
			"StartValidation() returned an error: %v",
			err,
		)
	}

	if err := fixture.recorder.RecordWorkflowTransition(
		context.Background(),
		engine.WorkflowTransitionObservation{
			Request:      fixture.request,
			Before:       fixture.workflowCreated,
			After:        workflowValidating,
			TransitionAt: fixture.baseTime.Add(time.Minute),
		},
	); err != nil {
		t.Fatalf(
			"RecordWorkflowTransition(VALIDATING) returned an error: %v",
			err,
		)
	}

	nodePending := fixture.newPendingNodeExecution(
		t,
		fixture.baseTime.Add(2*time.Minute),
	)

	if err := fixture.recorder.RecordNodeExecutionsCreation(
		context.Background(),
		engine.NodeExecutionsCreationObservation{
			Request:           fixture.request,
			WorkflowExecution: workflowValidating,
			Items: []engine.NodeExecutionCreationItem{
				{
					Definition: fixture.nodeDefinition,
					Execution:  nodePending,
				},
			},
		},
	); err != nil {
		t.Fatalf(
			"RecordNodeExecutionsCreation() returned an error: %v",
			err,
		)
	}

	workflowRunning := workflowValidating
	if err := workflowRunning.Start(
		fixture.baseTime.Add(3 * time.Minute),
	); err != nil {
		t.Fatalf(
			"Start() returned an error: %v",
			err,
		)
	}

	if err := fixture.recorder.RecordWorkflowTransition(
		context.Background(),
		engine.WorkflowTransitionObservation{
			Request:      fixture.request,
			Before:       workflowValidating,
			After:        workflowRunning,
			TransitionAt: fixture.baseTime.Add(3 * time.Minute),
		},
	); err != nil {
		t.Fatalf(
			"RecordWorkflowTransition(RUNNING) returned an error: %v",
			err,
		)
	}

	nodeReady := nodePending
	if err := nodeReady.MarkReady(
		fixture.baseTime.Add(4 * time.Minute),
	); err != nil {
		t.Fatalf(
			"MarkReady() returned an error: %v",
			err,
		)
	}

	fixture.recordNodeTransition(
		t,
		workflowRunning,
		nodePending,
		nodeReady,
		fixture.baseTime.Add(4*time.Minute),
		runtime.NodeResult{},
		false,
	)

	nodeRunning := nodeReady
	if err := nodeRunning.Start(
		fixture.baseTime.Add(5 * time.Minute),
	); err != nil {
		t.Fatalf(
			"node Start() returned an error: %v",
			err,
		)
	}

	fixture.recordNodeTransition(
		t,
		workflowRunning,
		nodeReady,
		nodeRunning,
		fixture.baseTime.Add(5*time.Minute),
		runtime.NodeResult{},
		false,
	)

	terminalPayload, err := runtime.NewInlinePayload(
		runtime.ContentTypeApplicationJSON,
		[]byte(`{"result":"ok"}`),
		map[string]string{
			"source": "unit-test",
		},
		1024,
	)
	if err != nil {
		t.Fatalf(
			"NewInlinePayload() returned an error: %v",
			err,
		)
	}

	changes, err := runtime.NewContextChanges(nil, nil)
	if err != nil {
		t.Fatalf(
			"NewContextChanges() returned an error: %v",
			err,
		)
	}

	result, err := runtime.NewTerminalNodeSuccessResult(
		terminalPayload,
		changes,
	)
	if err != nil {
		t.Fatalf(
			"NewTerminalNodeSuccessResult() returned an error: %v",
			err,
		)
	}

	nodeSucceeded := nodeRunning
	if err := nodeSucceeded.Succeed(
		fixture.baseTime.Add(6 * time.Minute),
	); err != nil {
		t.Fatalf(
			"Succeed() returned an error: %v",
			err,
		)
	}

	fixture.recordNodeTransition(
		t,
		workflowRunning,
		nodeRunning,
		nodeSucceeded,
		fixture.baseTime.Add(6*time.Minute),
		result,
		true,
	)

	workflowSucceeded := workflowRunning
	if err := workflowSucceeded.Succeed(
		fixture.baseTime.Add(7 * time.Minute),
	); err != nil {
		t.Fatalf(
			"workflow Succeed() returned an error: %v",
			err,
		)
	}

	if err := fixture.recorder.RecordWorkflowTransition(
		context.Background(),
		engine.WorkflowTransitionObservation{
			Request:      fixture.request,
			Before:       workflowRunning,
			After:        workflowSucceeded,
			TransitionAt: fixture.baseTime.Add(7 * time.Minute),
		},
	); err != nil {
		t.Fatalf(
			"RecordWorkflowTransition(SUCCEEDED) returned an error: %v",
			err,
		)
	}

	requireSuccessfulLifecycleCommands(t, fixture.store)
}

func TestRecorderDoesNotAdvanceStateWhenNodeCreationPersistenceFails(
	t *testing.T,
) {
	fixture := newRecorderLifecycleFixture(t)

	if err := fixture.recorder.RecordWorkflowCreation(
		context.Background(),
		engine.WorkflowCreationObservation{
			Request:           fixture.request,
			WorkflowExecution: fixture.workflowCreated,
		},
	); err != nil {
		t.Fatalf(
			"RecordWorkflowCreation() returned an error: %v",
			err,
		)
	}

	workflowValidating := fixture.workflowCreated
	if err := workflowValidating.StartValidation(
		fixture.baseTime.Add(time.Minute),
	); err != nil {
		t.Fatalf(
			"StartValidation() returned an error: %v",
			err,
		)
	}

	if err := fixture.recorder.RecordWorkflowTransition(
		context.Background(),
		engine.WorkflowTransitionObservation{
			Request:      fixture.request,
			Before:       fixture.workflowCreated,
			After:        workflowValidating,
			TransitionAt: fixture.baseTime.Add(time.Minute),
		},
	); err != nil {
		t.Fatalf(
			"RecordWorkflowTransition() returned an error: %v",
			err,
		)
	}

	nodePending := fixture.newPendingNodeExecution(
		t,
		fixture.baseTime.Add(2*time.Minute),
	)

	observation := engine.NodeExecutionsCreationObservation{
		Request:           fixture.request,
		WorkflowExecution: workflowValidating,
		Items: []engine.NodeExecutionCreationItem{
			{
				Definition: fixture.nodeDefinition,
				Execution:  nodePending,
			},
		},
	}

	sentinel := errors.New(
		"controlled node creation persistence failure",
	)
	fixture.store.createNodeExecutionsError = sentinel

	err := fixture.recorder.RecordNodeExecutionsCreation(
		context.Background(),
		observation,
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf(
			"first RecordNodeExecutionsCreation() error = %v, want sentinel",
			err,
		)
	}

	fixture.store.createNodeExecutionsError = nil

	if err := fixture.recorder.RecordNodeExecutionsCreation(
		context.Background(),
		observation,
	); err != nil {
		t.Fatalf(
			"retry RecordNodeExecutionsCreation() returned an error: %v",
			err,
		)
	}

	if len(fixture.store.createNodeExecutionCommands) != 2 {
		t.Fatalf(
			"create node command count = %d, want 2",
			len(fixture.store.createNodeExecutionCommands),
		)
	}

	first := fixture.store.createNodeExecutionCommands[0]
	second := fixture.store.createNodeExecutionCommands[1]

	if first.ExpectedWorkflowLockVersion() !=
		second.ExpectedWorkflowLockVersion() {
		t.Fatalf(
			"retry workflow lock version = %d, want %d",
			second.ExpectedWorkflowLockVersion(),
			first.ExpectedWorkflowLockVersion(),
		)
	}

	if first.ExpectedNextSequenceNumber() !=
		second.ExpectedNextSequenceNumber() {
		t.Fatalf(
			"retry next sequence = %d, want %d",
			second.ExpectedNextSequenceNumber(),
			first.ExpectedNextSequenceNumber(),
		)
	}

	if first.ExpectedWorkflowLockVersion() != 1 {
		t.Fatalf(
			"expected workflow lock version = %d, want 1",
			first.ExpectedWorkflowLockVersion(),
		)
	}

	if first.ExpectedNextSequenceNumber() !=
		repository.SequenceNumber(5) {
		t.Fatalf(
			"expected next sequence = %d, want 5",
			first.ExpectedNextSequenceNumber(),
		)
	}
}

func TestFailureSummarySeparatesSafeAndTechnicalDetails(
	t *testing.T,
) {
	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryDependency,
		"DATABASE_UNAVAILABLE",
		"A required dependency is unavailable",
		true,
		map[string]string{
			"internalError": "password=secret-value",
		},
	)
	if err != nil {
		t.Fatalf(
			"NewRuntimeFailure() returned an error: %v",
			err,
		)
	}

	summary, err := marshalFailureSummary(failure)
	if err != nil {
		t.Fatalf(
			"marshalFailureSummary() returned an error: %v",
			err,
		)
	}

	if bytes.Contains(summary, []byte("secret-value")) {
		t.Fatalf(
			"safe failure summary contains technical secret: %s",
			summary,
		)
	}

	technicalDetail := "connection refused for password=secret-value"

	record, err := buildExecutionError(
		execution.WorkflowExecutionID("execution-1"),
		workflow.CompanyID("company-1"),
		execution.NodeExecutionID("execution-1/node/node-1"),
		repository.ExecutionEventID("execution-1/event/00000000000000000001"),
		failure,
		technicalDetail,
		time.Date(
			2026,
			time.July,
			18,
			12,
			0,
			0,
			0,
			time.UTC,
		),
	)
	if err != nil {
		t.Fatalf(
			"buildExecutionError() returned an error: %v",
			err,
		)
	}

	if record.SafeMessage() != failure.Message() {
		t.Fatalf(
			"safe message = %q, want %q",
			record.SafeMessage(),
			failure.Message(),
		)
	}

	actualTechnicalDetail, exists := record.TechnicalDetail()
	if !exists || actualTechnicalDetail != technicalDetail {
		t.Fatalf(
			"technical detail = %q, exists = %t",
			actualTechnicalDetail,
			exists,
		)
	}

	if !bytes.Contains(
		record.Details().Bytes(),
		[]byte("secret-value"),
	) {
		t.Fatalf(
			"structured error details do not contain technical diagnostic metadata: %s",
			record.Details().Bytes(),
		)
	}
}

type recorderLifecycleFixture struct {
	baseTime time.Time

	store    *lifecycleStoreSpy
	recorder *Recorder

	request         engine.ExecutionRequest
	nodeDefinition  workflow.NodeDefinition
	workflowCreated execution.WorkflowExecution
}

func newRecorderLifecycleFixture(
	t *testing.T,
) recorderLifecycleFixture {
	t.Helper()

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

	nodeDefinition, err := workflow.NewNodeDefinition(
		workflow.NodeID("node-1"),
		workflow.PluginType("core.terminal"),
		workflow.PluginVersion("1.0.0"),
		[]byte(`{}`),
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeDefinition() returned an error: %v",
			err,
		)
	}

	definition, err := workflow.NewWorkflowDefinition(
		workflow.WorkflowID("workflow-1"),
		workflow.CompanyID("company-1"),
		"Recorder Test Workflow",
		1,
		[]workflow.NodeDefinition{
			nodeDefinition,
		},
		nil,
		[]byte(`{"purpose":"recorder-test"}`),
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowDefinition() returned an error: %v",
			err,
		)
	}

	executionID := execution.WorkflowExecutionID(
		"execution-1",
	)

	request, err := engine.NewExecutionRequest(
		executionID,
		definition,
		"correlation-1",
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewExecutionRequest() returned an error: %v",
			err,
		)
	}

	workflowCreated, err := execution.NewWorkflowExecution(
		executionID,
		definition.CompanyID(),
		definition.ID(),
		definition.Revision(),
		execution.ExecutionModeSync,
		baseTime,
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowExecution() returned an error: %v",
			err,
		)
	}

	store := &lifecycleStoreSpy{}
	recorder, err := NewRecorder(store)
	if err != nil {
		t.Fatalf(
			"NewRecorder() returned an error: %v",
			err,
		)
	}

	return recorderLifecycleFixture{
		baseTime:        baseTime,
		store:           store,
		recorder:        recorder,
		request:         request,
		nodeDefinition:  nodeDefinition,
		workflowCreated: workflowCreated,
	}
}

func (
	fixture recorderLifecycleFixture,
) newPendingNodeExecution(
	t *testing.T,
	createdAt time.Time,
) execution.NodeExecution {
	t.Helper()

	nodeExecution, err := execution.NewNodeExecution(
		execution.NodeExecutionID(
			"execution-1/node/node-1",
		),
		fixture.workflowCreated.ID(),
		fixture.nodeDefinition.ID(),
		createdAt,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeExecution() returned an error: %v",
			err,
		)
	}

	return nodeExecution
}

func (
	fixture recorderLifecycleFixture,
) recordNodeTransition(
	t *testing.T,
	workflowExecution execution.WorkflowExecution,
	before execution.NodeExecution,
	after execution.NodeExecution,
	transitionAt time.Time,
	result runtime.NodeResult,
	hasResult bool,
) {
	t.Helper()

	if err := fixture.recorder.RecordNodeTransition(
		context.Background(),
		engine.NodeTransitionObservation{
			Request:           fixture.request,
			WorkflowExecution: workflowExecution,
			Definition:        fixture.nodeDefinition,
			Before:            before,
			After:             after,
			TransitionAt:      transitionAt,
			Result:            result,
			HasResult:         hasResult,
		},
	); err != nil {
		t.Fatalf(
			"RecordNodeTransition(%s) returned an error: %v",
			after.Status(),
			err,
		)
	}
}

func requireSuccessfulLifecycleCommands(
	t *testing.T,
	store *lifecycleStoreSpy,
) {
	t.Helper()

	if len(store.createExecutionCommands) != 1 {
		t.Fatalf(
			"create execution command count = %d, want 1",
			len(store.createExecutionCommands),
		)
	}

	creation := store.createExecutionCommands[0]
	if creation.WorkflowExecution().NextSequenceNumber() !=
		repository.SequenceNumber(1) {
		t.Fatalf(
			"creation command next sequence = %d, want 1",
			creation.WorkflowExecution().NextSequenceNumber(),
		)
	}

	if len(creation.Timeline()) != 2 {
		t.Fatalf(
			"creation timeline length = %d, want 2",
			len(creation.Timeline()),
		)
	}

	if len(store.createNodeExecutionCommands) != 1 {
		t.Fatalf(
			"create node command count = %d, want 1",
			len(store.createNodeExecutionCommands),
		)
	}

	nodeCreation := store.createNodeExecutionCommands[0]
	requireWorkflowGuard(
		t,
		nodeCreation.ExpectedWorkflowLockVersion(),
		nodeCreation.ExpectedNextSequenceNumber(),
		1,
		5,
	)

	if len(nodeCreation.Timeline()) != 1 {
		t.Fatalf(
			"node creation timeline length = %d, want 1",
			len(nodeCreation.Timeline()),
		)
	}

	if len(store.workflowTransitionCommands) != 3 {
		t.Fatalf(
			"workflow transition command count = %d, want 3",
			len(store.workflowTransitionCommands),
		)
	}

	validating := store.workflowTransitionCommands[0]
	requireWorkflowGuard(
		t,
		validating.ExpectedLockVersion(),
		validating.ExpectedNextSequenceNumber(),
		0,
		3,
	)
	requireWorkflowTargetConcurrency(
		t,
		validating.WorkflowExecution(),
		1,
		5,
	)

	running := store.workflowTransitionCommands[1]
	requireWorkflowGuard(
		t,
		running.ExpectedLockVersion(),
		running.ExpectedNextSequenceNumber(),
		2,
		6,
	)
	requireWorkflowTargetConcurrency(
		t,
		running.WorkflowExecution(),
		3,
		8,
	)

	succeeded := store.workflowTransitionCommands[2]
	requireWorkflowGuard(
		t,
		succeeded.ExpectedLockVersion(),
		succeeded.ExpectedNextSequenceNumber(),
		6,
		14,
	)
	requireWorkflowTargetConcurrency(
		t,
		succeeded.WorkflowExecution(),
		7,
		16,
	)

	if succeeded.WorkflowExecution().TerminalOutputs().String() == "{}" {
		t.Fatal(
			"successful workflow terminal outputs are empty",
		)
	}

	if len(store.nodeTransitionCommands) != 3 {
		t.Fatalf(
			"node transition command count = %d, want 3",
			len(store.nodeTransitionCommands),
		)
	}

	expectedNodeTransitions := []struct {
		workflowLock int64
		sequence     repository.SequenceNumber
		nodeLock     int64
		targetLock   int64
		status       execution.NodeExecutionStatus
	}{
		{
			workflowLock: 3,
			sequence:     8,
			nodeLock:     0,
			targetLock:   1,
			status:       execution.NodeExecutionStatusReady,
		},
		{
			workflowLock: 4,
			sequence:     10,
			nodeLock:     1,
			targetLock:   2,
			status:       execution.NodeExecutionStatusRunning,
		},
		{
			workflowLock: 5,
			sequence:     12,
			nodeLock:     2,
			targetLock:   3,
			status:       execution.NodeExecutionStatusSucceeded,
		},
	}

	for index, expected := range expectedNodeTransitions {
		command := store.nodeTransitionCommands[index]

		requireWorkflowGuard(
			t,
			command.ExpectedWorkflowLockVersion(),
			command.ExpectedNextSequenceNumber(),
			expected.workflowLock,
			expected.sequence,
		)

		if command.ExpectedNodeLockVersion() !=
			expected.nodeLock {
			t.Fatalf(
				"node transition[%d] expected node lock = %d, want %d",
				index,
				command.ExpectedNodeLockVersion(),
				expected.nodeLock,
			)
		}

		if command.NodeExecution().LockVersion() !=
			expected.targetLock {
			t.Fatalf(
				"node transition[%d] target node lock = %d, want %d",
				index,
				command.NodeExecution().LockVersion(),
				expected.targetLock,
			)
		}

		if command.NodeExecution().Status() !=
			expected.status {
			t.Fatalf(
				"node transition[%d] status = %q, want %q",
				index,
				command.NodeExecution().Status(),
				expected.status,
			)
		}
	}

	if _, exists := store.
		nodeTransitionCommands[2].
		NodeExecution().
		OutputSummary(); !exists {
		t.Fatal(
			"successful terminal node has no output summary",
		)
	}
}

func requireWorkflowGuard(
	t *testing.T,
	actualLock int64,
	actualSequence repository.SequenceNumber,
	expectedLock int64,
	expectedSequence repository.SequenceNumber,
) {
	t.Helper()

	if actualLock != expectedLock {
		t.Fatalf(
			"workflow guard lock = %d, want %d",
			actualLock,
			expectedLock,
		)
	}

	if actualSequence != expectedSequence {
		t.Fatalf(
			"workflow guard sequence = %d, want %d",
			actualSequence,
			expectedSequence,
		)
	}
}

func requireWorkflowTargetConcurrency(
	t *testing.T,
	record repository.WorkflowExecutionRecord,
	expectedLock int64,
	expectedSequence repository.SequenceNumber,
) {
	t.Helper()

	if record.LockVersion() != expectedLock {
		t.Fatalf(
			"target workflow lock = %d, want %d",
			record.LockVersion(),
			expectedLock,
		)
	}

	if record.NextSequenceNumber() != expectedSequence {
		t.Fatalf(
			"target workflow sequence = %d, want %d",
			record.NextSequenceNumber(),
			expectedSequence,
		)
	}
}
