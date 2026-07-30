package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/features/workflowruntime/execution/model"
	workflowfeature "miletos-go/internal/features/workflowruntime/workflow"
	"miletos-go/internal/shared/database"
)

type ExecutionRepository struct {
	dbClient *database.Client
}

func NewExecutionRepository(dbClient *database.Client) *ExecutionRepository {
	return &ExecutionRepository{dbClient: dbClient}
}

func (repository *ExecutionRepository) FindIdempotent(
	ctx context.Context,
	companyID string,
	key string,
	fingerprint string,
) (model.Execution, bool, error) {
	if key == "" {
		return model.Execution{}, false, nil
	}
	var storedFingerprint, executionID string
	err := repository.dbClient.QueryRow(ctx, `
		SELECT request_fingerprint, workflow_execution_id
		FROM workflow_runtime.http_idempotency_keys
		WHERE company_id = $1 AND idempotency_key = $2`,
		companyID, key,
	).Scan(&storedFingerprint, &executionID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Execution{}, false, nil
	}
	if err != nil {
		return model.Execution{}, false, fmt.Errorf("find idempotent execution: %w", err)
	}
	if storedFingerprint != fingerprint {
		return model.Execution{}, false, ErrIdempotencyConflict
	}
	execution, err := repository.FindByID(ctx, companyID, executionID)
	return execution, true, err
}

func (repository *ExecutionRepository) Create(
	ctx context.Context,
	workflow workflowfeature.Workflow,
	snapshotID string,
	mode string,
	origin string,
	correlationID string,
	idempotencyKey string,
	fingerprint string,
) (model.Execution, error) {
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return model.Execution{}, fmt.Errorf("begin execution creation: %w", err)
	}
	defer transaction.Rollback(ctx)

	now := time.Now().UTC()
	execution := model.Execution{
		ID:               newID("exec_"),
		CompanyID:        workflow.CompanyID,
		WorkflowID:       workflow.ID,
		WorkflowRevision: workflow.Revision,
		SnapshotID:       snapshotID,
		Mode:             mode,
		Origin:           origin,
		CorrelationID:    correlationID,
		Status:           model.ExecutionCreated,
		CreatedAt:        now,
		UpdatedAt:        now,
		TerminalOutputs:  map[string]any{},
	}
	_, err = transaction.Exec(ctx, `
		INSERT INTO workflow_runtime.workflow_executions (
			workflow_execution_id, company_id, workflow_id, workflow_revision,
			snapshot_id, mode, execution_origin, correlation_id, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, $10, $10)`,
		execution.ID, workflow.CompanyID, workflow.ID, workflow.Revision,
		snapshotID, mode, origin, correlationID, execution.Status, now,
	)
	if err != nil {
		return model.Execution{}, fmt.Errorf("create execution: %w", err)
	}
	for _, node := range workflow.Nodes {
		version := node.Version
		if version == "" {
			version = "v1"
		}
		_, err = transaction.Exec(ctx, `
			INSERT INTO workflow_runtime.node_executions (
				node_execution_id, workflow_execution_id, company_id, node_id,
				plugin_type, plugin_version, status, attempt, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, 'PENDING', 1, $7, $7)`,
			newID("node_exec_"), execution.ID, workflow.CompanyID, node.ID,
			node.Type, version, now,
		)
		if err != nil {
			return model.Execution{}, fmt.Errorf("create node execution %s: %w", node.ID, err)
		}
	}
	if idempotencyKey != "" {
		_, err = transaction.Exec(ctx, `
			INSERT INTO workflow_runtime.http_idempotency_keys (
				company_id, idempotency_key, request_fingerprint,
				workflow_execution_id, state, created_at, accepted_at
			) VALUES ($1, $2, $3, $4, 'ACCEPTED', $5, $5)`,
			workflow.CompanyID, idempotencyKey, fingerprint, execution.ID, now,
		)
		if err != nil {
			return model.Execution{}, fmt.Errorf("save idempotency key: %w", err)
		}
	}
	if err := recordEvent(
		ctx, transaction, execution.ID, "", "WORKFLOW_CREATED", "",
		model.ExecutionCreated, "Workflow execution created", map[string]any{},
	); err != nil {
		return model.Execution{}, fmt.Errorf("record execution creation: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return model.Execution{}, fmt.Errorf("commit execution creation: %w", err)
	}
	return execution, nil
}

