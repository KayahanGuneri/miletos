//go:build integration

package execution_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	model "miletos-go/internal/features/workflow-runtime/execution/model"
	repository "miletos-go/internal/features/workflow-runtime/execution/repository"
)

const integrationOutboxDispatchTimeout = 2 * time.Second

func countNodeCommandOutbox(
	t *testing.T,
	pool *pgxpool.Pool,
	companyID string,
	executionID string,
) int {
	t.Helper()

	var count int

	if err := pool.QueryRow(
		context.Background(),
		`
SELECT COUNT(*)
FROM workflow_runtime.outbox_messages
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND operation_kind = 'NODE_COMMAND'`,
		companyID,
		executionID,
	).Scan(&count); err != nil {
		t.Fatalf(
			"count durable node commands for execution %s: %v",
			executionID,
			err,
		)
	}

	return count
}

func dispatchDueNodeCommands(
	t *testing.T,
	nodeQueue *recordingQueue,
	expectedNewJobs int,
) []model.NodeJob {
	t.Helper()

	if expectedNewJobs < 1 {
		t.Fatalf(
			"expected new job count must be positive, got %d",
			expectedNewJobs,
		)
	}

	client := engineDatabaseClient(t)
	outbox := repository.NewOutboxRepository(client)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		integrationOutboxDispatchTimeout,
	)
	defer cancel()

	owner := fmt.Sprintf(
		"integration_dispatcher_%d",
		time.Now().UTC().UnixNano(),
	)

	initialJobs := nodeQueue.snapshot()
	initialCount := len(initialJobs)
	targetCount := initialCount + expectedNewJobs

	for {
		currentJobs := nodeQueue.snapshot()

		if len(currentJobs) >= targetCount {
			newJobs := currentJobs[initialCount:]

			if len(newJobs) != expectedNewJobs {
				t.Fatalf(
					"dispatched job count = %d, want %d",
					len(newJobs),
					expectedNewJobs,
				)
			}

			return append(
				[]model.NodeJob(nil),
				newJobs...,
			)
		}

		remaining := targetCount - len(currentJobs)

		messages, err := outbox.ClaimDue(
			ctx,
			owner,
			remaining,
		)
		if err != nil {
			t.Fatalf(
				"claim due node-command outbox messages: %v",
				err,
			)
		}

		if len(messages) == 0 {
			select {
			case <-ctx.Done():
				t.Fatalf(
					"timed out waiting for %d due outbox message(s); "+
						"queue received %d",
					expectedNewJobs,
					len(nodeQueue.snapshot())-initialCount,
				)
			case <-time.After(time.Millisecond):
				continue
			}
		}

		for _, message := range messages {
			if err := nodeQueue.Push(
				ctx,
				message.Destination,
				message.MessageKey,
				message.Payload,
			); err != nil {
				releaseErr := outbox.Release(
					ctx,
					message,
				)

				if releaseErr != nil {
					t.Fatalf(
						"publish test outbox message: %v; "+
							"release claim: %v",
						err,
						releaseErr,
					)
				}

				t.Fatalf(
					"publish test outbox message: %v",
					err,
				)
			}

			if err := outbox.MarkPublished(
				ctx,
				message,
			); err != nil {
				t.Fatalf(
					"mark test outbox message published: %v",
					err,
				)
			}
		}
	}
}

func requireNodeJob(
	t *testing.T,
	jobs []model.NodeJob,
	nodeID string,
) model.NodeJob {
	t.Helper()

	for _, job := range jobs {
		if job.NodeID == nodeID {
			return job
		}
	}

	t.Fatalf(
		"node job %q was not found in %#v",
		nodeID,
		jobs,
	)

	return model.NodeJob{}
}
