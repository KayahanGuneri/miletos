package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	repository "miletos-go/internal/ports/persistence"
)

const claimQueuedAsyncNodeSQL = `
UPDATE workflow_runtime.node_executions
SET status='RUNNING', started_at=$7, updated_at=$7, lock_version=lock_version+1
WHERE company_id=$1 AND workflow_execution_id=$2 AND node_execution_id=$3
  AND status=$4 AND attempt=$5 AND lock_version=$6
`

const claimRetryPendingAsyncNodeSQL = `
UPDATE workflow_runtime.node_executions
SET status='RUNNING',
    attempt=$6,
    next_attempt_at=NULL,
    failure_summary=NULL,
    updated_at=$8,
    lock_version=lock_version+1
WHERE company_id=$1 AND workflow_execution_id=$2 AND node_execution_id=$3
  AND status=$4
  AND attempt=$5
  AND $6=attempt+1
  AND next_attempt_at IS NOT NULL
  AND $8>=next_attempt_at
  AND lock_version=$7
`

func claimAsyncNodeExecution(ctx context.Context, tx pgx.Tx, claim repository.AsyncNodeClaim) (int64, error) {
	if ctx == nil || tx == nil || !claim.IsValid() {
		return 0, fmt.Errorf("invalid async node claim")
	}
	workflowRecord, err := lockAsyncWorkflowTimelineRecord(ctx, tx, claim.CompanyID, claim.WorkflowExecutionID)
	if err != nil {
		return 0, err
	}
	nodeRecord, err := getAsyncNodeTimelineRecord(ctx, tx, claim.CompanyID, claim.WorkflowExecutionID, claim.NodeExecutionID)
	if err != nil {
		return 0, err
	}
	if nodeRecord.Status() != claim.ExpectedNodeStatus ||
		nodeRecord.Attempt() != claim.ExpectedNodeAttempt ||
		nodeRecord.LockVersion() != claim.ExpectedLockVersion {
		return 0, repository.NewStaleWriteError(
			"claim", "async node execution",
			errors.New("node state, attempt, or lock version changed"),
		)
	}
	timeline, err := buildAsyncNodeTransitionTimeline(workflowRecord, nodeRecord, execution.NodeExecutionStatusRunning, claim.StartedAt, "", "")
	if err != nil {
		return 0, err
	}
	started := claim.StartedAt.UTC()
	var rowsAffected int64
	switch claim.ExpectedNodeStatus {
	case execution.NodeExecutionStatusQueued:
		tag, execErr := tx.Exec(
			ctx, claimQueuedAsyncNodeSQL,
			claim.CompanyID.String(),
			claim.WorkflowExecutionID.String(),
			claim.NodeExecutionID.String(),
			claim.ExpectedNodeStatus.String(),
			claim.ExpectedNodeAttempt,
			claim.ExpectedLockVersion,
			started,
		)
		if execErr != nil {
			return 0, mapPostgreSQLError(
				"claim", "async node execution", execErr,
			)
		}
		rowsAffected = tag.RowsAffected()
	case execution.NodeExecutionStatusRetryPending:
		tag, execErr := tx.Exec(
			ctx, claimRetryPendingAsyncNodeSQL,
			claim.CompanyID.String(),
			claim.WorkflowExecutionID.String(),
			claim.NodeExecutionID.String(),
			claim.ExpectedNodeStatus.String(),
			claim.ExpectedNodeAttempt,
			claim.CommandAttempt,
			claim.ExpectedLockVersion,
			started,
		)
		if execErr != nil {
			return 0, mapPostgreSQLError(
				"claim", "async node execution", execErr,
			)
		}
		rowsAffected = tag.RowsAffected()
	default:
		return 0, fmt.Errorf("unsupported async node claim state")
	}
	if rowsAffected != 1 {
		return 0, repository.NewStaleWriteError(
			"claim", "async node execution",
			errors.New("node state, attempt, retry eligibility, or lock version changed"),
		)
	}
	attempt, err := execution.NewAttemptNumber(claim.CommandAttempt)
	if err != nil {
		return 0, err
	}
	attemptRecord, err := repository.NewNodeExecutionAttemptRecord(
		repository.NodeExecutionAttemptRecordParams{
			CompanyID:           claim.CompanyID,
			WorkflowExecutionID: claim.WorkflowExecutionID,
			NodeExecutionID:     claim.NodeExecutionID,
			Attempt:             attempt,
			Status:              execution.NodeExecutionStatusRunning,
			StartedAt:           started,
			CreatedAt:           started,
			UpdatedAt:           started,
		},
	)
	if err != nil {
		return 0, err
	}
	attemptMutation, err := repository.NewNodeAttemptStartMutation(
		attemptRecord,
	)
	if err != nil {
		return 0, err
	}
	if err := applyNodeAttemptMutationTransaction(
		ctx, tx, attemptMutation,
	); err != nil {
		return 0, err
	}
	if err := persistAsyncNodeTransitionTimeline(ctx, tx, workflowRecord, timeline); err != nil {
		return 0, err
	}
	return claim.ExpectedLockVersion + 1, nil
}