func (repository *ExecutionRepository) FindByID(
	ctx context.Context,
	companyID string,
	executionID string,
) (model.Execution, error) {
	execution, err := scanExecution(repository.dbClient.QueryRow(ctx, `
		SELECT workflow_execution_id, company_id, workflow_id, workflow_revision,
			snapshot_id, mode, execution_origin, COALESCE(correlation_id, ''), status,
			created_at, validating_at, queued_at, started_at, finished_at, updated_at,
			terminal_outputs, failure_summary, is_stalled
		FROM workflow_runtime.workflow_executions
		WHERE company_id = $1 AND workflow_execution_id = $2`,
		companyID, executionID,
	))
	return execution, mapNoRows(err)
}

func (repository *ExecutionRepository) List(
	ctx context.Context,
	companyID string,
	workflowID string,
	status string,
	after string,
	limit int,
) (model.Page[model.Execution], error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	query := `
		SELECT workflow_execution_id, company_id, workflow_id, workflow_revision,
			snapshot_id, mode, execution_origin, COALESCE(correlation_id, ''), status,
			created_at, validating_at, queued_at, started_at, finished_at, updated_at,
			terminal_outputs, failure_summary, is_stalled
		FROM workflow_runtime.workflow_executions
		WHERE company_id = $1`
	arguments := []any{companyID}
	if workflowID != "" {
		arguments = append(arguments, workflowID)
		query += fmt.Sprintf(" AND workflow_id = $%d", len(arguments))
	}
	if status != "" {
		arguments = append(arguments, status)
		query += fmt.Sprintf(" AND status = $%d", len(arguments))
	}
	if after != "" {
		arguments = append(arguments, after)
		query += fmt.Sprintf(` AND (created_at, workflow_execution_id) < (
			SELECT created_at, workflow_execution_id
			FROM workflow_runtime.workflow_executions
			WHERE company_id = $1 AND workflow_execution_id = $%d
		)`, len(arguments))
	}
	arguments = append(arguments, limit+1)
	query += fmt.Sprintf(" ORDER BY created_at DESC, workflow_execution_id DESC LIMIT $%d", len(arguments))

	rows, err := repository.dbClient.Query(ctx, query, arguments...)
	if err != nil {
		return model.Page[model.Execution]{}, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()
	items := make([]model.Execution, 0, limit+1)
	for rows.Next() {
		item, err := scanExecution(rows)
		if err != nil {
			return model.Page[model.Execution]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return model.Page[model.Execution]{}, err
	}
	page := model.Page[model.Execution]{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.HasNext = true
		page.Next = page.Items[len(page.Items)-1].ID
	}
	return page, nil
}

func (repository *ExecutionRepository) MarkExecutionQueued(
	ctx context.Context,
	companyID string,
	executionID string,
) error {
	now := time.Now().UTC()
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin execution queue transition: %w", err)
	}
	defer transaction.Rollback(ctx)
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.workflow_executions
		SET status = 'QUEUED', queued_at = COALESCE(queued_at, $3), updated_at = $3,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2 AND status = 'CREATED'`,
		companyID, executionID, now,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return nil
	}
	if err := recordEvent(
		ctx, transaction, executionID, "", "WORKFLOW_QUEUED",
		model.ExecutionCreated, model.ExecutionQueued, "Workflow queued", map[string]any{},
	); err != nil {
		return fmt.Errorf("record execution queued event: %w", err)
	}
	return transaction.Commit(ctx)
}

func (repository *ExecutionRepository) MarkExecutionRunning(
	ctx context.Context,
	companyID string,
	executionID string,
) error {
	now := time.Now().UTC()
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin execution start transition: %w", err)
	}
	defer transaction.Rollback(ctx)
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.workflow_executions
		SET status = 'RUNNING', started_at = COALESCE(started_at, $3), updated_at = $3,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND status IN ('CREATED', 'QUEUED')`,
		companyID, executionID, now,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return nil
	}
	if err := recordEvent(
		ctx, transaction, executionID, "", "WORKFLOW_STARTED",
		"", model.ExecutionRunning, "Workflow started", map[string]any{},
	); err != nil {
		return fmt.Errorf("record execution started event: %w", err)
	}
	return transaction.Commit(ctx)
}

