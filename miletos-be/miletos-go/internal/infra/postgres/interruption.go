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
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

const listInterruptedAttemptCandidatesSQL = `
SELECT
	n.company_id,
	n.workflow_execution_id,
	n.node_execution_id,
	n.node_id,
	n.attempt,
	n.lock_version,
	attempt.started_at
FROM workflow_runtime.node_executions AS n
JOIN workflow_runtime.workflow_executions AS w
  ON w.company_id=n.company_id
 AND w.workflow_execution_id=n.workflow_execution_id
JOIN workflow_runtime.node_execution_attempts AS attempt
  ON attempt.company_id=n.company_id
 AND attempt.workflow_execution_id=n.workflow_execution_id
 AND attempt.node_execution_id=n.node_execution_id
 AND attempt.attempt=n.attempt
WHERE w.mode='ASYNC'
  AND w.status='RUNNING'
  AND n.status='RUNNING'
  AND n.started_at IS NOT NULL
  AND attempt.attempt_status='RUNNING'
  AND attempt.finished_at IS NULL
  AND NOT EXISTS (
	SELECT 1
	FROM workflow_runtime.async_worker_results AS result
	WHERE result.company_id=n.company_id
	  AND result.workflow_execution_id=n.workflow_execution_id
	  AND result.node_execution_id=n.node_execution_id
	  AND result.attempt=n.attempt
  )
ORDER BY attempt.started_at, n.company_id, n.workflow_execution_id, n.node_execution_id
LIMIT $1`

const interruptedAttemptDurableResultExistsSQL = `
SELECT EXISTS (
	SELECT 1
	FROM workflow_runtime.async_worker_results
	WHERE company_id=$1
	  AND workflow_execution_id=$2
	  AND node_execution_id=$3
	  AND attempt=$4
)`

const finalizeInterruptedNodeSQL = `
UPDATE workflow_runtime.node_executions
SET
	status='TIMED_OUT',
	finished_at=$7,
	updated_at=$7,
	output_summary=NULL,
	failure_summary=$8::jsonb,
	next_attempt_at=NULL,
	lock_version=lock_version+1
WHERE company_id=$1
  AND workflow_execution_id=$2
  AND node_execution_id=$3
  AND node_id=$4
  AND status='RUNNING'
  AND attempt=$5
  AND lock_version=$6`

const lockInterruptedDescendantsSQL = `SELECT ` +
	nodeExecutionSelectColumns + `
FROM workflow_runtime.node_executions
WHERE company_id=$1
  AND workflow_execution_id=$2
  AND node_id=ANY($3::text[])
ORDER BY node_id, node_execution_id
FOR UPDATE`

const interruptedWorkflowTerminalStatusSQL = `
SELECT
	COALESCE(bool_and(status IN (
		'SUCCEEDED', 'FAILED', 'SKIPPED', 'CANCELLED', 'TIMED_OUT'
	)), false),
	COALESCE(bool_or(status='FAILED'), false),
	COALESCE(bool_or(status='TIMED_OUT'), false),
	COALESCE(bool_or(status='CANCELLED'), false)
FROM workflow_runtime.node_executions
WHERE company_id=$1 AND workflow_execution_id=$2`

var _ repository.InterruptedAttemptStore = (*Store)(nil)