const lockAsyncWorkerPublicationNodeSQL = `SELECT ` +
	nodeExecutionSelectColumns + `
FROM workflow_runtime.node_executions
WHERE company_id=$1
  AND workflow_execution_id=$2
  AND node_execution_id=$3
FOR UPDATE`

func publishAsyncWorkerResult(
	ctx context.Context,
	tx pgx.Tx,
	publication repository.AsyncWorkerResultPublication,
) error {
	if ctx == nil || tx == nil || !publication.IsValid() {
		return fmt.Errorf("invalid async worker result publication")
	}
	result := publication.DurableWorkerResult
	persisted, err := getDurableWorkerResult(
		ctx, tx, publication.CompanyID,
		publication.WorkflowExecutionID,
		publication.NodeExecutionID, result.Attempt(),
	)
	if err != nil {
		return err
	}
	equal, err := durableWorkerResultsEquivalent(persisted, result)
	if err != nil {
		return err
	}
	if !equal {
		return repository.NewConflictError(
			"publish", "durable worker result",
			errors.New("checkpoint differs from publication result"),
		)
	}
	nodeRecord, err := scanNodeExecutionRecord(
		tx.QueryRow(
			ctx, lockAsyncWorkerPublicationNodeSQL,
			publication.CompanyID.String(),
			publication.WorkflowExecutionID.String(),
			publication.NodeExecutionID.String(),
		),
	)
	if err != nil {
		return mapPostgreSQLError(
			"publish", "async worker result node", err,
		)
	}
	if nodeRecord.Status() != execution.NodeExecutionStatusRunning ||
		nodeRecord.Attempt() != result.Attempt() ||
		nodeRecord.LockVersion() != publication.ExpectedNodeLockVersion ||
		nodeRecord.NodeID() != result.NodeID() {
		return repository.NewStaleWriteError(
			"publish", "async worker result node",
			errors.New("node state, attempt, identity, or lock version changed"),
		)
	}
	if err := createOutboxMessage(
		ctx, tx, publication.NodeResultOutboxMessage,
	); err != nil {
		return err
	}
	return recordInboxMessage(
		ctx, tx, publication.CommandInboxMessage,
	)
}

const applyAsyncNodeResultSQL = `
UPDATE workflow_runtime.node_executions
SET status=$7,
    next_attempt_at=$8,
    finished_at=$9,
    updated_at=$10,
    output_summary=$11::jsonb,
    failure_summary=$12::jsonb,
    lock_version=lock_version+1
WHERE company_id=$1
  AND workflow_execution_id=$2
  AND node_execution_id=$3
  AND status=$4
  AND attempt=$5
  AND lock_version=$6
`

