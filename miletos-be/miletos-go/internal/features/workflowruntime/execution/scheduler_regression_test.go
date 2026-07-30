package execution

import (
	"context"
	"encoding/json"
	"testing"

	"miletos-go/internal/features/workflowruntime/execution/model"
	"miletos-go/internal/features/workflowruntime/execution/queue"
)

type capturedQueue struct {
	topic   string
	key     string
	payload []byte
}

func (captured *capturedQueue) Push(_ context.Context, topic, key string, payload []byte) error {
	captured.topic = topic
	captured.key = key
	captured.payload = append([]byte(nil), payload...)
	return nil
}

func (captured *capturedQueue) Consume(
	context.Context,
	string,
	func(context.Context, []byte) queue.RecordResult,
) error {
	return nil
}

func TestSchedulerPublishesWithNodeExecutionID(t *testing.T) {
	nodeQueue := &capturedQueue{}
	scheduler := NewScheduler(nil, nil, nodeQueue, "node-commands")
	job := model.NodeJob{
		CompanyID:       "company-1",
		WorkflowID:      "workflow-1",
		ExecutionID:     "execution-1",
		NodeID:          "branch-left",
		NodeExecutionID: "node-execution-left",
		Attempt:         1,
	}

	if err := scheduler.push(context.Background(), job); err != nil {
		t.Fatalf("push() error = %v", err)
	}
	if nodeQueue.key != job.NodeExecutionID {
		t.Fatalf("queue key = %q, want %q", nodeQueue.key, job.NodeExecutionID)
	}
	if nodeQueue.key == job.ExecutionID {
		t.Fatal("queue key incorrectly used the shared execution ID")
	}
	var published model.NodeJob
	if err := json.Unmarshal(nodeQueue.payload, &published); err != nil {
		t.Fatalf("decode published job: %v", err)
	}
	if published.NodeExecutionID != job.NodeExecutionID {
		t.Fatalf("published node execution ID = %q, want %q", published.NodeExecutionID, job.NodeExecutionID)
	}
}

func TestIndependentReadyBranchesUseIndependentKeys(t *testing.T) {
	leftQueue := &capturedQueue{}
	rightQueue := &capturedQueue{}
	leftScheduler := NewScheduler(nil, nil, leftQueue, "node-commands")
	rightScheduler := NewScheduler(nil, nil, rightQueue, "node-commands")

	left := model.NodeJob{
		ExecutionID: "execution-1", NodeID: "left", NodeExecutionID: "node-left",
	}
	right := model.NodeJob{
		ExecutionID: "execution-1", NodeID: "right", NodeExecutionID: "node-right",
	}
	if err := leftScheduler.push(context.Background(), left); err != nil {
		t.Fatalf("left push: %v", err)
	}
	if err := rightScheduler.push(context.Background(), right); err != nil {
		t.Fatalf("right push: %v", err)
	}
	if leftQueue.key == rightQueue.key {
		t.Fatalf("independent branches shared key %q", leftQueue.key)
	}
}
