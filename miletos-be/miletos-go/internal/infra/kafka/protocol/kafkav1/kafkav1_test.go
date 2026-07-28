package kafkav1

import (
	bytes "bytes"
	runtime "miletos-go/internal/engine/runtime"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	messaging "miletos-go/internal/ports/messaging"
	strings "strings"
	testing "testing"
	time "time"
)

func TestCodecRoundTripsNodeCommandAndResultEvent(t *testing.T) {
	codec, err := NewCodec(64*1024, testInlineLimit)
	if err != nil {
		t.Fatalf("NewCodec() returned an error: %v", err)
	}

	command, err := ToNodeCommandV1(testNodeCommand(t), testInlineLimit)
	if err != nil {
		t.Fatalf("ToNodeCommandV1() returned an error: %v", err)
	}
	encodedCommand, err := codec.EncodeNodeCommand(command)
	if err != nil {
		t.Fatalf("EncodeNodeCommand() returned an error: %v", err)
	}
	decodedCommand, err := codec.DecodeNodeCommand(encodedCommand)
	if err != nil {
		t.Fatalf("DecodeNodeCommand() returned an error: %v", err)
	}
	if decodedCommand.Envelope.MessageID != command.Envelope.MessageID {
		t.Fatal("command message ID changed during JSON round trip")
	}

	event, err := ToNodeResultEventV1(testSuccessEvent(t), testInlineLimit)
	if err != nil {
		t.Fatalf("ToNodeResultEventV1() returned an error: %v", err)
	}
	encodedEvent, err := codec.EncodeNodeResultEvent(event)
	if err != nil {
		t.Fatalf("EncodeNodeResultEvent() returned an error: %v", err)
	}
	decodedEvent, err := codec.DecodeNodeResultEvent(encodedEvent)
	if err != nil {
		t.Fatalf("DecodeNodeResultEvent() returned an error: %v", err)
	}
	if decodedEvent.Status != event.Status {
		t.Fatal("result status changed during JSON round trip")
	}
}

func TestCodecRejectsUnknownFieldsAtEveryStructBoundary(t *testing.T) {
	codec, err := NewCodec(64*1024, testInlineLimit)
	if err != nil {
		t.Fatalf("NewCodec() returned an error: %v", err)
	}
	command, _ := ToNodeCommandV1(testNodeCommand(t), testInlineLimit)
	encoded, err := codec.EncodeNodeCommand(command)
	if err != nil {
		t.Fatalf("EncodeNodeCommand() returned an error: %v", err)
	}

	topLevel := append([]byte(nil), encoded[:len(encoded)-1]...)
	topLevel = append(topLevel, []byte(`,"unknown":true}`)...)
	if _, err := codec.DecodeNodeCommand(topLevel); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("top-level unknown field error = %v", err)
	}

	nested := bytes.Replace(
		encoded,
		[]byte(`"message_id":"message-1"`),
		[]byte(`"message_id":"message-1","unknown":true`),
		1,
	)
	if _, err := codec.DecodeNodeCommand(nested); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("nested unknown field error = %v", err)
	}
}

func TestCodecRejectsEmptyTrailingAndOversizedMessages(t *testing.T) {
	codec, err := NewCodec(4096, 1024)
	if err != nil {
		t.Fatalf("NewCodec() returned an error: %v", err)
	}

	if _, err := codec.DecodeNodeCommand(nil); err == nil {
		t.Fatal("empty message was accepted")
	}
	if _, err := codec.DecodeNodeCommand([]byte(`{} {}`)); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
	if _, err := codec.DecodeNodeCommand(make([]byte, 4097)); err == nil {
		t.Fatal("oversized message was accepted")
	}
}

func TestCodecRejectsEncodedMessageAboveLimit(t *testing.T) {
	codec, err := NewCodec(512, 128)
	if err != nil {
		t.Fatalf("NewCodec() returned an error: %v", err)
	}
	command, err := ToNodeCommandV1(testNodeCommand(t), testInlineLimit)
	if err != nil {
		t.Fatalf("ToNodeCommandV1() returned an error: %v", err)
	}

	if _, err := codec.EncodeNodeCommand(command); err == nil ||
		!strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("encoded size error = %v", err)
	}
}

func TestNewCodecRejectsInvalidLimits(t *testing.T) {
	tests := [][2]int{{0, 1}, {1, 0}, {1024, 2048}}
	for _, limits := range tests {
		if _, err := NewCodec(limits[0], limits[1]); err == nil {
			t.Fatalf("NewCodec(%d, %d) accepted invalid limits", limits[0], limits[1])
		}
	}
}

