//go:build integration

package queue_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"miletos-go/internal/queue"
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
	kafka, err := queue.NewKafka(brokers, "miletos-integration-"+suffix, "miletos-integration-"+suffix)
	if err != nil {
		t.Fatalf("NewKafka() error = %v", err)
	}
	t.Cleanup(kafka.Close)
	return kafka, "miletos-integration-" + suffix
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
		consumeResult <- kafka.Consume(ctx, topic, func(_ context.Context, value []byte) error {
			select {
			case received <- append([]byte(nil), value...):
				cancel()
			default:
			}
			return nil
		})
	}()

	for ctx.Err() == nil {
		if err := kafka.Push(ctx, topic, "round-trip-key", payload); err != nil && ctx.Err() == nil {
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
	wantError := errors.New("handler failed")
	consumeResult := make(chan error, 1)
	go func() {
		consumeResult <- kafka.Consume(ctx, topic, func(context.Context, []byte) error {
			return wantError
		})
	}()

	var once sync.Once
	for ctx.Err() == nil {
		if err := kafka.Push(ctx, topic, "handler-error-key", []byte("payload")); err != nil && ctx.Err() == nil {
			t.Fatalf("Push() error = %v", err)
		}
		select {
		case err := <-consumeResult:
			once.Do(cancel)
			if !errors.Is(err, wantError) {
				t.Fatalf("Consume() error = %v, want %v", err, wantError)
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

	err := kafka.Consume(ctx, topic, func(context.Context, []byte) error {
		t.Fatal("handler was called for canceled context")
		return nil
	})

	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
}
