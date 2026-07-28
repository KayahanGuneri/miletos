package persistence

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"miletos-go/internal/engine"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

type workflowDefinitionDocument struct {
	ID        string                 `json:"id"`
	CompanyID string                 `json:"companyId"`
	Name      string                 `json:"name"`
	Revision  uint64                 `json:"revision"`
	Nodes     []workflowNodeDocument `json:"nodes"`
	Edges     []workflowEdgeDocument `json:"edges"`
	Metadata  json.RawMessage        `json:"metadata"`
}

type workflowNodeDocument struct {
	ID            string                    `json:"id"`
	PluginType    string                    `json:"pluginType"`
	PluginVersion string                    `json:"pluginVersion"`
	Configuration json.RawMessage           `json:"configuration"`
	Position      *workflowPositionDocument `json:"position,omitempty"`
}

type workflowPositionDocument struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type workflowEdgeDocument struct {
	ID               string `json:"id"`
	SourceNodeID     string `json:"sourceNodeId"`
	SourceOutputPort string `json:"sourceOutputPort"`
	TargetNodeID     string `json:"targetNodeId"`
	TargetInputPort  string `json:"targetInputPort"`
}

func validateCreationIdentity(request engine.ExecutionRequest,
	workflowExecution execution.WorkflowExecution) error {
	definition := request.Definition()
	if request.WorkflowExecutionID() != workflowExecution.ID() {
		return fmt.Errorf(
			"request and workflow execution IDs must match")
	}
	if definition.CompanyID() != workflowExecution.CompanyID() {
		return fmt.Errorf(
			"definition and workflow execution company IDs must match")
	}
	if definition.ID() != workflowExecution.WorkflowID() {
		return fmt.Errorf(
			"definition and workflow execution workflow IDs must match")
	}
	if definition.Revision() != workflowExecution.WorkflowRevision() {
		return fmt.Errorf("definition and workflow execution revisions must match")
	}
	if workflowExecution.Mode() != request.Mode() {
		return fmt.Errorf("request and workflow execution modes must match")
	}
	return nil
}

func buildDefinitionSnapshot(
	request engine.ExecutionRequest, workflowExecution execution.WorkflowExecution) (repository.DefinitionSnapshot, error) {
	definitionJSON, err := marshalWorkflowDefinition(request.Definition())
	if err != nil {
		return repository.DefinitionSnapshot{}, err
	}
	snapshotID := repository.DefinitionSnapshotID(workflowExecution.ID().String() +
		"/snapshot")
	return repository.NewDefinitionSnapshot(snapshotID, workflowExecution.CompanyID(),
		workflowExecution.WorkflowID(), workflowExecution.WorkflowRevision(), request.Definition().Name(),
		definitionJSON, workflowExecution.CreatedAt())
}

func marshalWorkflowDefinition(
	definition workflow.WorkflowDefinition) ([]byte, error) {
	nodes := definition.Nodes()
	nodeDocuments := make([]workflowNodeDocument, 0,
		len(nodes))
	for _, node := range nodes {
		var positionDocument *workflowPositionDocument
		if position, exists := node.Position(); exists {
			positionDocument = &workflowPositionDocument{X: position.X(),
				Y: position.Y()}
		}
		nodeDocuments = append(nodeDocuments,
			workflowNodeDocument{ID: node.ID().String(), PluginType: node.PluginType().String(),
				PluginVersion: node.PluginVersion().String(), Configuration: json.RawMessage(node.Configuration().Bytes()), Position: positionDocument},
		)
	}
	edges := definition.Edges()
	edgeDocuments := make([]workflowEdgeDocument,
		0, len(edges))
	for _, edge := range edges {
		edgeDocuments = append(
			edgeDocuments, workflowEdgeDocument{ID: edge.ID().String(),
				SourceNodeID: edge.SourceNodeID().String(), SourceOutputPort: edge.SourceOutputPort(), TargetNodeID: edge.TargetNodeID().String(),
				TargetInputPort: edge.TargetInputPort()})
	}
	return json.Marshal(
		workflowDefinitionDocument{ID: definition.ID().String(), CompanyID: definition.CompanyID().String(),
			Name: definition.Name(), Revision: definition.Revision(), Nodes: nodeDocuments,
			Edges: edgeDocuments, Metadata: json.RawMessage(definition.Metadata().Bytes())})
}

