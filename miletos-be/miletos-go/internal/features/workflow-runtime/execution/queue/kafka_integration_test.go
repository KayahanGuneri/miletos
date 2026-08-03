//go:build integration

package queue_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	queue "miletos-go/internal/features/workflow-runtime/execution/queue"
)

func integrationKafka(t *testing.T) (*queue.Kafka, string) {
	t.Helper()
	rawBrokers := strings.TrimSpace(os.Getenv("MILETOS_TEST_KAFKA_BROKERS"))
	if rawBrokers == "" {
		t.Skip("MILETOS_TEST_KAFKA_BROKERS is required for Kafka integration tests")
	}
	brokers := strings.Split(rawBrokers, ",")
	hosts := make([]string, 0, len(brokers))
	for index, broker := range brokers {
		brokers[index] = strings.TrimSpace(broker)
		host, _, err := net.SplitHostPort(brokers[index])
		if err != nil {
			t.Skip("MILETOS_TEST_KAFKA_BROKERS contains an uncertain broker address")
		}
		if !safeKafkaTestHost(host) {
			t.Skipf(
				"MILETOS_TEST_KAFKA_BROKERS is not explicitly test-safe (host %q)",
				host,
			)
		}
		hosts = append(hosts, host)
	}
	t.Logf("using test Kafka hosts %q", hosts)
	suffixBytes := make([]byte, 8)
	if _, err := rand.Read(suffixBytes); err != nil {
		t.Fatalf("create Kafka test suffix: %v", err)
	}
	suffix := hex.EncodeToString(suffixBytes)
	topic := "miletos-integration-" + suffix
	createKafkaTopic(t, brokers, topic)
	kafka, err := queue.NewKafka(brokers, topic, topic)
	if err != nil {
		t.Fatalf("NewKafka() error = %v", err)
	}
	t.Cleanup(kafka.Close)
	return kafka, topic
}

func createKafkaTopic(t *testing.T, brokers []string, topicName string) {
	t.Helper()
	admin, err := kgo.NewClient(kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatalf("create Kafka admin client: %v", err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request := kmsg.NewPtrCreateTopicsRequest()
	topic := kmsg.NewCreateTopicsRequestTopic()
	topic.Topic = topicName
	topic.NumPartitions = 1
	topic.ReplicationFactor = 1
	request.Topics = append(request.Topics, topic)
	response, err := request.RequestWith(ctx, admin)
	if err != nil {
		t.Fatalf("create Kafka integration topic: %v", err)
	}
	if len(response.Topics) != 1 {
		t.Fatalf("create Kafka topic response count = %d, want 1", len(response.Topics))
	}
	if err := kerr.ErrorForCode(response.Topics[0].ErrorCode); err != nil {
		t.Fatalf("create Kafka integration topic %q: %v", topicName, err)
	}
}

func safeKafkaTestHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.Contains(host, "prod") || strings.Contains(host, "stag") {
		return false
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		host == "kafka" || host == "redpanda" || strings.Contains(host, "test")
}

func TestKafkaPayloadRoundTripIntegration(t *testing.T) {
	kafka, topic := integrationKafka(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	payload := []byte(`{"message":"round-trip"}`)
	received := make(chan []byte, 1)
	consumeResult := make(chan error, 1)
	go func() {
		consumeResult <- kafka.Consume(ctx, topic, func(_ context.Context, value []byte) queue.RecordResult {
			select {
			case received <- append([]byte(nil), value...):
				cancel()
			default:
			}
			return queue.RecordResult{Disposition: queue.RecordHandled}
		})
	}()

	for ctx.Err() == nil {
		if err := kafka.Push(ctx, topic, "round-trip-key", payload); err != nil && ctx.Err() == nil {
			if strings.Contains(err.Error(), "UNKNOWN_TOPIC_OR_PARTITION") {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			t.Fatalf("Push() error = %v", err)
		}
		select {
		case got := <-received:
			if string(got) != string(payload) {
				t.Fatalf("payload = %s, want %s", got, payload)
			}
			<-consumeResult
			return
		default:
		}
	}
	t.Fatal("Kafka round trip timed out")
}

func TestKafkaConsumerHandlerErrorIntegration(t *testing.T) {
	kafka, topic := integrationKafka(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	consumeResult := make(chan error, 1)
	retried := make(chan struct{}, 1)
	go func() {
		consumeResult <- kafka.Consume(ctx, topic, func(context.Context, []byte) queue.RecordResult {
			select {
			case retried <- struct{}{}:
				cancel()
			default:
			}
			return queue.RecordResult{
				Disposition: queue.RecordRetry,
				Code:        "CONTROLLED_RETRY",
			}
		})
	}()

	var once sync.Once
	for ctx.Err() == nil {
		if err := kafka.Push(ctx, topic, "handler-error-key", []byte("payload")); err != nil && ctx.Err() == nil {
			if strings.Contains(err.Error(), "UNKNOWN_TOPIC_OR_PARTITION") {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			t.Fatalf("Push() error = %v", err)
		}
		select {
		case <-retried:
			once.Do(cancel)
			if err := <-consumeResult; err != nil {
				t.Fatalf("Consume() error = %v", err)
			}
			return
		default:
		}
	}
	t.Fatal("Kafka handler error test timed out")
}

func TestKafkaConsumeStopsForCancelledContextIntegration(t *testing.T) {
	kafka, topic := integrationKafka(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := kafka.Consume(ctx, topic, func(context.Context, []byte) queue.RecordResult {
		t.Fatal("handler was called for canceled context")
		return queue.RecordResult{Disposition: queue.RecordHandled}
	})

	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
}
