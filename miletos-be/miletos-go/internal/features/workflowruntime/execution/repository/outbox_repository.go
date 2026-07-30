package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"miletos-go/internal/features/workflowruntime/execution/model"
	"miletos-go/internal/shared/database"
)

type OutboxMessage struct {
	ID          string
	Destination string
	MessageKey  string
	Payload     []byte
	ClaimOwner  string
	LockVersion int64
}

type OutboxRepository struct {
	dbClient *database.Client
}

func NewOutboxRepository(dbClient *database.Client) *OutboxRepository {
	return &OutboxRepository{dbClient: dbClient}
}

func retryOperationKey(job model.NodeJob) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"%s\x00%s\x00%s\x00%d",
		job.CompanyID,
		job.ExecutionID,
		job.NodeExecutionID,
		job.Attempt+1,
	)))
	return "retry:" + hex.EncodeToString(digest[:])
}

func persistRetryCommand(
	ctx context.Context,
	transaction *database.Transaction,
	job model.NodeJob,
	destination string,
	availableAt time.Time,
) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encode retry node command: %w", err)
	}
	now := time.Now().UTC()
	if availableAt.Before(now) {
		now = availableAt
	}
	_, err = transaction.Exec(ctx, `
		INSERT INTO workflow_runtime.outbox_messages (
			message_id, company_id, workflow_execution_id,
			node_execution_id, node_id, attempt,
			operation_kind, operation_key, operation_discriminator,
			message_type, message_version, destination, message_key,
			encoded_payload, publication_state, available_at,
			created_at, lock_version
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			'NODE_COMMAND', $7, NULL,
			'NODE_COMMAND', 1, $8, $9,
			$10, 'PENDING', $11, $12, 0
		)
		ON CONFLICT (company_id, workflow_execution_id, operation_key)
		DO NOTHING`,
		newID("outbox_"),
		job.CompanyID,
		job.ExecutionID,
		job.NodeExecutionID,
		job.NodeID,
		job.Attempt+1,
		retryOperationKey(job),
		destination,
		job.NodeExecutionID,
		payload,
		availableAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("persist retry outbox command: %w", err)
	}
	return nil
}

func (repository *OutboxRepository) EnsureRetryCommand(
	ctx context.Context,
	job model.NodeJob,
	destination string,
	availableAt time.Time,
) error {
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin retry outbox repair: %w", err)
	}
	defer transaction.Rollback(ctx)
	if err := persistRetryCommand(
		ctx, transaction, job, destination, availableAt,
	); err != nil {
		return err
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit retry outbox repair: %w", err)
	}
	return nil
}

func (repository *OutboxRepository) ClaimDue(
	ctx context.Context,
	owner string,
	limit int,
) ([]OutboxMessage, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer transaction.Rollback(ctx)
	rows, err := transaction.Query(ctx, `
		SELECT message_id, destination, message_key, encoded_payload, lock_version
		FROM workflow_runtime.outbox_messages
		WHERE publication_state = 'PENDING'
		  AND available_at <= CURRENT_TIMESTAMP
		ORDER BY available_at, created_at, message_id
		FOR UPDATE SKIP LOCKED
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("select due outbox messages: %w", err)
	}
	defer rows.Close()
	messages := make([]OutboxMessage, 0)
	for rows.Next() {
		var message OutboxMessage
		if err := rows.Scan(
			&message.ID,
			&message.Destination,
			&message.MessageKey,
			&message.Payload,
			&message.LockVersion,
		); err != nil {
			return nil, fmt.Errorf("scan due outbox message: %w", err)
		}
		message.ClaimOwner = owner
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due outbox messages: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close due outbox rows: %w", err)
	}
	now := time.Now().UTC()
	for index := range messages {
		result, err := transaction.Exec(ctx, `
			UPDATE workflow_runtime.outbox_messages
			SET publication_state = 'PUBLISHING',
				claimed_at = $3,
				claim_owner = $2,
				lock_version = lock_version + 1
			WHERE message_id = $1
			  AND publication_state = 'PENDING'
			  AND lock_version = $4`,
			messages[index].ID,
			owner,
			now,
			messages[index].LockVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("claim outbox message: %w", err)
		}
		if result.RowsAffected() != 1 {
			return nil, ErrStateTransition
		}
		messages[index].LockVersion++
	}
	if err := transaction.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit outbox claim: %w", err)
	}
	return messages, nil
}

func (repository *OutboxRepository) MarkPublished(
	ctx context.Context,
	message OutboxMessage,
) error {
	result, err := repository.dbClient.Exec(ctx, `
		UPDATE workflow_runtime.outbox_messages
		SET publication_state = 'PUBLISHED',
			published_at = $4,
			claimed_at = NULL,
			claim_owner = NULL,
			lock_version = lock_version + 1
		WHERE message_id = $1
		  AND publication_state = 'PUBLISHING'
		  AND claim_owner = $2
		  AND lock_version = $3`,
		message.ID,
		message.ClaimOwner,
		message.LockVersion,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("mark outbox message published: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrStateTransition
	}
	return nil
}

func (repository *OutboxRepository) Release(
	ctx context.Context,
	message OutboxMessage,
) error {
	_, err := repository.dbClient.Exec(ctx, `
		UPDATE workflow_runtime.outbox_messages
		SET publication_state = 'PENDING',
			claimed_at = NULL,
			claim_owner = NULL,
			lock_version = lock_version + 1
		WHERE message_id = $1
		  AND publication_state = 'PUBLISHING'
		  AND claim_owner = $2
		  AND lock_version = $3`,
		message.ID,
		message.ClaimOwner,
		message.LockVersion,
	)
	return err
}

func (repository *OutboxRepository) ReleaseStaleClaims(
	ctx context.Context,
	staleBefore time.Time,
) error {
	_, err := repository.dbClient.Exec(ctx, `
		UPDATE workflow_runtime.outbox_messages
		SET publication_state = 'PENDING',
			claimed_at = NULL,
			claim_owner = NULL,
			lock_version = lock_version + 1
		WHERE publication_state = 'PUBLISHING'
		  AND claimed_at < $1`,
		staleBefore,
	)
	return err
}
