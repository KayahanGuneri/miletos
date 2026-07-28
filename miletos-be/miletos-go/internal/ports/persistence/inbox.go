package repository

import (
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strconv"
	"strings"
	"time"
)

const maximumConsumerIdentityCharacters = 200

type ConsumerIdentity string

func NewConsumerIdentity(value string) (ConsumerIdentity, error) {
	normalized, err := normalizeRequiredBoundedString(
		"consumerIdentity", value, maximumConsumerIdentityCharacters,
	)
	if err != nil {
		return "", err
	}
	return ConsumerIdentity(normalized), nil
}

func (identity ConsumerIdentity) String() string {
	return string(identity)
}

type InboxProcessingResult string

const (
	InboxProcessingApplied InboxProcessingResult = "APPLIED"
	InboxProcessingIgnored InboxProcessingResult = "IGNORED"
)

func (result InboxProcessingResult) IsValid() bool {
	switch result {
	case InboxProcessingApplied, InboxProcessingIgnored:
		return true
	default:
		return false
	}
}

type InboxSourcePosition struct {
	source    string
	partition int32
	offset    int64
	valid     bool
}

func NewInboxSourcePosition(
	source string, partition int32, offset int64,
) (InboxSourcePosition, error) {
	normalizedSource, err := normalizeRequiredBoundedString("source",
		source, maximumDestinationCharacters)
	if err != nil {
		return InboxSourcePosition{}, err
	}
	if partition < 0 {
		return InboxSourcePosition{}, newValidationError(
			"partition", "must not be negative")
	}
	if offset < 0 {
		return InboxSourcePosition{}, newValidationError("offset", "must not be negative")
	}
	return InboxSourcePosition{source: normalizedSource, partition: partition,
		offset: offset, valid: true}, nil
}

func (position InboxSourcePosition) Source() string { return position.source }

func (position InboxSourcePosition) Partition() int32 { return position.partition }

func (position InboxSourcePosition) Offset() int64 { return position.offset }

func (position InboxSourcePosition) IsValid() bool {
	if !position.valid {
		return false
	}
	_, err := NewInboxSourcePosition(position.source, position.partition, position.offset)
	return err == nil
}

type InboxMessageParams struct {
	ConsumerIdentity    ConsumerIdentity
	MessageID           MessageID
	CompanyID           workflow.CompanyID
	WorkflowExecutionID execution.WorkflowExecutionID
	NodeExecutionID     execution.NodeExecutionID
	MessageType         string
	MessageVersion      int
	ProcessingResult    InboxProcessingResult
	SourcePosition      *InboxSourcePosition
	ReceivedAt          time.Time
	ProcessedAt         time.Time
}

type InboxMessage struct {
	params InboxMessageParams

	hasNode bool
}

