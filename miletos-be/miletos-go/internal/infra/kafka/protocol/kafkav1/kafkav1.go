package kafkav1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/ports/messaging"
	"time"
)

type Codec struct {
	maximumMessageBytes       int
	maximumInlinePayloadBytes int
}

func NewCodec(
	maximumMessageBytes int, maximumInlinePayloadBytes int) (Codec, error) {
	if maximumMessageBytes <= 0 {
		return Codec{}, fmt.Errorf("maximum message bytes must be greater than zero")
	}
	if maximumInlinePayloadBytes <= 0 {
		return Codec{}, fmt.Errorf("maximum inline payload bytes must be greater than zero")
	}
	if maximumInlinePayloadBytes > maximumMessageBytes {
		return Codec{}, fmt.Errorf("maximum inline payload bytes must not exceed maximum message bytes")
	}
	return Codec{maximumMessageBytes: maximumMessageBytes,
		maximumInlinePayloadBytes: maximumInlinePayloadBytes}, nil
}
func (codec Codec) EncodeNodeCommand(command NodeCommandV1,
) ([]byte, error) {
	if err := codec.validate(); err != nil {
		return nil, err
	}
	if err := command.Validate(codec.maximumInlinePayloadBytes); err != nil {
		return nil, fmt.Errorf("validate node command v1: %w", err)
	}
	return codec.encode(command)
}
func (codec Codec) DecodeNodeCommand(data []byte,
) (NodeCommandV1, error) {
	var command NodeCommandV1
	if err := codec.decode(data, &command); err != nil {
		return NodeCommandV1{}, fmt.Errorf("decode node command v1: %w", err)
	}
	if err := command.Validate(codec.maximumInlinePayloadBytes); err != nil {
		return NodeCommandV1{}, fmt.Errorf("validate node command v1: %w", err)
	}
	return command, nil
}
func (codec Codec) EncodeNodeResultEvent(
	event NodeResultEventV1) ([]byte, error) {
	if err := codec.validate(); err != nil {
		return nil, err
	}
	if err := event.Validate(codec.maximumInlinePayloadBytes); err != nil {
		return nil, fmt.Errorf("validate node result event v1: %w", err)
	}
	return codec.encode(event)
}
func (codec Codec) DecodeNodeResultEvent(
	data []byte) (NodeResultEventV1, error) {
	var event NodeResultEventV1
	if err := codec.decode(data, &event); err != nil {
		return NodeResultEventV1{}, fmt.Errorf("decode node result event v1: %w", err)
	}
	if err := event.Validate(codec.maximumInlinePayloadBytes); err != nil {
		return NodeResultEventV1{}, fmt.Errorf("validate node result event v1: %w", err)
	}
	return event, nil
}
func (codec Codec) validate() error {
	if codec.maximumMessageBytes <= 0 || codec.maximumInlinePayloadBytes <= 0 {
		return fmt.Errorf("codec must be initialized")
	}
	if codec.maximumInlinePayloadBytes > codec.maximumMessageBytes {
		return fmt.Errorf("codec inline payload limit exceeds message limit")
	}
	return nil
}
func (codec Codec) encode(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON: %w", err)
	}
	if len(encoded) > codec.maximumMessageBytes {
		return nil, fmt.Errorf("encoded message size %d exceeds limit %d",
			len(encoded), codec.maximumMessageBytes)
	}
	return encoded, nil
}
func (codec Codec) decode(data []byte, target any) error {
	if err := codec.validate(); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("message must not be empty")
	}
	if len(data) > codec.maximumMessageBytes {
		return fmt.Errorf("message size %d exceeds limit %d", len(data),
			codec.maximumMessageBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("strict JSON decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("message must contain exactly one JSON value")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

const (
	MessageVersionV1             uint16 = 1
	MessageTypeNodeCommandV1            = "miletos.workflow.node.command"
	MessageTypeNodeResultEventV1        = "miletos.workflow.node.result"
)

type MessageEnvelopeV1 struct {
	MessageID           string    `json:"message_id"`
	MessageType         string    `json:"message_type"`
	MessageVersion      uint16    `json:"message_version"`
	CreatedAt           time.Time `json:"created_at"`
	CompanyID           string    `json:"company_id"`
	WorkflowID          string    `json:"workflow_id"`
	WorkflowExecutionID string    `json:"workflow_execution_id"`
	NodeID              string    `json:"node_id"`
	NodeExecutionID     string    `json:"node_execution_id"`
	Attempt             int16     `json:"attempt"`
	CorrelationID       string    `json:"correlation_id"`
	CausationID         string    `json:"causation_id"`
}
type ArtifactReferenceV1 struct {
	ID        string            `json:"id"`
	Location  string            `json:"location"`
	SizeBytes int64             `json:"size_bytes"`
	Checksum  string            `json:"checksum,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}
type PayloadV1 struct {
	ContentType string               `json:"content_type"`
	InlineData  []byte               `json:"inline_data"`
	Artifact    *ArtifactReferenceV1 `json:"artifact,omitempty"`
	Metadata    map[string]string    `json:"metadata,omitempty"`
}
type NodeInputV1 struct {
	Ports map[string][]PayloadV1 `json:"ports"`
}
type ContextChangesV1 struct {
	Set    map[string]json.RawMessage `json:"set"`
	Delete []string                   `json:"delete"`
}
type RuntimeFailureV1 struct {
	Category  string            `json:"category"`
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Retryable bool              `json:"retryable"`
	Details   map[string]string `json:"details,omitempty"`
}
type NodeCommandV1 struct {
	Envelope      MessageEnvelopeV1          `json:"envelope"`
	ExecutionMode string                     `json:"execution_mode"`
	PluginType    string                     `json:"plugin_type"`
	PluginVersion string                     `json:"plugin_version"`
	Configuration json.RawMessage            `json:"configuration"`
	Input         NodeInputV1                `json:"input"`
	Variables     map[string]json.RawMessage `json:"variables"`
	DeadlineAt    *time.Time                 `json:"deadline_at,omitempty"`
}
type NodeResultEventV1 struct {
	Envelope       MessageEnvelopeV1      `json:"envelope"`
	Status         string                 `json:"status"`
	Outputs        map[string][]PayloadV1 `json:"outputs"`
	TerminalOutput *PayloadV1             `json:"terminal_output,omitempty"`
	ContextChanges ContextChangesV1       `json:"context_changes"`
	Failure        *RuntimeFailureV1      `json:"failure,omitempty"`
	StartedAt      time.Time              `json:"started_at"`
	FinishedAt     time.Time              `json:"finished_at"`
}

func (command NodeCommandV1) Validate(
	maximumInlinePayloadBytes int) error {
	if _, err := metadataFromEnvelope(
		command.Envelope, MessageTypeNodeCommandV1); err != nil {
		return err
	}
	mode, err := execution.ParseExecutionMode(command.ExecutionMode)
	if err != nil {
		return fmt.Errorf("execution_mode: %w", err)
	}
	if mode != execution.ExecutionModeAsync {
		return fmt.Errorf("execution_mode must be ASYNC")
	}
	nodeID, err := workflow.NewNodeID(command.Envelope.NodeID)
	if err != nil {
		return fmt.Errorf("node_id: %w", err)
	}
	pluginType, err := workflow.NewPluginType(command.PluginType)
	if err != nil {
		return fmt.Errorf("plugin_type: %w", err)
	}
	pluginVersion, err := workflow.NewPluginVersion(command.PluginVersion)
	if err != nil {
		return fmt.Errorf("plugin_version: %w", err)
	}
	if _, err := workflow.NewNodeDefinition(
		nodeID, pluginType, pluginVersion,
		command.Configuration, nil); err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	if _, err := nodeInputFromV1(command.Input, maximumInlinePayloadBytes); err != nil {
		return fmt.Errorf("input: %w", err)
	}
	if _, err := runtimeValuesFromV1(command.Variables); err != nil {
		return fmt.Errorf("variables: %w", err)
	}
	if command.DeadlineAt != nil {
		deadline := command.DeadlineAt.UTC()
		if command.DeadlineAt.IsZero() {
			return fmt.Errorf("deadline_at must not be zero")
		}
		if !deadline.After(command.Envelope.CreatedAt.UTC()) {
			return fmt.Errorf("deadline_at must be after envelope created_at")
		}
	}
	return nil
}
func (event NodeResultEventV1) Validate(maximumInlinePayloadBytes int) error {
	if _, err := metadataFromEnvelope(event.Envelope, MessageTypeNodeResultEventV1); err != nil {
		return err
	}
	status := messaging.NodeResultStatus(event.Status)
	if !status.IsValid() {
		return fmt.Errorf("status must be SUCCEEDED, FAILED, CANCELLED, or TIMED_OUT")
	}
	startedAt := event.StartedAt.UTC()
	finishedAt := event.FinishedAt.UTC()
	if event.StartedAt.IsZero() || event.FinishedAt.IsZero() {
		return fmt.Errorf("started_at and finished_at must not be zero")
	}
	if finishedAt.Before(startedAt) {
		return fmt.Errorf("finished_at must not be before started_at")
	}
	result, interruptedFailure, err := resultPartsFromV1(event, maximumInlinePayloadBytes)
	if err != nil {
		return err
	}
	metadata, err := metadataFromEnvelope(
		event.Envelope, MessageTypeNodeResultEventV1)
	if err != nil {
		return err
	}
	switch status {
	case messaging.NodeResultStatusSucceeded,
		messaging.NodeResultStatusFailed:
		_, err = messaging.NewNodeResultEvent(metadata,
			result, startedAt, finishedAt,
		)
	case messaging.NodeResultStatusCancelled, messaging.NodeResultStatusTimedOut:
		_, err = messaging.NewInterruptedNodeResultEvent(metadata, status,
			interruptedFailure, startedAt, finishedAt,
		)
	}
	return err
}
func metadataFromEnvelope(envelope MessageEnvelopeV1, expectedType string,
) (messaging.MessageMetadata, error) {
	if envelope.MessageType != expectedType {
		return messaging.MessageMetadata{}, fmt.Errorf(
			"message_type must be %q", expectedType)
	}
	if envelope.MessageVersion != MessageVersionV1 {
		return messaging.MessageMetadata{}, fmt.Errorf(
			"message_version must be %d", MessageVersionV1)
	}
	companyID, err := workflow.NewCompanyID(envelope.CompanyID)
	if err != nil {
		return messaging.MessageMetadata{}, fmt.Errorf("company_id: %w", err)
	}
	workflowID, err := workflow.NewWorkflowID(envelope.WorkflowID)
	if err != nil {
		return messaging.MessageMetadata{}, fmt.Errorf("workflow_id: %w", err)
	}
	workflowExecutionID, err := execution.NewWorkflowExecutionID(envelope.WorkflowExecutionID)
	if err != nil {
		return messaging.MessageMetadata{}, fmt.Errorf("workflow_execution_id: %w", err)
	}
	nodeID, err := workflow.NewNodeID(envelope.NodeID)
	if err != nil {
		return messaging.MessageMetadata{}, fmt.Errorf("node_id: %w", err)
	}
	nodeExecutionID, err := execution.NewNodeExecutionID(envelope.NodeExecutionID)
	if err != nil {
		return messaging.MessageMetadata{}, fmt.Errorf("node_execution_id: %w", err)
	}
	metadata, err := messaging.NewMessageMetadata(messaging.MessageMetadataParams{
		MessageID: envelope.MessageID, CreatedAt: envelope.CreatedAt,
		CompanyID: companyID, WorkflowID: workflowID, WorkflowExecutionID: workflowExecutionID,
		NodeID: nodeID, NodeExecutionID: nodeExecutionID,
		Attempt: envelope.Attempt, CorrelationID: envelope.CorrelationID,
		CausationID: envelope.CausationID})
	if err != nil {
		return messaging.MessageMetadata{}, fmt.Errorf("envelope: %w", err)
	}
	return metadata, nil
}

var _ messaging.Codec = Codec{}

func (codec Codec) EncodeCommand(command messaging.NodeCommand) (messaging.EncodedMessage, error) {
	transportCommand, err := ToNodeCommandV1(command, codec.maximumInlinePayloadBytes)
	if err != nil {
		return messaging.EncodedMessage{}, err
	}
	payload, err := codec.EncodeNodeCommand(transportCommand)
	if err != nil {
		return messaging.EncodedMessage{}, err
	}
	return messaging.EncodedMessage{Descriptor: codec.CommandDescriptor(), Payload: payload}, nil
}
func (codec Codec) DecodeCommand(payload []byte) (messaging.NodeCommand, error) {
	transportCommand, err := codec.DecodeNodeCommand(payload)
	if err != nil {
		return messaging.NodeCommand{}, err
	}
	return FromNodeCommandV1(transportCommand, codec.maximumInlinePayloadBytes)
}
func (codec Codec) EncodeResult(event messaging.NodeResultEvent) (messaging.EncodedMessage, error) {
	transportEvent, err := ToNodeResultEventV1(event, codec.maximumInlinePayloadBytes)
	if err != nil {
		return messaging.EncodedMessage{}, err
	}
	payload, err := codec.EncodeNodeResultEvent(transportEvent)
	if err != nil {
		return messaging.EncodedMessage{}, err
	}
	return messaging.EncodedMessage{
		Descriptor: codec.ResultDescriptor(), Payload: payload}, nil
}
func (codec Codec) DecodeResult(payload []byte) (messaging.NodeResultEvent, error) {
	transportEvent, err := codec.DecodeNodeResultEvent(payload)
	if err != nil {
		return messaging.NodeResultEvent{}, err
	}
	return FromNodeResultEventV1(transportEvent, codec.maximumInlinePayloadBytes)
}
func (Codec) CommandDescriptor() messaging.MessageDescriptor {
	return messaging.MessageDescriptor{
		Type: MessageTypeNodeCommandV1, Version: int(MessageVersionV1)}
}
func (Codec) ResultDescriptor() messaging.MessageDescriptor {
	return messaging.MessageDescriptor{Type: MessageTypeNodeResultEventV1, Version: int(MessageVersionV1)}
}
