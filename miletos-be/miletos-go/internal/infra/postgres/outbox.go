package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

var _ repository.OutboxStore = (*Store)(nil)
var _ repository.OutboxPublicationStore = (*Store)(nil)

const outboxColumns = `message_id, company_id, workflow_execution_id, node_execution_id, node_id, attempt, operation_kind, operation_key, operation_discriminator, message_type, message_version, destination, message_key, encoded_payload, publication_state, created_at, available_at, claimed_at, claim_owner, published_at, lock_version`
const qualifiedOutboxColumns = `outbox.message_id, outbox.company_id, outbox.workflow_execution_id, outbox.node_execution_id, outbox.node_id, outbox.attempt, outbox.operation_kind, outbox.operation_key, outbox.operation_discriminator, outbox.message_type, outbox.message_version, outbox.destination, outbox.message_key, outbox.encoded_payload, outbox.publication_state, outbox.created_at, outbox.available_at, outbox.claimed_at, outbox.claim_owner, outbox.published_at, outbox.lock_version`
const insertOutboxSQL = `INSERT INTO workflow_runtime.outbox_messages (` + outboxColumns + `) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NULL,NULL,NULL,$18)`

func (store *Store) CreateOutboxMessage(ctx context.Context, message repository.OutboxMessage) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	return createOutboxMessage(ctx, store.pool, message)
}
func createOutboxMessage(ctx context.Context, db asyncPersistenceDatabase, message repository.OutboxMessage) error {
	if ctx == nil {
		return fmt.Errorf("create outbox message context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil {
		return fmt.Errorf("create outbox message database must not be nil")
	}
	if !message.IsValid() || message.PublicationState() != repository.OutboxPublicationPending || message.LockVersion() != 0 {
		return fmt.Errorf("outbox message must be a valid initial pending message")
	}
	nodeExecutionID, nodeID, attempt := any(nil), any(nil), any(nil)
	if value, exists := message.NodeExecutionID(); exists {
		nodeExecutionID = value.String()
	}
	if value, exists := message.NodeID(); exists {
		nodeID = value.String()
	}
	if value, exists := message.Attempt(); exists {
		attempt = value
	}
	discriminator := any(nil)
	if value, exists := message.OperationDiscriminator(); exists {
		discriminator = value
	}
	_, err := db.Exec(ctx, insertOutboxSQL, message.MessageID().String(), message.CompanyID().String(), message.WorkflowExecutionID().String(), nodeExecutionID, nodeID, attempt,
		message.OperationKind().String(), message.OperationKey(), discriminator, message.MessageType(), message.MessageVersion(), message.Destination(), message.MessageKey(),
		message.EncodedPayload(), message.PublicationState(), message.CreatedAt(), message.AvailableAt(), message.LockVersion())
	if err != nil {
		return mapOutboxWriteError("create", err)
	}
	return nil
}

const claimOutboxSQL = `WITH candidates AS (
 SELECT message_id FROM workflow_runtime.outbox_messages WHERE publication_state='PENDING' AND available_at <= $2
 ORDER BY available_at, created_at, message_id FOR UPDATE SKIP LOCKED LIMIT $1
), claimed AS (
 UPDATE workflow_runtime.outbox_messages AS outbox SET publication_state='PUBLISHING', claimed_at=$2, claim_owner=$3, lock_version=outbox.lock_version+1
 FROM candidates WHERE outbox.message_id=candidates.message_id
 RETURNING ` +
	qualifiedOutboxColumns + `
) SELECT ` +
	outboxColumns + ` FROM claimed ORDER BY available_at, created_at, message_id`

func (store *Store) ClaimPublishableOutboxMessages(ctx context.Context, request repository.OutboxClaimRequest) ([]repository.OutboxMessage, error) {
	if !store.IsValid() {
		return nil, fmt.Errorf("PostgreSQL store must be valid")
	}
	return claimPublishableOutboxMessages(ctx, store.pool, request)
}
func claimPublishableOutboxMessages(ctx context.Context, db asyncPersistenceDatabase, request repository.OutboxClaimRequest) ([]repository.OutboxMessage, error) {
	if ctx == nil {
		return nil, fmt.Errorf("claim outbox context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if db == nil || !request.IsValid() {
		return nil, fmt.Errorf("claim outbox database and request must be valid")
	}
	rows, err := db.Query(ctx, claimOutboxSQL, request.BatchLimit(), request.ClaimedAt(), request.ClaimOwner())
	if err != nil {
		return nil, mapPostgreSQLError("claim", "outbox messages", err)
	}
	defer rows.Close()
	messages := make([]repository.OutboxMessage, 0)
	for rows.Next() {
		message, err := scanOutboxMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgreSQLError("claim", "outbox messages", err)
	}
	return messages, nil
}

const markOutboxPublishedSQL = `WITH target AS MATERIALIZED (
 SELECT message_id FROM workflow_runtime.outbox_messages WHERE message_id=$1
), updated AS (
 UPDATE workflow_runtime.outbox_messages SET publication_state='PUBLISHED', published_at=$4, claimed_at=NULL, claim_owner=NULL, lock_version=lock_version+1
 WHERE message_id=$1 AND publication_state='PUBLISHING' AND claim_owner=$2 AND lock_version=$3 RETURNING message_id
) SELECT EXISTS(SELECT 1 FROM target), EXISTS(SELECT 1 FROM updated)`

func (store *Store) MarkOutboxMessagePublished(ctx context.Context, command repository.MarkOutboxPublishedCommand) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	return markOutboxMessagePublished(ctx, store.pool, command)
}
func markOutboxMessagePublished(ctx context.Context, db asyncPersistenceDatabase, command repository.MarkOutboxPublishedCommand) error {
	if ctx == nil {
		return fmt.Errorf("mark outbox context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || !command.IsValid() {
		return fmt.Errorf("mark outbox database and command must be valid")
	}
	var found, updated bool
	err := db.QueryRow(ctx, markOutboxPublishedSQL, command.MessageID().String(), command.ClaimOwner(), command.ExpectedLockVersion(), command.PublishedAt()).Scan(&found, &updated)
	if err != nil {
		return mapPostgreSQLError("mark published", "outbox message", err)
	}
	if !found {
		return repository.NewNotFoundError("mark published", "outbox message", errors.New("outbox message does not exist"))
	}
	if !updated {
		return repository.NewStaleWriteError("mark published", "outbox message", errors.New("outbox claim owner, state, or version does not match"))
	}
	return nil
}

const releaseStaleOutboxSQL = `WITH candidates AS (
 SELECT message_id FROM workflow_runtime.outbox_messages WHERE publication_state='PUBLISHING' AND claimed_at < $1
 ORDER BY claimed_at, message_id FOR UPDATE SKIP LOCKED LIMIT $2
), released AS (
 UPDATE workflow_runtime.outbox_messages AS outbox SET publication_state='PENDING', claimed_at=NULL, claim_owner=NULL, lock_version=outbox.lock_version+1
 FROM candidates WHERE outbox.message_id=candidates.message_id RETURNING outbox.message_id
) SELECT COUNT(*) FROM released`

func (store *Store) ReleaseStaleOutboxClaims(ctx context.Context, request repository.ReleaseStaleOutboxClaimsRequest) (int64, error) {
	if !store.IsValid() {
		return 0, fmt.Errorf("PostgreSQL store must be valid")
	}
	return releaseStaleOutboxClaims(ctx, store.pool, request)
}
func releaseStaleOutboxClaims(ctx context.Context, db asyncPersistenceDatabase, request repository.ReleaseStaleOutboxClaimsRequest) (int64, error) {
	if ctx == nil {
		return 0, fmt.Errorf("release stale outbox context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if db == nil || !request.IsValid() {
		return 0, fmt.Errorf("release stale outbox database and request must be valid")
	}
	var count int64
	if err := db.QueryRow(ctx, releaseStaleOutboxSQL, request.StaleBefore(), request.BatchLimit()).Scan(&count); err != nil {
		return 0, mapPostgreSQLError("release stale", "outbox claims", err)
	}
	return count, nil
}

func scanOutboxMessage(row rowScanner) (repository.OutboxMessage, error) {
	var messageID, companyID, executionID, operationKind, operationKey, messageType, destination, messageKey, state string
	var nodeExecutionID, nodeID, discriminator, claimOwner *string
	var attempt *int16
	var version int
	var payload []byte
	var createdAt, availableAt time.Time
	var claimedAt, publishedAt *time.Time
	var lockVersion int64
	if err := row.Scan(&messageID, &companyID, &executionID, &nodeExecutionID, &nodeID, &attempt, &operationKind, &operationKey, &discriminator, &messageType, &version, &destination,
		&messageKey, &payload, &state, &createdAt, &availableAt, &claimedAt, &claimOwner, &publishedAt, &lockVersion); err != nil {
		return repository.OutboxMessage{}, mapPostgreSQLError("hydrate", "outbox message", err)
	}
	params := repository.OutboxMessageParams{MessageID: repository.MessageID(messageID), CompanyID: workflow.CompanyID(companyID),
		WorkflowExecutionID: execution.WorkflowExecutionID(executionID), OperationKind: repository.OutboxOperationKind(operationKind), MessageType: messageType, MessageVersion: version,
		Destination: destination, MessageKey: messageKey, EncodedPayload: payload, PublicationState: repository.OutboxPublicationState(state), CreatedAt: createdAt,
		AvailableAt: availableAt, LockVersion: lockVersion}
	if nodeExecutionID != nil {
		params.NodeExecutionID = execution.NodeExecutionID(*nodeExecutionID)
	}
	if nodeID != nil {
		params.NodeID = workflow.NodeID(*nodeID)
	}
	if attempt != nil {
		params.Attempt = *attempt
	}
	if discriminator != nil {
		params.OperationDiscriminator = *discriminator
	}
	if claimedAt != nil {
		params.ClaimedAt = *claimedAt
	}
	if claimOwner != nil {
		params.ClaimOwner = *claimOwner
	}
	if publishedAt != nil {
		params.PublishedAt = *publishedAt
	}
	message, err := repository.NewOutboxMessage(params)
	if err != nil || message.OperationKey() != operationKey {
		if err == nil {
			err = errors.New("persisted operation key does not match message identity")
		}
		return repository.OutboxMessage{}, &databaseError{operation: "hydrate", resource: "outbox message", cause: err}
	}
	return message, nil
}
func mapOutboxWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "outbox_messages_pk":
			return repository.NewConflictError(operation, "outbox message ID", err)
		case "outbox_messages_operation_uk":
			return repository.NewConflictError(operation, "outbox operation key", err)
		}
	}
	return mapPostgreSQLError(operation, "outbox message", err)
}