func buildCreatedWorkflowRecord(
	request engine.ExecutionRequest, workflowExecution execution.WorkflowExecution, snapshotID repository.DefinitionSnapshotID,
) (repository.WorkflowExecutionRecord, error) {
	return repository.NewWorkflowExecutionRecord(repository.WorkflowExecutionRecordParams{
		ID: workflowExecution.ID(), CompanyID: workflowExecution.CompanyID(), WorkflowID: workflowExecution.WorkflowID(),
		WorkflowRevision: workflowExecution.WorkflowRevision(), SnapshotID: snapshotID, Mode: workflowExecution.Mode(),
		CorrelationID: request.CorrelationID(), Status: execution.WorkflowExecutionStatusCreated, CreatedAt: workflowExecution.CreatedAt(),
		UpdatedAt: workflowExecution.CreatedAt(), TerminalOutputs: []byte(`{}`), NextSequenceNumber: repository.SequenceNumber(1),
		LockVersion: 0})
}

func validateWorkflowObservationIdentity(
	current repository.WorkflowExecutionRecord, request engine.ExecutionRequest, observed execution.WorkflowExecution,
) error {
	if !request.IsValid() {
		return fmt.Errorf(
			"execution request must be valid")
	}
	definition := request.Definition()
	if request.WorkflowExecutionID() != current.ID() || observed.ID() != current.ID() {
		return fmt.Errorf(
			"workflow execution IDs must match persistence state")
	}
	if definition.CompanyID() != current.CompanyID() || observed.CompanyID() != current.CompanyID() {
		return fmt.Errorf("workflow company IDs must match persistence state")
	}
	if definition.ID() != current.WorkflowID() ||
		observed.WorkflowID() != current.WorkflowID() {
		return fmt.Errorf("workflow definition IDs must match persistence state")
	}
	if definition.Revision() != current.WorkflowRevision() || observed.WorkflowRevision() != current.WorkflowRevision() {
		return fmt.Errorf(
			"workflow revisions must match persistence state")
	}
	if request.Mode() != current.Mode() {
		return fmt.Errorf(
			"execution request mode must match persistence state")
	}
	if observed.Mode() != current.Mode() {
		return fmt.Errorf(
			"observed workflow execution mode must match persistence state")
	}
	return nil
}

func validateWorkflowTransitionObservation(current repository.WorkflowExecutionRecord,
	observation engine.WorkflowTransitionObservation) error {
	if err := validateWorkflowObservationIdentity(
		current, observation.Request, observation.After,
	); err != nil {
		return err
	}
	if observation.TransitionAt.IsZero() {
		return fmt.Errorf("workflow transition time must not be zero")
	}
	if observation.Before.ID() != current.ID() || observation.After.ID() != current.ID() {
		return fmt.Errorf(
			"workflow transition execution IDs must match persistence state")
	}
	if observation.Before.CompanyID() != current.CompanyID() || observation.After.CompanyID() != current.CompanyID() {
		return fmt.Errorf("workflow transition company IDs must match persistence state")
	}
	if observation.Before.Status() != current.Status() {
		return fmt.Errorf("workflow transition previous status %s does not match persisted status %s", observation.Before.Status(),
			current.Status())
	}
	if !observation.Before.Status().CanTransitionTo(observation.After.Status()) {
		return fmt.Errorf("workflow transition from %s to %s is not allowed",
			observation.Before.Status(), observation.After.Status())
	}
	if observation.HasFailure &&
		!observation.Failure.IsValid() {
		return fmt.Errorf("workflow transition failure must be valid")
	}
	return nil
}

