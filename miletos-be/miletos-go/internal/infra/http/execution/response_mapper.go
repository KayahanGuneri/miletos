package executionhttp

import (
	"encoding/json"
	"time"

	"miletos-go/internal/features/execution"
	executionfeature "miletos-go/internal/features/execution/application"
	repository "miletos-go/internal/ports/persistence"
)

func mapExecutionDefinitionResponse(
	workflowExecutionID execution.WorkflowExecutionID, snapshot repository.DefinitionSnapshot) ExecutionDefinitionResponse {
	definition := append(json.RawMessage(nil),
		snapshot.DefinitionJSON().Bytes()...,
	)
	return ExecutionDefinitionResponse{
		ExecutionID: workflowExecutionID.String(), SnapshotID: snapshot.
				ID().String(),
		WorkflowID:       snapshot.WorkflowID().String(),
		WorkflowRevision: snapshot.WorkflowRevision(),
		WorkflowName:     snapshot.WorkflowName(),
		Definition:       definition,
		CreatedAt: snapshot.CreatedAt().UTC().
			Format(time.RFC3339Nano),
	}
}

func mapExecutionResponse(
	outcome executionfeature.ExecutionOutcome) ExecutionResponse {
	var startedAt *string
	var finishedAt *string
	if outcome.StartedAt != nil {
		value := outcome.StartedAt.
			UTC().Format(timeRFC3339Nano)
		startedAt =
			&value
	}
	if outcome.FinishedAt != nil {
		value := outcome.
			FinishedAt.UTC().Format(
			timeRFC3339Nano)
		finishedAt = &value
	}
	return ExecutionResponse{ExecutionID: outcome.ExecutionID,
		WorkflowID:       outcome.WorkflowID,
		WorkflowRevision: outcome.WorkflowRevision, Mode: outcome.Mode,
		Status:        outcome.Status,
		CorrelationID: outcome.CorrelationID, CreatedAt: outcome.
				CreatedAt.UTC().Format(
			timeRFC3339Nano),
		StartedAt: startedAt, FinishedAt: finishedAt,
		ScheduledRoots: outcome.ScheduledRoots}
}

func mapExecutionErrorResponse(
	record repository.ExecutionErrorRecord) ExecutionErrorResponse {
	response :=
		ExecutionErrorResponse{ErrorID: record.ID().
			String(), WorkflowExecutionID: record.
			WorkflowExecutionID().String(),
			Category:    record.Category().String(),
			Code:        record.Code(),
			SafeMessage: record.SafeMessage(),
			Retryable:   record.Retryable(),
			Details: sanitizeExecutionErrorDetails(record.
				Details().Bytes()),
			CreatedAt: record.CreatedAt().
				UTC().Format(time.RFC3339Nano)}
	if value, exists := record.NodeExecutionID(); exists {
		response.NodeExecutionID = value.String()
	}
	if value, exists :=
		record.RelatedEventID(); exists {
		response.RelatedEventID = value.String()
	}
	return response
}

func sanitizeExecutionErrorDetails(raw []byte,
) json.RawMessage {
	details := decodeExecutionErrorDetails(
		raw)
	safeDetails := make(map[string]string)
	for key, value := range details {
		if _, allowed := executionErrorSafeDetailKeys[key]; !allowed {
			continue
		}
		safeDetails[key] = value
	}
	encoded, err :=
		json.Marshal(safeDetails)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(
		encoded)
}

func decodeExecutionErrorDetails(raw []byte,
) map[string]string {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw,
		&envelope); err != nil {
		return nil
	}
	if len(envelope) == 1 {
		nested, exists := envelope["details"]
		if exists {
			var legacyDetails map[string]string
			if err := json.Unmarshal(
				nested, &legacyDetails); err == nil {
				return legacyDetails
			}
		}
	}
	var flatDetails map[string]string
	if err :=
		json.Unmarshal(raw, &flatDetails); err != nil {
		return nil
	}
	return flatDetails
}

func mapExecutionErrorPageResponse(
	page repository.Page[repository.ExecutionErrorRecord]) ExecutionErrorPageResponse {
	records :=
		page.Items()
	items :=
		make([]ExecutionErrorResponse, len(records))
	for index, record := range records {
		items[index] = mapExecutionErrorResponse(
			record)
	}
	next := ""
	if token, exists := page.Next(); exists {
		next = token.String()
	}
	return ExecutionErrorPageResponse{Items: items,
		Next:    next,
		HasNext: page.HasNext()}
}

func mapExecutionEventResponse(record repository.ExecutionEventRecord,
) ExecutionEventResponse {
	response := ExecutionEventResponse{
		EventID: record.ID().String(),
		WorkflowExecutionID: record.WorkflowExecutionID().
			String(), SequenceNumber: record.
			SequenceNumber().Int64(),
		Type: record.Type().String(),
		Metadata: append(json.RawMessage(nil),
			record.Metadata().Bytes()...,
		), CreatedAt: record.
			CreatedAt().UTC().Format(
			time.RFC3339Nano)}
	if value, exists := record.
		NodeExecutionID(); exists {
		response.NodeExecutionID =
			value.String()
	}
	if value, exists := record.PreviousStatus(); exists {
		response.PreviousStatus = value
	}
	if value, exists :=
		record.NewStatus(); exists {
		response.NewStatus = value
	}
	if value, exists := record.
		CorrelationID(); exists {
		response.CorrelationID =
			value
	}
	if value, exists := record.CausationID(); exists {
		response.CausationID = value
	}
	if value, exists :=
		record.SafeMessage(); exists {
		response.SafeMessage = value
	}
	return response
}

