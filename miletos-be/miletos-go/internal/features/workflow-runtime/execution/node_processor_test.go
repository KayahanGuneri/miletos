package execution_test

import (
	"context"
	"strings"
	"testing"
	"time"

	execution "miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/execution/queue"
)

type consumingQueue struct {
	payload  []byte
	observed chan<- queue.RecordResult
}

func (queue consumingQueue) Push(context.Context, string, string, []byte) error {
	return nil
}

func (queue consumingQueue) Consume(
	ctx context.Context,
	_ string,
	handler func(context.Context, []byte) queue.RecordResult,
) error {
	result := handler(ctx, queue.payload)
	if queue.observed != nil {
		queue.observed <- result
	}
	return nil
}

func TestNodeProcessorRejectsMissingQueue(t *testing.T) {
	processor, err := execution.NewNodeProcessor(
		nil, nil, nil, nil, nil, "commands", 3, time.Millisecond, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewNodeProcessor() error = %v", err)
	}

	err = processor.Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestNodeProcessorRejectsMalformedJob(t *testing.T) {
	observed := make(chan queue.RecordResult, 1)
	processor, err := execution.NewNodeProcessor(
		nil, nil, nil, nil,
		consumingQueue{payload: []byte("{invalid"), observed: observed},
		"commands", 3, time.Millisecond, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewNodeProcessor() error = %v", err)
	}

	err = processor.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	result := <-observed
	if result.Disposition != queue.RecordDeadLetter || result.Code != "MALFORMED_NODE_JOB" {
		t.Fatalf("record result = %#v", result)
	}
}

func TestNewNodeProcessorRejectsInvalidRetryPolicy(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		delay    time.Duration
	}{
		{name: "zero attempts", attempts: 0, delay: time.Second},
		{name: "excessive attempts", attempts: 32768, delay: time.Second},
		{name: "zero delay", attempts: 3},
		{name: "negative delay", attempts: 3, delay: -time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			processor, err := execution.NewNodeProcessor(
				nil, nil, nil, nil, nil, "commands", test.attempts, test.delay, nil, nil,
			)
			if err == nil || processor != nil {
				t.Fatalf("NewNodeProcessor() = (%#v, %v), want constructor error", processor, err)
			}
		})
	}
}