func buildTransitionedWorkflowRecord(current repository.WorkflowExecutionRecord, observation engine.WorkflowTransitionObservation,
	terminalOutputs map[string]payloadSummary, timelineLength int) (repository.WorkflowExecutionRecord, error) {
	params := workflowRecordParams(current)
	params.Status = observation.After.Status()
	params.UpdatedAt = observation.TransitionAt.UTC()
	params.LockVersion = current.LockVersion() + 1
	nextSequence, err := addSequence(
		current.NextSequenceNumber(), timelineLength)
	if err != nil {
		return repository.WorkflowExecutionRecord{}, err
	}
	params.NextSequenceNumber = nextSequence
	switch params.Status {
	case execution.WorkflowExecutionStatusValidating:
		params.ValidatingAt = observation.TransitionAt
	case execution.WorkflowExecutionStatusQueued:
		params.QueuedAt = observation.TransitionAt
	case execution.WorkflowExecutionStatusRunning:
		params.StartedAt = observation.TransitionAt
	case execution.WorkflowExecutionStatusRejected, execution.WorkflowExecutionStatusSucceeded, execution.WorkflowExecutionStatusFailed,
		execution.WorkflowExecutionStatusCancelled, execution.WorkflowExecutionStatusTimedOut:
		params.FinishedAt = observation.TransitionAt
	default:
		return repository.WorkflowExecutionRecord{}, fmt.Errorf(
			"unsupported workflow transition target %s", params.Status)
	}
	if params.Status.IsTerminal() {
		encodedOutputs, err := marshalTerminalOutputs(terminalOutputs)
		if err != nil {
			return repository.WorkflowExecutionRecord{}, fmt.Errorf(
				"marshal terminal outputs: %w", err)
		}
		params.TerminalOutputs = encodedOutputs
	}
	params.FailureSummary = nil
	params.IsStalled = observation.Stalled
	if observation.HasFailure {
		failureSummary, err := marshalFailureSummary(observation.Failure)
		if err != nil {
			return repository.WorkflowExecutionRecord{}, err
		}
		params.FailureSummary = failureSummary
	}
	return repository.NewWorkflowExecutionRecord(params)
}

func advanceWorkflowConcurrency(current repository.WorkflowExecutionRecord,
	timelineLength int, lockIncrement int64) (repository.WorkflowExecutionRecord, error) {
	if lockIncrement < 0 || current.LockVersion() > math.MaxInt64-lockIncrement {
		return repository.WorkflowExecutionRecord{}, fmt.Errorf(
			"workflow lock version cannot be advanced")
	}
	nextSequence, err := addSequence(current.NextSequenceNumber(),
		timelineLength)
	if err != nil {
		return repository.WorkflowExecutionRecord{}, err
	}
	params := workflowRecordParams(current)
	params.NextSequenceNumber = nextSequence
	params.LockVersion = current.LockVersion() + lockIncrement
	return repository.NewWorkflowExecutionRecord(params)
}