func (store *Store) ListInterruptedAttemptCandidates(
	ctx context.Context,
	request repository.InterruptedAttemptAuditRequest,
) ([]repository.InterruptedAttemptCandidate, error) {
	if !store.IsValid() {
		return nil, fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return nil, fmt.Errorf("interrupted attempt list context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !request.IsValid() {
		return nil, fmt.Errorf("interrupted attempt audit request must be valid")
	}
	rows, err := store.pool.Query(
		ctx, listInterruptedAttemptCandidatesSQL, request.BatchLimit())
	if err != nil {
		return nil, mapPostgreSQLError(
			"list", "interrupted attempt candidates", err)
	}
	defer rows.Close()
	candidates := make([]repository.InterruptedAttemptCandidate, 0)
	for rows.Next() {
		var params repository.InterruptedAttemptCandidateParams
		var companyID, workflowExecutionID, nodeExecutionID, nodeID string
		var attempt int16
		if err := rows.Scan(
			&companyID,
			&workflowExecutionID,
			&nodeExecutionID,
			&nodeID,
			&attempt,
			&params.ExpectedNodeLockVersion,
			&params.AttemptStartedAt,
		); err != nil {
			return nil, mapPostgreSQLError(
				"list", "interrupted attempt candidates", err)
		}
		params.CompanyID = workflow.CompanyID(companyID)
		params.WorkflowExecutionID = execution.WorkflowExecutionID(workflowExecutionID)
		params.NodeExecutionID = execution.NodeExecutionID(nodeExecutionID)
		params.NodeID = workflow.NodeID(nodeID)
		params.Attempt = execution.AttemptNumber(attempt)
		candidate, err := repository.NewInterruptedAttemptCandidate(params)
		if err != nil {
			return nil, fmt.Errorf(
				"reconstruct interrupted attempt candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgreSQLError(
			"list", "interrupted attempt candidates", err)
	}
	return candidates, nil
}

func (store *Store) TryWithInterruptedAttemptOwnership(
	ctx context.Context,
	candidate repository.InterruptedAttemptCandidate,
	work repository.InterruptedAttemptOwnershipWork,
) (bool, error) {
	lock := repository.AsyncNodeExecutionLock{
		CompanyID:           candidate.CompanyID(),
		WorkflowExecutionID: candidate.WorkflowExecutionID(),
		NodeExecutionID:     candidate.NodeExecutionID(),
		Attempt:             candidate.Attempt().Int16(),
	}
	return store.withAsyncNodeExecutionSessionLock(
		ctx, lock, false, work)
}

func (store *Store) FinalizeInterruptedAttempt(
	ctx context.Context,
	finalization repository.InterruptedAttemptFinalization,
) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("interrupted attempt finalization context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !finalization.IsValid() {
		return fmt.Errorf("interrupted attempt finalization must be valid")
	}
	return store.withinTransaction(
		ctx, "finalize interrupted attempt", func(tx pgx.Tx) error {
			return finalizeInterruptedAttemptTransaction(ctx, tx, finalization)
		})
}

func finalizeInterruptedAttemptTransaction(
	ctx context.Context,
	tx pgx.Tx,
	finalization repository.InterruptedAttemptFinalization,
) error {
	candidate := finalization.Candidate()
	companyID := candidate.CompanyID()
	workflowExecutionID := candidate.WorkflowExecutionID()
	nodeExecutionID := candidate.NodeExecutionID()
	attempt := candidate.Attempt()
	interruptedAt := finalization.InterruptedAt()
	workflowRecord, err := lockAsyncWorkflowTimelineRecord(
		ctx, tx, companyID, workflowExecutionID)
	if err != nil {
		return err
	}
	if workflowRecord.Mode() != execution.ExecutionModeAsync ||
		workflowRecord.Status() != execution.WorkflowExecutionStatusRunning {
		return repository.NewStaleWriteError(
			"finalize", "interrupted workflow execution",
			errors.New("workflow is no longer a running async execution"))
	}
	source, err := getInterruptedNodeForUpdate(ctx, tx, candidate)
	if err != nil {
		return err
	}
	sourceStartedAt, hasStartedAt := source.StartedAt()
	if source.Status() != execution.NodeExecutionStatusRunning ||
		source.NodeID() != candidate.NodeID() ||
		source.Attempt() != attempt.Int16() ||
		source.LockVersion() != candidate.ExpectedNodeLockVersion() ||
		!hasStartedAt ||
		!source.UpdatedAt().Equal(candidate.AttemptStartedAt()) ||
		sourceStartedAt.After(candidate.AttemptStartedAt()) {
		return repository.NewStaleWriteError(
			"finalize", "interrupted node execution",
			errors.New("node identity, state, attempt, start time, or version changed"))
	}
	var durableResultExists bool
	if err := tx.QueryRow(
		ctx,
		interruptedAttemptDurableResultExistsSQL,
		companyID.String(),
		workflowExecutionID.String(),
		nodeExecutionID.String(),
		attempt.Int16(),
	).Scan(&durableResultExists); err != nil {
		return mapPostgreSQLError(
			"check", "interrupted attempt durable result", err)
	}
	if durableResultExists {
		return repository.NewStaleWriteError(
			"finalize", "interrupted attempt",
			errors.New("durable worker result already exists"))
	}
	descendants, err := lockInterruptedDescendants(
		ctx, tx, candidate, finalization.DescendantNodeIDs())
	if err != nil {
		return err
	}
	timeline, sourceEventID, err := buildInterruptedAttemptTimeline(
		workflowRecord, source, descendants, finalization)
	if err != nil {
		return err
	}
	nextSequence, err := nextSequenceAfterTimeline(
		workflowRecord.NextSequenceNumber(), len(timeline))
	if err != nil {
		return err
	}
	tag, err := tx.Exec(
		ctx,
		finalizeInterruptedNodeSQL,
		companyID.String(),
		workflowExecutionID.String(),
		nodeExecutionID.String(),
		candidate.NodeID().String(),
		attempt.Int16(),
		candidate.ExpectedNodeLockVersion(),
		interruptedAt,
		finalization.FailureSummary().String(),
	)
	if err != nil {
		return mapPostgreSQLError(
			"finalize", "interrupted node execution", err)
	}
	if tag.RowsAffected() != 1 {
		return repository.NewStaleWriteError(
			"finalize", "interrupted node execution",
			errors.New("node state no longer matches the interrupted attempt"))
	}
	attemptCompletion, err := repository.NewNodeAttemptCompletion(
		repository.NodeAttemptCompletionParams{
			CompanyID:           companyID,
			WorkflowExecutionID: workflowExecutionID,
			NodeExecutionID:     nodeExecutionID,
			ExpectedAttempt:     attempt,
			Status:              execution.NodeExecutionStatusTimedOut,
			FinishedAt:          interruptedAt,
			FailureSummary:      finalization.FailureSummary().Bytes(),
			RetryDecision:       finalization.RetryDecision(),
		})
	if err != nil {
		return err
	}
	if err := completeNodeAttemptTransaction(
		ctx, tx, attemptCompletion); err != nil {
		return err
	}
	for _, descendant := range descendants {
		if descendant.Status() == execution.NodeExecutionStatusSkipped {
			continue
		}
		tag, err := tx.Exec(
			ctx,
			skipAsyncNodeExecutionSQL,
			companyID.String(),
			workflowExecutionID.String(),
			descendant.ID().String(),
			descendant.LockVersion(),
			interruptedAt,
		)
		if err != nil {
			return mapPostgreSQLError(
				"skip", "interrupted descendant node execution", err)
		}
		if tag.RowsAffected() != 1 {
			return repository.NewStaleWriteError(
				"skip", "interrupted descendant node execution",
				errors.New("descendant state no longer matches pending version"))
		}
	}
	workflowStatus, err := interruptedWorkflowTerminalStatus(
		ctx, tx, companyID, workflowExecutionID)
	if err != nil {
		return err
	}
	if workflowStatus.IsTerminal() {
		workflowTimeline, err := buildInterruptedWorkflowTimeline(
			workflowRecord, workflowStatus, interruptedAt,
			nextSequence, nodeExecutionID)
		if err != nil {
			return err
		}
		timeline = append(timeline, workflowTimeline...)
		nextSequence, err = nextSequenceAfterTimeline(
			workflowRecord.NextSequenceNumber(), len(timeline))
		if err != nil {
			return err
		}
		tag, err = tx.Exec(
			ctx,
			completeAsyncWorkflowSQL,
			companyID.String(),
			workflowExecutionID.String(),
			workflowRecord.LockVersion(),
			workflowStatus.String(),
			interruptedAt,
			workflowRecord.NextSequenceNumber().Int64(),
			nextSequence.Int64(),
		)
		if err != nil {
			return mapPostgreSQLError(
				"complete", "interrupted workflow execution", err)
		}
	} else {
		tag, err = tx.Exec(
			ctx,
			advanceAsyncWorkflowTimelineSQL,
			companyID.String(),
			workflowExecutionID.String(),
			workflowRecord.NextSequenceNumber().Int64(),
			nextSequence.Int64(),
		)
		if err != nil {
			return mapPostgreSQLError(
				"advance", "interrupted workflow timeline", err)
		}
	}
	if tag.RowsAffected() != 1 {
		return repository.NewStaleWriteError(
			"finalize", "interrupted workflow execution",
			errors.New("workflow state, version, or timeline changed"))
	}
	actualNext, err := insertTimelineEntriesTransaction(
		ctx, tx, workflowRecord.NextSequenceNumber(), timeline)
	if err != nil {
		return err
	}
	if actualNext != nextSequence {
		return fmt.Errorf(
			"interrupted execution timeline sequence allocation is inconsistent")
	}
	executionError, err := interruptedExecutionError(
		finalization, sourceEventID)
	if err != nil {
		return err
	}
	return insertExecutionErrorsTransaction(
		ctx, tx, []repository.ExecutionErrorRecord{executionError})
}

func getInterruptedNodeForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	candidate repository.InterruptedAttemptCandidate,
) (repository.NodeExecutionRecord, error) {
	row := tx.QueryRow(
		ctx,
		`SELECT `+nodeExecutionSelectColumns+`
FROM workflow_runtime.node_executions
WHERE company_id=$1
  AND workflow_execution_id=$2
  AND node_execution_id=$3
FOR UPDATE`,
		candidate.CompanyID().String(),
		candidate.WorkflowExecutionID().String(),
		candidate.NodeExecutionID().String(),
	)
	record, err := scanNodeExecutionRecord(row)
	if err != nil {
		return repository.NodeExecutionRecord{},
			mapPostgreSQLError("lock", "interrupted node execution", err)
	}
	return record, nil
}

