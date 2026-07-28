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

var _ repository.InboxStore = (*Store)(nil)

const insertInboxSQL = `INSERT INTO workflow_runtime.inbox_messages (
 consumer_name,message_id,company_id,workflow_execution_id,node_execution_id,message_type,message_version,processing_result,
 source_name,source_partition,source_offset,received_at,processed_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

func (store *Store) HasProcessedInboxMessage(ctx context.Context, consumer repository.ConsumerIdentity, messageID repository.MessageID) (bool, error) {
	if !store.IsValid() {
		return false, fmt.Errorf("PostgreSQL store must be valid")
	}
	return hasProcessedInboxMessage(ctx, store.pool, consumer, messageID)
}
func hasProcessedInboxMessage(ctx context.Context, db asyncPersistenceDatabase, consumer repository.ConsumerIdentity, messageID repository.MessageID) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("has processed inbox context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if db == nil {
		return false, fmt.Errorf("inbox database must not be nil")
	}
	validatedConsumer, err := repository.NewConsumerIdentity(consumer.String())
	if err != nil {
		return false, fmt.Errorf("consumer identity must be valid")
	}
	validatedMessageID, err := repository.NewMessageID(messageID.String())
	if err != nil {
		return false, fmt.Errorf("message ID must be valid")
	}
	var exists bool
	err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workflow_runtime.inbox_messages WHERE consumer_name=$1 AND message_id=$2)`, validatedConsumer.String(),
		validatedMessageID.String()).Scan(&exists)
	if err != nil {
		return false, mapPostgreSQLError("check", "inbox message", err)
	}
	return exists, nil
}
func (store *Store) RecordInboxMessage(ctx context.Context, message repository.InboxMessage) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	return recordInboxMessage(ctx, store.pool, message)
}
func recordInboxMessage(ctx context.Context, db asyncPersistenceDatabase, message repository.InboxMessage) error {
	if ctx == nil {
		return fmt.Errorf("record inbox context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil || !message.IsValid() {
		return fmt.Errorf("inbox database and message must be valid")
	}
	nodeExecutionID := any(nil)
	if value, exists := message.NodeExecutionID(); exists {
		nodeExecutionID = value.String()
	}
	source, partition, offset := any(nil), any(nil), any(nil)
	if position, exists := message.SourcePosition(); exists {
		source = position.Source()
		partition = position.Partition()
		offset = position.Offset()
	}
	_, err := db.Exec(ctx, insertInboxSQL, message.ConsumerIdentity().String(), message.MessageID().String(), message.CompanyID().String(), message.WorkflowExecutionID().String(),
		nodeExecutionID, message.MessageType(), message.MessageVersion(), message.ProcessingResult().String(), source, partition, offset, message.ReceivedAt(), message.ProcessedAt())
	if err != nil {
		return mapInboxWriteError("record", err)
	}
	return nil
}

const selectInboxSQL = `SELECT consumer_name,message_id,company_id,workflow_execution_id,node_execution_id,message_type,message_version,processing_result,source_name,source_partition,source_offset,received_at,processed_at FROM workflow_runtime.inbox_messages WHERE consumer_name=$1 AND message_id=$2`

func getInboxMessage(ctx context.Context, db asyncPersistenceDatabase, consumer repository.ConsumerIdentity, messageID repository.MessageID) (repository.InboxMessage, error) {
	return scanInboxMessage(db.QueryRow(ctx, selectInboxSQL, consumer.String(), messageID.String()))
}

func scanInboxMessage(row rowScanner) (repository.InboxMessage, error) {
	var consumer, messageID, companyID, executionID, messageType, result string
	var nodeID, source *string
	var version int
	var partition *int32
	var offset *int64
	var receivedAt, processedAt time.Time
	if err := row.Scan(&consumer, &messageID, &companyID, &executionID, &nodeID, &messageType, &version, &result, &source, &partition, &offset, &receivedAt,
		&processedAt); err != nil {
		return repository.InboxMessage{}, mapPostgreSQLError("hydrate", "inbox message", err)
	}
	params := repository.InboxMessageParams{ConsumerIdentity: repository.ConsumerIdentity(consumer), MessageID: repository.MessageID(messageID),
		CompanyID: workflow.CompanyID(companyID), WorkflowExecutionID: execution.WorkflowExecutionID(executionID), MessageType: messageType, MessageVersion: version,
		ProcessingResult: repository.InboxProcessingResult(result), ReceivedAt: receivedAt, ProcessedAt: processedAt}
	if nodeID != nil {
		params.NodeExecutionID = execution.NodeExecutionID(*nodeID)
	}
	if (source == nil) != (partition == nil) || (source == nil) != (offset == nil) {
		return repository.InboxMessage{}, &databaseError{operation: "hydrate", resource: "inbox message", cause: errors.New("incomplete persisted source position")}
	}
	if source != nil {
		position, err := repository.NewInboxSourcePosition(*source, *partition, *offset)
		if err != nil {
			return repository.InboxMessage{}, &databaseError{operation: "hydrate", resource: "inbox message", cause: err}
		}
		params.SourcePosition = &position
	}
	message, err := repository.NewInboxMessage(params)
	if err != nil {
		return repository.InboxMessage{}, &databaseError{operation: "hydrate", resource: "inbox message", cause: err}
	}
	return message, nil
}
func mapInboxWriteError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "inbox_messages_pk" {
		return repository.NewConflictError(operation, "inbox consumer/message identity", err)
	}
	return mapPostgreSQLError(operation, "inbox message", err)
}
