package repository

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"math"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"strconv"
	"strings"
	"time"
)

const (
	MaximumAsyncEncodedPayloadBytes    = 16 * 1024 * 1024
	MaximumOutboxPublicationBatchLimit = 1000
	maximumMessageIDCharacters         = 200
	maximumMessageTypeCharacters       = 128
	maximumDestinationCharacters       = 249
	maximumMessageKeyCharacters        = 512
	maximumClaimOwnerCharacters        = 200
)

type OutboxClaimRequest struct {
	claimOwner string
	claimedAt  time.Time
	batchLimit int
}

func NewOutboxClaimRequest(
	claimOwner string, claimedAt time.Time, batchLimit int,
) (OutboxClaimRequest, error) {
	normalizedOwner, err := normalizeRequiredBoundedString("claimOwner",
		claimOwner, maximumClaimOwnerCharacters)
	if err != nil {
		return OutboxClaimRequest{}, err
	}
	normalizedClaimedAt, err := normalizeRequiredRecordTime("claimedAt",
		claimedAt)
	if err != nil {
		return OutboxClaimRequest{}, err
	}
	if err := validateOutboxPublicationBatchLimit(batchLimit); err != nil {
		return OutboxClaimRequest{}, err
	}
	return OutboxClaimRequest{claimOwner: normalizedOwner,
		claimedAt: normalizedClaimedAt, batchLimit: batchLimit}, nil
}
func (request OutboxClaimRequest) ClaimOwner() string   { return request.claimOwner }
func (request OutboxClaimRequest) ClaimedAt() time.Time { return request.claimedAt }
func (request OutboxClaimRequest) BatchLimit() int      { return request.batchLimit }
func (request OutboxClaimRequest) IsValid() bool {
	_, err := NewOutboxClaimRequest(request.claimOwner, request.claimedAt,
		request.batchLimit)
	return err == nil
}

type MarkOutboxPublishedCommand struct {
	messageID           MessageID
	claimOwner          string
	expectedLockVersion int64
	publishedAt         time.Time
}

func NewMarkOutboxPublishedCommand(messageID MessageID, claimOwner string,
	expectedLockVersion int64, publishedAt time.Time) (MarkOutboxPublishedCommand, error) {
	normalizedMessageID, err := NewMessageID(messageID.String())
	if err != nil {
		return MarkOutboxPublishedCommand{}, err
	}
	normalizedOwner, err := normalizeRequiredBoundedString(
		"claimOwner", claimOwner, maximumClaimOwnerCharacters,
	)
	if err != nil {
		return MarkOutboxPublishedCommand{}, err
	}
	// Claiming increments the initial non-negative lock version, so a mark
	// command can only target a positive version returned by claim.
	if expectedLockVersion <= 0 {
		return MarkOutboxPublishedCommand{}, newValidationError(
			"expectedLockVersion", "must be greater than zero for a claimed message")
	}
	if expectedLockVersion == math.MaxInt64 {
		return MarkOutboxPublishedCommand{}, newValidationError(
			"expectedLockVersion", "must allow lock version increment")
	}
	normalizedPublishedAt, err := normalizeRequiredRecordTime(
		"publishedAt", publishedAt)
	if err != nil {
		return MarkOutboxPublishedCommand{}, err
	}
	return MarkOutboxPublishedCommand{messageID: normalizedMessageID,
		claimOwner: normalizedOwner, expectedLockVersion: expectedLockVersion, publishedAt: normalizedPublishedAt,
	}, nil
}
func (command MarkOutboxPublishedCommand) MessageID() MessageID { return command.messageID }
func (command MarkOutboxPublishedCommand) ClaimOwner() string   { return command.claimOwner }
func (command MarkOutboxPublishedCommand) ExpectedLockVersion() int64 {
	return command.expectedLockVersion
}
func (command MarkOutboxPublishedCommand) PublishedAt() time.Time {
	return command.publishedAt
}
func (command MarkOutboxPublishedCommand) IsValid() bool {
	_, err := NewMarkOutboxPublishedCommand(
		command.messageID, command.claimOwner, command.expectedLockVersion,
		command.publishedAt)
	return err == nil
}

type ReleaseStaleOutboxClaimsRequest struct {
	staleBefore time.Time
	batchLimit  int
}