func mapExecutionEventPageResponse(page repository.Page[repository.ExecutionEventRecord],
) ExecutionEventPageResponse {
	records := page.Items()
	items := make(
		[]ExecutionEventResponse, len(records))
	for index, record := range records {
		items[index] = mapExecutionEventResponse(record)
	}
	next := ""
	if token, exists :=
		page.Next(); exists {
		next =
			token.String()
	}
	return ExecutionEventPageResponse{Items: items,
		Next: next, HasNext: page.HasNext(),
	}
}

func mapExecutionLogResponse(
	record repository.ExecutionLogRecord) ExecutionLogResponse {
	response :=
		ExecutionLogResponse{LogID: record.ID().
			String(), WorkflowExecutionID: record.
			WorkflowExecutionID().String(),
			SequenceNumber: record.SequenceNumber().Int64(),
			Level: record.Level().
				String(), Message: record.
				Message(), Metadata: append(
				json.RawMessage(nil), record.Metadata().
					Bytes()...),
			CreatedAt: record.CreatedAt().UTC().
				Format(time.RFC3339Nano),
		}
	if value, exists :=
		record.NodeExecutionID(); exists {
		response.NodeExecutionID = value.String()
	}
	return response
}

func mapExecutionLogPageResponse(page repository.Page[repository.ExecutionLogRecord],
) ExecutionLogPageResponse {
	records := page.Items()
	items := make(
		[]ExecutionLogResponse, len(records))
	for index, record := range records {
		items[index] = mapExecutionLogResponse(record)
	}
	next := ""
	if token, exists :=
		page.Next(); exists {
		next =
			token.String()
	}
	return ExecutionLogPageResponse{Items: items,
		Next: next, HasNext: page.HasNext(),
	}
}

func mapNodeExecutionResponse(
	record repository.NodeExecutionRecord) NodeExecutionResponse {
	response :=
		NodeExecutionResponse{NodeExecutionID: record.ID().
			String(), WorkflowExecutionID: record.
			WorkflowExecutionID().String(),
			NodeID: record.NodeID().String(),
			PluginType: record.PluginType().
				String(), PluginVersion: record.
				PluginVersion().String(),
			Status:  record.Status().String(),
			Attempt: record.Attempt(),
			CreatedAt: record.CreatedAt().
				UTC().Format(time.RFC3339Nano), UpdatedAt: record.
				UpdatedAt().UTC().Format(
				time.RFC3339Nano)}
	if value, exists := record.
		ReadyAt(); exists {
		response.ReadyAt =
			value.UTC().Format(
				time.RFC3339Nano)
	}
	if value, exists := record.
		QueuedAt(); exists {
		response.QueuedAt =
			value.UTC().Format(
				time.RFC3339Nano)
	}
	if value, exists := record.
		StartedAt(); exists {
		response.StartedAt =
			value.UTC().Format(
				time.RFC3339Nano)
	}
	if value, exists := record.
		FinishedAt(); exists {
		response.FinishedAt =
			value.UTC().Format(
				time.RFC3339Nano)
	}
	if value, exists := record.
		InputSummary(); exists {
		response.InputSummary =
			append(json.RawMessage(nil), value.Bytes()...,
			)
	}
	if value, exists := record.OutputSummary(); exists {
		response.OutputSummary = append(
			json.RawMessage(nil), value.Bytes()...)
	}
	if value, exists :=
		record.FailureSummary(); exists {
		response.FailureSummary = append(json.RawMessage(nil),
			value.Bytes()...)
	}
	return response
}

func mapNodeExecutionPageResponse(page repository.Page[repository.NodeExecutionRecord],
) NodeExecutionPageResponse {
	records := page.Items()
	items := make(
		[]NodeExecutionResponse, len(records))
	for index, record := range records {
		items[index] = mapNodeExecutionResponse(record)
	}
	next := ""
	if token, exists :=
		page.Next(); exists {
		next =
			token.String()
	}
	return NodeExecutionPageResponse{Items: items,
		Next: next, HasNext: page.HasNext(),
	}
}

func mapExecutionSummaryResponse(record repository.WorkflowExecutionRecord,
) ExecutionSummaryResponse {
	correlationID, _ := record.CorrelationID()
	return ExecutionSummaryResponse{ExecutionID: record.
		ID().String(),
		WorkflowID:       record.WorkflowID().String(),
		WorkflowRevision: record.WorkflowRevision(),
		Mode:             record.Mode().String(),
		Status: record.Status().
			String(), CorrelationID: correlationID,
		CreatedAt: record.CreatedAt().
			UTC().Format(timeRFC3339Nano), ValidatingAt: formatOptionalExecutionTime(
			record.ValidatingAt),
		QueuedAt:  formatOptionalExecutionTime(record.QueuedAt),
		StartedAt: formatOptionalExecutionTime(record.StartedAt), FinishedAt: formatOptionalExecutionTime(
			record.FinishedAt),
		UpdatedAt: record.UpdatedAt().UTC().
			Format(timeRFC3339Nano),
		IsStalled: record.IsStalled()}
}

func mapExecutionPageResponse(
	page repository.Page[repository.WorkflowExecutionRecord]) ExecutionPageResponse {
	records :=
		page.Items()
	items :=
		make([]ExecutionSummaryResponse, 0,
			len(records))
	for _, record := range records {
		items =
			append(items, mapExecutionSummaryResponse(
				record))
	}
	next := ""
	if token, exists := page.Next(); exists {
		next = token.String()
	}
	return ExecutionPageResponse{
		Items: items, Next: next,
		HasNext: page.HasNext()}
}

func formatOptionalExecutionTime(
	getter func() (time.Time, bool,
	)) *string {
	value, exists :=
		getter()
	if !exists {
		return nil
	}
	formatted := value.UTC().
		Format(timeRFC3339Nano)
	return &formatted
}