func TestNodeCommandV1RejectsUnsupportedEnvelopeAndMode(t *testing.T) {
	command, err := ToNodeCommandV1(testNodeCommand(t), testInlineLimit)
	if err != nil {
		t.Fatalf("ToNodeCommandV1() returned an error: %v", err)
	}

	tests := map[string]func(NodeCommandV1) NodeCommandV1{
		"unknown version": func(value NodeCommandV1) NodeCommandV1 {
			value.Envelope.MessageVersion = 2
			return value
		},
		"wrong type": func(value NodeCommandV1) NodeCommandV1 {
			value.Envelope.MessageType = MessageTypeNodeResultEventV1
			return value
		},
		"sync mode": func(value NodeCommandV1) NodeCommandV1 {
			value.ExecutionMode = "SYNC"
			return value
		},
		"attempt 2": func(value NodeCommandV1) NodeCommandV1 {
			value.Envelope.Attempt = 2
			return value
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if err := mutate(command).Validate(testInlineLimit); err == nil {
				t.Fatal("invalid command contract was accepted")
			}
		})
	}
}

func TestNodeCommandV1RejectsInvalidDeadlineAndConfiguration(t *testing.T) {
	command, err := ToNodeCommandV1(testNodeCommand(t), testInlineLimit)
	if err != nil {
		t.Fatalf("ToNodeCommandV1() returned an error: %v", err)
	}

	deadline := command.Envelope.CreatedAt.Add(-time.Second)
	command.DeadlineAt = &deadline
	if err := command.Validate(testInlineLimit); err == nil {
		t.Fatal("expired command deadline was accepted")
	}

	command, _ = ToNodeCommandV1(testNodeCommand(t), testInlineLimit)
	command.Configuration = []byte(`[]`)
	if err := command.Validate(testInlineLimit); err == nil {
		t.Fatal("non-object node configuration was accepted")
	}
}

func TestNodeResultEventV1RejectsInvalidStatusCombinations(t *testing.T) {
	success, err := ToNodeResultEventV1(testSuccessEvent(t), testInlineLimit)
	if err != nil {
		t.Fatalf("ToNodeResultEventV1() returned an error: %v", err)
	}

	failure, err := ToNodeResultEventV1(testFailureEvent(t), testInlineLimit)
	if err != nil {
		t.Fatalf("ToNodeResultEventV1() returned an error: %v", err)
	}

	tests := map[string]NodeResultEventV1{
		"success with failure": func() NodeResultEventV1 {
			value := success
			value.Failure = failure.Failure
			return value
		}(),
		"failure without failure": func() NodeResultEventV1 {
			value := failure
			value.Failure = nil
			return value
		}(),
		"failure with outputs": func() NodeResultEventV1 {
			value := failure
			value.Outputs = success.Outputs
			return value
		}(),
		"unknown status": func() NodeResultEventV1 {
			value := success
			value.Status = "RETRIED"
			return value
		}(),
	}

	for name, event := range tests {
		t.Run(name, func(t *testing.T) {
			if err := event.Validate(testInlineLimit); err == nil {
				t.Fatal("invalid result event contract was accepted")
			}
		})
	}
}

func TestNodeResultEventV1RejectsMismatchedInterruptedFailure(t *testing.T) {
	event, err := ToNodeResultEventV1(
		testInterruptedEvent(t, messaging.NodeResultStatusTimedOut),
		testInlineLimit,
	)
	if err != nil {
		t.Fatalf("ToNodeResultEventV1() returned an error: %v", err)
	}

	event.Failure.Category = "CANCELED"
	if err := event.Validate(testInlineLimit); err == nil {
		t.Fatal("TIMED_OUT event accepted CANCELED failure")
	}
}

func TestNodeCommandMapperRoundTrip(t *testing.T) {
	original := testNodeCommand(t)

	transport, err := ToNodeCommandV1(original, testInlineLimit)
	if err != nil {
		t.Fatalf("ToNodeCommandV1() returned an error: %v", err)
	}
	if transport.Envelope.MessageType != MessageTypeNodeCommandV1 ||
		transport.Envelope.MessageVersion != MessageVersionV1 {
		t.Fatal("node command envelope type/version mismatch")
	}
	if transport.ExecutionMode != "ASYNC" {
		t.Fatalf("ExecutionMode = %q, want ASYNC", transport.ExecutionMode)
	}

	roundTrip, err := FromNodeCommandV1(transport, testInlineLimit)
	if err != nil {
		t.Fatalf("FromNodeCommandV1() returned an error: %v", err)
	}
	if !roundTrip.IsValid() {
		t.Fatal("round-trip node command is invalid")
	}
	if roundTrip.Metadata().MessageID() != original.Metadata().MessageID() {
		t.Fatal("message ID changed during round trip")
	}
	if roundTrip.NodeDefinition().PluginType() != original.NodeDefinition().PluginType() {
		t.Fatal("plugin type changed during round trip")
	}
	if roundTrip.Input().TotalPayloadCount() != 1 {
		t.Fatalf("round-trip input payload count = %d, want 1", roundTrip.Input().TotalPayloadCount())
	}
}