func NewInboxMessage(params InboxMessageParams) (InboxMessage, error) {
	consumerIdentity, err := NewConsumerIdentity(params.ConsumerIdentity.String())
	if err != nil {
		return InboxMessage{}, err
	}
	messageID, err := NewMessageID(params.MessageID.String())
	if err != nil {
		return InboxMessage{}, err
	}
	companyID, workflowExecutionID, err := normalizeAsyncExecutionScope(params.CompanyID,
		params.WorkflowExecutionID)
	if err != nil {
		return InboxMessage{}, err
	}
	nodeExecutionID, hasNode, err := normalizeOptionalNodeExecutionID(params.NodeExecutionID)
	if err != nil {
		return InboxMessage{}, err
	}
	messageType, err := normalizeRequiredBoundedString("messageType",
		params.MessageType, maximumMessageTypeCharacters)
	if err != nil {
		return InboxMessage{}, err
	}
	if params.MessageVersion <= 0 {
		return InboxMessage{}, newValidationError(
			"messageVersion", "must be greater than zero")
	}
	if !params.ProcessingResult.IsValid() {
		return InboxMessage{}, newValidationError("processingResult", "must contain a supported inbox processing result")
	}
	receivedAt, err := normalizeRequiredRecordTime("receivedAt", params.ReceivedAt)
	if err != nil {
		return InboxMessage{}, err
	}
	processedAt, err := normalizeRequiredRecordTime("processedAt", params.ProcessedAt)
	if err != nil {
		return InboxMessage{}, err
	}
	if processedAt.Before(receivedAt) {
		return InboxMessage{}, newValidationError("processedAt",
			"must not be before receivedAt")
	}
	var sourcePosition InboxSourcePosition
	hasSourcePosition := params.SourcePosition != nil
	if hasSourcePosition {
		if !params.SourcePosition.IsValid() {
			return InboxMessage{}, newValidationError(
				"sourcePosition", "must be valid when provided")
		}
		sourcePosition = *params.SourcePosition
	}
	return InboxMessage{params: InboxMessageParams{ConsumerIdentity: consumerIdentity, MessageID: messageID, CompanyID: companyID, WorkflowExecutionID: workflowExecutionID, NodeExecutionID: nodeExecutionID, MessageType: messageType, MessageVersion: params.MessageVersion, ProcessingResult: params.ProcessingResult, SourcePosition: optionalInboxSourcePosition(sourcePosition,
		hasSourcePosition), ReceivedAt: receivedAt, ProcessedAt: processedAt}, hasNode: hasNode,
	}, nil
}

func NewInboxIdentityKey(consumerIdentity ConsumerIdentity, messageID MessageID,
) (string, error) {
	consumer, err := NewConsumerIdentity(consumerIdentity.String())
	if err != nil {
		return "", err
	}
	message, err := NewMessageID(messageID.String())
	if err != nil {
		return "", err
	}
	return deterministicAsyncIdentity("inbox",
		consumer.String(), message.String()), nil
}

func (message InboxMessage) ConsumerIdentity() ConsumerIdentity {
	return message.params.ConsumerIdentity
}

func (message InboxMessage) MessageID() MessageID { return message.params.MessageID }

func (message InboxMessage) CompanyID() workflow.CompanyID { return message.params.CompanyID }

func (message InboxMessage) WorkflowExecutionID() execution.WorkflowExecutionID {
	return message.params.WorkflowExecutionID
}

func (message InboxMessage) NodeExecutionID() (execution.NodeExecutionID, bool) {
	return message.params.NodeExecutionID, message.hasNode
}

func (message InboxMessage) MessageType() string { return message.params.MessageType }

func (message InboxMessage) MessageVersion() int { return message.params.MessageVersion }

func (message InboxMessage) ProcessingResult() InboxProcessingResult {
	return message.params.ProcessingResult
}

func (message InboxMessage) SourcePosition() (InboxSourcePosition, bool) {
	if message.params.SourcePosition == nil {
		return InboxSourcePosition{}, false
	}
	return *message.params.SourcePosition, true

}

func (message InboxMessage) ReceivedAt() time.Time { return message.params.ReceivedAt }

func (message InboxMessage) ProcessedAt() time.Time { return message.params.ProcessedAt }

func (message InboxMessage) IdentityKey() string {
	key, _ := NewInboxIdentityKey(message.params.ConsumerIdentity, message.params.MessageID)
	return key
}

func (message InboxMessage) IsValid() bool {
	_, err := NewInboxMessage(message.params)
	return err == nil

}

func (result InboxProcessingResult) String() string { return string(result) }

func (position InboxSourcePosition) DiagnosticKey() string {
	if !position.IsValid() {
		return ""
	}
	return strings.Join([]string{position.source,
		strconv.FormatInt(int64(position.partition), 10), strconv.FormatInt(position.offset, 10)},
		":")
}

func optionalInboxSourcePosition(value InboxSourcePosition, exists bool) *InboxSourcePosition {
	if !exists {
		return nil
	}
	copy := value
	return &copy
}
