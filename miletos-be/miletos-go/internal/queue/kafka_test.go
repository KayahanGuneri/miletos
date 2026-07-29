package queue_test

import (
	"context"
	"strings"
	"testing"

	"miletos-go/internal/queue"
)

func TestNewKafkaRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		brokers []string
		client  string
		group   string
		want    string
	}{
		{name: "brokers", client: "client", group: "group", want: "brokers"},
		{name: "client", brokers: []string{"127.0.0.1:1"}, group: "group", want: "client ID"},
		{name: "group", brokers: []string{"127.0.0.1:1"}, client: "client", want: "consumer group"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := queue.NewKafka(test.brokers, test.client, test.group)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewKafka() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestKafkaRejectsInvalidPushAndConsumeArgumentsWithoutConnecting(t *testing.T) {
	kafka, err := queue.NewKafka([]string{"127.0.0.1:1"}, "client", "group")
	if err != nil {
		t.Fatalf("NewKafka() error = %v", err)
	}
	defer kafka.Close()

	if err := kafka.Push(context.Background(), "", "key", []byte("payload")); err == nil {
		t.Fatal("empty topic Push() error = nil")
	}
	if err := kafka.Push(context.Background(), "topic", "key", nil); err == nil {
		t.Fatal("empty payload Push() error = nil")
	}
	if err := kafka.Consume(context.Background(), "", func(context.Context, []byte) error {
		return nil
	}); err == nil {
		t.Fatal("empty topic Consume() error = nil")
	}
	if err := kafka.Consume(context.Background(), "topic", nil); err == nil {
		t.Fatal("nil handler Consume() error = nil")
	}
}