func TestNodeResultEventMapperRoundTripsSupportedStatuses(t *testing.T) {
	events := map[string]messaging.NodeResultEvent{
		"success":   testSuccessEvent(t),
		"failure":   testFailureEvent(t),
		"cancelled": testInterruptedEvent(t, messaging.NodeResultStatusCancelled),
		"timed out": testInterruptedEvent(t, messaging.NodeResultStatusTimedOut),
	}

	for name, original := range events {
		t.Run(name, func(t *testing.T) {
			transport, err := ToNodeResultEventV1(original, testInlineLimit)
			if err != nil {
				t.Fatalf("ToNodeResultEventV1() returned an error: %v", err)
			}
			roundTrip, err := FromNodeResultEventV1(transport, testInlineLimit)
			if err != nil {
				t.Fatalf("FromNodeResultEventV1() returned an error: %v", err)
			}
			if roundTrip.Status() != original.Status() || !roundTrip.IsValid() {
				t.Fatalf("round-trip status = %s, want %s", roundTrip.Status(), original.Status())
			}
		})
	}
}

func TestPayloadMapperSupportsArtifactReferences(t *testing.T) {
	reference, err := runtime.NewArtifactReference(
		"artifact-1",
		"s3://bucket/object",
		runtime.ContentTypeApplicationOctetStream,
		8192,
		"sha256:abc",
		map[string]string{"tenant": "safe"},
	)
	if err != nil {
		t.Fatalf("NewArtifactReference() returned an error: %v", err)
	}
	payload, err := runtime.NewArtifactPayload(reference, map[string]string{"kind": "binary"})
	if err != nil {
		t.Fatalf("NewArtifactPayload() returned an error: %v", err)
	}

	transport, err := payloadToV1(payload, testInlineLimit)
	if err != nil {
		t.Fatalf("payloadToV1() returned an error: %v", err)
	}
	if transport.Artifact == nil || transport.InlineData != nil {
		t.Fatal("artifact payload transport source mismatch")
	}

	roundTrip, err := payloadFromV1(transport, testInlineLimit)
	if err != nil {
		t.Fatalf("payloadFromV1() returned an error: %v", err)
	}
	artifact, exists := roundTrip.Artifact()
	if !exists || artifact.ID() != reference.ID() || artifact.Location() != reference.Location() {
		t.Fatal("artifact reference changed during round trip")
	}
}

func TestPayloadMapperRejectsOversizedInlineAndAmbiguousSources(t *testing.T) {
	oversized := PayloadV1{
		ContentType: runtime.ContentTypeApplicationJSON.String(),
		InlineData:  make([]byte, testInlineLimit+1),
	}
	if _, err := payloadFromV1(oversized, testInlineLimit); err == nil {
		t.Fatal("oversized inline payload was accepted")
	}

	ambiguous := PayloadV1{
		ContentType: runtime.ContentTypeApplicationJSON.String(),
		InlineData:  []byte(`{}`),
		Artifact: &ArtifactReferenceV1{
			ID:       "artifact-1",
			Location: "s3://bucket/object",
		},
	}
	if _, err := payloadFromV1(ambiguous, testInlineLimit); err == nil {
		t.Fatal("payload with inline and artifact sources was accepted")
	}
}

const testInlineLimit = 4096

func testNodeCommand(t *testing.T) messaging.NodeCommand {
	t.Helper()
	metadata := testMetadata(t)
	node, err := workflow.NewNodeDefinition(
		metadata.NodeID(),
		workflow.PluginType("core.pass-through"),
		workflow.PluginVersion("v1"),
		[]byte(`{"enabled":true}`),
		nil,
	)
	if err != nil {
		t.Fatalf("NewNodeDefinition() returned an error: %v", err)
	}

	payload := testPayload(t, `{"value":"transport"}`)
	input, err := runtime.NewNodeInput(
		map[string][]runtime.Payload{"input": {payload}},
	)
	if err != nil {
		t.Fatalf("NewNodeInput() returned an error: %v", err)
	}

	variable, err := runtime.NewRuntimeValue([]byte(`{"region":"eu"}`))
	if err != nil {
		t.Fatalf("NewRuntimeValue() returned an error: %v", err)
	}
	deadline := metadata.CreatedAt().Add(5 * time.Minute)

	command, err := messaging.NewNodeCommand(
		metadata,
		node,
		input,
		map[string]runtime.RuntimeValue{"settings": variable},
		&deadline,
	)
	if err != nil {
		t.Fatalf("NewNodeCommand() returned an error: %v", err)
	}
	return command
}