func (repository *ExecutionRepository) Finalize(
	ctx context.Context,
	companyID string,
	executionID string,
	status string,
	outputs map[string]any,
	failure map[string]any,
) error {
	now := time.Now().UTC()
	encodedOutputs, err := encodeJSON(outputs)
	if err != nil {
		return err
	}
	var encodedFailure any
	if failure != nil {
		encodedFailure, err = encodeJSON(failure)
		if err != nil {
			return err
		}
	}
	transaction, err := repository.dbClient.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin execution finalization: %w", err)
	}
	defer transaction.Rollback(ctx)
	command, err := transaction.Exec(ctx, `
		UPDATE workflow_runtime.workflow_executions
		SET status = $3, terminal_outputs = $4, failure_summary = $5,
			finished_at = $6, updated_at = $6, is_stalled = FALSE,
			lock_version = lock_version + 1
		WHERE company_id = $1 AND workflow_execution_id = $2
		  AND status NOT IN ('SUCCEEDED', 'FAILED', 'CANCELLED', 'TIMED_OUT')`,
		companyID, executionID, status, encodedOutputs, encodedFailure, now,
	)
	if err != nil {
		return fmt.Errorf("finalize execution: %w", err)
	}
	if command.RowsAffected() == 0 {
		var currentStatus string
		if err := transaction.QueryRow(ctx, `
			SELECT status
			FROM workflow_runtime.workflow_executions
			WHERE company_id = $1 AND workflow_execution_id = $2`,
			companyID, executionID,
		).Scan(&currentStatus); err != nil {
			return fmt.Errorf("inspect final execution state: %w", mapNoRows(err))
		}
		if currentStatus == status {
			return nil
		}
		return fmt.Errorf(
			"%w: cannot finalize execution from status %s",
			ErrStateTransition,
			currentStatus,
		)
	}
	eventType, message := "WORKFLOW_SUCCEEDED", "Workflow succeeded"
	if status == model.ExecutionFailed {
		eventType, message = "WORKFLOW_FAILED", "Workflow failed"
	}
	if err := recordEvent(
		ctx, transaction, executionID, "", eventType,
		model.ExecutionRunning, status, message, map[string]any{},
	); err != nil {
		return fmt.Errorf("record workflow finalization event: %w", err)
	}
	if status == model.ExecutionFailed {
		if err := recordLog(
			ctx, transaction, executionID, "", "ERROR", "Workflow execution failed",
			map[string]any{"status": status},
		); err != nil {
			return fmt.Errorf("record workflow failure log: %w", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit execution finalization: %w", err)
	}
	return nil
}

func scanExecution(row rowScanner) (model.Execution, error) {
	var execution model.Execution
	var validatingAt, queuedAt, startedAt, finishedAt sql.NullTime
	var outputs, failure []byte
	err := row.Scan(
		&execution.ID, &execution.CompanyID, &execution.WorkflowID,
		&execution.WorkflowRevision, &execution.SnapshotID, &execution.Mode,
		&execution.Origin, &execution.CorrelationID, &execution.Status, &execution.CreatedAt,
		&validatingAt, &queuedAt, &startedAt, &finishedAt, &execution.UpdatedAt,
		&outputs, &failure, &execution.IsStalled,
	)
	if err != nil {
		return model.Execution{}, err
	}
	execution.ValidatingAt = optionalTime(validatingAt)
	execution.QueuedAt = optionalTime(queuedAt)
	execution.StartedAt = optionalTime(startedAt)
	execution.FinishedAt = optionalTime(finishedAt)
	execution.TerminalOutputs = decodeObject(outputs)
	execution.Failure = decodeObject(failure)
	return execution, nil
}
