package kafka

import (
	"context"
	"fmt"
	"time"

	repository "miletos-go/internal/ports/persistence"
)

type OutboxPublisher struct {
	store        repository.OutboxPublicationStore
	producer     MessageProducer
	claimOwner   string
	batchLimit   int
	pollInterval time.Duration
	staleAfter   time.Duration
	now          func() time.Time
}

func NewOutboxPublisher(store repository.OutboxPublicationStore, producer MessageProducer, claimOwner string, batchLimit int, pollInterval,
	staleAfter time.Duration) (*OutboxPublisher, error) {
	if store == nil || producer == nil || claimOwner == "" || batchLimit <= 0 || pollInterval <= 0 || staleAfter <= 0 {
		return nil, fmt.Errorf("invalid outbox publisher configuration")
	}
	return &OutboxPublisher{store: store, producer: producer, claimOwner: claimOwner, batchLimit: batchLimit, pollInterval: pollInterval, staleAfter: staleAfter, now: time.Now}, nil
}
func (publisher *OutboxPublisher) PublishBatch(ctx context.Context) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf("publish batch context must not be nil")
	}
	now := publisher.now().UTC()
	if _, err := publisher.store.ReleaseStaleOutboxClaims(ctx, mustReleaseRequest(now.Add(-publisher.staleAfter), publisher.batchLimit)); err != nil {
		return 0, err
	}
	claimed, err := publisher.store.ClaimPublishableOutboxMessages(ctx, mustClaimRequest(publisher.claimOwner, now, publisher.batchLimit))
	if err != nil {
		return 0, err
	}
	published := 0
	for _, message := range claimed {
		headers := map[string]string{"message_type": message.MessageType(), "message_version": fmt.Sprint(message.MessageVersion())}
		if err := publisher.producer.Publish(ctx, message.Destination(), message.MessageKey(), message.EncodedPayload(), headers); err != nil {
			return published, err
		}
		command := mustMarkCommand(message, publisher.claimOwner, publisher.now().UTC())
		if err := publisher.store.MarkOutboxMessagePublished(ctx, command); err != nil {
			return published, err
		}
		published++
	}
	return published, nil
}
func (publisher *OutboxPublisher) Run(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("publisher context must not be nil")
	}
	ticker := time.NewTicker(publisher.pollInterval)
	defer ticker.Stop()
	for {
		if _, err := publisher.PublishBatch(ctx); err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func mustClaimRequest(owner string, at time.Time, limit int) repository.OutboxClaimRequest {
	request, _ := repository.NewOutboxClaimRequest(owner, at, limit)
	return request
}
func mustReleaseRequest(before time.Time, limit int) repository.ReleaseStaleOutboxClaimsRequest {
	request, _ := repository.NewReleaseStaleOutboxClaimsRequest(before, limit)
	return request
}
func mustMarkCommand(message repository.OutboxMessage, owner string, at time.Time) repository.MarkOutboxPublishedCommand {
	command, _ := repository.NewMarkOutboxPublishedCommand(message.MessageID(), owner, message.LockVersion(), at)
	return command
}
