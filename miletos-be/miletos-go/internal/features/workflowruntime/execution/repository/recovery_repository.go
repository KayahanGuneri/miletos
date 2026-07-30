package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflowruntime/execution/model"
	workflowfeature "miletos-go/internal/features/workflowruntime/workflow"
)

func (repository *ExecutionRepository) CopySuccessfulNodes(
	ctx context.Context,
	sourceExecutionID string,
	recoveryExecutionID string,
	nodeIDs []string,
) error {
	var companyID string
	if err := repository.dbClient.QueryRow(ctx, `
		SELECT source.company_id
		FROM workflow_runtime.workflow_executions source
		JOIN workflow_runtime.workflow_executions target
		  ON target.company_id = source.company_id
		WHERE source.workflow_execution_id = $1
		  AND target.workflow_execution_id = $2`,
		sourceExecutionID, recoveryExecutionID,
	).Scan(&companyID); err != nil {
		return mapNoRows(err)
	}
	return repository.CopySuccessfulNodesForCompany(
		ctx, companyID, sourceExecutionID, recoveryExecutionID, nodeIDs,
	)
}

func (repository *ExecutionRepository) CopySuccessfulNodesForCompany(
	ctx context.Context,
	companyID string,
	sourceExecutionID string,
	recoveryExecutionID string,
	nodeIDs []string,
) error {
	if len(nodeIDs) == 0 {
		return nil
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin preserved-node copy: %w", err)
	}
	defer transaction.Rollback(ctx)
	var sourceCount, targetCount int64
	if err := transaction.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (
				WHERE workflow_execution_id = $2 AND status = 'SUCCEEDED'
			),
			COUNT(*) FILTER (WHERE workflow_execution_id = $3)
		FROM workflow_runtime.node_executions
		WHERE company_id = $1
		  AND workflow_execution_id IN ($2, $3)
		  AND node_id = ANY($4)`,
		companyID, sourceExecutionID, recoveryExecutionID, nodeIDs,
	).Scan(&sourceCount, &targetCount); err != nil {
		return fmt.Errorf("verify preserved nodes: %w", err)
	}
	expectedCount := int64(len(nodeIDs))
	if sourceCount != expectedCount || targetCount != expectedCount {
		return fmt.Errorf("%w: preserved node set is incomplete", ErrStateTransition)
	}
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.node_executions target
		SET status = 'SUCCEEDED',
			output_summary = source.output_summary,
			failure_summary = NULL,
			started_at = target.created_at,
			finished_at = target.created_at,
			updated_at = target.created_at,
			next_attempt_at = NULL,
			lock_version = CASE
				WHEN target.status = 'SUCCEEDED' THEN target.lock_version
				ELSE target.lock_version + 1
			END
		FROM workflow_runtime.node_executions source
		WHERE target.company_id = $1
		  AND target.workflow_execution_id = $2
		  AND target.node_id = ANY($4)
		  AND target.status IN ('PENDING', 'SUCCEEDED')
		  AND source.company_id = $1
		  AND source.workflow_execution_id = $3
		  AND source.node_id = target.node_id
		  AND source.status = 'SUCCEEDED'`,
		companyID, recoveryExecutionID, sourceExecutionID, nodeIDs,
	)
	if err != nil {
		return fmt.Errorf("copy preserved nodes: %w", err)
	}
	if command.RowsAffected() != expectedCount {
		return fmt.Errorf("%w: preserved node copy was partial", ErrStateTransition)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit preserved-node copy: %w", err)
	}
	return nil
}

