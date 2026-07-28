package kafka

import (
	context "context"
	errors "errors"
	config "miletos-go/internal/config"
	repository "miletos-go/internal/ports/persistence"
	os "os"
	testing "testing"
	time "time"
)

func TestKafkaProducerConsumerIntegration(t *testing.T) {
	broker := os.Getenv("MILETOS_KAFKA_TEST_BROKER")
	if broker == "" {
		t.Skip("MILETOS_KAFKA_TEST_BROKER is not configured")
	}
	configuration := config.KafkaConfig{Enabled: true, Brokers: []string{broker}, EngineClientID: "miletos-test-engine", WorkerClientID: "miletos-test-worker", CommandTopic: "miletos.test.commands", EventTopic: "miletos.test.events", WorkerGroupID: "miletos-test-workers", EngineGroupID: "miletos-test-engine", MaxMessageBytes: 1024 * 1024}
	topic := "miletos.cp08.integration"
	producer, err := NewProducer(
		configuration.Brokers,
		configuration.EngineClientID,
		configuration.MaxMessageBytes,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer producer.Close()
	consumer, err := NewConsumer(
		configuration.Brokers,
		configuration.EngineClientID,
		"miletos-cp08-integration-group",
		topic,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer consumer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	received := make(chan Delivery, 1)
	consumerErr := make(chan error, 1)
	go func() {
		consumerErr <- consumer.Run(ctx, func(_ context.Context, delivery Delivery) error { received <- delivery; return nil })
	}()
	key := "workflow-integration"
	payload := []byte(`{"message_id":"integration-message","message_version":1}`)
	if err := producer.Publish(ctx, topic, key, payload, map[string]string{"message_type": "test"}); err != nil {
		t.Fatal(err)
	}
	select {
	case delivery := <-received:
		if string(delivery.Key) != key || string(delivery.Value) != string(payload) {
			t.Fatalf("delivery key/value mismatch: %q %s", delivery.Key, delivery.Value)
		}
	case err := <-consumerErr:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cancel()
}

type fakePublisherStore struct {
	message repository.OutboxMessage
	marked  bool
	claimed bool
}

func (store *fakePublisherStore) ClaimPublishableOutboxMessages(context.Context, repository.OutboxClaimRequest) ([]repository.OutboxMessage, error) {
	store.claimed = true
	return []repository.OutboxMessage{store.message}, nil
}
func (store *fakePublisherStore) MarkOutboxMessagePublished(_ context.Context, command repository.MarkOutboxPublishedCommand) error {
	store.marked = command.MessageID() == store.message.MessageID()
	return nil
}
func (store *fakePublisherStore) ReleaseStaleOutboxClaims(context.Context, repository.ReleaseStaleOutboxClaimsRequest) (int64, error) {
	return 0, nil
}

type fakeProducer struct {
	err    error
	called bool
}

func (producer *fakeProducer) Publish(context.Context, string, string, []byte, map[string]string) error {
	producer.called = true
	return producer.err
}
func (producer *fakeProducer) Close() {}

func TestOutboxPublisherPublishesAndMarksAfterSuccess(t *testing.T) {
	message, err := repository.NewOutboxMessage(repository.OutboxMessageParams{
		MessageID: "publisher-test", CompanyID: "company", WorkflowExecutionID: "execution",
		OperationKind: repository.OutboxOperationWorkflowEvent, OperationDiscriminator: "publisher-test-op",
		MessageType: "test", MessageVersion: 1, Destination: "topic", MessageKey: "execution",
		EncodedPayload: []byte(`{"version":1}`), PublicationState: repository.OutboxPublicationPublishing,
		CreatedAt: time.Now().UTC(), ClaimedAt: time.Now().UTC(), ClaimOwner: "publisher", LockVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &fakePublisherStore{message: message}
	producer := &fakeProducer{}
	publisher, err := NewOutboxPublisher(store, producer, "publisher", 10, time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	count, err := publisher.PublishBatch(context.Background())
	if err != nil || count != 1 || !store.claimed || !producer.called || !store.marked {
		t.Fatalf("count=%d err=%v claimed=%v called=%v marked=%v", count, err, store.claimed, producer.called, store.marked)
	}
}

func TestOutboxPublisherDoesNotMarkOnPublishFailure(t *testing.T) {
	message, err := repository.NewOutboxMessage(repository.OutboxMessageParams{
		MessageID: "publisher-fail", CompanyID: "company", WorkflowExecutionID: "execution",
		OperationKind: repository.OutboxOperationWorkflowEvent, OperationDiscriminator: "publisher-fail-op",
		MessageType: "test", MessageVersion: 1, Destination: "topic", MessageKey: "execution",
		EncodedPayload: []byte(`{"version":1}`), PublicationState: repository.OutboxPublicationPublishing,
		CreatedAt: time.Now().UTC(), ClaimedAt: time.Now().UTC(), ClaimOwner: "publisher", LockVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := &fakePublisherStore{message: message}
	producer := &fakeProducer{err: errors.New("broker unavailable")}
	publisher, err := NewOutboxPublisher(store, producer, "publisher", 10, time.Second, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.PublishBatch(context.Background()); err == nil || store.marked {
		t.Fatalf("err=%v marked=%v", err, store.marked)
	}
}
