//go:build integration

package execution_test

import (
	"context"
	"errors"
	"testing"

	execution "miletos-go/internal/features/workflowruntime/execution"
	model "miletos-go/internal/features/workflowruntime/execution/model"
	repository "miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/plugin"
	workflowfeature "miletos-go/internal/features/workflowruntime/workflow"
)

func integrationExecutionService(
	t *testing.T,
) (
	*execution.ExecutionService,
	*repository.ExecutionRepository,
	*recordingQueue,
) {
	t.Helper()
	_ = engineIntegrationDatabase(t)
	client := engineDatabaseClient(t)
	workflows := workflowfeature.NewWorkflowRepository(client)
	executions := repository.NewExecutionRepository(client)
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	nodeQueue := &recordingQueue{}
	scheduler := execution.NewScheduler(workflows, executions, nodeQueue, "commands")
	processor := execution.NewNodeProcessor(
		workflows, executions, registry, scheduler, nodeQueue, "commands", 1, 0,
	)
	service := execution.NewExecutionService(
		workflowfeature.NewWorkflowService(workflows, registry),
		executions, scheduler, processor, true,
	)
	return service, executions, nodeQueue
}

func TestExecutionServiceSyncIntegration(t *testing.T) {
	service, executions, _ := integrationExecutionService(t)
	workflow := engineWorkflow()

	outcome, err := service.ExecuteSync(
		context.Background(), workflow,
		nil, "corr-sync",
	)

	if err != nil {
		t.Fatalf("ExecuteSync() error = %v", err)
	}
	if outcome.Execution.Status != model.ExecutionSucceeded ||
		outcome.Execution.CorrelationID != "corr-sync" {
		t.Fatalf("sync outcome = %#v", outcome)
	}
	nodes, err := executions.ListAllNodes(
		context.Background(), workflow.CompanyID, outcome.Execution.ID,
	)
	if err != nil || len(nodes) != 2 {
		t.Fatalf("ListAllNodes() = (%#v, %v)", nodes, err)
	}
	for _, node := range nodes {
		if node.Status != model.NodeSucceeded {
			t.Errorf("node %s status = %s", node.NodeID, node.Status)
		}
	}
}

func TestExecutionServiceAsyncIdempotencyIntegration(t *testing.T) {
	service, _, nodeQueue := integrationExecutionService(t)
	workflow := engineWorkflow()

	first, err := service.ExecuteAsync(
		context.Background(), workflow, nil, "corr-async", "key-1", "fingerprint-1",
	)
	if err != nil {
		t.Fatalf("ExecuteAsync() error = %v", err)
	}
	if first.Execution.Status != model.ExecutionQueued || first.ScheduledRoots != 1 {
		t.Fatalf("async outcome = %#v", first)
	}
	replay, err := service.ExecuteAsync(
		context.Background(), workflow, nil, "corr-async", "key-1", "fingerprint-1",
	)
	if err != nil || !replay.Replayed || replay.Execution.ID != first.Execution.ID {
		t.Fatalf("replay outcome = (%#v, %v)", replay, err)
	}
	if len(nodeQueue.snapshot()) != 1 {
		t.Fatalf("duplicate scheduling jobs = %#v", nodeQueue.snapshot())
	}
	if _, err := service.ExecuteAsync(
		context.Background(), workflow, nil, "corr-async", "key-1", "different",
	); !errors.Is(err, repository.ErrIdempotencyConflict) {
		t.Fatalf("conflicting ExecuteAsync() error = %v", err)
	}
}

func TestExecutionServiceTerminalReplayDoesNotRescheduleIntegration(t *testing.T) {
	terminalStatuses := []string{
		model.ExecutionSucceeded,
		model.ExecutionFailed,
		model.ExecutionCancelled,
		model.ExecutionRejected,
		model.ExecutionTimedOut,
	}
	for _, status := range terminalStatuses {
		t.Run(status, func(t *testing.T) {
			service, executions, nodeQueue := integrationExecutionService(t)
			workflow := engineWorkflow()
			first, err := service.ExecuteAsync(
				context.Background(), workflow, nil,
				"corr-terminal", "terminal-key", "terminal-fingerprint",
			)
			if err != nil {
				t.Fatalf("ExecuteAsync() error = %v", err)
			}
			outputs := map[string]any{"preserved": "output"}
			failure := map[string]any{"message": "preserved error"}
			if err := executions.Finalize(
				context.Background(), workflow.CompanyID, first.Execution.ID,
				status, outputs, failure,
			); err != nil {
				t.Fatalf("Finalize(%s) error = %v", status, err)
			}
			beforeJobs := len(nodeQueue.snapshot())

			for replayNumber := 1; replayNumber <= 2; replayNumber++ {
				replay, err := service.ExecuteAsync(
					context.Background(), workflow, nil,
					"corr-terminal", "terminal-key", "terminal-fingerprint",
				)
				if err != nil || !replay.Replayed || replay.ScheduledRoots != 0 ||
					replay.Execution.Status != status ||
					replay.Execution.TerminalOutputs["preserved"] != "output" ||
					replay.Execution.Failure["message"] != "preserved error" {
					t.Fatalf("terminal replay #%d = (%#v, %v)", replayNumber, replay, err)
				}
			}
			if jobs := len(nodeQueue.snapshot()); jobs != beforeJobs {
				t.Fatalf("terminal replay queued %d jobs, had %d", jobs, beforeJobs)
			}
		})
	}
}
