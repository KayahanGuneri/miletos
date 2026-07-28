package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

const lockAsyncCoordinationSQL = `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`

func lockAsyncCoordination(ctx context.Context, tx pgx.Tx, key string) error {
	if ctx == nil || tx == nil || key == "" {
		return fmt.Errorf("invalid async coordination lock")
	}
	if _, err := tx.Exec(ctx, lockAsyncCoordinationSQL, key); err != nil {
		return mapPostgreSQLError("lock", "async coordination", err)
	}
	return nil
}
func (transaction *asyncPersistenceTransaction) LockAsyncWorkflowCoordination(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) error {
	return lockAsyncCoordination(ctx, transaction.tx, "workflow|"+companyID.String()+"|"+workflowExecutionID.String())
}
func (transaction *asyncPersistenceTransaction) LockAsyncNodeCoordination(ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeID workflow.NodeID) error {
	return lockAsyncCoordination(ctx, transaction.tx, "node|"+companyID.String()+"|"+workflowExecutionID.String()+"|"+nodeID.String())
}
func (transaction *asyncPersistenceTransaction) GetAsyncNodeExecution(ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID) (repository.NodeExecutionRecord, error) {
	row := transaction.tx.QueryRow(ctx,
		`SELECT `+nodeExecutionSelectColumns+` FROM workflow_runtime.node_executions WHERE company_id=$1 AND workflow_execution_id=$2 AND node_execution_id=$3`, companyID.String(),
		workflowExecutionID.String(), nodeExecutionID.String())
	record, err := scanNodeExecutionRecord(row)
	if err != nil {
		return repository.NodeExecutionRecord{}, mapPostgreSQLError("get", "async node execution", err)
	}
	return record, nil
}
func (transaction *asyncPersistenceTransaction) GetAsyncWorkflowExecution(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) (repository.WorkflowExecutionRecord, error) {
	row := transaction.tx.QueryRow(ctx, `SELECT `+workflowExecutionSelectColumns+` FROM workflow_runtime.workflow_executions WHERE company_id=$1 AND workflow_execution_id=$2`,
		companyID.String(), workflowExecutionID.String())
	record, err := scanWorkflowExecutionRecord(row)
	if err != nil {
		return repository.WorkflowExecutionRecord{}, mapPostgreSQLError("get", "async workflow execution", err)
	}
	return record, nil
}
func (transaction *asyncPersistenceTransaction) ListAsyncNodeExecutions(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) ([]repository.NodeExecutionRecord, error) {
	rows, err := transaction.tx.Query(ctx,
		`SELECT `+nodeExecutionSelectColumns+` FROM workflow_runtime.node_executions WHERE company_id=$1 AND workflow_execution_id=$2 ORDER BY node_id, node_execution_id`,
		companyID.String(), workflowExecutionID.String())
	if err != nil {
		return nil, mapPostgreSQLError("list", "async node executions", err)
	}
	defer rows.Close()
	records := make([]repository.NodeExecutionRecord, 0)
	for rows.Next() {
		record, err := scanNodeExecutionRecord(rows)
		if err != nil {
			return nil, mapPostgreSQLError("hydrate", "async node execution", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgreSQLError("list", "async node executions", err)
	}
	return records, nil
}

const skipAsyncNodeExecutionSQL = `
UPDATE workflow_runtime.node_executions
SET status='SKIPPED', finished_at=$5, updated_at=$5, lock_version=lock_version+1
WHERE company_id=$1 AND workflow_execution_id=$2 AND node_execution_id=$3
  AND status='PENDING' AND lock_version=$4`

func (transaction *asyncPersistenceTransaction) SkipAsyncNodeExecution(ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID, expectedLockVersion int64, finishedAt time.Time) error {
	workflowRecord, err := lockAsyncWorkflowTimelineRecord(ctx, transaction.tx, companyID, workflowExecutionID)
	if err != nil {
		return err
	}
	nodeRecord, err := getAsyncNodeTimelineRecord(ctx, transaction.tx, companyID, workflowExecutionID, nodeExecutionID)
	if err != nil {
		return err
	}
	timeline, err := buildAsyncNodeTransitionTimeline(workflowRecord, nodeRecord, execution.NodeExecutionStatusSkipped, finishedAt, "", "")
	if err != nil {
		return err
	}
	tag, err := transaction.tx.Exec(ctx, skipAsyncNodeExecutionSQL, companyID.String(), workflowExecutionID.String(), nodeExecutionID.String(), expectedLockVersion, finishedAt)
	if err != nil {
		return mapPostgreSQLError("skip", "async node execution", err)
	}
	if tag.RowsAffected() != 1 {
		return repository.NewStaleWriteError("skip", "async node execution", errors.New("node state no longer matches pending version"))
	}
	return persistAsyncNodeTransitionTimeline(ctx, transaction.tx, workflowRecord, timeline)
}

const completeAsyncWorkflowSQL = `
UPDATE workflow_runtime.workflow_executions
SET status=$4, finished_at=$5, updated_at=$5, next_sequence_number=$7, lock_version=lock_version+1
WHERE company_id=$1 AND workflow_execution_id=$2
  AND status='RUNNING' AND lock_version=$3 AND next_sequence_number=$6`

func (transaction *asyncPersistenceTransaction) CompleteAsyncWorkflow(ctx context.Context, completion repository.AsyncWorkflowCompletion) error {
	if !completion.IsValid() {
		return fmt.Errorf("async workflow completion must be valid")
	}
	workflowRecord, err := lockAsyncWorkflowTimelineRecord(ctx, transaction.tx, completion.CompanyID, completion.WorkflowExecutionID)
	if err != nil {
		return err
	}
	timeline, err := buildAsyncWorkflowTransitionTimeline(workflowRecord, completion.Status, completion.FinishedAt)
	if err != nil {
		return err
	}
	nextSequence, err := nextSequenceAfterTimeline(workflowRecord.NextSequenceNumber(), len(timeline))
	if err != nil {
		return err
	}
	tag, err := transaction.tx.Exec(ctx, completeAsyncWorkflowSQL,
		completion.CompanyID.String(), completion.WorkflowExecutionID.String(), completion.ExpectedLockVersion,
		completion.Status.String(), completion.FinishedAt, workflowRecord.NextSequenceNumber().Int64(),
		nextSequence.Int64())
	if err != nil {
		return mapPostgreSQLError("complete", "async workflow execution", err)
	}
	if tag.RowsAffected() != 1 {
		return repository.NewStaleWriteError("complete", "async workflow execution", errors.New("workflow state no longer matches running version"))
	}
	actualNext, err := insertTimelineEntriesTransaction(ctx, transaction.tx, workflowRecord.NextSequenceNumber(), timeline)
	if err != nil {
		return err
	}
	if actualNext != nextSequence {
		return fmt.Errorf("async workflow transition timeline sequence allocation is inconsistent")
	}
	return nil
}

var _ repository.AsyncNodeExecutionLocker = (*Store)(nil)

const acquireAsyncExecutionLockSQL = `SELECT pg_advisory_lock(hashtextextended($1, 0))`
const tryAcquireAsyncExecutionLockSQL = `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`
const releaseAsyncExecutionLockSQL = `SELECT pg_advisory_unlock(hashtextextended($1, 0))`

func asyncNodeExecutionLockKey(lock repository.AsyncNodeExecutionLock) string {
	return fmt.Sprintf(
		"async-node|%s|%s|%s|%d",
		lock.CompanyID,
		lock.WorkflowExecutionID,
		lock.NodeExecutionID,
		lock.Attempt,
	)
}

func (store *Store) WithAsyncNodeExecutionLock(ctx context.Context, lock repository.AsyncNodeExecutionLock, work repository.AsyncNodeExecutionLockWork) error {
	_, err := store.withAsyncNodeExecutionSessionLock(
		ctx, lock, true, work)
	return err
}

func (store *Store) withAsyncNodeExecutionSessionLock(
	ctx context.Context,
	lock repository.AsyncNodeExecutionLock,
	blocking bool,
	work func(context.Context) error,
) (bool, error) {
	if !store.IsValid() || ctx == nil || !lock.IsValid() || work == nil {
		return false, fmt.Errorf("async node execution lock dependencies must be valid")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	connection, err := store.pool.Acquire(ctx)
	if err != nil {
		return false, mapPostgreSQLError(
			"acquire", "async node execution session", err)
	}
	defer connection.Release()
	key := asyncNodeExecutionLockKey(lock)
	acquired := false
	if blocking {
		if _, err := connection.Exec(
			ctx, acquireAsyncExecutionLockSQL, key); err != nil {
			return false, mapPostgreSQLError(
				"lock", "async node execution", err)
		}
		acquired = true
	} else if err := connection.QueryRow(
		ctx, tryAcquireAsyncExecutionLockSQL, key).Scan(&acquired); err != nil {
		return false, mapPostgreSQLError(
			"lock", "async node execution", err)
	}
	if !acquired {
		return false, nil
	}
	unlocked := false
	defer func() {
		if unlocked {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = connection.Exec(releaseCtx, releaseAsyncExecutionLockSQL, key)
	}()
	workErr := work(ctx)
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var released bool
	if err := connection.QueryRow(releaseCtx, releaseAsyncExecutionLockSQL, key).Scan(&released); err != nil {
		return true, fmt.Errorf(
			"release async node execution lock: %w",
			mapPostgreSQLError("unlock", "async node execution", err))
	}
	unlocked = true
	if !released {
		return true, fmt.Errorf(
			"async node execution lock was not owned by acquired session")
	}
	return true, workErr
}

var _ repository.AsyncInputStore = (*Store)(nil)

const asyncInputColumns = `async_input_id, company_id, workflow_execution_id, target_node_execution_id, source_node_execution_id, target_node_id, source_node_id, edge_id, source_output_port, target_input_port, source_attempt, payload_json, created_at`
const insertAsyncInputSQL = `INSERT INTO workflow_runtime.async_node_inputs (` + asyncInputColumns + `)
SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13
FROM workflow_runtime.node_executions AS target
JOIN workflow_runtime.node_executions AS source
  ON source.workflow_execution_id=$3 AND source.company_id=$2
WHERE target.node_execution_id=$4
  AND target.workflow_execution_id=$3
  AND target.company_id=$2
  AND target.node_id=$6
  AND source.node_execution_id=$5
  AND source.node_id=$7`

const scheduleAsyncNodeExecutionSQL = `
UPDATE workflow_runtime.node_executions
SET
	status = 'QUEUED',
	ready_at = $6,
	queued_at = $6,
	updated_at = $6,
	lock_version = lock_version + 1
WHERE company_id = $1
  AND workflow_execution_id = $2
  AND node_execution_id = $3
  AND node_id = $4
  AND attempt = $5
  AND status = 'PENDING'
  AND lock_version = $7
`

func scheduleAsyncNodeExecution(ctx context.Context,
	tx pgx.Tx, schedule repository.AsyncNodeSchedule) error {
	if ctx == nil || tx == nil || !schedule.IsValid() {
		return fmt.Errorf("invalid async node schedule operation")
	}
	workflowRecord, err := lockAsyncWorkflowTimelineRecord(ctx,
		tx, schedule.CompanyID, schedule.WorkflowExecutionID,
	)
	if err != nil {
		return err
	}
	nodeRecord, err := getAsyncNodeTimelineRecord(
		ctx, tx, schedule.CompanyID,
		schedule.WorkflowExecutionID, schedule.NodeExecutionID)
	if err != nil {
		return err
	}
	timeline, err := buildAsyncNodeTransitionTimeline(workflowRecord,
		nodeRecord, execution.NodeExecutionStatusQueued, schedule.QueuedAt,
		"", "")
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		scheduleAsyncNodeExecutionSQL, schedule.CompanyID.String(), schedule.WorkflowExecutionID.String(),
		schedule.NodeExecutionID.String(), schedule.NodeID.String(), schedule.Attempt,
		schedule.QueuedAt, schedule.ExpectedLockVersion)
	if err != nil {
		return mapPostgreSQLError("schedule", "async node execution", err)
	}
	if tag.RowsAffected() != 1 {
		return repository.NewStaleWriteError("schedule",
			"async node execution", errors.New("node is no longer pending at the expected version"))
	}
	if err := persistAsyncNodeTransitionTimeline(ctx, tx, workflowRecord, timeline); err != nil {
		return err
	}
	if err := createOutboxMessage(ctx, tx, schedule.OutboxMessage); err != nil {
		return err
	}
	return nil
}
