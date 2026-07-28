package kafkav1

import (
	"encoding/json"
	"fmt"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/ports/messaging"
)

func ToNodeCommandV1(command messaging.NodeCommand, maximumInlinePayloadBytes int,
) (NodeCommandV1, error) {
	if !command.IsValid() {
		return NodeCommandV1{}, fmt.Errorf("node command must be valid")
	}
	input, err := nodeInputToV1(command.Input(), maximumInlinePayloadBytes)
	if err != nil {
		return NodeCommandV1{}, fmt.Errorf("map node input: %w", err)
	}
	variables, err := runtimeValuesToV1(command.Variables())
	if err != nil {
		return NodeCommandV1{}, fmt.Errorf("map variables: %w", err)
	}
	node := command.NodeDefinition()
	transport := NodeCommandV1{Envelope: envelopeFromMetadata(
		command.Metadata(), MessageTypeNodeCommandV1),
		ExecutionMode: execution.ExecutionModeAsync.String(),
		PluginType:    node.PluginType().String(), PluginVersion: node.PluginVersion().String(), Configuration: json.RawMessage(node.Configuration().Bytes()),
		Input: input, Variables: variables,
	}
	if deadline, exists := command.DeadlineAt(); exists {
		value := deadline
		transport.DeadlineAt = &value
	}
	if err := transport.Validate(maximumInlinePayloadBytes); err != nil {
		return NodeCommandV1{}, fmt.Errorf("mapped node command is invalid: %w", err)
	}
	return transport, nil
}
func FromNodeCommandV1(
	transport NodeCommandV1, maximumInlinePayloadBytes int) (messaging.NodeCommand, error) {
	if err := transport.Validate(maximumInlinePayloadBytes); err != nil {
		return messaging.NodeCommand{}, fmt.Errorf("node command v1: %w", err)
	}
	metadata, err := metadataFromEnvelope(transport.Envelope,
		MessageTypeNodeCommandV1)
	if err != nil {
		return messaging.NodeCommand{}, err
	}
	nodeID, _ := workflow.NewNodeID(transport.Envelope.NodeID)
	pluginType, _ := workflow.NewPluginType(transport.PluginType)
	pluginVersion, _ := workflow.NewPluginVersion(transport.PluginVersion)
	node, err := workflow.NewNodeDefinition(nodeID,
		pluginType, pluginVersion, transport.Configuration,
		nil)
	if err != nil {
		return messaging.NodeCommand{}, fmt.Errorf("map node definition: %w", err)
	}
	input, err := nodeInputFromV1(transport.Input, maximumInlinePayloadBytes)
	if err != nil {
		return messaging.NodeCommand{}, fmt.Errorf("map node input: %w", err)
	}
	variables, err := runtimeValuesFromV1(transport.Variables)
	if err != nil {
		return messaging.NodeCommand{}, fmt.Errorf("map variables: %w", err)
	}
	return messaging.NewNodeCommand(metadata,
		node, input, variables,
		transport.DeadlineAt)
}
func ToNodeResultEventV1(event messaging.NodeResultEvent,
	maximumInlinePayloadBytes int) (NodeResultEventV1, error) {
	if !event.IsValid() {
		return NodeResultEventV1{}, fmt.Errorf("node result event must be valid")
	}
	transport := NodeResultEventV1{Envelope: envelopeFromMetadata(event.Metadata(),
		MessageTypeNodeResultEventV1), Status: event.Status().String(),
		Outputs: map[string][]PayloadV1{}, ContextChanges: ContextChangesV1{Set: map[string]json.RawMessage{}},
		StartedAt: event.StartedAt(), FinishedAt: event.FinishedAt(),
	}
	if result, exists := event.Result(); exists {
		outputs, err := payloadCollectionsToV1(result.OutputsSnapshot(), maximumInlinePayloadBytes)
		if err != nil {
			return NodeResultEventV1{}, fmt.Errorf("map outputs: %w", err)
		}
		transport.Outputs = outputs
		if terminal, exists := result.TerminalOutput(); exists {
			mapped, err := payloadToV1(terminal, maximumInlinePayloadBytes)
			if err != nil {
				return NodeResultEventV1{}, fmt.Errorf("map terminal output: %w", err)
			}
			transport.TerminalOutput = &mapped
		}
		changes, err := contextChangesToV1(result.ContextChanges())
		if err != nil {
			return NodeResultEventV1{}, fmt.Errorf("map context changes: %w", err)
		}
		transport.ContextChanges = changes
	}
	if failure, exists := event.Failure(); exists {
		mapped := failureToV1(failure)
		transport.Failure = &mapped
	}
	if err := transport.Validate(maximumInlinePayloadBytes); err != nil {
		return NodeResultEventV1{}, fmt.Errorf("mapped node result event is invalid: %w", err)
	}
	return transport, nil
}
func FromNodeResultEventV1(transport NodeResultEventV1, maximumInlinePayloadBytes int,
) (messaging.NodeResultEvent, error) {
	if err := transport.Validate(maximumInlinePayloadBytes); err != nil {
		return messaging.NodeResultEvent{}, fmt.Errorf("node result event v1: %w", err)
	}
	metadata, err := metadataFromEnvelope(
		transport.Envelope, MessageTypeNodeResultEventV1)
	if err != nil {
		return messaging.NodeResultEvent{}, err
	}
	result, interruptedFailure, err := resultPartsFromV1(transport,
		maximumInlinePayloadBytes)
	if err != nil {
		return messaging.NodeResultEvent{}, err
	}
	status := messaging.NodeResultStatus(transport.Status)
	if status == messaging.NodeResultStatusSucceeded || status == messaging.NodeResultStatusFailed {
		return messaging.NewNodeResultEvent(metadata, result,
			transport.StartedAt, transport.FinishedAt)
	}
	return messaging.NewInterruptedNodeResultEvent(
		metadata, status, interruptedFailure,
		transport.StartedAt, transport.FinishedAt)
}
func envelopeFromMetadata(
	metadata messaging.MessageMetadata, messageType string) MessageEnvelopeV1 {
	return MessageEnvelopeV1{MessageID: metadata.MessageID(), MessageType: messageType,
		MessageVersion: MessageVersionV1, CreatedAt: metadata.CreatedAt(),
		CompanyID: metadata.CompanyID().String(), WorkflowID: metadata.WorkflowID().String(), WorkflowExecutionID: metadata.WorkflowExecutionID().String(),
		NodeID: metadata.NodeID().String(), NodeExecutionID: metadata.NodeExecutionID().String(),
		Attempt: metadata.Attempt(), CorrelationID: metadata.CorrelationID(),
		CausationID: metadata.CausationID()}
}
func nodeInputToV1(input runtime.NodeInput,
	maximumInlinePayloadBytes int) (NodeInputV1, error) {
	if !input.IsValid() {
		return NodeInputV1{}, fmt.Errorf("node input must be valid")
	}
	ports := make(map[string][]PayloadV1, input.PortCount())
	for _, port := range input.Ports() {
		payloads, exists, err := input.Payloads(port)
		if err != nil {
			return NodeInputV1{}, err
		}
		if !exists {
			return NodeInputV1{}, fmt.Errorf("input port %q is unavailable", port)
		}
		mapped, err := payloadSliceToV1(payloads, maximumInlinePayloadBytes)
		if err != nil {
			return NodeInputV1{}, fmt.Errorf("port %q: %w", port, err)
		}
		ports[port] = mapped
	}
	return NodeInputV1{Ports: ports}, nil
}
func nodeInputFromV1(
	input NodeInputV1, maximumInlinePayloadBytes int) (runtime.NodeInput, error) {
	ports := make(map[string][]runtime.Payload, len(input.Ports))
	for port, payloads := range input.Ports {
		if len(payloads) == 0 {
			return runtime.NodeInput{}, fmt.Errorf("port %q must contain at least one payload", port)
		}
		mapped := make([]runtime.Payload, len(payloads))
		for index, payload := range payloads {
			value, err := payloadFromV1(payload, maximumInlinePayloadBytes)
			if err != nil {
				return runtime.NodeInput{}, fmt.Errorf("port %q payload %d: %w", port, index, err)
			}
			mapped[index] = value
		}
		ports[port] = mapped
	}
	return runtime.NewNodeInput(ports)
}
func payloadCollectionsToV1(
	collections map[string][]runtime.Payload, maximumInlinePayloadBytes int) (map[string][]PayloadV1, error) {
	result := make(map[string][]PayloadV1, len(collections))
	for port, payloads := range collections {
		mapped, err := payloadSliceToV1(payloads, maximumInlinePayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("port %q: %w", port, err)
		}
		result[port] = mapped
	}
	return result, nil
}
func payloadSliceToV1(
	payloads []runtime.Payload, maximumInlinePayloadBytes int) ([]PayloadV1, error) {
	result := make([]PayloadV1, len(payloads))
	for index, payload := range payloads {
		mapped, err := payloadToV1(payload, maximumInlinePayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("payload %d: %w", index, err)
		}
		result[index] = mapped
	}
	return result, nil
}
func payloadToV1(
	payload runtime.Payload, maximumInlinePayloadBytes int) (PayloadV1, error) {
	if data, inline := payload.InlineData(); inline {
		if len(data) > maximumInlinePayloadBytes {
			return PayloadV1{}, fmt.Errorf("inline payload exceeds limit")
		}
		return PayloadV1{ContentType: payload.ContentType().String(),
			InlineData: data, Metadata: payload.Metadata()}, nil
	}
	artifact, exists := payload.Artifact()
	if !exists {
		return PayloadV1{}, fmt.Errorf("payload source is unavailable")
	}
	return PayloadV1{ContentType: payload.ContentType().String(),
		InlineData: nil, Artifact: &ArtifactReferenceV1{ID: artifact.ID(),
			Location: artifact.Location(), SizeBytes: artifact.SizeBytes(), Checksum: artifact.Checksum(),
			Metadata: artifact.Metadata()}, Metadata: payload.Metadata(),
	}, nil
}
func payloadFromV1(payload PayloadV1, maximumInlinePayloadBytes int,
) (runtime.Payload, error) {
	contentType, err := runtime.NewContentType(payload.ContentType)
	if err != nil {
		return runtime.Payload{}, err
	}
	if payload.Artifact == nil {
		if payload.InlineData == nil {
			return runtime.Payload{}, fmt.Errorf("inline_data must be present for an inline payload")
		}
		return runtime.NewInlinePayload(contentType,
			payload.InlineData, payload.Metadata, maximumInlinePayloadBytes,
		)
	}
	if payload.InlineData != nil {
		return runtime.Payload{}, fmt.Errorf("artifact payload must not contain inline_data")
	}
	reference, err := runtime.NewArtifactReference(payload.Artifact.ID,
		payload.Artifact.Location, contentType, payload.Artifact.SizeBytes,
		payload.Artifact.Checksum, payload.Artifact.Metadata)
	if err != nil {
		return runtime.Payload{}, err
	}
	return runtime.NewArtifactPayload(reference, payload.Metadata)
}
func runtimeValuesToV1(values map[string]runtime.RuntimeValue,
) (map[string]json.RawMessage, error) {
	if len(values) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	result := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		if !value.IsValid() {
			return nil, fmt.Errorf("variable %q must be valid", key)
		}
		result[key] = json.RawMessage(value.Bytes())
	}
	return result, nil
}
func runtimeValuesFromV1(values map[string]json.RawMessage) (map[string]runtime.RuntimeValue, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make(map[string]runtime.RuntimeValue, len(values))
	for key, value := range values {
		runtimeValue, err := runtime.NewRuntimeValue(value)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", key, err)
		}
		result[key] = runtimeValue
	}
	changes, err := runtime.NewContextChanges(result, nil)
	if err != nil {
		return nil, err
	}
	return changes.SetValues(), nil
}
func contextChangesToV1(
	changes runtime.ContextChanges) (ContextChangesV1, error) {
	if !changes.IsValid() {
		return ContextChangesV1{}, fmt.Errorf("context changes must be valid")
	}
	set, err := runtimeValuesToV1(changes.SetValues())
	if err != nil {
		return ContextChangesV1{}, err
	}
	return ContextChangesV1{
		Set: set, Delete: changes.DeleteKeys()}, nil
}
func contextChangesFromV1(
	changes ContextChangesV1) (runtime.ContextChanges, error) {
	set, err := runtimeValuesFromV1(changes.Set)
	if err != nil {
		return runtime.ContextChanges{}, err
	}
	return runtime.NewContextChanges(set, changes.Delete)
}
func failureToV1(failure runtime.RuntimeFailure) RuntimeFailureV1 {
	return RuntimeFailureV1{
		Category: failure.Category().String(), Code: failure.Code(), Message: failure.Message(),
		Retryable: failure.Retryable(), Details: failure.Details()}
}
func failureFromV1(failure RuntimeFailureV1) (runtime.RuntimeFailure, error) {
	return runtime.NewRuntimeFailure(runtime.FailureCategory(failure.Category), failure.Code,
		failure.Message, failure.Retryable, failure.Details,
	)
}
func resultPartsFromV1(event NodeResultEventV1, maximumInlinePayloadBytes int,
) (runtime.NodeResult, runtime.RuntimeFailure, error) {
	status := messaging.NodeResultStatus(event.Status)
	outputs := make(map[string][]runtime.Payload, len(event.Outputs))
	for port, payloads := range event.Outputs {
		if len(payloads) == 0 {
			return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("output port %q must contain at least one payload", port)
		}
		mapped := make([]runtime.Payload, len(payloads))
		for index, payload := range payloads {
			value, err := payloadFromV1(payload, maximumInlinePayloadBytes)
			if err != nil {
				return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("output port %q payload %d: %w", port,
					index, err)
			}
			mapped[index] = value
		}
		outputs[port] = mapped
	}
	changes, err := contextChangesFromV1(event.ContextChanges)
	if err != nil {
		return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("context_changes: %w", err)
	}
	var failure runtime.RuntimeFailure
	if event.Failure != nil {
		failure, err = failureFromV1(*event.Failure)
		if err != nil {
			return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("failure: %w", err)
		}
	}
	switch status {
	case messaging.NodeResultStatusSucceeded:
		if event.Failure != nil {
			return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("SUCCEEDED result must not contain failure")
		}
		if len(outputs) > 0 && event.TerminalOutput != nil {
			return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("SUCCEEDED result cannot contain routed outputs and terminal output together")
		}
		if event.TerminalOutput != nil {
			terminal, err := payloadFromV1(*event.TerminalOutput, maximumInlinePayloadBytes)
			if err != nil {
				return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("terminal_output: %w", err)
			}
			result, err := runtime.NewTerminalNodeSuccessResult(terminal, changes)
			return result, runtime.RuntimeFailure{}, err
		}
		result, err := runtime.NewNodeSuccessResult(outputs, changes)
		return result, runtime.RuntimeFailure{}, err
	case messaging.NodeResultStatusFailed:
		if event.Failure == nil {
			return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("FAILED result must contain failure")
		}
		if len(outputs) > 0 || event.TerminalOutput != nil || !changes.IsEmpty() {
			return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("FAILED result must not contain outputs or context changes")
		}
		result, err := runtime.NewNodeFailureResult(failure)
		return result, runtime.RuntimeFailure{}, err
	case messaging.NodeResultStatusCancelled, messaging.NodeResultStatusTimedOut:
		if event.Failure == nil {
			return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("%s result must contain failure",
				status)
		}
		if len(outputs) > 0 || event.TerminalOutput != nil || !changes.IsEmpty() {
			return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("%s result must not contain outputs or context changes",
				status)
		}
		return runtime.NodeResult{}, failure, nil
	default:
		return runtime.NodeResult{}, runtime.RuntimeFailure{}, fmt.Errorf("unsupported result status %q", status)
	}
}
