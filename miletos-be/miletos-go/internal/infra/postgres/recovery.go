package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

var _ repository.PartialRecoveryStore = (*Store)(nil)

const selectPartialRecoverySQL = `
SELECT
    company_id,
    idempotency_key,
    request_fingerprint,
    source_workflow_execution_id,
    recovery_workflow_execution_id,
    preserved_node_count,
    scheduled_node_count,
    reset_node_count,
    created_at
FROM workflow_runtime.partial_recovery_requests
WHERE company_id=$1 AND idempotency_key=$2
`

const insertPartialRecoverySQL = `
INSERT INTO workflow_runtime.partial_recovery_requests (
    company_id,
    idempotency_key,
    request_fingerprint,
    source_workflow_execution_id,
    recovery_workflow_execution_id,
    recovery_plan,
    preserved_node_count,
    scheduled_node_count,
    reset_node_count,
    created_at
)
VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10)
`

func (store *Store) CreatePartialRecovery(
	ctx context.Context,
	command repository.PartialRecoveryCommand,
) (repository.PartialRecoveryRecord, bool, error) {
	if !store.IsValid() {
		return repository.PartialRecoveryRecord{}, false, fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.PartialRecoveryRecord{}, false, fmt.Errorf("partial recovery context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.PartialRecoveryRecord{}, false, err
	}
	if !command.IsValid() {
		return repository.PartialRecoveryRecord{}, false, fmt.Errorf("partial recovery command must be valid")
	}

	var result repository.PartialRecoveryRecord
	created := false
	err := store.withinTransaction(ctx, "create partial recovery", func(tx pgx.Tx) error {
		lockKey := "partial-recovery|" + command.CompanyID().String() + "|" + command.IdempotencyKey()
		if err := lockAsyncCoordination(ctx, tx, lockKey); err != nil {
			return err
		}
		existing, found, err := getPartialRecovery(
			ctx, tx, command.CompanyID().String(), command.IdempotencyKey())
		if err != nil {
			return err
		}
		if found {
			result = existing
			return nil
		}

		source, err := lockAsyncWorkflowTimelineRecord(
			ctx, tx, command.CompanyID(), command.SourceWorkflowExecutionID())
		if err != nil {
			return err
		}
		if source.Status() != command.ExpectedSourceStatus() ||
			source.LockVersion() != command.ExpectedSourceLockVersion() {
			return repository.NewStaleWriteError(
				"recover", "source workflow execution",
				errors.New("source status or lock version changed"))
		}

		timeline := command.Timeline()
		nextSequence, err := nextSequenceAfterTimeline(
			command.RecoveryWorkflowExecution().NextSequenceNumber(), len(timeline))
		if err != nil {
			return err
		}
		if err := insertWorkflowExecutionTransaction(
			ctx, tx, command.RecoveryWorkflowExecution(), nextSequence); err != nil {
			return err
		}
		for _, plan := range command.NodePlans() {
			if err := insertNodeExecutionTransaction(ctx, tx, plan.NodeExecution()); err != nil {
				return err
			}
		}
		for _, variable := range command.ContextVariables() {
			write, err := repository.NewAsyncContextWrite(variable, 0)
			if err != nil {
				return err
			}
			if err := compareAndSwapAsyncContextVariable(ctx, tx, write); err != nil {
				return err
			}
		}
		for _, input := range command.NodeInputs() {
			if err := createAsyncNodeInput(ctx, tx, input); err != nil {
				return err
			}
		}
		for _, message := range command.OutboxMessages() {
			if err := createOutboxMessage(ctx, tx, message); err != nil {
				return err
			}
		}
		actualNext, err := insertTimelineEntriesTransaction(
			ctx, tx, command.RecoveryWorkflowExecution().NextSequenceNumber(), timeline)
		if err != nil {
			return err
		}
		if actualNext != nextSequence {
			return fmt.Errorf("partial recovery timeline sequence allocation is inconsistent")
		}

		preserved, scheduled, reset := countRecoveryNodePlans(command.NodePlans())
		result, err = repository.NewPartialRecoveryRecord(
			command.CompanyID(), command.IdempotencyKey(), command.RequestFingerprint(),
			command.SourceWorkflowExecutionID(), command.RecoveryWorkflowExecution().ID(),
			preserved, scheduled, reset, command.CreatedAt())
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			ctx, insertPartialRecoverySQL,
			result.CompanyID().String(), result.IdempotencyKey(), result.RequestFingerprint(),
			result.SourceWorkflowExecutionID().String(),
			result.RecoveryWorkflowExecutionID().String(),
			command.RecoveryPlan().String(),
			result.PreservedNodeCount(), result.ScheduledNodeCount(), result.ResetNodeCount(),
			result.CreatedAt(),
		); err != nil {
			return mapPostgreSQLError("create", "partial recovery request", err)
		}
		created = true
		return nil
	})
	if err != nil {
		return repository.PartialRecoveryRecord{}, false, err
	}
	return result, created, nil
}

func getPartialRecovery(
	ctx context.Context,
	tx pgx.Tx,
	companyID string,
	idempotencyKey string,
) (repository.PartialRecoveryRecord, bool, error) {
	var persistedCompanyID, persistedKey, fingerprint string
	var sourceExecutionID, recoveryExecutionID string
	var preserved, scheduled, reset int
	var createdAt time.Time
	err := tx.QueryRow(ctx, selectPartialRecoverySQL, companyID, idempotencyKey).Scan(
		&persistedCompanyID, &persistedKey, &fingerprint,
		&sourceExecutionID, &recoveryExecutionID,
		&preserved, &scheduled, &reset, &createdAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.PartialRecoveryRecord{}, false, nil
	}
	if err != nil {
		return repository.PartialRecoveryRecord{}, false,
			mapPostgreSQLError("get", "partial recovery request", err)
	}
	record, err := repository.NewPartialRecoveryRecord(
		workflow.CompanyID(persistedCompanyID), persistedKey, fingerprint,
		execution.WorkflowExecutionID(sourceExecutionID),
		execution.WorkflowExecutionID(recoveryExecutionID),
		preserved, scheduled, reset, createdAt,
	)
	if err != nil {
		return repository.PartialRecoveryRecord{}, false,
			&databaseError{operation: "hydrate", resource: "partial recovery request", cause: err}
	}
	return record, true, nil
}

func countRecoveryNodePlans(plans []repository.RecoveryNodePlan) (int, int, int) {
	var preserved, scheduled, reset int
	for _, plan := range plans {
		switch plan.Disposition() {
		case repository.RecoveryNodePreserved:
			preserved++
		case repository.RecoveryNodeScheduled:
			scheduled++
		case repository.RecoveryNodeReset:
			reset++
		}
	}
	return preserved, scheduled, reset
}
