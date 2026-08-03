//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	model "miletos-go/internal/features/workflow-runtime/execution/model"
)

func TestQueueNodeCommandPersistsStateAndOutboxAtomicallyIntegration(
	t *testing.T,
) {
	ctx := context.Background()
	pool := integrationDatabase(t)
	workflow := integrationWorkflow()

	_, execution := seedIntegrationExecution(
		t,
		pool,
		workflow,
		"ASYNC",
	)

	repository := NewExecutionRepository(
		integrationDatabaseClient(t),
	)

	const destination = "miletos.workflow.node.commands.v1"

	payload := map[string]any{
		"message": "atomic queue transition",
	}

	queued, changed, err := repository.QueueNodeCommand(
		ctx,
		execution,
		"root",
		payload,
		destination,
	)
	if err != nil {
		t.Fatalf("QueueNodeCommand() error = %v", err)
	}

	if !changed {
		t.Fatal("QueueNodeCommand() changed = false, want true")
	}

	if queued.Status != model.NodeQueued {
		t.Fatalf(
			"queued node status = %q, want %q",
			queued.Status,
			model.NodeQueued,
		)
	}

	if queued.Input["message"] != payload["message"] {
		t.Fatalf(
			"queued node input = %#v, want %#v",
			queued.Input,
			payload,
		)
	}

	var (
		nodeStatus       string
		encodedInput     []byte
		operationKey     string
		operationKind    string
		messageType      string
		messageVersion   int
		persistedTopic   string
		messageKey       string
		encodedJob       []byte
		publicationState string
		outboxAttempt    int
	)

	err = pool.QueryRow(ctx, `
SELECT
node.status,
node.input_summary,
outbox.operation_key,
outbox.operation_kind,
outbox.message_type,
outbox.message_version,
outbox.destination,
outbox.message_key,
outbox.encoded_payload,
outbox.publication_state,
outbox.attempt
FROM workflow_runtime.node_executions AS node
JOIN workflow_runtime.outbox_messages AS outbox
  ON outbox.company_id = node.company_id
 AND outbox.workflow_execution_id =
     node.workflow_execution_id
 AND outbox.node_execution_id =
     node.node_execution_id
WHERE node.company_id = $1
  AND node.workflow_execution_id = $2
  AND node.node_execution_id = $3
  AND outbox.operation_kind = 'NODE_COMMAND'`,
		execution.CompanyID,
		execution.ID,
		queued.ID,
	).Scan(
		&nodeStatus,
		&encodedInput,
		&operationKey,
		&operationKind,
		&messageType,
		&messageVersion,
		&persistedTopic,
		&messageKey,
		&encodedJob,
		&publicationState,
		&outboxAttempt,
	)
	if err != nil {
		t.Fatalf(
			"load atomic node/outbox state: %v",
			err,
		)
	}

	if nodeStatus != string(model.NodeQueued) {
		t.Fatalf(
			"persisted node status = %q, want %q",
			nodeStatus,
			model.NodeQueued,
		)
	}

	var persistedInput map[string]any

	if err := json.Unmarshal(
		encodedInput,
		&persistedInput,
	); err != nil {
		t.Fatalf("decode persisted input: %v", err)
	}

	if persistedInput["message"] != payload["message"] {
		t.Fatalf(
			"persisted input = %#v, want %#v",
			persistedInput,
			payload,
		)
	}

	if !strings.HasPrefix(
		operationKey,
		"node-command:",
	) {
		t.Fatalf(
			"operation key = %q",
			operationKey,
		)
	}

	if operationKind != "NODE_COMMAND" {
		t.Fatalf(
			"operation kind = %q",
			operationKind,
		)
	}

	if messageType != "NODE_COMMAND" {
		t.Fatalf(
			"message type = %q",
			messageType,
		)
	}

	if messageVersion != 1 {
		t.Fatalf(
			"message version = %d, want 1",
			messageVersion,
		)
	}

	if persistedTopic != destination {
		t.Fatalf(
			"destination = %q, want %q",
			persistedTopic,
			destination,
		)
	}

	if messageKey != queued.ID {
		t.Fatalf(
			"message key = %q, want %q",
			messageKey,
			queued.ID,
		)
	}

	if publicationState != "PENDING" {
		t.Fatalf(
			"publication state = %q, want PENDING",
			publicationState,
		)
	}

	if outboxAttempt != queued.Attempt {
		t.Fatalf(
			"outbox attempt = %d, want %d",
			outboxAttempt,
			queued.Attempt,
		)
	}

	var persistedJob model.NodeJob

	if err := json.Unmarshal(
		encodedJob,
		&persistedJob,
	); err != nil {
		t.Fatalf("decode persisted node job: %v", err)
	}

	if persistedJob.CompanyID != execution.CompanyID ||
		persistedJob.WorkflowID != execution.WorkflowID ||
		persistedJob.ExecutionID != execution.ID ||
		persistedJob.NodeID != queued.NodeID ||
		persistedJob.NodeExecutionID != queued.ID ||
		persistedJob.Attempt != queued.Attempt ||
		persistedJob.CorrelationID !=
			execution.CorrelationID ||
		persistedJob.Origin != execution.Origin {
		t.Fatalf(
			"persisted node job = %#v",
			persistedJob,
		)
	}

	replayed, changed, err := repository.QueueNodeCommand(
		ctx,
		execution,
		"root",
		payload,
		destination,
	)
	if err != nil {
		t.Fatalf(
			"replayed QueueNodeCommand() error = %v",
			err,
		)
	}

	if changed {
		t.Fatal(
			"replayed QueueNodeCommand() changed = true",
		)
	}

	if replayed.ID != "" {
		t.Fatalf(
			"replayed node = %#v, want zero value",
			replayed,
		)
	}

	var rootOutboxCount int

	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM workflow_runtime.outbox_messages
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND node_execution_id = $3
  AND operation_kind = 'NODE_COMMAND'`,
		execution.CompanyID,
		execution.ID,
		queued.ID,
	).Scan(&rootOutboxCount); err != nil {
		t.Fatalf("count root outbox rows: %v", err)
	}

	if rootOutboxCount != 1 {
		t.Fatalf(
			"root outbox count = %d, want 1",
			rootOutboxCount,
		)
	}

	terminalBefore, err := repository.FindNode(
		ctx,
		execution.CompanyID,
		execution.ID,
		"terminal",
	)
	if err != nil {
		t.Fatalf(
			"find terminal before rollback test: %v",
			err,
		)
	}

	_, changed, err = repository.QueueNodeCommand(
		ctx,
		execution,
		"terminal",
		map[string]any{
			"message": "must roll back",
		},
		"",
	)
	if err == nil {
		t.Fatal(
			"QueueNodeCommand() with blank destination " +
				"returned nil error",
		)
	}

	if changed {
		t.Fatal(
			"failed QueueNodeCommand() changed = true",
		)
	}

	terminalAfter, err := repository.FindNode(
		ctx,
		execution.CompanyID,
		execution.ID,
		"terminal",
	)
	if err != nil {
		t.Fatalf(
			"find terminal after rollback test: %v",
			err,
		)
	}

	if terminalAfter.Status != model.NodePending {
		t.Fatalf(
			"terminal status after failed transaction = %q, "+
				"want %q",
			terminalAfter.Status,
			model.NodePending,
		)
	}

	if terminalAfter.LockVersion !=
		terminalBefore.LockVersion {
		t.Fatalf(
			"terminal lock version changed from %d to %d",
			terminalBefore.LockVersion,
			terminalAfter.LockVersion,
		)
	}

	var terminalOutboxCount int

	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM workflow_runtime.outbox_messages
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND node_execution_id = $3`,
		execution.CompanyID,
		execution.ID,
		terminalAfter.ID,
	).Scan(&terminalOutboxCount); err != nil {
		t.Fatalf(
			"count terminal outbox rows: %v",
			err,
		)
	}

	if terminalOutboxCount != 0 {
		t.Fatalf(
			"terminal outbox count = %d, want 0",
			terminalOutboxCount,
		)
	}

	var terminalQueuedEventCount int

	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM workflow_runtime.execution_events
WHERE workflow_execution_id = $1
  AND node_execution_id = $2
  AND event_type = 'NODE_QUEUED'`,
		execution.ID,
		terminalAfter.ID,
	).Scan(&terminalQueuedEventCount); err != nil {
		t.Fatalf(
			"count terminal queued events: %v",
			err,
		)
	}

	if terminalQueuedEventCount != 0 {
		t.Fatalf(
			"terminal queued event count = %d, want 0",
			terminalQueuedEventCount,
		)
	}
}