func applyAsyncNodeResult(
	ctx context.Context,
	tx pgx.Tx,
	application repository.AsyncNodeResultApplication,
) error {
	if ctx == nil || tx == nil || !application.IsValid() {
		return fmt.Errorf("invalid async node result application")
	}
	result := application.DurableWorkerResult
	persisted, err := getDurableWorkerResult(
		ctx, tx, application.CompanyID,
		application.WorkflowExecutionID,
		application.NodeExecutionID,
		application.ExpectedNodeAttempt,
	)
	if err != nil {
		return err
	}
	equal, err := durableWorkerResultsEquivalent(persisted, result)
	if err != nil {
		return err
	}
	if !equal {
		return repository.NewConflictError(
			"apply", "async node result",
			errors.New("durable checkpoint differs from result application"),
		)
	}
	workflowRecord, err := lockAsyncWorkflowTimelineRecord(
		ctx, tx, application.CompanyID,
		application.WorkflowExecutionID,
	)
	if err != nil {
		return err
	}
	nodeRecord, err := scanNodeExecutionRecord(
		tx.QueryRow(
			ctx, lockAsyncWorkerPublicationNodeSQL,
			application.CompanyID.String(),
			application.WorkflowExecutionID.String(),
			application.NodeExecutionID.String(),
		),
	)
	if err != nil {
		return mapPostgreSQLError(
			"apply", "async node result", err,
		)
	}
	if nodeRecord.Status() != execution.NodeExecutionStatusRunning ||
		nodeRecord.Attempt() != application.ExpectedNodeAttempt ||
		nodeRecord.LockVersion() != application.ExpectedNodeLockVersion ||
		nodeRecord.NodeID() != result.NodeID() {
		return repository.NewStaleWriteError(
			"apply", "async node result",
			errors.New("node state, attempt, identity, or lock version changed"),
		)
	}
	completion, exists := application.AttemptMutation.Completion()
	if !exists {
		return fmt.Errorf("async node result must contain attempt completion")
	}
	failure, hasFailure := durableWorkerFailure(result)
	failureCategory, failureCode := "", ""
	if hasFailure {
		failureCategory = failure.Category().String()
		failureCode = failure.Code()
	}
	timeline, err := buildAsyncNodeTransitionTimeline(
		workflowRecord, nodeRecord,
		application.TargetNodeStatus,
		application.AppliedAt,
		failureCategory, failureCode,
	)
	if err != nil {
		return err
	}
	var nextAttemptAt any
	if value, exists := completion.NextAttemptAt(); exists {
		nextAttemptAt = value
	}
	var finishedAt any
	if application.TargetNodeStatus.IsTerminal() {
		finishedAt = application.AppliedAt
	}
	tag, err := tx.Exec(
		ctx, applyAsyncNodeResultSQL,
		application.CompanyID.String(),
		application.WorkflowExecutionID.String(),
		application.NodeExecutionID.String(),
		execution.NodeExecutionStatusRunning.String(),
		application.ExpectedNodeAttempt,
		application.ExpectedNodeLockVersion,
		application.TargetNodeStatus.String(),
		nextAttemptAt,
		finishedAt,
		application.AppliedAt,
		optionalStringValue(completion.OutputSummary),
		optionalStringValue(completion.FailureSummary),
	)
	if err != nil {
		return mapPostgreSQLError(
			"apply", "async node result", err,
		)
	}
	if tag.RowsAffected() != 1 {
		return repository.NewStaleWriteError(
			"apply", "async node result",
			errors.New("node is no longer RUNNING at the expected attempt and version"),
		)
	}
	if err := applyNodeAttemptMutationTransaction(
		ctx, tx, application.AttemptMutation,
	); err != nil {
		return err
	}
	if err := persistAsyncNodeTransitionTimeline(
		ctx, tx, workflowRecord, timeline,
	); err != nil {
		return err
	}
	if hasFailure {
		event, exists := timeline[0].Event()
		if !exists {
			return fmt.Errorf("async node result timeline must start with an event")
		}
		executionError, err := asyncExecutionError(
			application, event.ID(), failure,
		)
		if err != nil {
			return err
		}
		if err := insertExecutionErrorsTransaction(
			ctx, tx, []repository.ExecutionErrorRecord{executionError},
		); err != nil {
			return err
		}
	}
	if application.TargetNodeStatus ==
		execution.NodeExecutionStatusRetryPending {
		if err := createOutboxMessage(
			ctx, tx, application.RetryCommandOutbox,
		); err != nil {
			return err
		}
	}
	return recordInboxMessage(
		ctx, tx, application.ResultInboxMessage,
	)
}

