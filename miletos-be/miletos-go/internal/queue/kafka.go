package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

type Kafka struct {
	producer      *kgo.Client
	brokers       []string
	clientID      string
	consumerGroup string
	concurrency   int
}

func NewKafka(brokers []string, clientID, consumerGroup string) (*Kafka, error) {
	return NewKafkaWithConcurrency(brokers, clientID, consumerGroup, 2)
}

func NewKafkaWithConcurrency(
	brokers []string,
	clientID string,
	consumerGroup string,
	concurrency int,
) (*Kafka, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("Kafka brokers are required")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("Kafka client ID is required")
	}
	if strings.TrimSpace(consumerGroup) == "" {
		return nil, fmt.Errorf("Kafka consumer group is required")
	}
	if concurrency < 2 || concurrency > 5 {
		return nil, fmt.Errorf("Kafka node concurrency must be between 2 and 5")
	}
	producer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ClientID(clientID),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka producer: %w", err)
	}
	return &Kafka{
		producer:      producer,
		brokers:       append([]string(nil), brokers...),
		clientID:      clientID,
		consumerGroup: consumerGroup,
		concurrency:   concurrency,
	}, nil
}

func (kafka *Kafka) Push(ctx context.Context, topic, key string, payload []byte) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("Kafka topic is required")
	}
	if len(payload) == 0 {
		return fmt.Errorf("Kafka payload is required")
	}
	result := kafka.producer.ProduceSync(ctx, &kgo.Record{
		Topic: topic,
		Key:   []byte(key),
		Value: payload,
	})
	if err := result.FirstErr(); err != nil {
		return fmt.Errorf("push Kafka message: %w", err)
	}
	return nil
}

func (kafka *Kafka) Consume(
	ctx context.Context,
	topic string,
	handler func(context.Context, []byte) error,
) error {
	if strings.TrimSpace(topic) == "" {
		return fmt.Errorf("Kafka topic is required")
	}
	if handler == nil {
		return fmt.Errorf("Kafka message handler is required")
	}
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(kafka.brokers...),
		kgo.ClientID(kafka.clientID+"-consumer"),
		kgo.ConsumerGroup(kafka.consumerGroup),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return fmt.Errorf("create Kafka consumer: %w", err)
	}
	defer consumer.Close()

	for {
		fetches := consumer.PollFetches(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if fetches.IsClientClosed() {
			return nil
		}
		if fetchErrors := fetches.Errors(); len(fetchErrors) > 0 {
			return fmt.Errorf("consume Kafka message: %w", fetchErrors[0].Err)
		}
		records := make([]*kgo.Record, 0)
		fetches.EachRecord(func(record *kgo.Record) {
			records = append(records, record)
		})
		var waitGroup sync.WaitGroup
		var errorMutex sync.Mutex
		var processingError error
		semaphore := make(chan struct{}, kafka.concurrency)
		submitted := make([]*kgo.Record, 0, len(records))
		for _, record := range records {
			select {
			case <-ctx.Done():
				waitGroup.Wait()
				return nil
			case semaphore <- struct{}{}:
			}
			submitted = append(submitted, record)
			waitGroup.Add(1)
			go func(current *kgo.Record) {
				defer waitGroup.Done()
				defer func() { <-semaphore }()
				if err := invokeHandler(ctx, handler, current.Value); err != nil {
					errorMutex.Lock()
					if processingError == nil {
						processingError = err
					}
					errorMutex.Unlock()
				}
			}(record)
		}
		waitGroup.Wait()
		if ctx.Err() != nil {
			return nil
		}
		if processingError != nil {
			return processingError
		}
		if len(submitted) > 0 {
			if err := consumer.CommitRecords(ctx, submitted...); err != nil {
				return fmt.Errorf("commit Kafka offsets: %w", err)
			}
		}
	}
}

func invokeHandler(
	ctx context.Context,
	handler func(context.Context, []byte) error,
	payload []byte,
) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("Kafka message handler panicked")
		}
	}()
	return handler(ctx, payload)
}

func (kafka *Kafka) Close() {
	if kafka != nil && kafka.producer != nil {
		kafka.producer.Close()
	}
}
