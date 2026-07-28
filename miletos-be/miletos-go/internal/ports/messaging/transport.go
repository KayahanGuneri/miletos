package messaging

import (
	"context"
	"fmt"
)

// Delivery is a transport-neutral message delivery presented to engine consumers.
type Delivery struct {
	Topic     string
	Partition int32
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   map[string]string
}

// DeliveryHandler processes one transport-neutral delivery.
type DeliveryHandler func(context.Context, Delivery) error

// ConsumerRunner is the inbound messaging port consumed by engine/worker loops.
type ConsumerRunner interface {
	Run(context.Context, DeliveryHandler) error
	Close()
}

// MessageDescriptor identifies a stable wire message contract without exposing
// a concrete broker or protocol package to engine code.
type MessageDescriptor struct {
	Type    string
	Version int
}

func NewMessageDescriptor(
	messageType string,
	version int,
) (MessageDescriptor, error) {
	if messageType == "" || version <= 0 {
		return MessageDescriptor{}, fmt.Errorf("message descriptor must be valid")
	}
	return MessageDescriptor{Type: messageType, Version: version}, nil
}

// EncodedMessage carries protocol-owned bytes plus their stable contract identity.
type EncodedMessage struct {
	Descriptor MessageDescriptor
	Payload    []byte
}

// Codec is the engine-owned serialization port. Concrete Kafka protocol mapping
// remains in infrastructure/kafka/protocol.
type Codec interface {
	EncodeCommand(NodeCommand) (EncodedMessage, error)
	DecodeCommand([]byte) (NodeCommand, error)
	EncodeResult(NodeResultEvent) (EncodedMessage, error)
	DecodeResult([]byte) (NodeResultEvent, error)
	CommandDescriptor() MessageDescriptor
	ResultDescriptor() MessageDescriptor
}
