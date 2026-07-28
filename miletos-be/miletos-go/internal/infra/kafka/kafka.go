package kafka

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"miletos-go/internal/ports/messaging"
)

type Delivery = messaging.Delivery
type DeliveryHandler = messaging.DeliveryHandler
type Consumer struct{ client *kgo.Client }

func NewConsumer(
	brokers []string,
	clientID string,
	groupID string,
	topics ...string,
) (*Consumer, error) {
	if !validKafkaAddresses(brokers) ||
		!validKafkaName(clientID) ||
		!validKafkaName(groupID) ||
		!validKafkaNames(topics) {
		return nil, fmt.Errorf("invalid Kafka consumer configuration")
	}
	options := []kgo.Opt{kgo.SeedBrokers(brokers...), kgo.ClientID(clientID),
		kgo.ConsumerGroup(groupID), kgo.ConsumeTopics(topics...), kgo.DisableAutoCommit(),
	}
	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("create Kafka consumer: %w", err)
	}
	return &Consumer{client: client}, nil
}
func (consumer *Consumer) Run(ctx context.Context, handler DeliveryHandler) error {
	if consumer == nil || consumer.client == nil || handler == nil {
		return fmt.Errorf("Kafka consumer is not initialized")
	}
	if ctx == nil {
		return fmt.Errorf("consumer context must not be nil")
	}
	for {
		fetches := consumer.client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return nil
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			return fmt.Errorf("consume Kafka messages: %v", errs[0].Err)
		}
		var handlerErr error
		var handlerMu sync.Mutex
		var wait sync.WaitGroup
		fetches.EachRecord(func(record *kgo.Record) {
			wait.Add(1)
			go func(record *kgo.Record) {
				defer wait.Done()
				delivery := Delivery{Topic: record.Topic, Partition: record.Partition, Offset: record.Offset, Key: append([]byte(nil), record.Key...), Value: append([]byte(nil),
					record.Value...), Headers: make(map[string]string, len(record.Headers))}
				for _, header := range record.Headers {
					delivery.Headers[header.Key] = string(header.Value)
				}
				if err := handler(ctx, delivery); err != nil {
					handlerMu.Lock()
					if handlerErr == nil {
						handlerErr = err
					}
					handlerMu.Unlock()
					return
				}
				consumer.client.MarkCommitRecords(record)
			}(record)
		})
		wait.Wait()
		if handlerErr != nil {
			return handlerErr
		}
		if err := consumer.client.CommitRecords(ctx); err != nil {
			return fmt.Errorf("commit Kafka offsets: %w", err)
		}
	}
}
func (consumer *Consumer) Close() {
	if consumer != nil && consumer.client != nil {
		consumer.client.Close()
	}
}
func CheckTopics(
	ctx context.Context,
	brokers []string,
	clientID string,
	topics ...string,
) error {
	if ctx == nil ||
		!validKafkaAddresses(brokers) ||
		!validKafkaName(clientID) ||
		!validKafkaNames(topics) {
		return fmt.Errorf("invalid Kafka topic check configuration")
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ClientID(clientID),
	)
	if err != nil {
		return fmt.Errorf("create Kafka metadata client: %w", err)
	}
	defer client.Close()
	request := kmsg.NewPtrMetadataRequest()
	for _, topic := range topics {
		topicCopy := topic
		request.Topics = append(request.Topics, kmsg.MetadataRequestTopic{Topic: &topicCopy})
	}
	metadata, err := client.RequestCachedMetadata(ctx, request, 0)
	if err != nil {
		return fmt.Errorf("Kafka metadata check: %w", err)
	}
	for _, topic := range topics {
		found := false
		for _, metadataTopic := range metadata.Topics {
			if metadataTopic.Topic != nil && *metadataTopic.Topic == topic && metadataTopic.ErrorCode == 0 {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Kafka topic %q not found", topic)
		}
	}
	return nil
}

// MessageProducer is the transport boundary used by the outbox publisher.
// Kafka client types intentionally do not escape this package.
type MessageProducer interface {
	Publish(ctx context.Context, destination, key string, payload []byte, headers map[string]string) error
	Close()
}
type Producer struct {
	client              *kgo.Client
	maximumMessageBytes int
}

var _ MessageProducer = (*Producer)(nil)

func NewProducer(
	brokers []string,
	clientID string,
	maximumMessageBytes int,
) (*Producer, error) {
	if !validKafkaAddresses(brokers) {
		return nil, fmt.Errorf("invalid Kafka configuration")
	}
	if !validKafkaName(clientID) {
		return nil, fmt.Errorf("Kafka client ID must not be empty")
	}
	if maximumMessageBytes <= 0 {
		return nil, fmt.Errorf("Kafka maximum message bytes must be positive")
	}
	options := []kgo.Opt{kgo.SeedBrokers(brokers...),
		kgo.ClientID(clientID), kgo.RequiredAcks(kgo.AllISRAcks())}
	client, err := kgo.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("create Kafka client: %w", err)
	}
	return &Producer{
		client:              client,
		maximumMessageBytes: maximumMessageBytes,
	}, nil
}
func (producer *Producer) Publish(ctx context.Context, destination, key string, payload []byte, headers map[string]string) error {
	if producer == nil || producer.client == nil {
		return fmt.Errorf("Kafka producer is not initialized")
	}
	if ctx == nil {
		return fmt.Errorf("publish context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if destination == "" || key == "" {
		return fmt.Errorf("destination and key must not be empty")
	}
	if len(payload) == 0 || len(payload) > producer.maximumMessageBytes {
		return fmt.Errorf("Kafka payload size is invalid")
	}
	record := &kgo.Record{Topic: destination, Key: []byte(key), Value: append([]byte(nil), payload...)}
	for name, value := range headers {
		record.Headers = append(record.Headers, kgo.RecordHeader{Key: name, Value: []byte(value)})
	}
	results := producer.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("publish Kafka message: %w", err)
	}
	return nil
}
func (producer *Producer) Close() {
	if producer != nil && producer.client != nil {
		producer.client.Close()
	}
}

func validKafkaAddresses(values []string) bool {
	return validKafkaNames(values)
}

func validKafkaNames(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !validKafkaName(value) {
			return false
		}
	}
	return true
}

func validKafkaName(value string) bool {
	normalized := strings.TrimSpace(value)
	return normalized != "" && normalized == value
}