func buildNodeCreationRecordsAndTimeline(observation engine.NodeExecutionsCreationObservation,
	firstSequence repository.SequenceNumber) ([]repository.NodeExecutionRecord,
	[]repository.TimelineEntry, error) {
	if len(observation.Items) == 0 {
		return nil, nil,
			fmt.Errorf("node creation observation must contain at least one item")
	}
	nodeRecords := make(
		[]repository.NodeExecutionRecord, 0, len(observation.Items),
	)
	knownExecutionIDs := make(
		map[string]struct{}, len(observation.Items))
	for _, item := range observation.Items {
		if item.Execution.Status() !=
			execution.NodeExecutionStatusPending {
			return nil, nil,
				fmt.Errorf("initial node execution %s must be PENDING", item.Execution.ID())
		}
		if item.Execution.WorkflowExecutionID() != observation.WorkflowExecution.ID() {
			return nil,
				nil, fmt.Errorf("node execution %s belongs to another workflow execution",
					item.Execution.ID())
		}
		if item.Execution.NodeID() != item.Definition.ID() {
			return nil, nil, fmt.Errorf(
				"node execution and definition IDs must match")
		}
		key := item.Execution.ID().String()
		if _, exists := knownExecutionIDs[key]; exists {
			return nil, nil, fmt.Errorf(
				"duplicate node execution ID %s", item.Execution.ID())
		}
		knownExecutionIDs[key] = struct{}{}
		var retryPolicy execution.RetryPolicy
		if item.HasRetryPolicy {
			if !item.RetryPolicy.IsValid() {
				return nil, nil, fmt.Errorf(
					"node execution %s retry policy must be valid", item.Execution.ID())
			}
			retryPolicy = item.RetryPolicy
		}
		record, err := repository.NewNodeExecutionRecord(repository.NodeExecutionRecordParams{ID: item.Execution.ID(),
			WorkflowExecutionID: item.Execution.WorkflowExecutionID(), CompanyID: observation.WorkflowExecution.CompanyID(), NodeID: item.Definition.ID(),
			PluginType: item.Definition.PluginType(), PluginVersion: item.Definition.PluginVersion(), Status: execution.NodeExecutionStatusPending,
			Attempt: 1, RetryPolicy: retryPolicy,
			CreatedAt: item.Execution.CreatedAt(), UpdatedAt: item.Execution.CreatedAt(),
			LockVersion: 0})
		if err != nil {
			return nil, nil,
				fmt.Errorf("build node execution record %s: %w", item.Execution.ID(),
					err)
		}
		nodeRecords = append(nodeRecords,
			record)
	}
	timeline, err := buildNodeCreationTimeline(observation,
		nodeRecords, firstSequence)
	if err != nil {
		return nil, nil, err
	}
	return nodeRecords, timeline, nil
}

func validateNodeTransitionObservation(workflowRecord repository.WorkflowExecutionRecord,
	current repository.NodeExecutionRecord, observation engine.NodeTransitionObservation) error {
	if err := validateWorkflowObservationIdentity(workflowRecord, observation.Request,
		observation.WorkflowExecution); err != nil {
		return err
	}
	if observation.TransitionAt.IsZero() {
		return fmt.Errorf("node transition time must not be zero")
	}
	if observation.WorkflowExecution.ID() !=
		workflowRecord.ID() || observation.WorkflowExecution.CompanyID() != workflowRecord.CompanyID() {
		return fmt.Errorf("node transition workflow identity must match persistence state")
	}
	if observation.Before.ID() != current.ID() ||
		observation.After.ID() != current.ID() {
		return fmt.Errorf("node transition execution IDs must match persistence state")
	}
	if observation.Before.Status() != current.Status() {
		return fmt.Errorf("node transition previous status %s does not match persisted status %s",
			observation.Before.Status(), current.Status())
	}
	if observation.Definition.ID() != current.NodeID() ||
		observation.After.NodeID() != current.NodeID() {
		return fmt.Errorf("node transition node identity must match persistence state")
	}
	if !observation.Before.Status().CanTransitionTo(observation.After.Status()) {
		return fmt.Errorf("node transition from %s to %s is not allowed", observation.Before.Status(),
			observation.After.Status())
	}
	if observation.HasResult && !observation.Result.IsValid() {
		return fmt.Errorf("node transition result must be valid")
	}
	if observation.HasFailure &&
		!observation.Failure.IsValid() {
		return fmt.Errorf("node transition failure must be valid")
	}
	if observation.HasRetryDecision &&
		!observation.RetryDecision.IsValid() {
		return fmt.Errorf("node transition retry decision must be valid")
	}
	if observation.HasRetryDecision && !observation.HasFailure {
		return fmt.Errorf("node transition retry decision requires a failure")
	}
	if observation.After.Status() == execution.NodeExecutionStatusRetryPending {
		if observation.NextAttemptAt.IsZero() {
			return fmt.Errorf("retry-pending node transition must contain next attempt time")
		}
		if !observation.HasFailure {
			return fmt.Errorf("retry-pending node transition must contain a failure")
		}
		if !observation.HasRetryDecision ||
			observation.RetryDecision.Kind() != execution.RetryDecisionRetry {
			return fmt.Errorf("retry-pending node transition must contain a RETRY decision")
		}
	} else if !observation.NextAttemptAt.IsZero() {
		return fmt.Errorf("next attempt time must be absent unless node is RETRY_PENDING")
	}
	if observation.HasRetryDecision &&
		observation.After.Status() != execution.NodeExecutionStatusRetryPending &&
		observation.After.Status() != execution.NodeExecutionStatusFailed {
		return fmt.Errorf("retry decision is not supported for the node transition")
	}
	if observation.HasRetryDecision &&
		observation.After.Status() == execution.NodeExecutionStatusFailed &&
		observation.RetryDecision.Kind() == execution.RetryDecisionRetry {
		return fmt.Errorf("terminal failed node transition cannot contain a RETRY decision")
	}
	return nil
}