func lockInterruptedDescendants(
	ctx context.Context,
	tx pgx.Tx,
	candidate repository.InterruptedAttemptCandidate,
	nodeIDs []workflow.NodeID,
) ([]repository.NodeExecutionRecord, error) {
	if len(nodeIDs) == 0 {
		return []repository.NodeExecutionRecord{}, nil
	}
	requested := make(map[workflow.NodeID]struct{}, len(nodeIDs))
	values := make([]string, len(nodeIDs))
	for index, nodeID := range nodeIDs {
		if nodeID == candidate.NodeID() {
			return nil, interruptedDescendantStale(
				"interrupted source node is included as a descendant")
		}
		if _, exists := requested[nodeID]; exists {
			return nil, interruptedDescendantStale(
				"duplicate descendant node identity")
		}
		requested[nodeID] = struct{}{}
		values[index] = nodeID.String()
	}
	rows, err := tx.Query(
		ctx,
		lockInterruptedDescendantsSQL,
		candidate.CompanyID().String(),
		candidate.WorkflowExecutionID().String(),
		values,
	)
	if err != nil {
		return nil, mapPostgreSQLError(
			"lock", "interrupted descendant node executions", err)
	}
	defer rows.Close()
	records := make([]repository.NodeExecutionRecord, 0, len(nodeIDs))
	found := make(map[workflow.NodeID]struct{}, len(nodeIDs))
	for rows.Next() {
		record, err := scanNodeExecutionRecord(rows)
		if err != nil {
			return nil, mapPostgreSQLError(
				"lock", "interrupted descendant node execution", err)
		}
		if record.CompanyID() != candidate.CompanyID() ||
			record.WorkflowExecutionID() != candidate.WorkflowExecutionID() {
			return nil, interruptedDescendantStale(
				"descendant belongs to a different execution scope")
		}
		if record.NodeID() == candidate.NodeID() ||
			record.ID() == candidate.NodeExecutionID() {
			return nil, interruptedDescendantStale(
				"interrupted source node was returned as a descendant")
		}
		if _, exists := requested[record.NodeID()]; !exists {
			return nil, interruptedDescendantStale(
				"unexpected descendant node identity")
		}
		if _, exists := found[record.NodeID()]; exists {
			return nil, interruptedDescendantStale(
				"duplicate descendant node row")
		}
		if record.Status() != execution.NodeExecutionStatusPending &&
			record.Status() != execution.NodeExecutionStatusSkipped {
			return nil, interruptedDescendantStale(fmt.Sprintf(
				"descendant %s is in unsafe state %s",
				record.ID(), record.Status()))
		}
		found[record.NodeID()] = struct{}{}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, mapPostgreSQLError(
			"lock", "interrupted descendant node executions", err)
	}
	if len(records) != len(requested) {
		return nil, repository.NewNotFoundError(
			"lock", "interrupted descendant node set",
			errors.New("one or more requested descendant nodes are missing"))
	}
	return records, nil
}