func NewReleaseStaleOutboxClaimsRequest(staleBefore time.Time,
	batchLimit int) (ReleaseStaleOutboxClaimsRequest, error) {
	normalizedStaleBefore, err := normalizeRequiredRecordTime(
		"staleBefore", staleBefore)
	if err != nil {
		return ReleaseStaleOutboxClaimsRequest{}, err
	}
	if err := validateOutboxPublicationBatchLimit(batchLimit); err != nil {
		return ReleaseStaleOutboxClaimsRequest{}, err
	}
	return ReleaseStaleOutboxClaimsRequest{
		staleBefore: normalizedStaleBefore, batchLimit: batchLimit}, nil
}
func (request ReleaseStaleOutboxClaimsRequest) StaleBefore() time.Time {
	return request.staleBefore
}
func (request ReleaseStaleOutboxClaimsRequest) BatchLimit() int { return request.batchLimit }
func (request ReleaseStaleOutboxClaimsRequest) IsValid() bool {
	_, err := NewReleaseStaleOutboxClaimsRequest(request.staleBefore,
		request.batchLimit)
	return err == nil
}
func validateOutboxPublicationBatchLimit(batchLimit int) error {
	if batchLimit <= 0 {
		return newValidationError("batchLimit",
			"must be greater than zero")
	}
	if batchLimit > MaximumOutboxPublicationBatchLimit {
		return newValidationError("batchLimit",
			"exceeds the maximum outbox publication batch limit")
	}
	return nil
}

type MessageID string

func NewMessageID(value string) (MessageID, error) {
	normalized, err := normalizeRequiredBoundedString("messageID", value,
		maximumMessageIDCharacters)
	if err != nil {
		return "", err
	}
	return MessageID(normalized), nil
}
func (id MessageID) String() string { return string(id) }

type OutboxOperationKind string

const (
	OutboxOperationWorkflowEvent OutboxOperationKind = "WORKFLOW_EVENT"
	OutboxOperationNodeCommand   OutboxOperationKind = "NODE_COMMAND"
	OutboxOperationNodeResult    OutboxOperationKind = "NODE_RESULT"
)

func (kind OutboxOperationKind) IsValid() bool {
	switch kind {
	case OutboxOperationWorkflowEvent,
		OutboxOperationNodeCommand, OutboxOperationNodeResult:
		return true
	default:
		return false
	}
}

type OutboxPublicationState string

const (
	OutboxPublicationPending    OutboxPublicationState = "PENDING"
	OutboxPublicationPublishing OutboxPublicationState = "PUBLISHING"
	OutboxPublicationPublished  OutboxPublicationState = "PUBLISHED"
)

func (state OutboxPublicationState) IsValid() bool {
	switch state {
	case OutboxPublicationPending, OutboxPublicationPublishing, OutboxPublicationPublished:
		return true
	default:
		return false
	}
}

type OutboxMessageParams struct {
	MessageID              MessageID
	CompanyID              workflow.CompanyID
	WorkflowExecutionID    execution.WorkflowExecutionID
	NodeExecutionID        execution.NodeExecutionID
	NodeID                 workflow.NodeID
	Attempt                int16
	OperationKind          OutboxOperationKind
	OperationDiscriminator string
	MessageType            string
	MessageVersion         int
	Destination            string
	MessageKey             string
	EncodedPayload         []byte
	PublicationState       OutboxPublicationState
	CreatedAt              time.Time
	AvailableAt            time.Time
	ClaimedAt              time.Time
	ClaimOwner             string
	PublishedAt            time.Time
	LockVersion            int64
}
type OutboxMessage struct {
	params OutboxMessageParams

	hasNode bool

	operationKey string
}