func buildTransitionedNodeRecord(current repository.NodeExecutionRecord, observation engine.NodeTransitionObservation,
) (repository.NodeExecutionRecord, payloadSummary,
	bool, error) {
	params := nodeRecordParams(current)
	params.Status = observation.After.Status()
	params.UpdatedAt = observation.TransitionAt.UTC()
	params.LockVersion = current.LockVersion() + 1
	params.OutputSummary = nil
	params.FailureSummary = nil
	switch params.Status {
	case execution.NodeExecutionStatusReady:
		params.ReadyAt = observation.TransitionAt
	case execution.NodeExecutionStatusQueued:
		params.QueuedAt = observation.TransitionAt
	case execution.NodeExecutionStatusRunning:
		if current.Status() == execution.NodeExecutionStatusRetryPending {
			if current.Attempt() == math.MaxInt16 {
				return repository.NodeExecutionRecord{},
					payloadSummary{}, false, fmt.Errorf(
						"node execution attempt cannot be incremented")
			}
			params.Attempt = current.Attempt() + 1
			params.NextAttemptAt = time.Time{}
		} else {
			params.StartedAt = observation.TransitionAt
		}
	case execution.NodeExecutionStatusRetryPending:
		params.NextAttemptAt = observation.NextAttemptAt
	case execution.NodeExecutionStatusSucceeded, execution.NodeExecutionStatusFailed, execution.NodeExecutionStatusSkipped,
		execution.NodeExecutionStatusCancelled, execution.NodeExecutionStatusTimedOut:
		params.FinishedAt = observation.TransitionAt
		params.NextAttemptAt = time.Time{}
	default:
		return repository.NodeExecutionRecord{},
			payloadSummary{}, false, fmt.Errorf(
				"unsupported node transition target %s", params.Status)
	}
	var terminalOutput payloadSummary
	hasTerminalOutput := false
	if params.Status ==
		execution.NodeExecutionStatusSucceeded && observation.HasResult {
		resultSummary, value, exists, err :=
			summarizeNodeResult(observation.Result)
		if err != nil {
			return repository.NodeExecutionRecord{}, payloadSummary{},
				false, err
		}
		params.OutputSummary = resultSummary
		terminalOutput = value
		hasTerminalOutput = exists
	}
	if observation.HasFailure {
		failureSummary, err := marshalFailureSummary(observation.Failure)
		if err != nil {
			return repository.NodeExecutionRecord{},
				payloadSummary{}, false, err
		}
		params.FailureSummary = failureSummary
	}
	updated, err := repository.NewNodeExecutionRecord(params)
	if err != nil {
		return repository.NodeExecutionRecord{}, payloadSummary{},
			false, err
	}
	return updated, terminalOutput,
		hasTerminalOutput, nil
}