func interruptedDescendantStale(reason string) error {
	return repository.NewStaleWriteError(
		"lock", "interrupted descendant node set", errors.New(reason))
}

func interruptedWorkflowTerminalStatus(
	ctx context.Context,
	tx pgx.Tx,
	companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID,
) (execution.WorkflowExecutionStatus, error) {
	var allTerminal, hasFailed, hasTimedOut, hasCancelled bool
	err := tx.QueryRow(
		ctx, interruptedWorkflowTerminalStatusSQL,
		companyID.String(), workflowExecutionID.String(),
	).Scan(&allTerminal, &hasFailed, &hasTimedOut, &hasCancelled)
	if err != nil {
		return "", mapPostgreSQLError(
			"aggregate", "interrupted workflow node states", err)
	}
	if !allTerminal {
		return "", nil
	}
	switch {
	case hasFailed:
		return execution.WorkflowExecutionStatusFailed, nil
	case hasTimedOut:
		return execution.WorkflowExecutionStatusTimedOut, nil
	case hasCancelled:
		return execution.WorkflowExecutionStatusCancelled, nil
	default:
		return execution.WorkflowExecutionStatusSucceeded, nil
	}
}

func buildInterruptedAttemptTimeline(
	workflowRecord repository.WorkflowExecutionRecord,
	source repository.NodeExecutionRecord,
	descendants []repository.NodeExecutionRecord,
	finalization repository.InterruptedAttemptFinalization,
) ([]repository.TimelineEntry, repository.ExecutionEventID, error) {
	entries := make([]repository.TimelineEntry, 0, 2+len(descendants)*2)
	sequence := workflowRecord.NextSequenceNumber()
	sourceMetadata, err := json.Marshal(map[string]any{
		"attempt":         finalization.Candidate().Attempt().Int16(),
		"failureCategory": finalization.Failure().Category().String(),
		"failureCode":     finalization.Failure().Code(),
		"nodeId":          source.NodeID().String(),
		"ownership":       "LOST",
		"pluginType":      source.PluginType().String(),
		"pluginVersion":   source.PluginVersion().String(),
	})
	if err != nil {
		return nil, "", fmt.Errorf(
			"marshal interrupted attempt timeline metadata: %w", err)
	}
	sourceEntries, sourceEventID, next, err := buildInterruptionTimelinePair(
		workflowRecord, source.ID(),
		repository.ExecutionEventTypeNodeTimedOut,
		source.Status().String(), execution.NodeExecutionStatusTimedOut.String(),
		repository.ExecutionLogLevelError,
		"Node attempt was interrupted after worker ownership was lost",
		sourceMetadata, finalization.InterruptedAt(), sequence)
	if err != nil {
		return nil, "", err
	}
	entries = append(entries, sourceEntries...)
	sequence = next
	for _, descendant := range descendants {
		if descendant.Status() == execution.NodeExecutionStatusSkipped {
			continue
		}
		metadata, err := json.Marshal(map[string]any{
			"nodeId":        descendant.NodeID().String(),
			"pluginType":    descendant.PluginType().String(),
			"pluginVersion": descendant.PluginVersion().String(),
			"reason":        "UPSTREAM_INTERRUPTED",
		})
		if err != nil {
			return nil, "", fmt.Errorf(
				"marshal interrupted descendant timeline metadata: %w", err)
		}
		descendantEntries, _, next, err := buildInterruptionTimelinePair(
			workflowRecord, descendant.ID(),
			repository.ExecutionEventTypeNodeSkipped,
			descendant.Status().String(), execution.NodeExecutionStatusSkipped.String(),
			repository.ExecutionLogLevelWarn,
			"Node execution skipped because an upstream attempt was interrupted",
			metadata, finalization.InterruptedAt(), sequence)
		if err != nil {
			return nil, "", err
		}
		entries = append(entries, descendantEntries...)
		sequence = next
	}
	return entries, sourceEventID, nil
}