func NewOutboxMessage(params OutboxMessageParams) (OutboxMessage, error) {
	messageID, err := NewMessageID(params.MessageID.String())
	if err != nil {
		return OutboxMessage{}, err
	}
	companyID, workflowExecutionID, err := normalizeAsyncExecutionScope(
		params.CompanyID, params.WorkflowExecutionID)
	if err != nil {
		return OutboxMessage{}, err
	}
	if !params.OperationKind.IsValid() {
		return OutboxMessage{}, newValidationError(
			"operationKind", "must contain a supported outbox operation kind")
	}
	nodeExecutionID, nodeID, attempt, hasNode, discriminator, err :=
		normalizeOutboxOperationIdentity(params)
	if err != nil {
		return OutboxMessage{}, err
	}
	messageType, err := normalizeRequiredBoundedString(
		"messageType", params.MessageType, maximumMessageTypeCharacters,
	)
	if err != nil {
		return OutboxMessage{}, err
	}
	if params.MessageVersion <= 0 {
		return OutboxMessage{}, newValidationError("messageVersion", "must be greater than zero")
	}
	destination, err := normalizeRequiredBoundedString("destination", params.Destination,
		maximumDestinationCharacters)
	if err != nil {
		return OutboxMessage{}, err
	}
	messageKey, err := normalizeRequiredBoundedString("messageKey", params.MessageKey,
		maximumMessageKeyCharacters)
	if err != nil {
		return OutboxMessage{}, err
	}
	if len(params.EncodedPayload) == 0 {
		return OutboxMessage{}, newValidationError("encodedPayload",
			"must not be empty")
	}
	if len(params.EncodedPayload) > MaximumAsyncEncodedPayloadBytes {
		return OutboxMessage{}, newValidationError(
			"encodedPayload", "exceeds the maximum async encoded payload size")
	}
	createdAt, err := normalizeRequiredRecordTime("createdAt", params.CreatedAt)
	if err != nil {
		return OutboxMessage{}, err
	}
	availableAt, err := normalizeRequiredRecordTime(
		"availableAt", params.AvailableAt,
	)
	if err != nil {
		return OutboxMessage{}, err
	}
	if availableAt.Before(createdAt) {
		return OutboxMessage{}, newValidationError(
			"availableAt", "must not be before createdAt")
	}
	claimedAt, claimOwner, publishedAt, err := normalizeOutboxPublication(params.PublicationState,
		createdAt, params.ClaimedAt, params.ClaimOwner,
		params.PublishedAt)
	if err != nil {
		return OutboxMessage{}, err
	}
	if params.LockVersion < 0 {
		return OutboxMessage{}, newValidationError("lockVersion",
			"must not be negative")
	}
	operationKey := deterministicAsyncIdentity(
		"outbox",
		params.OperationKind.String(),
		companyID.String(),
		workflowExecutionID.String(),
		nodeExecutionID.String(),
		nodeID.String(),
		strconv.Itoa(int(attempt)),
		discriminator,
	)
	return OutboxMessage{params: OutboxMessageParams{MessageID: messageID, CompanyID: companyID, WorkflowExecutionID: workflowExecutionID, NodeExecutionID: nodeExecutionID, NodeID: nodeID, Attempt: attempt, OperationKind: params.OperationKind, OperationDiscriminator: discriminator, MessageType: messageType, MessageVersion: params.MessageVersion, Destination: destination, MessageKey: messageKey, EncodedPayload: bytes.Clone(params.EncodedPayload), PublicationState: params.PublicationState, CreatedAt: createdAt, AvailableAt: availableAt, ClaimedAt: claimedAt, ClaimOwner: claimOwner, PublishedAt: publishedAt, LockVersion: params.LockVersion}, hasNode: hasNode, operationKey: operationKey}, nil
}
func normalizeOutboxOperationIdentity(
	params OutboxMessageParams) (execution.NodeExecutionID,
	workflow.NodeID, int16, bool,
	string, error) {
	if params.OperationKind == OutboxOperationWorkflowEvent {
		if params.NodeExecutionID.String() != "" || params.NodeID.String() != "" ||
			params.Attempt != 0 {
			return "", "", 0, false, "", newValidationError("operationKind",
				"workflow event must not contain node identity")
		}
		discriminator, err := normalizeRequiredBoundedString("operationDiscriminator",
			params.OperationDiscriminator, maximumMessageKeyCharacters)
		return "", "", 0, false, discriminator, err
	}
	if strings.TrimSpace(params.OperationDiscriminator) != "" {
		return "", "", 0, false, "", newValidationError("operationDiscriminator",
			"must be empty for node operations")
	}
	nodeExecutionID, err := execution.NewNodeExecutionID(params.NodeExecutionID.String())
	if err != nil {
		return "", "", 0, false, "", newValidationError(
			"nodeExecutionID", err.Error())
	}
	nodeID, err := workflow.NewNodeID(params.NodeID.String())
	if err != nil {
		return "", "", 0, false, "", newValidationError("nodeID",
			err.Error())
	}
	if params.Attempt <= 0 {
		return "", "", 0, false, "", newValidationError(
			"attempt", "must be greater than zero")
	}
	return nodeExecutionID, nodeID, params.Attempt, true, "", nil
}
func normalizeOutboxPublication(
	state OutboxPublicationState, createdAt time.Time, claimedAt time.Time,
	claimOwner string, publishedAt time.Time) (time.Time, string, time.Time, error) {
	if !state.IsValid() {
		return time.Time{}, "", time.Time{}, newValidationError("publicationState",
			"must contain a supported publication state")
	}
	switch state {
	case OutboxPublicationPending:
		if !claimedAt.IsZero() || strings.TrimSpace(claimOwner) != "" || !publishedAt.IsZero() {
			return time.Time{}, "", time.Time{}, newValidationError("publicationState",
				"pending message must not contain claim or publication data")
		}
		return time.Time{}, "", time.Time{}, nil
	case OutboxPublicationPublishing:
		normalizedClaimedAt, err := normalizeOptionalRecordTime("claimedAt",
			claimedAt, createdAt)
		if err != nil || normalizedClaimedAt.IsZero() {
			if err != nil {
				return time.Time{}, "", time.Time{}, err
			}
			return time.Time{}, "", time.Time{}, newValidationError("claimedAt",
				"must be set for a publishing message")
		}
		normalizedOwner, err := normalizeRequiredBoundedString("claimOwner",
			claimOwner, maximumClaimOwnerCharacters)
		if err != nil {
			return time.Time{}, "", time.Time{}, err
		}
		if !publishedAt.IsZero() {
			return time.Time{}, "", time.Time{}, newValidationError(
				"publishedAt", "must be empty for a publishing message")
		}
		return normalizedClaimedAt, normalizedOwner, time.Time{}, nil
	case OutboxPublicationPublished:
		if !claimedAt.IsZero() || strings.TrimSpace(claimOwner) != "" {
			return time.Time{}, "", time.Time{}, newValidationError("publicationState", "published message must not retain claim data")
		}
		normalizedPublishedAt, err := normalizeOptionalRecordTime("publishedAt", publishedAt,
			createdAt)
		if err != nil || normalizedPublishedAt.IsZero() {
			if err != nil {
				return time.Time{}, "", time.Time{}, err
			}
			return time.Time{}, "", time.Time{}, newValidationError("publishedAt", "must be set for a published message")
		}
		return time.Time{}, "", normalizedPublishedAt, nil
	}
	return time.Time{}, "", time.Time{}, newValidationError("publicationState", "must contain a supported publication state")
}
func (kind OutboxOperationKind) String() string { return string(kind) }
func deterministicAsyncIdentity(prefix string, parts ...string) string {
	digest := sha256.New()
	writeIdentityPart(digest, prefix)
	for _, part := range parts {
		writeIdentityPart(digest, part)
	}
	return prefix + ":" + hex.EncodeToString(digest.Sum(nil))
}
func writeIdentityPart(target hash.Hash, value string) {
	_, _ = fmt.Fprintf(target, "%d:", len(value))
	_, _ = target.Write([]byte(value))
}
func (message OutboxMessage) MessageID() MessageID          { return message.params.MessageID }
func (message OutboxMessage) CompanyID() workflow.CompanyID { return message.params.CompanyID }
func (message OutboxMessage) WorkflowExecutionID() execution.WorkflowExecutionID {
	return message.params.WorkflowExecutionID
}
func (message OutboxMessage) NodeExecutionID() (execution.NodeExecutionID, bool) {
	return message.params.NodeExecutionID, message.hasNode
}
func (message OutboxMessage) NodeID() (workflow.NodeID, bool) {
	return message.params.NodeID, message.hasNode
}
func (message OutboxMessage) Attempt() (int16, bool)             { return message.params.Attempt, message.hasNode }
func (message OutboxMessage) OperationKind() OutboxOperationKind { return message.params.OperationKind }
func (message OutboxMessage) OperationDiscriminator() (string, bool) {
	if message.params.OperationKind != OutboxOperationWorkflowEvent || message.params.OperationDiscriminator == "" {
		return "", false
	}
	return message.params.OperationDiscriminator, true
}
func (message OutboxMessage) OperationKey() string { return message.operationKey }
func (message OutboxMessage) MessageType() string  { return message.params.MessageType }
func (message OutboxMessage) MessageVersion() int  { return message.params.MessageVersion }
func (message OutboxMessage) Destination() string  { return message.params.Destination }
func (message OutboxMessage) MessageKey() string   { return message.params.MessageKey }
func (message OutboxMessage) EncodedPayload() []byte {
	return bytes.Clone(message.params.EncodedPayload)
}
func (message OutboxMessage) PublicationState() OutboxPublicationState {
	return message.params.PublicationState
}
func (message OutboxMessage) CreatedAt() time.Time { return message.params.CreatedAt }
func (message OutboxMessage) AvailableAt() time.Time {
	return message.params.AvailableAt
}
func (message OutboxMessage) ClaimedAt() (time.Time, bool) {
	return message.params.ClaimedAt, !message.params.ClaimedAt.IsZero()
}
func (message OutboxMessage) ClaimOwner() (string, bool) {
	return message.params.ClaimOwner, message.params.ClaimOwner != ""
}
func (message OutboxMessage) PublishedAt() (time.Time, bool) {
	return message.params.PublishedAt, !message.params.PublishedAt.IsZero()
}
func (message OutboxMessage) LockVersion() int64 { return message.params.LockVersion }
func (message OutboxMessage) IsValid() bool {
	_, err := NewOutboxMessage(message.params)
	return err == nil

}
