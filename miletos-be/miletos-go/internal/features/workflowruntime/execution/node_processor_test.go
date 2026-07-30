package execution_test

import (
	"context"
	"strings"
	"testing"

	execution "miletos-go/internal/features/workflowruntime/execution"
	"miletos-go/internal/features/workflowruntime/execution/queue"
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
	processor := execution.NewNodeProcessor(nil, nil, nil, nil, nil, "commands", 3, 0)

	err := processor.Run(context.Background())

	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestNodeProcessorRejectsMalformedJob(t *testing.T) {
	observed := make(chan queue.RecordResult, 1)
	processor := execution.NewNodeProcessor(
		nil, nil, nil, nil,
		consumingQueue{payload: []byte("{invalid"), observed: observed},
		"commands", 3, 0,
	)

	err := processor.Run(context.Background())

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	result := <-observed
	if result.Disposition != queue.RecordDeadLetter || result.Code != "MALFORMED_NODE_JOB" {
		t.Fatalf("record result = %#v", result)
	}
}
