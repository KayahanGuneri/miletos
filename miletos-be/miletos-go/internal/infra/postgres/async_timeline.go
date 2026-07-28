package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

const advanceAsyncWorkflowTimelineSQL = `
UPDATE workflow_runtime.workflow_executions
SET next_sequence_number=$4
WHERE company_id=$1 AND workflow_execution_id=$2
  AND next_sequence_number=$3`

func lockAsyncWorkflowTimelineRecord(ctx context.Context, tx pgx.Tx, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) (repository.WorkflowExecutionRecord, error) {
	if ctx == nil || tx == nil {
		return repository.WorkflowExecutionRecord{}, fmt.Errorf("async workflow timeline dependencies must be valid")
	}
	row := tx.QueryRow(ctx, `SELECT `+workflowExecutionSelectColumns+` FROM workflow_runtime.workflow_executions WHERE company_id=$1 AND workflow_execution_id=$2 FOR UPDATE`,
		companyID.String(), workflowExecutionID.String())
	record, err := scanWorkflowExecutionRecord(row)
	if err != nil {
		return repository.WorkflowExecutionRecord{}, mapPostgreSQLError("lock", "async workflow timeline", err)
	}
	return record, nil
}
func getAsyncNodeTimelineRecord(ctx context.Context, tx pgx.Tx, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	nodeExecutionID execution.NodeExecutionID) (repository.NodeExecutionRecord, error) {
	if ctx == nil || tx == nil {
		return repository.NodeExecutionRecord{}, fmt.Errorf("async node timeline dependencies must be valid")
	}
	row := tx.QueryRow(ctx, `SELECT `+nodeExecutionSelectColumns+` FROM workflow_runtime.node_executions WHERE company_id=$1 AND workflow_execution_id=$2 AND node_execution_id=$3`,
		companyID.String(), workflowExecutionID.String(), nodeExecutionID.String())
	record, err := scanNodeExecutionRecord(row)
	if err != nil {
		return repository.NodeExecutionRecord{}, mapPostgreSQLError("get", "async node timeline", err)
	}
	return record, nil
}
func persistAsyncNodeTransitionTimeline(ctx context.Context, tx pgx.Tx, workflowRecord repository.WorkflowExecutionRecord, entries []repository.TimelineEntry) error {
	if len(entries) == 0 {
		return fmt.Errorf("async node transition timeline must not be empty")
	}
	nextSequence, err := nextSequenceAfterTimeline(workflowRecord.NextSequenceNumber(), len(entries))
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, advanceAsyncWorkflowTimelineSQL, workflowRecord.CompanyID().String(), workflowRecord.ID().String(), workflowRecord.NextSequenceNumber().Int64(),
		nextSequence.Int64())
	if err != nil {
		return mapPostgreSQLError("advance", "async workflow timeline", err)
	}
	if tag.RowsAffected() != 1 {
		return repository.NewStaleWriteError("advance", "async workflow timeline", errors.New("workflow timeline sequence changed"))
	}
	actualNext, err := insertTimelineEntriesTransaction(ctx, tx, workflowRecord.NextSequenceNumber(), entries)
	if err != nil {
		return err
	}
	if actualNext != nextSequence {
		return fmt.Errorf("async node transition timeline sequence allocation is inconsistent")
	}
	return nil
}
func buildAsyncNodeTransitionTimeline(workflowRecord repository.WorkflowExecutionRecord, nodeRecord repository.NodeExecutionRecord, target execution.NodeExecutionStatus,
	at time.Time, failureCategory string, failureCode string) ([]repository.TimelineEntry, error) {
	eventType, safeMessage, level, err := asyncNodeTransitionDescription(target)
	if err != nil {
		return nil, err
	}
	metadataValue := map[string]any{
		"nodeId": nodeRecord.NodeID().String(), "pluginType": nodeRecord.PluginType().String(), "pluginVersion": nodeRecord.PluginVersion().String(),
	}
	if failureCategory != "" {
		metadataValue["failureCategory"] = failureCategory
	}
	if failureCode != "" {
		metadataValue["failureCode"] = failureCode
	}
	metadata, err := json.Marshal(metadataValue)
	if err != nil {
		return nil, fmt.Errorf("marshal async node timeline metadata: %w", err)
	}
	firstSequence := workflowRecord.NextSequenceNumber()
	logSequence, err := nextSequenceAfterTimeline(firstSequence, 1)
	if err != nil {
		return nil, err
	}
	correlationID, _ := workflowRecord.CorrelationID()
	event, err := repository.NewExecutionEventDraft(repository.ExecutionEventDraftParams{
		ID: asyncTimelineEventID(workflowRecord.ID(), firstSequence), WorkflowExecutionID: workflowRecord.ID(), CompanyID: workflowRecord.CompanyID(),
		NodeExecutionID: nodeRecord.ID(), Type: eventType, PreviousStatus: nodeRecord.Status().String(),
		NewStatus: target.String(), CorrelationID: correlationID, SafeMessage: safeMessage,
		Metadata: metadata, CreatedAt: at})
	if err != nil {
		return nil, err
	}
	eventEntry, err := repository.NewEventTimelineEntry(event)
	if err != nil {
		return nil, err
	}
	logDraft, err := repository.NewExecutionLogDraft(repository.ExecutionLogDraftParams{ID: asyncTimelineLogID(workflowRecord.ID(), logSequence),
		WorkflowExecutionID: workflowRecord.ID(), CompanyID: workflowRecord.CompanyID(), NodeExecutionID: nodeRecord.ID(),
		Level: level, Message: safeMessage, Metadata: metadata,
		CreatedAt: at})
	if err != nil {
		return nil, err
	}
	logEntry, err := repository.NewLogTimelineEntry(logDraft)
	if err != nil {
		return nil, err
	}
	return []repository.TimelineEntry{eventEntry, logEntry}, nil
}
func buildAsyncWorkflowTransitionTimeline(workflowRecord repository.WorkflowExecutionRecord, target execution.WorkflowExecutionStatus, at time.Time) ([]repository.TimelineEntry,
	error) {
	eventType, safeMessage, level, err := asyncWorkflowTransitionDescription(target)
	if err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(map[string]any{"stalled": workflowRecord.IsStalled()})
	if err != nil {
		return nil, fmt.Errorf("marshal async workflow timeline metadata: %w", err)
	}
	firstSequence := workflowRecord.NextSequenceNumber()
	logSequence, err := nextSequenceAfterTimeline(firstSequence, 1)
	if err != nil {
		return nil, err
	}
	correlationID, _ := workflowRecord.CorrelationID()
	event, err := repository.NewExecutionEventDraft(repository.ExecutionEventDraftParams{ID: asyncTimelineEventID(workflowRecord.ID(), firstSequence),
		WorkflowExecutionID: workflowRecord.ID(),
		CompanyID:           workflowRecord.CompanyID(), Type: eventType, PreviousStatus: workflowRecord.Status().String(),
		NewStatus: target.String(), CorrelationID: correlationID, SafeMessage: safeMessage,
		Metadata: metadata, CreatedAt: at})
	if err != nil {
		return nil, err
	}
	eventEntry, err := repository.NewEventTimelineEntry(event)
	if err != nil {
		return nil, err
	}
	logDraft, err := repository.NewExecutionLogDraft(repository.ExecutionLogDraftParams{ID: asyncTimelineLogID(workflowRecord.ID(), logSequence),
		WorkflowExecutionID: workflowRecord.ID(), CompanyID: workflowRecord.CompanyID(), Level: level,
		Message: safeMessage, Metadata: metadata, CreatedAt: at,
	})
	if err != nil {
		return nil, err
	}
	logEntry, err := repository.NewLogTimelineEntry(logDraft)
	if err != nil {
		return nil, err
	}
	return []repository.TimelineEntry{eventEntry, logEntry}, nil
}
func asyncNodeTransitionDescription(status execution.NodeExecutionStatus) (repository.ExecutionEventType, string, repository.ExecutionLogLevel, error) {
	switch status {
	case execution.NodeExecutionStatusReady:
		return repository.ExecutionEventTypeNodeReady, "Node execution is ready", repository.ExecutionLogLevelInfo, nil
	case execution.NodeExecutionStatusQueued:
		return repository.ExecutionEventTypeNodeQueued, "Node execution queued", repository.ExecutionLogLevelInfo, nil
	case execution.NodeExecutionStatusRunning:
		return repository.ExecutionEventTypeNodeStarted, "Node execution started", repository.ExecutionLogLevelInfo, nil
	case execution.NodeExecutionStatusRetryPending:
		return repository.ExecutionEventTypeNodeRetryPending, "Node execution retry scheduled", repository.ExecutionLogLevelWarn, nil
	case execution.NodeExecutionStatusSucceeded:
		return repository.ExecutionEventTypeNodeSucceeded, "Node execution succeeded", repository.ExecutionLogLevelInfo, nil
	case execution.NodeExecutionStatusFailed:
		return repository.ExecutionEventTypeNodeFailed, "Node execution failed", repository.ExecutionLogLevelError, nil
	case execution.NodeExecutionStatusSkipped:
		return repository.ExecutionEventTypeNodeSkipped, "Node execution skipped", repository.ExecutionLogLevelWarn, nil
	case execution.NodeExecutionStatusCancelled:
		return repository.ExecutionEventTypeNodeCancelled, "Node execution was canceled", repository.ExecutionLogLevelWarn, nil
	case execution.NodeExecutionStatusTimedOut:
		return repository.ExecutionEventTypeNodeTimedOut, "Node execution timed out", repository.ExecutionLogLevelError, nil
	default:
		return "", "", "", fmt.Errorf("unsupported async node transition status %s", status)
	}
}
func asyncWorkflowTransitionDescription(status execution.WorkflowExecutionStatus) (repository.ExecutionEventType, string, repository.ExecutionLogLevel, error) {
	switch status {
	case execution.WorkflowExecutionStatusSucceeded:
		return repository.ExecutionEventTypeWorkflowSucceeded, "Workflow execution succeeded", repository.ExecutionLogLevelInfo, nil
	case execution.WorkflowExecutionStatusFailed:
		return repository.ExecutionEventTypeWorkflowFailed, "Workflow execution failed", repository.ExecutionLogLevelError, nil
	case execution.WorkflowExecutionStatusCancelled:
		return repository.ExecutionEventTypeWorkflowCancelled, "Workflow execution was canceled", repository.ExecutionLogLevelWarn, nil
	case execution.WorkflowExecutionStatusTimedOut:
		return repository.ExecutionEventTypeWorkflowTimedOut, "Workflow execution timed out", repository.ExecutionLogLevelError, nil
	default:
		return "", "", "", fmt.Errorf("unsupported async workflow transition status %s", status)
	}
}
func asyncTimelineEventID(workflowExecutionID execution.WorkflowExecutionID, sequence repository.SequenceNumber) repository.ExecutionEventID {
	return repository.ExecutionEventID(fmt.Sprintf("%s/event/%020d", workflowExecutionID.String(), sequence.Int64()))
}
func asyncTimelineLogID(workflowExecutionID execution.WorkflowExecutionID, sequence repository.SequenceNumber) repository.ExecutionLogID {
	return repository.ExecutionLogID(fmt.Sprintf("%s/log/%020d", workflowExecutionID.String(), sequence.Int64()))
}
