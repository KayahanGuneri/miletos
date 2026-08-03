//go:build integration

package execution_test

import (
	"context"
	"encoding/json"
	"testing"

	model "miletos-go/internal/features/workflow-runtime/execution/model"
	repository "miletos-go/internal/features/workflow-runtime/execution/repository"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
)

func TestSchedulerPersistsRootCommandWithoutDirectQueuePublicationIntegration(
	t *testing.T,
) {
	ctx := context.Background()
	pool := engineIntegrationDatabase(t)
	workflow := engineWorkflow()

	execution := seedEngineExecution(
		t,
		pool,
		workflow,
		"ASYNC",
	)

	client := engineDatabaseClient(t)
	workflows := workflowfeature.NewWorkflowRepository(client)
	executions := repository.NewExecutionRepository(client)

	nodeQueue := &recordingQueue{}

	const commandTopic = "miletos.workflow.node.commands.v1"

	scheduler := newEngineScheduler(
		t,
		workflows,
		executions,
		nodeQueue,
		commandTopic,
	)

	scheduled, err := scheduler.Activate(
		ctx,
		execution,
		nil,
	)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	if scheduled != 1 {
		t.Fatalf(
			"Activate() scheduled = %d, want 1",
			scheduled,
		)
	}

	directJobs := nodeQueue.snapshot()

	if len(directJobs) != 0 {
		t.Fatalf(
			"direct queue publication count = %d, want 0",
			len(directJobs),
		)
	}

	var (
		nodeStatus       string
		inputSummary     []byte
		publicationState string
		destination      string
		encodedJob       []byte
	)

	err = pool.QueryRow(ctx, `
SELECT
node.status,
node.input_summary,
outbox.publication_state,
outbox.destination,
outbox.encoded_payload
FROM workflow_runtime.node_executions AS node
JOIN workflow_runtime.outbox_messages AS outbox
  ON outbox.company_id = node.company_id
 AND outbox.workflow_execution_id =
     node.workflow_execution_id
 AND outbox.node_execution_id =
     node.node_execution_id
 AND outbox.operation_kind = 'NODE_COMMAND'
WHERE node.company_id = $1
  AND node.workflow_execution_id = $2
  AND node.node_id = 'root'`,
		execution.CompanyID,
		execution.ID,
	).Scan(
		&nodeStatus,
		&inputSummary,
		&publicationState,
		&destination,
		&encodedJob,
	)
	if err != nil {
		t.Fatalf(
			"load scheduled root and outbox: %v",
			err,
		)
	}

	if nodeStatus != string(model.NodeQueued) {
		t.Fatalf(
			"root node status = %q, want %q",
			nodeStatus,
			model.NodeQueued,
		)
	}

	var outboxCount int

	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM workflow_runtime.outbox_messages
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND node_execution_id = (
  SELECT node_execution_id
  FROM workflow_runtime.node_executions
  WHERE company_id = $1
    AND workflow_execution_id = $2
    AND node_id = 'root'
  )
  AND operation_kind = 'NODE_COMMAND'`,
		execution.CompanyID,
		execution.ID,
	).Scan(&outboxCount); err != nil {
		t.Fatalf(
			"count scheduled root outbox commands: %v",
			err,
		)
	}

	if outboxCount != 1 {
		t.Fatalf(
			"root outbox count = %d, want 1",
			outboxCount,
		)
	}

	if publicationState != "PENDING" {
		t.Fatalf(
			"publication state = %q, want PENDING",
			publicationState,
		)
	}

	if destination != commandTopic {
		t.Fatalf(
			"destination = %q, want %q",
			destination,
			commandTopic,
		)
	}

	var persistedJob model.NodeJob

	if err := json.Unmarshal(
		encodedJob,
		&persistedJob,
	); err != nil {
		t.Fatalf(
			"decode persisted root command: %v",
			err,
		)
	}

	if persistedJob.CompanyID != execution.CompanyID ||
		persistedJob.WorkflowID != workflow.ID ||
		persistedJob.ExecutionID != execution.ID ||
		persistedJob.NodeID != "root" ||
		persistedJob.NodeExecutionID == "" ||
		persistedJob.Attempt != 1 ||
		persistedJob.CorrelationID != execution.CorrelationID ||
		persistedJob.Origin != execution.Origin {
		t.Fatalf(
			"persisted root job = %#v",
			persistedJob,
		)
	}

	var decodedInput map[string]any

	if err := json.Unmarshal(
		inputSummary,
		&decodedInput,
	); err != nil {
		t.Fatalf(
			"decode root input summary: %v",
			err,
		)
	}

	if decodedInput == nil {
		t.Fatal("root input summary is nil")
	}
}
