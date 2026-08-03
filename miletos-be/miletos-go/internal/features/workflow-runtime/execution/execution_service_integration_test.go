//go:build integration

package execution_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	execution "miletos-go/internal/features/workflow-runtime/execution"
	model "miletos-go/internal/features/workflow-runtime/execution/model"
	repository "miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
)

func integrationExecutionService(
	t *testing.T,
) (
	*execution.ExecutionService,
	*repository.ExecutionRepository,
	*pgxpool.Pool,
) {
	t.Helper()

	pool := engineIntegrationDatabase(t)
	client := engineDatabaseClient(t)

	workflows := workflowfeature.NewWorkflowRepository(client)
	executions := repository.NewExecutionRepository(client)

	registry := plugin.NewNodeRegistry()

	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf(
			"RegisterBuiltinNodes() error = %v",
			err,
		)
	}

	nodeQueue := &recordingQueue{}

	scheduler := execution.NewScheduler(
		workflows,
		executions,
		nodeQueue,
		"commands",
		registry,
	)

	processor := newEngineNodeProcessor(
		t,
		workflows,
		executions,
		registry,
		scheduler,
		nodeQueue,
		"commands",
		1,
		time.Millisecond,
	)

	service := execution.NewExecutionService(
		workflowfeature.NewWorkflowService(
			workflows,
			registry,
		),
		executions,
		scheduler,
		processor,
		true,
	)

	return service, executions, pool
}

func TestExecutionServiceSyncIntegration(
	t *testing.T,
) {
	service, executions, _ := integrationExecutionService(t)
	workflow := engineWorkflow()

	outcome, err := service.ExecuteSync(
		context.Background(),
		workflow,
		nil,
		"corr-sync",
	)
	if err != nil {
		t.Fatalf(
			"ExecuteSync() error = %v",
			err,
		)
	}

	if outcome.Execution.Status != model.ExecutionSucceeded ||
		outcome.Execution.CorrelationID != "corr-sync" {
		t.Fatalf(
			"sync outcome = %#v",
			outcome,
		)
	}

	nodes, err := executions.ListAllNodes(
		context.Background(),
		workflow.CompanyID,
		outcome.Execution.ID,
	)
	if err != nil || len(nodes) != 2 {
		t.Fatalf(
			"ListAllNodes() = (%#v, %v)",
			nodes,
			err,
		)
	}

	for _, node := range nodes {
		if node.Status != model.NodeSucceeded {
			t.Errorf(
				"node %s status = %s",
				node.NodeID,
				node.Status,
			)
		}
	}
}

func TestExecutionServiceAsyncIdempotencyIntegration(
	t *testing.T,
) {
	service, _, pool := integrationExecutionService(t)
	workflow := engineWorkflow()

	first, err := service.ExecuteAsync(
		context.Background(),
		workflow,
		nil,
		"corr-async",
		"key-1",
		strings.Repeat("a", 64),
	)
	if err != nil {
		t.Fatalf(
			"ExecuteAsync() error = %v",
			err,
		)
	}

	if first.Execution.Status != model.ExecutionQueued ||
		first.ScheduledEntryNodes != 1 {
		t.Fatalf(
			"async outcome = %#v",
			first,
		)
	}

	replay, err := service.ExecuteAsync(
		context.Background(),
		workflow,
		nil,
		"corr-async",
		"key-1",
		strings.Repeat("a", 64),
	)
	if err != nil ||
		!replay.Replayed ||
		replay.Execution.ID != first.Execution.ID {
		t.Fatalf(
			"replay outcome = (%#v, %v)",
			replay,
			err,
		)
	}

	commandCount := countNodeCommandOutbox(
		t,
		pool,
		workflow.CompanyID,
		first.Execution.ID,
	)

	if commandCount != 1 {
		t.Fatalf(
			"durable scheduling command count = %d, want 1",
			commandCount,
		)
	}

	_, err = service.ExecuteAsync(
		context.Background(),
		workflow,
		nil,
		"corr-async",
		"key-1",
		strings.Repeat("b", 64),
	)

	if !errors.Is(
		err,
		repository.ErrIdempotencyConflict,
	) {
		t.Fatalf(
			"conflicting ExecuteAsync() error = %v",
			err,
		)
	}
}

func TestExecutionServiceTerminalReplayDoesNotRescheduleIntegration(
	t *testing.T,
) {
	terminalStatuses := []model.ExecutionStatus{
		model.ExecutionSucceeded,
		model.ExecutionFailed,
		model.ExecutionCancelled,
		model.ExecutionRejected,
		model.ExecutionTimedOut,
	}

	for _, status := range terminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			service, executions, pool :=
				integrationExecutionService(t)

			workflow := engineWorkflow()

			first, err := service.ExecuteAsync(
				context.Background(),
				workflow,
				nil,
				"corr-terminal",
				"terminal-key",
				strings.Repeat("c", 64),
			)
			if err != nil {
				t.Fatalf(
					"ExecuteAsync() error = %v",
					err,
				)
			}

			outputs := map[string]any{
				"preserved": "output",
			}

			failure := map[string]any{
				"message": "preserved error",
			}

			if err := executions.Finalize(
				context.Background(),
				workflow.CompanyID,
				first.Execution.ID,
				status,
				outputs,
				failure,
			); err != nil {
				t.Fatalf(
					"Finalize(%s) error = %v",
					status,
					err,
				)
			}

			beforeCommands := countNodeCommandOutbox(
				t,
				pool,
				workflow.CompanyID,
				first.Execution.ID,
			)

			if beforeCommands != 1 {
				t.Fatalf(
					"initial durable command count = %d, want 1",
					beforeCommands,
				)
			}

			for replayNumber := 1; replayNumber <= 2; replayNumber++ {
				replay, err := service.ExecuteAsync(
					context.Background(),
					workflow,
					nil,
					"corr-terminal",
					"terminal-key",
					strings.Repeat("c", 64),
				)

				if err != nil ||
					!replay.Replayed ||
					replay.ScheduledEntryNodes != 0 ||
					replay.Execution.Status != status {
					t.Fatalf(
						"terminal replay #%d = (%#v, %v)",
						replayNumber,
						replay,
						err,
					)
				}

				if replay.Execution.TerminalOutputs["preserved"] !=
					"output" {
					t.Fatalf(
						"terminal replay #%d outputs = %#v",
						replayNumber,
						replay.Execution.TerminalOutputs,
					)
				}

				if replay.Execution.Failure["message"] !=
					"preserved error" {
					t.Fatalf(
						"terminal replay #%d failure = %#v",
						replayNumber,
						replay.Execution.Failure,
					)
				}
			}

			afterCommands := countNodeCommandOutbox(
				t,
				pool,
				workflow.CompanyID,
				first.Execution.ID,
			)

			if afterCommands != beforeCommands {
				t.Fatalf(
					"terminal replay durable commands = %d, had %d",
					afterCommands,
					beforeCommands,
				)
			}
		})
	}
}
