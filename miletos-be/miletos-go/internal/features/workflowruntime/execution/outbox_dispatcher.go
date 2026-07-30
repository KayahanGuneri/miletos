package execution

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"miletos-go/internal/features/workflowruntime/execution/queue"
	"miletos-go/internal/features/workflowruntime/execution/repository"
)

const (
	outboxDispatchInterval = 250 * time.Millisecond
	outboxClaimStaleAfter  = 2 * time.Minute
	outboxClaimBatchSize   = 50
)

type OutboxDispatcher struct {
	outbox *repository.OutboxRepository
	queue  queue.Queue
	owner  string
}

func NewOutboxDispatcher(
	outbox *repository.OutboxRepository,
	nodeQueue queue.Queue,
) (*OutboxDispatcher, error) {
	identity := make([]byte, 12)
	if _, err := rand.Read(identity); err != nil {
		return nil, fmt.Errorf("generate outbox dispatcher identity: %w", err)
	}
	return &OutboxDispatcher{
		outbox: outbox,
		queue:  nodeQueue,
		owner:  "runtime_" + hex.EncodeToString(identity),
	}, nil
}

func (dispatcher *OutboxDispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(outboxDispatchInterval)
	defer ticker.Stop()
	dispatcher.dispatch(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dispatcher.dispatch(ctx)
		}
	}
}

func (dispatcher *OutboxDispatcher) dispatch(ctx context.Context) {
	if dispatcher.outbox == nil || dispatcher.queue == nil {
		return
	}
	if err := dispatcher.outbox.ReleaseStaleClaims(
		ctx, time.Now().UTC().Add(-outboxClaimStaleAfter),
	); err != nil {
		if ctx.Err() == nil {
			slog.Warn("release stale outbox claims failed", "error", err)
		}
		return
	}
	messages, err := dispatcher.outbox.ClaimDue(
		ctx, dispatcher.owner, outboxClaimBatchSize,
	)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("claim due outbox messages failed", "error", err)
		}
		return
	}
	for _, message := range messages {
		if err := dispatcher.queue.Push(
			ctx, message.Destination, message.MessageKey, message.Payload,
		); err != nil {
			if releaseErr := dispatcher.outbox.Release(ctx, message); releaseErr != nil &&
				ctx.Err() == nil {
				slog.Warn(
					"release unpublished outbox message failed",
					"messageId", message.ID,
					"error", releaseErr,
				)
			}
			continue
		}
		if err := dispatcher.outbox.MarkPublished(ctx, message); err != nil &&
			ctx.Err() == nil {
			slog.Warn(
				"mark published outbox message failed",
				"messageId", message.ID,
				"error", err,
			)
		}
	}
}