func asyncExecutionError(
	application repository.AsyncNodeResultApplication,
	relatedEventID repository.ExecutionEventID,
	failure runtime.RuntimeFailure,
) (repository.ExecutionErrorRecord, error) {
	details, err := json.Marshal(
		map[string]any{"details": failure.Details()},
	)
	if err != nil {
		return repository.ExecutionErrorRecord{},
			fmt.Errorf("marshal async execution error details: %w", err)
	}
	technicalDetail, _ :=
		application.DurableWorkerResult.TechnicalDetail()
	return repository.NewExecutionErrorRecord(
		repository.ExecutionErrorRecordParams{
			ID: repository.ExecutionErrorID(
				relatedEventID.String() + "/error",
			),
			WorkflowExecutionID: application.WorkflowExecutionID,
			CompanyID:           application.CompanyID,
			NodeExecutionID:     application.NodeExecutionID,
			RelatedEventID:      relatedEventID,
			Category:            failure.Category(),
			Code: truncateAsyncErrorValue(
				failure.Code(), 128,
			),
			SafeMessage: truncateAsyncErrorValue(
				failure.Message(), 2000,
			),
			TechnicalDetail: technicalDetail,
			Retryable:       failure.Retryable(),
			Details:         details,
			CreatedAt:       application.AppliedAt,
		},
	)
}

func truncateAsyncErrorValue(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}

func durableWorkerFailure(
	result repository.DurableWorkerResult,
) (runtime.RuntimeFailure, bool) {
	if failure, exists := result.Failure(); exists {
		return failure, true
	}
	nodeResult, exists := result.Result()
	if !exists {
		return runtime.RuntimeFailure{}, false
	}
	return nodeResult.Failure()
}

func durableWorkerResultsEquivalent(left, right repository.DurableWorkerResult) (bool, error) {
	if left.IdentityKey() != right.IdentityKey() || left.Status() != right.Status() ||
		left.NodeID() != right.NodeID() ||
		!left.StartedAt().UTC().Truncate(time.Microsecond).Equal(right.StartedAt().UTC().Truncate(time.Microsecond)) ||
		!left.FinishedAt().UTC().Truncate(time.Microsecond).Equal(right.FinishedAt().UTC().Truncate(time.Microsecond)) ||
		!left.CreatedAt().UTC().Truncate(time.Microsecond).Equal(right.CreatedAt().UTC().Truncate(time.Microsecond)) {
		return false, nil
	}
	leftTechnicalDetail, leftHasTechnicalDetail :=
		left.TechnicalDetail()
	rightTechnicalDetail, rightHasTechnicalDetail :=
		right.TechnicalDetail()
	if leftHasTechnicalDetail != rightHasTechnicalDetail ||
		leftTechnicalDetail != rightTechnicalDetail {
		return false, nil
	}
	leftOutputs, leftTerminal, leftContext, leftFailure, err := encodeDurableWorkerResultParts(left)
	if err != nil {
		return false, err
	}
	rightOutputs, rightTerminal, rightContext, rightFailure, err := encodeDurableWorkerResultParts(right)
	if err != nil {
		return false, err
	}
	return leftOutputs == rightOutputs &&
		leftContext == rightContext &&
		optionalJSONStringEqual(leftTerminal, rightTerminal) &&
		optionalJSONStringEqual(leftFailure, rightFailure), nil
}

func optionalJSONStringEqual(left, right any) bool {
	switch leftValue := left.(type) {
	case nil:
		return right == nil
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	default:
		return false
	}
}
