package messaging

import (
	"testing"
	"time"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

func TestMessageMetadataNormalizesAndValidatesIdentity(t *testing.T) {
	metadata := testMessageMetadata(t)

	if metadata.MessageID() != "message-1" {
		t.Fatalf("MessageID() = %q, want message-1", metadata.MessageID())
	}
	if metadata.Attempt() != 1 {
		t.Fatalf("Attempt() = %d, want 1", metadata.Attempt())
	}
	if !metadata.IsValid() {
		t.Fatal("message metadata must be valid")
	}
}

func TestMessageMetadataRejectsUnsupportedAttempt(t *testing.T) {
	params := testMessageMetadataParams()
	params.Attempt = 2

	if _, err := NewMessageMetadata(params); err == nil {
		t.Fatal("attempt 2 was accepted")
	}
}

func TestNodeCommandStoresDefensiveRuntimeSnapshots(t *testing.T) {
	metadata := testMessageMetadata(t)
	node := testNodeDefinition(t)
	payload := testInlinePayload(t, `{"value":"original"}`)
	input, err := runtime.NewNodeInput(map[string][]runtime.Payload{"input": {payload}})
	if err != nil {
		t.Fatalf("NewNodeInput() returned an error: %v", err)
	}
	variable, err := runtime.NewRuntimeValue([]byte(`{"enabled":true}`))
	if err != nil {
		t.Fatalf("NewRuntimeValue() returned an error: %v", err)
	}
	variables := map[string]runtime.RuntimeValue{"settings": variable}
	deadline := metadata.CreatedAt().Add(time.Minute)

	command, err := NewNodeCommand(metadata, node, input, variables, &deadline)
	if err != nil {
		t.Fatalf("NewNodeCommand() returned an error: %v", err)
	}

	delete(variables, "settings")
	returned := command.Variables()
	delete(returned, "settings")

	if len(command.Variables()) != 1 {
		t.Fatal("command variables were mutated through caller-owned maps")
	}
	if !command.IsValid() {
		t.Fatal("node command must be valid")
	}
	if actual, exists := command.DeadlineAt(); !exists || !actual.Equal(deadline) {
		t.Fatalf("DeadlineAt() = %v, %t; want %v, true", actual, exists, deadline)
	}
}

func TestNodeCommandRejectsMismatchedNodeIdentityAndExpiredDeadline(t *testing.T) {
	metadata := testMessageMetadata(t)
	node := testNodeDefinitionWithID(t, "other-node")
	input, err := runtime.NewNodeInput(nil)
	if err != nil {
		t.Fatalf("NewNodeInput() returned an error: %v", err)
	}

	if _, err := NewNodeCommand(metadata, node, input, nil, nil); err == nil {
		t.Fatal("mismatched node identity was accepted")
	}

	node = testNodeDefinition(t)
	deadline := metadata.CreatedAt()
	if _, err := NewNodeCommand(metadata, node, input, nil, &deadline); err == nil {
		t.Fatal("non-future deadline was accepted")
	}
}

func TestNodeResultEventSupportsSuccessFailureCancellationAndTimeout(t *testing.T) {
	metadata := testMessageMetadata(t)
	startedAt := metadata.CreatedAt().Add(-2 * time.Second)
	finishedAt := metadata.CreatedAt().Add(-time.Second)

	success, err := runtime.NewNodeSuccessResult(nil, runtime.ContextChanges{})
	if err != nil {
		t.Fatalf("NewNodeSuccessResult() returned an error: %v", err)
	}
	successEvent, err := NewNodeResultEvent(metadata, success, startedAt, finishedAt)
	if err != nil {
		t.Fatalf("NewNodeResultEvent(success) returned an error: %v", err)
	}
	if successEvent.Status() != NodeResultStatusSucceeded || !successEvent.IsValid() {
		t.Fatal("successful event is invalid")
	}

	failure := testRuntimeFailure(t, runtime.FailureCategoryExecution)
	failedResult, err := runtime.NewNodeFailureResult(failure)
	if err != nil {
		t.Fatalf("NewNodeFailureResult() returned an error: %v", err)
	}
	failedEvent, err := NewNodeResultEvent(metadata, failedResult, startedAt, finishedAt)
	if err != nil {
		t.Fatalf("NewNodeResultEvent(failure) returned an error: %v", err)
	}
	if failedEvent.Status() != NodeResultStatusFailed || !failedEvent.IsValid() {
		t.Fatal("failed event is invalid")
	}

	cancelledFailure := testRuntimeFailure(t, runtime.FailureCategoryCanceled)
	cancelledEvent, err := NewInterruptedNodeResultEvent(
		metadata,
		NodeResultStatusCancelled,
		cancelledFailure,
		startedAt,
		finishedAt,
	)
	if err != nil || !cancelledEvent.IsValid() {
		t.Fatalf("cancelled event is invalid: %v", err)
	}

	timeoutFailure := testRuntimeFailure(t, runtime.FailureCategoryTimeout)
	timedOutEvent, err := NewInterruptedNodeResultEvent(
		metadata,
		NodeResultStatusTimedOut,
		timeoutFailure,
		startedAt,
		finishedAt,
	)
	if err != nil || !timedOutEvent.IsValid() {
		t.Fatalf("timed-out event is invalid: %v", err)
	}
}

func TestInterruptedNodeResultEventRequiresMatchingFailureCategory(t *testing.T) {
	metadata := testMessageMetadata(t)
	startedAt := metadata.CreatedAt().Add(-2 * time.Second)
	finishedAt := metadata.CreatedAt().Add(-time.Second)
	failure := testRuntimeFailure(t, runtime.FailureCategoryExecution)

	if _, err := NewInterruptedNodeResultEvent(
		metadata,
		NodeResultStatusTimedOut,
		failure,
		startedAt,
		finishedAt,
	); err == nil {
		t.Fatal("TIMED_OUT event accepted non-timeout failure")
	}
}

func testMessageMetadata(t *testing.T) MessageMetadata {
	t.Helper()
	metadata, err := NewMessageMetadata(testMessageMetadataParams())
	if err != nil {
		t.Fatalf("NewMessageMetadata() returned an error: %v", err)
	}
	return metadata
}

func testMessageMetadataParams() MessageMetadataParams {
	return MessageMetadataParams{
		MessageID: " message-1 ",
		CreatedAt: time.Date(2026, time.July, 18, 10, 0, 0, 0, time.FixedZone("test", 3*60*60)),

		CompanyID:           workflow.CompanyID("company-1"),
		WorkflowID:          workflow.WorkflowID("workflow-1"),
		WorkflowExecutionID: execution.WorkflowExecutionID("execution-1"),
		NodeID:              workflow.NodeID("node-1"),
		NodeExecutionID:     execution.NodeExecutionID("node-execution-1"),

		Attempt: 1,

		CorrelationID: " correlation-1 ",
		CausationID:   " causation-1 ",
	}
}

func testNodeDefinition(t *testing.T) workflow.NodeDefinition {
	t.Helper()
	return testNodeDefinitionWithID(t, "node-1")
}

func testNodeDefinitionWithID(t *testing.T, id string) workflow.NodeDefinition {
	t.Helper()
	node, err := workflow.NewNodeDefinition(
		workflow.NodeID(id),
		workflow.PluginType("core.pass-through"),
		workflow.PluginVersion("v1"),
		[]byte(`{"enabled":true}`),
		nil,
	)
	if err != nil {
		t.Fatalf("NewNodeDefinition() returned an error: %v", err)
	}
	return node
}

func testInlinePayload(t *testing.T, data string) runtime.Payload {
	t.Helper()
	payload, err := runtime.NewInlinePayload(
		runtime.ContentTypeApplicationJSON,
		[]byte(data),
		map[string]string{"source": "test"},
		4096,
	)
	if err != nil {
		t.Fatalf("NewInlinePayload() returned an error: %v", err)
	}
	return payload
}

func testRuntimeFailure(t *testing.T, category runtime.FailureCategory) runtime.RuntimeFailure {
	t.Helper()
	failure, err := runtime.NewRuntimeFailure(
		category,
		"TEST_FAILURE",
		"Controlled test failure",
		false,
		map[string]string{"safe": "true"},
	)
	if err != nil {
		t.Fatalf("NewRuntimeFailure() returned an error: %v", err)
	}
	return failure
}
