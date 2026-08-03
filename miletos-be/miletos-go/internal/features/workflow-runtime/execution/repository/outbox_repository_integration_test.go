//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	model "miletos-go/internal/features/workflow-runtime/execution/model"
)

func TestPersistNodeCommandCreatesIdempotentPendingOutboxMessageIntegration(
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

	client := integrationDatabaseClient(t)
	executions := NewExecutionRepository(client)

	node, err := executions.FindNode(
		ctx,
		workflow.CompanyID,
		execution.ID,
		"root",
	)
	if err != nil {
		t.Fatalf("FindNode() error = %v", err)
	}

	job := model.NodeJob{
		CompanyID:       workflow.CompanyID,
		WorkflowID:      workflow.ID,
		ExecutionID:     execution.ID,
		NodeID:          node.NodeID,
		NodeExecutionID: node.ID,
		Attempt:         node.Attempt,
		CorrelationID:   execution.CorrelationID,
		Origin:          model.ExecutionOriginManualDirect,
		Payload: map[string]any{
			"message": "transactional outbox integration",
		},
	}

	operationKey := nodeCommandOperationKey(job)

	if !strings.HasPrefix(operationKey, "node-command:") {
		t.Fatalf(
			"operation key %q does not have node-command prefix",
			operationKey,
		)
	}

	if len(operationKey) > 96 {
		t.Fatalf(
			"operation key length = %d, maximum is 96",
			len(operationKey),
		)
	}

	availableAt := time.Now().
		UTC().
		Truncate(time.Microsecond).
		Add(2 * time.Second)

	persist := func() {
		t.Helper()

		transaction, beginErr := client.Begin(ctx)
		if beginErr != nil {
			t.Fatalf("begin transaction: %v", beginErr)
		}
		defer transaction.Rollback(ctx)

		if persistErr := persistNodeCommand(
			ctx,
			transaction,
			job,
			job.Attempt,
			operationKey,
			"miletos.workflow.node.commands.v1",
			availableAt,
		); persistErr != nil {
			t.Fatalf("persistNodeCommand() error = %v", persistErr)
		}

		if commitErr := transaction.Commit(ctx); commitErr != nil {
			t.Fatalf("commit transaction: %v", commitErr)
		}
	}

	persist()
	persist()

	var (
		nodeExecutionID    string
		nodeID             string
		attempt            int
		operationKind      string
		persistedKey       string
		messageType        string
		messageVersion     int
		destination        string
		messageKey         string
		encodedPayload     []byte
		publicationState   string
		persistedAvailable time.Time
		persistedCreatedAt time.Time
	)

	err = pool.QueryRow(ctx, `
SELECT
node_execution_id,
node_id,
attempt,
operation_kind,
operation_key,
message_type,
message_version,
destination,
message_key,
encoded_payload,
publication_state,
available_at,
created_at
FROM workflow_runtime.outbox_messages
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND operation_key = $3`,
		job.CompanyID,
		job.ExecutionID,
		operationKey,
	).Scan(
		&nodeExecutionID,
		&nodeID,
		&attempt,
		&operationKind,
		&persistedKey,
		&messageType,
		&messageVersion,
		&destination,
		&messageKey,
		&encodedPayload,
		&publicationState,
		&persistedAvailable,
		&persistedCreatedAt,
	)
	if err != nil {
		t.Fatalf("load persisted outbox command: %v", err)
	}

	if nodeExecutionID != job.NodeExecutionID {
		t.Fatalf(
			"node_execution_id = %q, want %q",
			nodeExecutionID,
			job.NodeExecutionID,
		)
	}

	if nodeID != job.NodeID {
		t.Fatalf("node_id = %q, want %q", nodeID, job.NodeID)
	}

	if attempt != job.Attempt {
		t.Fatalf("attempt = %d, want %d", attempt, job.Attempt)
	}

	if operationKind != "NODE_COMMAND" {
		t.Fatalf(
			"operation_kind = %q, want NODE_COMMAND",
			operationKind,
		)
	}

	if persistedKey != operationKey {
		t.Fatalf(
			"operation_key = %q, want %q",
			persistedKey,
			operationKey,
		)
	}

	if messageType != "NODE_COMMAND" {
		t.Fatalf(
			"message_type = %q, want NODE_COMMAND",
			messageType,
		)
	}

	if messageVersion != 1 {
		t.Fatalf("message_version = %d, want 1", messageVersion)
	}

	if destination != "miletos.workflow.node.commands.v1" {
		t.Fatalf("destination = %q", destination)
	}

	if messageKey != job.NodeExecutionID {
		t.Fatalf(
			"message_key = %q, want %q",
			messageKey,
			job.NodeExecutionID,
		)
	}

	if publicationState != "PENDING" {
		t.Fatalf(
			"publication_state = %q, want PENDING",
			publicationState,
		)
	}

	if !persistedAvailable.Equal(availableAt) {
		t.Fatalf(
			"available_at = %v, want %v",
			persistedAvailable,
			availableAt,
		)
	}

	if persistedCreatedAt.After(persistedAvailable) {
		t.Fatalf(
			"created_at %v is after available_at %v",
			persistedCreatedAt,
			persistedAvailable,
		)
	}

	var persistedJob model.NodeJob

	if err := json.Unmarshal(encodedPayload, &persistedJob); err != nil {
		t.Fatalf("decode persisted node job: %v", err)
	}

	if persistedJob.CompanyID != job.CompanyID ||
		persistedJob.WorkflowID != job.WorkflowID ||
		persistedJob.ExecutionID != job.ExecutionID ||
		persistedJob.NodeID != job.NodeID ||
		persistedJob.NodeExecutionID != job.NodeExecutionID ||
		persistedJob.Attempt != job.Attempt ||
		persistedJob.CorrelationID != job.CorrelationID ||
		persistedJob.Origin != job.Origin {
		t.Fatalf("persisted job = %#v, want %#v", persistedJob, job)
	}

	var count int

	if err := pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM workflow_runtime.outbox_messages
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND operation_key = $3`,
		job.CompanyID,
		job.ExecutionID,
		operationKey,
	).Scan(&count); err != nil {
		t.Fatalf("count persisted outbox commands: %v", err)
	}

	if count != 1 {
		t.Fatalf("outbox command count = %d, want 1", count)
	}
}