func buildInterruptedWorkflowTimeline(
	workflowRecord repository.WorkflowExecutionRecord,
	target execution.WorkflowExecutionStatus,
	at time.Time,
	sequence repository.SequenceNumber,
	interruptedNodeExecutionID execution.NodeExecutionID,
) ([]repository.TimelineEntry, error) {
	eventType, safeMessage, level, err :=
		asyncWorkflowTransitionDescription(target)
	if err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(map[string]any{
		"interruptedNodeExecutionId": interruptedNodeExecutionID.String(),
		"stalled":                    workflowRecord.IsStalled(),
	})
	if err != nil {
		return nil, fmt.Errorf(
			"marshal interrupted workflow timeline metadata: %w", err)
	}
	entries, _, _, err := buildInterruptionTimelinePair(
		workflowRecord, "", eventType,
		workflowRecord.Status().String(), target.String(), level,
		safeMessage, metadata, at, sequence)
	return entries, err
}

func buildInterruptionTimelinePair(
	workflowRecord repository.WorkflowExecutionRecord,
	nodeExecutionID execution.NodeExecutionID,
	eventType repository.ExecutionEventType,
	previousStatus string,
	newStatus string,
	level repository.ExecutionLogLevel,
	message string,
	metadata []byte,
	at time.Time,
	sequence repository.SequenceNumber,
) ([]repository.TimelineEntry, repository.ExecutionEventID, repository.SequenceNumber, error) {
	eventID := asyncTimelineEventID(workflowRecord.ID(), sequence)
	correlationID, _ := workflowRecord.CorrelationID()
	event, err := repository.NewExecutionEventDraft(
		repository.ExecutionEventDraftParams{
			ID:                  eventID,
			WorkflowExecutionID: workflowRecord.ID(),
			CompanyID:           workflowRecord.CompanyID(),
			NodeExecutionID:     nodeExecutionID,
			Type:                eventType,
			PreviousStatus:      previousStatus,
			NewStatus:           newStatus,
			CorrelationID:       correlationID,
			SafeMessage:         message,
			Metadata:            metadata,
			CreatedAt:           at,
		})
	if err != nil {
		return nil, "", 0, err
	}
	eventEntry, err := repository.NewEventTimelineEntry(event)
	if err != nil {
		return nil, "", 0, err
	}
	logSequence, err := nextSequenceAfterTimeline(sequence, 1)
	if err != nil {
		return nil, "", 0, err
	}
	logDraft, err := repository.NewExecutionLogDraft(
		repository.ExecutionLogDraftParams{
			ID: asyncTimelineLogID(
				workflowRecord.ID(), logSequence),
			WorkflowExecutionID: workflowRecord.ID(),
			CompanyID:           workflowRecord.CompanyID(),
			NodeExecutionID:     nodeExecutionID,
			Level:               level,
			Message:             message,
			Metadata:            metadata,
			CreatedAt:           at,
		})
	if err != nil {
		return nil, "", 0, err
	}
	logEntry, err := repository.NewLogTimelineEntry(logDraft)
	if err != nil {
		return nil, "", 0, err
	}
	nextSequence, err := nextSequenceAfterTimeline(sequence, 2)
	if err != nil {
		return nil, "", 0, err
	}
	return []repository.TimelineEntry{
		eventEntry,
		logEntry,
	}, eventID, nextSequence, nil
}

func interruptedExecutionError(
	finalization repository.InterruptedAttemptFinalization,
	relatedEventID repository.ExecutionEventID,
) (repository.ExecutionErrorRecord, error) {
	failure := finalization.Failure()
	details, err := json.Marshal(map[string]any{
		"attempt":   finalization.Candidate().Attempt().Int16(),
		"ownership": "lost",
	})
	if err != nil {
		return repository.ExecutionErrorRecord{}, fmt.Errorf(
			"marshal interrupted execution error details: %w", err)
	}
	return repository.NewExecutionErrorRecord(
		repository.ExecutionErrorRecordParams{
			ID: repository.ExecutionErrorID(
				relatedEventID.String() + "/error"),
			WorkflowExecutionID: finalization.Candidate().WorkflowExecutionID(),
			CompanyID:           finalization.Candidate().CompanyID(),
			NodeExecutionID:     finalization.Candidate().NodeExecutionID(),
			RelatedEventID:      relatedEventID,
			Category:            runtime.FailureCategoryTimeout,
			Code:                failure.Code(),
			SafeMessage:         failure.Message(),
			Retryable:           false,
			Details:             details,
			CreatedAt:           finalization.InterruptedAt(),
		})
}
