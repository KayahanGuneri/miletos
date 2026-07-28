package persistence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
	"unicode/utf8"

	"miletos-go/internal/engine"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

type failureSummary struct {
	Category  string `json:"category"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type payloadSummary struct {
	Source       string   `json:"source"`
	ContentType  string   `json:"contentType"`
	SizeBytes    int64    `json:"sizeBytes"`
	SHA256       string   `json:"sha256,omitempty"`
	ArtifactID   string   `json:"artifactId,omitempty"`
	Checksum     string   `json:"checksum,omitempty"`
	MetadataKeys []string `json:"metadataKeys,omitempty"`
}

type outputPortSummary struct {
	Port     string           `json:"port"`
	Payloads []payloadSummary `json:"payloads"`
}

type contextChangesSummary struct {
	SetKeys    []string `json:"setKeys,omitempty"`
	DeleteKeys []string `json:"deleteKeys,omitempty"`
}

type nodeResultSummary struct {
	Status             string                `json:"status"`
	OutputPayloadCount int                   `json:"outputPayloadCount"`
	OutputPorts        []outputPortSummary   `json:"outputPorts,omitempty"`
	TerminalOutput     *payloadSummary       `json:"terminalOutput,omitempty"`
	ContextChanges     contextChangesSummary `json:"contextChanges"`
}

func marshalFailureSummary(
	failure runtime.RuntimeFailure) ([]byte, error) {
	if !failure.IsValid() {
		return nil, fmt.Errorf("runtime failure must be valid")
	}
	return marshalJSONObject(
		failureSummary{Category: failure.Category().String(), Code: failure.Code(),
			Message: failure.Message(), Retryable: failure.Retryable()},
	)
}

func marshalFailureDetails(failure runtime.RuntimeFailure) ([]byte, error) {
	if !failure.IsValid() {
		return nil, fmt.Errorf("runtime failure must be valid")
	}
	return marshalJSONObject(map[string]any{"details": failure.Details()})
}

func summarizeNodeResult(result runtime.NodeResult,
) ([]byte, payloadSummary,
	bool, error) {
	if !result.IsValid() {
		return nil, payloadSummary{},
			false, fmt.Errorf("node result must be valid")
	}
	ports := result.OutputPorts()
	portSummaries := make([]outputPortSummary,
		0, len(ports))
	for _, port := range ports {
		payloads, exists, err := result.OutputPayloads(
			port)
		if err != nil {
			return nil, payloadSummary{}, false,
				fmt.Errorf("read output payloads for port %s: %w", port,
					err)
		}
		if !exists {
			continue
		}
		payloadSummaries := make(
			[]payloadSummary, 0, len(payloads),
		)
		for _, payload := range payloads {
			summary, err := summarizePayload(payload)
			if err != nil {
				return nil,
					payloadSummary{}, false, fmt.Errorf(
						"summarize output payload for port %s: %w", port, err,
					)
			}
			payloadSummaries = append(payloadSummaries, summary)
		}
		portSummaries = append(portSummaries, outputPortSummary{
			Port: port, Payloads: payloadSummaries},
		)
	}
	changes := result.ContextChanges()
	summary := nodeResultSummary{Status: result.Status().String(),
		OutputPayloadCount: result.TotalOutputPayloadCount(), OutputPorts: portSummaries, ContextChanges: contextChangesSummary{
			SetKeys: changes.SetKeys(), DeleteKeys: changes.DeleteKeys()},
	}
	var terminalSummary payloadSummary
	hasTerminalSummary := false
	if terminalPayload, exists := result.TerminalOutput(); exists {
		value, err := summarizePayload(terminalPayload)
		if err != nil {
			return nil, payloadSummary{},
				false, fmt.Errorf("summarize terminal output: %w",
					err)
		}
		terminalSummary = value
		hasTerminalSummary = true
		summary.TerminalOutput = &terminalSummary
	}
	encoded, err := marshalJSONObject(summary)
	if err != nil {
		return nil,
			payloadSummary{}, false, err
	}
	return encoded,
		terminalSummary, hasTerminalSummary, nil
}

func summarizePayload(
	payload runtime.Payload) (payloadSummary, error) {
	metadataKeys := make(
		[]string, 0, len(payload.Metadata()),
	)
	for key := range payload.Metadata() {
		metadataKeys = append(metadataKeys, key)
	}
	sort.Strings(metadataKeys)
	summary := payloadSummary{
		ContentType: payload.ContentType().String(), MetadataKeys: metadataKeys}
	if inlineData, exists := payload.InlineData(); exists {
		digest := sha256.Sum256(inlineData)
		summary.Source = "INLINE"
		summary.SizeBytes = int64(len(inlineData))
		summary.SHA256 = hex.EncodeToString(digest[:])
		return summary, nil
	}
	artifact, exists := payload.Artifact()
	if !exists {
		return payloadSummary{}, fmt.Errorf("payload exposes neither inline data nor artifact reference")
	}
	summary.Source = "ARTIFACT"
	summary.SizeBytes = artifact.SizeBytes()
	summary.ArtifactID = artifact.ID()
	summary.Checksum = artifact.Checksum()
	return summary, nil
}

func marshalTerminalOutputs(
	values map[string]payloadSummary) ([]byte, error) {
	if len(values) == 0 {
		return []byte(`{}`), nil
	}
	copied := make(map[string]payloadSummary, len(values))
	for nodeID, summary := range values {
		copied[nodeID] = summary
	}
	return marshalJSONObject(copied)
}

func marshalJSONObject(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal JSON object: %w", err)
	}
	if len(encoded) == 0 || encoded[0] != '{' {
		return nil, fmt.Errorf("encoded value must be a JSON object")
	}
	return encoded, nil
}

const (
	maximumPersistedErrorCodeCharacters       = 128
	maximumPersistedSafeMessageCharacters     = 2000
	maximumPersistedTechnicalDetailCharacters = 8000
)

func buildWorkflowCreationTimeline(
	request engine.ExecutionRequest, workflowRecord repository.WorkflowExecutionRecord) ([]repository.TimelineEntry, error) {
	firstSequence := workflowRecord.NextSequenceNumber()
	metadata, err := marshalJSONObject(
		map[string]any{"mode": workflowRecord.Mode().String(), "workflowRevision": workflowRecord.WorkflowRevision()})
	if err != nil {
		return nil, err
	}
	event, err := repository.NewExecutionEventDraft(repository.ExecutionEventDraftParams{ID: eventID(workflowRecord.ID(), firstSequence),
		WorkflowExecutionID: workflowRecord.ID(), CompanyID: workflowRecord.CompanyID(), Type: repository.ExecutionEventTypeWorkflowCreated,
		NewStatus: execution.WorkflowExecutionStatusCreated.String(), CorrelationID: request.CorrelationID(), SafeMessage: "Workflow execution created",
		Metadata: metadata, CreatedAt: workflowRecord.CreatedAt()},
	)
	if err != nil {
		return nil, err
	}
	eventEntry, err := repository.NewEventTimelineEntry(event)
	if err != nil {
		return nil, err
	}
	logSequence, err := sequenceAt(firstSequence, 1)
	if err != nil {
		return nil, err
	}
	logDraft, err := repository.NewExecutionLogDraft(repository.ExecutionLogDraftParams{ID: logID(workflowRecord.ID(), logSequence),
		WorkflowExecutionID: workflowRecord.ID(), CompanyID: workflowRecord.CompanyID(), Level: repository.ExecutionLogLevelInfo,
		Message: "Workflow execution created", Metadata: metadata, CreatedAt: workflowRecord.CreatedAt(),
	})
	if err != nil {
		return nil, err
	}
	logEntry, err := repository.NewLogTimelineEntry(logDraft)
	if err != nil {
		return nil, err
	}
	return []repository.TimelineEntry{
		eventEntry, logEntry}, nil
}

func buildWorkflowTransitionTimeline(
	observation engine.WorkflowTransitionObservation, firstSequence repository.SequenceNumber) (
	[]repository.TimelineEntry, repository.ExecutionEventID, error,
) {
	eventType, safeMessage, level, err := workflowTransitionDescription(
		observation.After.Status())
	if err != nil {
		return nil, "", err
	}
	metadataValue := map[string]any{"stalled": observation.Stalled}
	if observation.HasFailure {
		metadataValue["failureCategory"] =
			observation.Failure.Category().String()
		metadataValue["failureCode"] = observation.Failure.Code()
	}
	metadata, err := marshalJSONObject(metadataValue)
	if err != nil {
		return nil, "", err
	}
	eventIdentifier := eventID(observation.After.ID(),
		firstSequence)
	event, err := repository.NewExecutionEventDraft(repository.ExecutionEventDraftParams{ID: eventIdentifier,
		WorkflowExecutionID: observation.After.ID(), CompanyID: observation.After.CompanyID(), Type: eventType,
		PreviousStatus: observation.Before.Status().String(), NewStatus: observation.After.Status().String(), CorrelationID: observation.Request.CorrelationID(),
		SafeMessage: safeMessage, Metadata: metadata, CreatedAt: observation.TransitionAt,
	})
	if err != nil {
		return nil, "", err
	}
	eventEntry, err := repository.NewEventTimelineEntry(event)
	if err != nil {
		return nil, "", err
	}
	logSequence, err := sequenceAt(firstSequence, 1)
	if err != nil {
		return nil, "", err
	}
	logDraft, err := repository.NewExecutionLogDraft(repository.ExecutionLogDraftParams{
		ID: logID(observation.After.ID(), logSequence), WorkflowExecutionID: observation.After.ID(), CompanyID: observation.After.CompanyID(),
		Level: level, Message: safeMessage, Metadata: metadata,
		CreatedAt: observation.TransitionAt})
	if err != nil {
		return nil, "", err
	}
	logEntry, err := repository.NewLogTimelineEntry(logDraft)
	if err != nil {
		return nil, "", err
	}
	return []repository.TimelineEntry{eventEntry, logEntry}, eventIdentifier, nil
}

func buildNodeCreationTimeline(
	observation engine.NodeExecutionsCreationObservation, nodeRecords []repository.NodeExecutionRecord, firstSequence repository.SequenceNumber,
) ([]repository.TimelineEntry, error) {
	if len(nodeRecords) != len(observation.Items) {
		return nil, fmt.Errorf(
			"node record count must match observation item count")
	}
	timeline := make([]repository.TimelineEntry,
		0, len(nodeRecords))
	for index, record := range nodeRecords {
		sequence, err := sequenceAt(
			firstSequence, index)
		if err != nil {
			return nil, err
		}
		metadata, err := marshalJSONObject(map[string]any{
			"nodeId": record.NodeID().String(), "pluginType": record.PluginType().String(), "pluginVersion": record.PluginVersion().String(),
		})
		if err != nil {
			return nil, err
		}
		event, err := repository.NewExecutionEventDraft(repository.ExecutionEventDraftParams{ID: eventID(record.WorkflowExecutionID(), sequence),
			WorkflowExecutionID: record.WorkflowExecutionID(), CompanyID: record.CompanyID(), NodeExecutionID: record.ID(),
			Type: repository.ExecutionEventTypeNodeCreated, NewStatus: execution.NodeExecutionStatusPending.String(), CorrelationID: observation.Request.CorrelationID(),
			SafeMessage: "Node execution created", Metadata: metadata, CreatedAt: record.CreatedAt(),
		})
		if err != nil {
			return nil, err
		}
		entry, err := repository.NewEventTimelineEntry(event)
		if err != nil {
			return nil, err
		}
		timeline = append(timeline, entry)
	}
	return timeline, nil
}

func buildNodeTransitionTimeline(
	observation engine.NodeTransitionObservation, firstSequence repository.SequenceNumber) (
	[]repository.TimelineEntry, repository.ExecutionEventID, error,
) {
	eventType, safeMessage, level, err := nodeTransitionDescription(
		observation.After.Status())
	if err != nil {
		return nil, "", err
	}
	metadataValue := map[string]any{"nodeId": observation.Definition.ID().String(), "pluginType": observation.Definition.PluginType().String(),
		"pluginVersion": observation.Definition.PluginVersion().String()}
	if observation.HasFailure {
		metadataValue["failureCategory"] = observation.Failure.Category().String()
		metadataValue["failureCode"] = observation.Failure.Code()
	}
	metadata, err := marshalJSONObject(metadataValue)
	if err != nil {
		return nil, "", err
	}
	eventIdentifier := eventID(observation.WorkflowExecution.ID(), firstSequence)
	event, err := repository.NewExecutionEventDraft(
		repository.ExecutionEventDraftParams{ID: eventIdentifier, WorkflowExecutionID: observation.WorkflowExecution.ID(),
			CompanyID: observation.WorkflowExecution.CompanyID(), NodeExecutionID: observation.After.ID(), Type: eventType,
			PreviousStatus: observation.Before.Status().String(), NewStatus: observation.After.Status().String(), CorrelationID: observation.Request.CorrelationID(),
			SafeMessage: safeMessage, Metadata: metadata, CreatedAt: observation.TransitionAt,
		})
	if err != nil {
		return nil, "", err
	}
	eventEntry, err := repository.NewEventTimelineEntry(event)
	if err != nil {
		return nil, "", err
	}
	logSequence, err := sequenceAt(firstSequence, 1)
	if err != nil {
		return nil, "", err
	}
	logDraft, err := repository.NewExecutionLogDraft(repository.ExecutionLogDraftParams{
		ID: logID(observation.WorkflowExecution.ID(), logSequence), WorkflowExecutionID: observation.WorkflowExecution.ID(), CompanyID: observation.WorkflowExecution.CompanyID(),
		NodeExecutionID: observation.After.ID(), Level: level, Message: safeMessage,
		Metadata: metadata, CreatedAt: observation.TransitionAt},
	)
	if err != nil {
		return nil, "", err
	}
	logEntry, err := repository.NewLogTimelineEntry(logDraft)
	if err != nil {
		return nil, "", err
	}
	return []repository.TimelineEntry{eventEntry,
			logEntry}, eventIdentifier,
		nil
}

func buildWorkflowTransitionErrors(observation engine.WorkflowTransitionObservation, eventID repository.ExecutionEventID,
) ([]repository.ExecutionErrorRecord, error) {
	if !observation.HasFailure {
		return nil, nil
	}
	record, err := buildExecutionError(
		observation.After.ID(), observation.After.CompanyID(), "",
		eventID, observation.Failure, observation.TechnicalDetail,
		observation.TransitionAt)
	if err != nil {
		return nil, err
	}
	return []repository.ExecutionErrorRecord{record}, nil
}

func buildNodeTransitionErrors(observation engine.NodeTransitionObservation, eventID repository.ExecutionEventID,
) ([]repository.ExecutionErrorRecord, error) {
	if !observation.HasFailure {
		return nil, nil
	}
	record, err := buildExecutionError(
		observation.WorkflowExecution.ID(), observation.WorkflowExecution.CompanyID(), observation.After.ID(),
		eventID, observation.Failure, observation.TechnicalDetail,
		observation.TransitionAt)
	if err != nil {
		return nil, err
	}
	return []repository.ExecutionErrorRecord{record}, nil
}

func buildExecutionError(workflowExecutionID execution.WorkflowExecutionID, companyID workflow.CompanyID,
	nodeExecutionID execution.NodeExecutionID, relatedEventID repository.ExecutionEventID, failure runtime.RuntimeFailure,
	technicalDetail string, createdAt time.Time) (repository.ExecutionErrorRecord, error) {
	if !failure.IsValid() {
		return repository.ExecutionErrorRecord{}, fmt.Errorf("runtime failure must be valid")
	}
	details, err := marshalFailureDetails(failure)
	if err != nil {
		return repository.ExecutionErrorRecord{}, err
	}
	return repository.NewExecutionErrorRecord(
		repository.ExecutionErrorRecordParams{ID: errorID(workflowExecutionID,
			relatedEventID), WorkflowExecutionID: workflowExecutionID,
			CompanyID: companyID, NodeExecutionID: nodeExecutionID, RelatedEventID: relatedEventID,
			Category: failure.Category(), Code: truncateRunes(failure.Code(),
				maximumPersistedErrorCodeCharacters), SafeMessage: truncateRunes(
				failure.Message(), maximumPersistedSafeMessageCharacters),
			TechnicalDetail: truncateRunes(technicalDetail, maximumPersistedTechnicalDetailCharacters), Retryable: failure.Retryable(), Details: details,
			CreatedAt: createdAt})
}

func workflowTransitionDescription(
	status execution.WorkflowExecutionStatus) (repository.ExecutionEventType,
	string, repository.ExecutionLogLevel, error,
) {
	switch status {
	case execution.WorkflowExecutionStatusValidating:
		return repository.ExecutionEventTypeWorkflowValidating, "Workflow validation started", repository.ExecutionLogLevelInfo,
			nil
	case execution.WorkflowExecutionStatusRejected:
		return repository.ExecutionEventTypeWorkflowRejected, "Workflow validation failed", repository.ExecutionLogLevelError,
			nil
	case execution.WorkflowExecutionStatusQueued:
		return repository.ExecutionEventTypeWorkflowQueued, "Workflow execution queued", repository.ExecutionLogLevelInfo,
			nil
	case execution.WorkflowExecutionStatusRunning:
		return repository.ExecutionEventTypeWorkflowStarted, "Workflow execution started", repository.ExecutionLogLevelInfo,
			nil
	case execution.WorkflowExecutionStatusSucceeded:
		return repository.ExecutionEventTypeWorkflowSucceeded, "Workflow execution succeeded", repository.ExecutionLogLevelInfo,
			nil
	case execution.WorkflowExecutionStatusFailed:
		return repository.ExecutionEventTypeWorkflowFailed, "Workflow execution failed", repository.ExecutionLogLevelError,
			nil
	case execution.WorkflowExecutionStatusCancelled:
		return repository.ExecutionEventTypeWorkflowCancelled, "Workflow execution was canceled", repository.ExecutionLogLevelWarn,
			nil
	case execution.WorkflowExecutionStatusTimedOut:
		return repository.ExecutionEventTypeWorkflowTimedOut, "Workflow execution timed out", repository.ExecutionLogLevelError,
			nil
	default:
		return "", "", "", fmt.Errorf("unsupported workflow transition status %s", status)
	}
}

func nodeTransitionDescription(status execution.NodeExecutionStatus,
) (repository.ExecutionEventType, string,
	repository.ExecutionLogLevel, error) {
	switch status {
	case execution.NodeExecutionStatusReady:
		return repository.ExecutionEventTypeNodeReady,
			"Node execution is ready", repository.ExecutionLogLevelInfo, nil
	case execution.NodeExecutionStatusQueued:
		return repository.ExecutionEventTypeNodeQueued,
			"Node execution queued", repository.ExecutionLogLevelInfo, nil
	case execution.NodeExecutionStatusRunning:
		return repository.ExecutionEventTypeNodeStarted,
			"Node execution started", repository.ExecutionLogLevelInfo, nil
	case execution.NodeExecutionStatusRetryPending:
		return repository.ExecutionEventTypeNodeRetryPending,
			"Node execution retry scheduled", repository.ExecutionLogLevelWarn, nil
	case execution.NodeExecutionStatusSucceeded:
		return repository.ExecutionEventTypeNodeSucceeded,
			"Node execution succeeded", repository.ExecutionLogLevelInfo, nil
	case execution.NodeExecutionStatusFailed:
		return repository.ExecutionEventTypeNodeFailed,
			"Node execution failed", repository.ExecutionLogLevelError, nil
	case execution.NodeExecutionStatusSkipped:
		return repository.ExecutionEventTypeNodeSkipped,
			"Node execution skipped", repository.ExecutionLogLevelWarn, nil
	case execution.NodeExecutionStatusCancelled:
		return repository.ExecutionEventTypeNodeCancelled,
			"Node execution was canceled", repository.ExecutionLogLevelWarn, nil
	case execution.NodeExecutionStatusTimedOut:
		return repository.ExecutionEventTypeNodeTimedOut,
			"Node execution timed out", repository.ExecutionLogLevelError, nil
	default:
		return "", "", "", fmt.Errorf(
			"unsupported node transition status %s", status)
	}
}

func eventID(workflowExecutionID execution.WorkflowExecutionID, sequence repository.SequenceNumber,
) repository.ExecutionEventID {
	return repository.ExecutionEventID(fmt.Sprintf(
		"%s/event/%020d", workflowExecutionID.String(), sequence.Int64(),
	))
}

func logID(workflowExecutionID execution.WorkflowExecutionID,
	sequence repository.SequenceNumber) repository.ExecutionLogID {
	return repository.ExecutionLogID(
		fmt.Sprintf("%s/log/%020d", workflowExecutionID.String(),
			sequence.Int64()))
}

func errorID(
	_ execution.WorkflowExecutionID, relatedEventID repository.ExecutionEventID) repository.ExecutionErrorID {
	return repository.ExecutionErrorID(relatedEventID.String() + "/error")
}

func sequenceAt(
	first repository.SequenceNumber, offset int) (repository.SequenceNumber, error) {
	if !first.IsValid() {
		return 0, fmt.Errorf("first sequence number must be valid")
	}
	if offset < 0 || first.Int64() > math.MaxInt64-int64(offset) {
		return 0, fmt.Errorf(
			"timeline sequence overflow")
	}
	return repository.NewSequenceNumber(first.Int64() + int64(offset))
}

func truncateRunes(value string, maximum int,
) string {
	if maximum <= 0 || utf8.RuneCountInString(value) <= maximum {
		return value
	}
	runes := []rune(value)
	return string(runes[:maximum])
}