func (repository *ExecutionRepository) ReserveRecovery(
	ctx context.Context,
	workflow workflowfeature.Workflow,
	correlationID string,
	key string,
	fingerprint string,
	sourceExecutionID string,
	preserved int,
	scheduled int,
	reset int,
) (model.Execution, error) {
	definition, err := json.Marshal(workflow)
	if err != nil {
		return model.Execution{}, fmt.Errorf("encode recovery workflow: %w", err)
	}
	plan, err := encodeJSON(map[string]any{
		"preservedNodeCount": preserved,
		"scheduledNodeCount": scheduled,
		"resetNodeCount":     reset,
	})
	if err != nil {
		return model.Execution{}, err
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return model.Execution{}, fmt.Errorf("begin recovery reservation: %w", err)
	}
	defer transaction.Rollback(ctx)
	now := time.Now().UTC()
	snapshotID := newID("snapshot_")
	execution := model.Execution{
		ID: newID("exec_"), CompanyID: workflow.CompanyID, WorkflowID: workflow.ID,
		WorkflowRevision: workflow.Revision, SnapshotID: snapshotID, Mode: "ASYNC",
		CorrelationID: correlationID, Status: model.ExecutionCreated,
		CreatedAt: now, UpdatedAt: now, TerminalOutputs: map[string]any{},
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO workflow_runtime.workflow_definition_snapshots (
			snapshot_id, company_id, workflow_id, workflow_revision,
			workflow_name, definition_json, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		snapshotID, workflow.CompanyID, workflow.ID, workflow.Revision,
		workflow.Name, definition, now,
	); err != nil {
		return model.Execution{}, fmt.Errorf("save recovery snapshot: %w", err)
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO workflow_runtime.workflow_executions (
			workflow_execution_id, company_id, workflow_id, workflow_revision,
			snapshot_id, mode, correlation_id, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'ASYNC', NULLIF($6, ''), 'CREATED', $7, $7)`,
		execution.ID, workflow.CompanyID, workflow.ID, workflow.Revision,
		snapshotID, correlationID, now,
	); err != nil {
		return model.Execution{}, fmt.Errorf("create recovery execution: %w", err)
	}
	for _, node := range workflow.Nodes {
		version := node.Version
		if version == "" {
			version = "v1"
		}
		if _, err := transaction.Exec(ctx, `
			INSERT INTO workflow_runtime.node_executions (
				node_execution_id, workflow_execution_id, company_id, node_id,
				plugin_type, plugin_version, status, attempt, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', 1, $7, $7)`,
			newID("node_exec_"), execution.ID, workflow.CompanyID,
			node.ID, node.Type, version, now,
		); err != nil {
			return model.Execution{}, fmt.Errorf("create recovery node %s: %w", node.ID, err)
		}
	}
	if _, err := transaction.Exec(ctx, `
		INSERT INTO workflow_runtime.partial_recovery_requests (
			company_id, idempotency_key, request_fingerprint,
			source_workflow_execution_id, recovery_workflow_execution_id,
			recovery_plan, preserved_node_count, scheduled_node_count,
			reset_node_count, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		workflow.CompanyID, key, fingerprint, sourceExecutionID, execution.ID,
		plan, preserved, scheduled, reset, now,
	); err != nil {
		return model.Execution{}, mapRecoveryInsertError(err)
	}
	if err := recordEvent(
		ctx, transaction, execution.ID, "", "WORKFLOW_CREATED", "",
		model.ExecutionCreated, "Workflow execution created", map[string]any{},
	); err != nil {
		return model.Execution{}, fmt.Errorf("record recovery creation: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return model.Execution{}, fmt.Errorf("commit recovery reservation: %w", err)
	}
	return execution, nil
}

func (repository *ExecutionRepository) SaveRecoveryRequest(
	ctx context.Context,
	companyID string,
	key string,
	fingerprint string,
	sourceExecutionID string,
	recoveryExecutionID string,
	preserved int,
	scheduled int,
	reset int,
) error {
	plan, err := encodeJSON(map[string]any{
		"preservedNodeCount": preserved,
		"scheduledNodeCount": scheduled,
		"resetNodeCount":     reset,
	})
	if err != nil {
		return err
	}
	_, err = repository.dbClient.Exec(ctx, `
		INSERT INTO workflow_runtime.partial_recovery_requests (
			company_id, idempotency_key, request_fingerprint,
			source_workflow_execution_id, recovery_workflow_execution_id,
			recovery_plan, preserved_node_count, scheduled_node_count,
			reset_node_count, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		companyID, key, fingerprint, sourceExecutionID, recoveryExecutionID,
		plan, preserved, scheduled, reset, time.Now().UTC(),
	)
	return mapRecoveryInsertError(err)
}

func (repository *ExecutionRepository) FindRecoveryBySource(
	ctx context.Context,
	companyID string,
	sourceExecutionID string,
) (key string, recoveryID string, fingerprint string, found bool, err error) {
	err = repository.dbClient.QueryRow(ctx, `
		SELECT idempotency_key, recovery_workflow_execution_id, request_fingerprint
		FROM workflow_runtime.partial_recovery_requests
		WHERE company_id = $1 AND source_workflow_execution_id = $2`,
		companyID, sourceExecutionID,
	).Scan(&key, &recoveryID, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", false, nil
	}
	return key, recoveryID, fingerprint, err == nil, err
}

func mapRecoveryInsertError(err error) error {
	if err == nil {
		return nil
	}
	var postgresError interface{ SQLState() string }
	if errors.As(err, &postgresError) && postgresError.SQLState() == "23505" {
		switch {
		case strings.Contains(err.Error(), "partial_recovery_requests_pk"):
			return ErrIdempotencyConflict
		case strings.Contains(err.Error(), "partial_recovery_requests_source_uk"),
			strings.Contains(err.Error(), "partial_recovery_requests_recovery_uk"):
			return ErrRecoveryConflict
		}
	}
	return err
}

func (repository *ExecutionRepository) FindRecovery(
	ctx context.Context,
	companyID string,
	key string,
) (
	sourceID, recoveryID, fingerprint string,
	preserved, scheduled, reset int,
	createdAt time.Time,
	found bool,
	err error,
) {
	err = repository.dbClient.QueryRow(ctx, `
		SELECT source_workflow_execution_id, recovery_workflow_execution_id,
			request_fingerprint, preserved_node_count, scheduled_node_count,
			reset_node_count, created_at
		FROM workflow_runtime.partial_recovery_requests
		WHERE company_id = $1 AND idempotency_key = $2`,
		companyID, key,
	).Scan(&sourceID, &recoveryID, &fingerprint, &preserved, &scheduled, &reset, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", 0, 0, 0, time.Time{}, false, nil
	}
	return sourceID, recoveryID, fingerprint, preserved, scheduled, reset, createdAt, err == nil, err
}
