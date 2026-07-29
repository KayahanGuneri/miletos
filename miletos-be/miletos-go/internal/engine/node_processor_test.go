package engine_test

import (
	"context"
	"strings"
	"testing"

	"miletos-go/internal/engine"
)

type consumingQueue struct {
	payload []byte
}

func (queue consumingQueue) Push(context.Context, string, string, []byte) error {
	return nil
}

func (queue consumingQueue) Consume(
	ctx context.Context,
	_ string,
	handler func(context.Context, []byte) error,
) error {
	return handler(ctx, queue.payload)
}

func TestNodeProcessorRejectsMissingQueue(t *testing.T) {
	processor := engine.NewNodeProcessor(nil, nil, nil, nil, nil, "commands", 3, 0)

	err := processor.Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestNodeProcessorRejectsMalformedJob(t *testing.T) {
	processor := engine.NewNodeProcessor(
		nil, nil, nil, nil,
		consumingQueue{payload: []byte("{invalid")},
		"commands", 3, 0,
	)

	err := processor.Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), "decode node job") {
		t.Fatalf("Run() error = %v", err)
	}
}