func testSuccessEvent(t *testing.T) messaging.NodeResultEvent {
	t.Helper()
	payload := testPayload(t, `{"value":"result"}`)
	value, err := runtime.NewRuntimeValue([]byte(`"complete"`))
	if err != nil {
		t.Fatalf("NewRuntimeValue() returned an error: %v", err)
	}
	changes, err := runtime.NewContextChanges(
		map[string]runtime.RuntimeValue{"state": value},
		[]string{"temporary"},
	)
	if err != nil {
		t.Fatalf("NewContextChanges() returned an error: %v", err)
	}
	result, err := runtime.NewNodeSuccessResult(
		map[string][]runtime.Payload{"output": {payload}},
		changes,
	)
	if err != nil {
		t.Fatalf("NewNodeSuccessResult() returned an error: %v", err)
	}
	metadata := testResultMetadata(t)
	event, err := messaging.NewNodeResultEvent(
		metadata,
		result,
		metadata.CreatedAt().Add(-2*time.Second),
		metadata.CreatedAt().Add(-time.Second),
	)
	if err != nil {
		t.Fatalf("NewNodeResultEvent() returned an error: %v", err)
	}
	return event
}

func testFailureEvent(t *testing.T) messaging.NodeResultEvent {
	t.Helper()
	failure := testFailure(t, runtime.FailureCategoryExecution)
	result, err := runtime.NewNodeFailureResult(failure)
	if err != nil {
		t.Fatalf("NewNodeFailureResult() returned an error: %v", err)
	}
	metadata := testResultMetadata(t)
	event, err := messaging.NewNodeResultEvent(
		metadata,
		result,
		metadata.CreatedAt().Add(-2*time.Second),
		metadata.CreatedAt().Add(-time.Second),
	)
	if err != nil {
		t.Fatalf("NewNodeResultEvent() returned an error: %v", err)
	}
	return event
}

func testInterruptedEvent(
	t *testing.T,
	status messaging.NodeResultStatus,
) messaging.NodeResultEvent {
	t.Helper()
	category := runtime.FailureCategoryCanceled
	if status == messaging.NodeResultStatusTimedOut {
		category = runtime.FailureCategoryTimeout
	}
	metadata := testResultMetadata(t)
	event, err := messaging.NewInterruptedNodeResultEvent(
		metadata,
		status,
		testFailure(t, category),
		metadata.CreatedAt().Add(-2*time.Second),
		metadata.CreatedAt().Add(-time.Second),
	)
	if err != nil {
		t.Fatalf("NewInterruptedNodeResultEvent() returned an error: %v", err)
	}
	return event
}

func testMetadata(t *testing.T) messaging.MessageMetadata {
	t.Helper()
	return testMetadataAt(
		t,
		time.Date(2026, time.July, 18, 7, 0, 0, 0, time.UTC),
	)
}

func testResultMetadata(t *testing.T) messaging.MessageMetadata {
	t.Helper()
	return testMetadataAt(
		t,
		time.Date(2026, time.July, 18, 7, 0, 3, 0, time.UTC),
	)
}

func testMetadataAt(t *testing.T, createdAt time.Time) messaging.MessageMetadata {
	t.Helper()
	metadata, err := messaging.NewMessageMetadata(
		messaging.MessageMetadataParams{
			MessageID: "message-1",
			CreatedAt: createdAt,

			CompanyID:           workflow.CompanyID("company-1"),
			WorkflowID:          workflow.WorkflowID("workflow-1"),
			WorkflowExecutionID: execution.WorkflowExecutionID("execution-1"),
			NodeID:              workflow.NodeID("node-1"),
			NodeExecutionID:     execution.NodeExecutionID("node-execution-1"),

			Attempt: 1,

			CorrelationID: "correlation-1",
			CausationID:   "causation-1",
		},
	)
	if err != nil {
		t.Fatalf("NewMessageMetadata() returned an error: %v", err)
	}
	return metadata
}

func testPayload(t *testing.T, data string) runtime.Payload {
	t.Helper()
	payload, err := runtime.NewInlinePayload(
		runtime.ContentTypeApplicationJSON,
		[]byte(data),
		map[string]string{"source": "test"},
		testInlineLimit,
	)
	if err != nil {
		t.Fatalf("NewInlinePayload() returned an error: %v", err)
	}
	return payload
}

func testFailure(t *testing.T, category runtime.FailureCategory) runtime.RuntimeFailure {
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