func buildNodeAttemptMutation(
	mode execution.ExecutionMode,
	observation engine.NodeTransitionObservation,
	current repository.NodeExecutionRecord,
	updated repository.NodeExecutionRecord,
) (repository.NodeAttemptMutation, bool, error) {
	switch mode {
	case execution.ExecutionModeSync:
	case execution.ExecutionModeAsync:
		return repository.NodeAttemptMutation{}, false, nil
	default:
		return repository.NodeAttemptMutation{}, false,
			fmt.Errorf("workflow execution mode must be supported")
	}
	transitionAt := observation.TransitionAt.UTC()
	if updated.Status() == execution.NodeExecutionStatusRunning {
		switch current.Status() {
		case execution.NodeExecutionStatusReady,
			execution.NodeExecutionStatusQueued,
			execution.NodeExecutionStatusRetryPending:
			attempt, err := execution.NewAttemptNumber(updated.Attempt())
			if err != nil {
				return repository.NodeAttemptMutation{}, false, err
			}
			record, err := repository.NewNodeExecutionAttemptRecord(
				repository.NodeExecutionAttemptRecordParams{
					CompanyID:           updated.CompanyID(),
					WorkflowExecutionID: updated.WorkflowExecutionID(),
					NodeExecutionID:     updated.ID(),
					Attempt:             attempt,
					Status:              execution.NodeExecutionStatusRunning,
					StartedAt:           transitionAt,
					CreatedAt:           transitionAt,
					UpdatedAt:           transitionAt,
				},
			)
			if err != nil {
				return repository.NodeAttemptMutation{}, false,
					fmt.Errorf("build running node attempt record: %w", err)
			}
			mutation, err := repository.NewNodeAttemptStartMutation(record)
			if err != nil {
				return repository.NodeAttemptMutation{}, false,
					fmt.Errorf("build node attempt START mutation: %w", err)
			}
			return mutation, true, nil
		default:
			return repository.NodeAttemptMutation{}, false, nil
		}
	}
	if current.Status() != execution.NodeExecutionStatusRunning {
		return repository.NodeAttemptMutation{}, false, nil
	}
	attemptStatus, completesAttempt := nodeAttemptCompletionStatus(
		updated.Status(), observation,
	)
	if !completesAttempt {
		return repository.NodeAttemptMutation{}, false, nil
	}
	attempt, err := execution.NewAttemptNumber(current.Attempt())
	if err != nil {
		return repository.NodeAttemptMutation{}, false, err
	}
	outputSummary := optionalSummaryBytes(updated.OutputSummary)
	failureSummary := optionalSummaryBytes(updated.FailureSummary)
	retryDecision := execution.RetryDecision{}
	if observation.HasRetryDecision {
		retryDecision = observation.RetryDecision
	}
	nextAttemptAt, _ := updated.NextAttemptAt()
	completion, err := repository.NewNodeAttemptCompletion(
		repository.NodeAttemptCompletionParams{
			CompanyID:           updated.CompanyID(),
			WorkflowExecutionID: updated.WorkflowExecutionID(),
			NodeExecutionID:     updated.ID(),
			ExpectedAttempt:     attempt,
			Status:              attemptStatus,
			FinishedAt:          transitionAt,
			OutputSummary:       outputSummary,
			FailureSummary:      failureSummary,
			RetryDecision:       retryDecision,
			NextAttemptAt:       nextAttemptAt,
		},
	)
	if err != nil {
		return repository.NodeAttemptMutation{}, false,
			fmt.Errorf("build node attempt completion: %w", err)
	}
	mutation, err := repository.NewNodeAttemptCompleteMutation(completion)
	if err != nil {
		return repository.NodeAttemptMutation{}, false,
			fmt.Errorf("build node attempt COMPLETE mutation: %w", err)
	}
	return mutation, true, nil
}

func nodeAttemptCompletionStatus(
	target execution.NodeExecutionStatus,
	observation engine.NodeTransitionObservation,
) (execution.NodeExecutionStatus, bool) {
	if target == execution.NodeExecutionStatusRetryPending {
		if observation.HasFailure &&
			observation.Failure.Category() == runtime.FailureCategoryTimeout {
			return execution.NodeExecutionStatusTimedOut, true
		}
		return execution.NodeExecutionStatusFailed, true
	}
	switch target {
	case execution.NodeExecutionStatusSucceeded,
		execution.NodeExecutionStatusFailed,
		execution.NodeExecutionStatusCancelled,
		execution.NodeExecutionStatusTimedOut:
		return target, true
	default:
		return "", false
	}
}

