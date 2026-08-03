package persistence

import (
	"encoding/json"

	"miletos-go/internal/features/workflow-runtime/execution/model"
)

func ToExecution(record ExecutionRecord) model.Execution {
	return model.Execution{
		ID: record.WorkflowExecutionID, CompanyID: record.CompanyID,
		WorkflowID: record.WorkflowID, WorkflowRevision: record.WorkflowRevision,
		SnapshotID: record.SnapshotID, Mode: record.Mode,
		Origin:        model.ExecutionOrigin(record.ExecutionOrigin),
		CorrelationID: record.CorrelationID, Status: model.ExecutionStatus(record.Status),
		CreatedAt: record.CreatedAt, ValidatingAt: record.ValidatingAt,
		QueuedAt: record.QueuedAt, StartedAt: record.StartedAt,
		FinishedAt: record.FinishedAt, UpdatedAt: record.UpdatedAt,
		TerminalOutputs: decodeObject(record.TerminalOutputs),
		Failure:         decodeObject(record.FailureSummary), IsStalled: record.IsStalled,
	}
}

func ToExecutions(records []ExecutionRecord) []model.Execution {
	items := make([]model.Execution, 0, len(records))
	for _, record := range records {
		items = append(items, ToExecution(record))
	}
	return items
}

func ToNodeExecution(record NodeExecutionRecord) model.NodeExecution {
	return model.NodeExecution{
		ID: record.NodeExecutionID, ExecutionID: record.ExecutionID,
		CompanyID: record.CompanyID, NodeID: record.NodeID,
		Type: record.PluginType, Version: record.PluginVersion,
		Status: model.NodeStatus(record.Status), Attempt: record.Attempt,
		CreatedAt: record.CreatedAt, ReadyAt: record.ReadyAt, QueuedAt: record.QueuedAt,
		StartedAt: record.StartedAt, FinishedAt: record.FinishedAt,
		NextAttemptAt: record.NextAttemptAt, UpdatedAt: record.UpdatedAt,
		Input: decodeObject(record.InputSummary), Output: decodeObject(record.OutputSummary),
		Failure: decodeObject(record.FailureSummary), LockVersion: record.LockVersion,
	}
}

func ToNodeExecutions(records []NodeExecutionRecord) []model.NodeExecution {
	items := make([]model.NodeExecution, 0, len(records))
	for _, record := range records {
		items = append(items, ToNodeExecution(record))
	}
	return items
}

func ToExecutionEvents(records []ExecutionEventRecord) []model.ExecutionEvent {
	items := make([]model.ExecutionEvent, 0, len(records))
	for _, record := range records {
		items = append(items, model.ExecutionEvent{
			ID: record.EventID, ExecutionID: record.ExecutionID,
			NodeExecutionID: record.NodeExecutionID, Sequence: record.Sequence,
			Type: record.EventType, PreviousStatus: record.PreviousStatus,
			NewStatus: record.NewStatus, CorrelationID: record.CorrelationID,
			CausationID: record.CausationID, Message: record.SafeMessage,
			Metadata: decodeObject(record.Metadata), CreatedAt: record.CreatedAt,
		})
	}
	return items
}

func ToExecutionLogs(records []ExecutionLogRecord) []model.ExecutionLog {
	items := make([]model.ExecutionLog, 0, len(records))
	for _, record := range records {
		items = append(items, model.ExecutionLog{
			ID: record.LogID, ExecutionID: record.ExecutionID,
			NodeExecutionID: record.NodeExecutionID, Sequence: record.Sequence,
			Level: record.Level, Message: record.Message,
			Metadata: decodeObject(record.Metadata), CreatedAt: record.CreatedAt,
		})
	}
	return items
}

func ToExecutionErrors(records []ExecutionErrorRecord) []model.ExecutionError {
	items := make([]model.ExecutionError, 0, len(records))
	for _, record := range records {
		items = append(items, model.ExecutionError{
			ID: record.ErrorID, ExecutionID: record.ExecutionID,
			NodeExecutionID: record.NodeExecutionID, RelatedEventID: stringValue(record.RelatedEventID),
			Category: model.FailureCategory(record.Category),
			Code:     model.FailureCode(record.Code), Message: record.SafeMessage,
			Retryable: record.Retryable, Details: decodeObject(record.Details),
			CreatedAt: record.CreatedAt,
		})
	}
	return items
}

func decodeObject(encoded []byte) map[string]any {
	if len(encoded) == 0 {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(encoded, &value) != nil {
		return nil
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