func optionalSummaryBytes(
	getter func() (repository.JSONObject, bool),
) []byte {
	value, exists := getter()
	if !exists {
		return nil
	}
	return value.Bytes()
}

func workflowRecordParams(record repository.WorkflowExecutionRecord,
) repository.WorkflowExecutionRecordParams {
	correlationID, _ := record.CorrelationID()
	validatingAt, _ := record.ValidatingAt()
	queuedAt, _ := record.QueuedAt()
	startedAt, _ := record.StartedAt()
	finishedAt, _ := record.FinishedAt()
	var failureSummary []byte
	if value, exists := record.FailureSummary(); exists {
		failureSummary = value.Bytes()
	}
	return repository.WorkflowExecutionRecordParams{ID: record.ID(), CompanyID: record.CompanyID(),
		WorkflowID: record.WorkflowID(), WorkflowRevision: record.WorkflowRevision(), SnapshotID: record.SnapshotID(),
		Mode: record.Mode(), CorrelationID: correlationID, Status: record.Status(),
		CreatedAt: record.CreatedAt(), ValidatingAt: validatingAt, QueuedAt: queuedAt,
		StartedAt: startedAt, FinishedAt: finishedAt, UpdatedAt: record.UpdatedAt(),
		TerminalOutputs: record.TerminalOutputs().Bytes(), FailureSummary: failureSummary, IsStalled: record.IsStalled(),
		NextSequenceNumber: record.NextSequenceNumber(), LockVersion: record.LockVersion()}
}

func nodeRecordParams(
	record repository.NodeExecutionRecord) repository.NodeExecutionRecordParams {
	retryPolicy, _ := record.RetryPolicy()
	nextAttemptAt, _ := record.NextAttemptAt()
	readyAt, _ := record.ReadyAt()
	queuedAt, _ := record.QueuedAt()
	startedAt, _ := record.StartedAt()
	finishedAt, _ := record.FinishedAt()
	var inputSummary []byte
	if value, exists := record.InputSummary(); exists {
		inputSummary = value.Bytes()
	}
	var outputSummary []byte
	if value, exists := record.OutputSummary(); exists {
		outputSummary = value.Bytes()
	}
	var failureSummary []byte
	if value, exists := record.FailureSummary(); exists {
		failureSummary = value.Bytes()
	}
	return repository.NodeExecutionRecordParams{ID: record.ID(),
		WorkflowExecutionID: record.WorkflowExecutionID(), CompanyID: record.CompanyID(), NodeID: record.NodeID(),
		PluginType: record.PluginType(), PluginVersion: record.PluginVersion(), Status: record.Status(),
		Attempt: record.Attempt(), RetryPolicy: retryPolicy, NextAttemptAt: nextAttemptAt,
		CreatedAt: record.CreatedAt(), ReadyAt: readyAt,
		QueuedAt: queuedAt, StartedAt: startedAt, FinishedAt: finishedAt,
		UpdatedAt: record.UpdatedAt(), InputSummary: inputSummary, OutputSummary: outputSummary,
		FailureSummary: failureSummary, LockVersion: record.LockVersion()}
}

func addSequence(
	current repository.SequenceNumber, increment int) (repository.SequenceNumber, error) {
	if !current.IsValid() {
		return 0, fmt.Errorf("current sequence number must be valid")
	}
	if increment <= 0 {
		return 0, fmt.Errorf("sequence increment must be greater than zero")
	}
	if current.Int64() > math.MaxInt64-int64(increment) {
		return 0, fmt.Errorf(
			"sequence number overflow")
	}
	return repository.NewSequenceNumber(current.Int64() + int64(increment))
}
