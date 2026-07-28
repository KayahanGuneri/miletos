package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"miletos-go/internal/engine/graph"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"miletos-go/internal/ports/messaging"
	repository "miletos-go/internal/ports/persistence"
	sharedclock "miletos-go/internal/shared/clock"
)

var (
	ErrPartialRecoveryUnavailable = errors.New("partial recovery is unavailable")
	ErrPartialRecoveryUnsupported = errors.New("partial recovery is unsupported for the source execution")
	ErrPartialRecoveryUnsafe      = errors.New("partial recovery cannot preserve execution invariants")
	ErrPartialRecoveryKeyReused   = errors.New("partial recovery idempotency key was reused")
)

type PartialRecoveryPersistence interface {
	repository.WorkflowExecutionReader
	repository.NodeExecutionReader
	repository.WorkflowSnapshotRepository
	repository.AsyncContextStore
	repository.AsyncInputStore
	repository.PartialRecoveryStore
}

type PartialRecoveryOutcome struct {
	SourceExecutionID   execution.WorkflowExecutionID
	RecoveryExecutionID execution.WorkflowExecutionID
	Status              execution.WorkflowExecutionStatus
	PreservedNodeCount  int
	ScheduledNodeCount  int
	ResetNodeCount      int
	CreatedAt           time.Time
}

type PartialRecoveryService struct {
	persistence PartialRecoveryPersistence
	coordinator AsyncCoordinator
	clock       sharedclock.Clock
}

func NewPartialRecoveryService(
	persistence PartialRecoveryPersistence,
	coordinator AsyncCoordinator,
	clock sharedclock.Clock,
) (PartialRecoveryService, error) {
	if persistence == nil || coordinator.Codec == nil ||
		coordinator.Topic == "" || clock == nil {
		return PartialRecoveryService{}, fmt.Errorf("partial recovery dependencies must be valid")
	}
	return PartialRecoveryService{
		persistence: persistence,
		coordinator: coordinator,
		clock:       clock,
	}, nil
}

func (service PartialRecoveryService) IsValid() bool {
	return service.persistence != nil && service.coordinator.Codec != nil &&
		service.coordinator.Topic != "" && service.clock != nil
}

func (service PartialRecoveryService) Recover(
	ctx context.Context,
	companyID workflow.CompanyID,
	sourceExecutionID execution.WorkflowExecutionID,
	idempotencyKey string,
	requestFingerprint string,
) (PartialRecoveryOutcome, bool, error) {
	if ctx == nil {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("partial recovery context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	if !service.IsValid() {
		return PartialRecoveryOutcome{}, false, ErrPartialRecoveryUnavailable
	}
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("partial recovery company ID: %w", err)
	}
	normalizedSourceID, err := execution.NewWorkflowExecutionID(sourceExecutionID.String())
	if err != nil {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("partial recovery source execution ID: %w", err)
	}
	if idempotencyKey == "" || requestFingerprint == "" {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("partial recovery idempotency data must not be empty")
	}

	source, err := service.persistence.GetWorkflowExecution(
		ctx, normalizedCompanyID, normalizedSourceID)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	if source.Mode() != execution.ExecutionModeAsync ||
		(source.Status() != execution.WorkflowExecutionStatusFailed &&
			source.Status() != execution.WorkflowExecutionStatusTimedOut) {
		return PartialRecoveryOutcome{}, false, ErrPartialRecoveryUnsupported
	}
	snapshot, err := service.persistence.GetByID(ctx, normalizedCompanyID, source.SnapshotID())
	if err != nil {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("load partial recovery snapshot: %w", err)
	}
	definition, err := workflow.DecodePersistedDefinition(snapshot.DefinitionJSON().Bytes())
	if err != nil {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("decode partial recovery snapshot: %w", err)
	}
	if definition.CompanyID() != normalizedCompanyID ||
		definition.ID() != source.WorkflowID() ||
		definition.Revision() != source.WorkflowRevision() {
		return PartialRecoveryOutcome{}, false, ErrPartialRecoveryUnsafe
	}
	builtGraph, validation := graph.BuildValidated(definition)
	if !validation.IsValid() {
		return PartialRecoveryOutcome{}, false, ErrPartialRecoveryUnsafe
	}
	topologicalOrder, available := builtGraph.TopologicalOrder()
	if !available {
		return PartialRecoveryOutcome{}, false, ErrPartialRecoveryUnsafe
	}
	sourceNodes, err := service.listAllNodeExecutions(
		ctx, normalizedCompanyID, normalizedSourceID)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	sourceByNode, downstream, err := validatePartialRecoverySource(
		builtGraph, topologicalOrder, sourceNodes, normalizedCompanyID, normalizedSourceID)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}

	recoveryAt := service.clock.Now().UTC()
	if recoveryAt.IsZero() {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("partial recovery clock returned zero time")
	}
	recoveryExecutionID := deterministicRecoveryExecutionID(
		normalizedCompanyID, normalizedSourceID, idempotencyKey)
	correlationID, exists := source.CorrelationID()
	if !exists {
		return PartialRecoveryOutcome{}, false, ErrPartialRecoveryUnsafe
	}
	recoveryExecution, err := repository.NewWorkflowExecutionRecord(
		repository.WorkflowExecutionRecordParams{
			ID: recoveryExecutionID, CompanyID: normalizedCompanyID,
			WorkflowID: source.WorkflowID(), WorkflowRevision: source.WorkflowRevision(),
			SnapshotID: source.SnapshotID(), Mode: execution.ExecutionModeAsync,
			CorrelationID: correlationID, Status: execution.WorkflowExecutionStatusRunning,
			CreatedAt: recoveryAt, ValidatingAt: recoveryAt, QueuedAt: recoveryAt,
			StartedAt: recoveryAt, UpdatedAt: recoveryAt, TerminalOutputs: []byte("{}"),
			NextSequenceNumber: repository.SequenceNumber(1), LockVersion: 0,
		})
	if err != nil {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("create recovery workflow record: %w", err)
	}

	nodePlans, planByNode, err := buildRecoveryNodePlans(
		recoveryExecution, topologicalOrder, sourceByNode, downstream, recoveryAt)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	contextVariables, err := service.copyRecoveryContext(
		ctx, normalizedCompanyID, normalizedSourceID, recoveryExecutionID, recoveryAt)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	nodeInputs, err := service.copyRecoveryInputs(
		ctx, builtGraph, normalizedCompanyID, normalizedSourceID, recoveryExecutionID,
		nodePlans, planByNode, recoveryAt)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	outboxes, err := service.buildRecoveryOutboxes(
		definition, recoveryExecution, nodePlans, correlationID, normalizedSourceID, recoveryAt)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	timeline, err := buildRecoveryTimeline(recoveryExecution, normalizedSourceID, recoveryAt)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	planJSON, err := marshalRecoveryPlan(normalizedSourceID, nodePlans)
	if err != nil {
		return PartialRecoveryOutcome{}, false, err
	}
	command, err := repository.NewPartialRecoveryCommand(
		repository.PartialRecoveryCommandParams{
			CompanyID: normalizedCompanyID, IdempotencyKey: idempotencyKey,
			RequestFingerprint:        requestFingerprint,
			SourceWorkflowExecutionID: normalizedSourceID,
			ExpectedSourceStatus:      source.Status(),
			ExpectedSourceLockVersion: source.LockVersion(),
			RecoveryWorkflowExecution: recoveryExecution,
			NodePlans:                 nodePlans, ContextVariables: contextVariables,
			NodeInputs: nodeInputs, OutboxMessages: outboxes,
			Timeline: timeline, RecoveryPlan: planJSON, CreatedAt: recoveryAt,
		})
	if err != nil {
		return PartialRecoveryOutcome{}, false, fmt.Errorf("create partial recovery command: %w", err)
	}
	record, created, err := service.persistence.CreatePartialRecovery(ctx, command)
	if err != nil {
		if repository.IsStaleWrite(err) || repository.IsConflict(err) {
			return PartialRecoveryOutcome{}, false, ErrPartialRecoveryUnsafe
		}
		return PartialRecoveryOutcome{}, false, err
	}
	if record.RequestFingerprint() != requestFingerprint ||
		record.SourceWorkflowExecutionID() != normalizedSourceID {
		return PartialRecoveryOutcome{}, false, ErrPartialRecoveryKeyReused
	}
	return partialRecoveryOutcome(record), !created, nil
}

func (service PartialRecoveryService) listAllNodeExecutions(
	ctx context.Context,
	companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID,
) ([]repository.NodeExecutionRecord, error) {
	request, err := repository.NewPageRequest(repository.MaximumPageLimit, "")
	if err != nil {
		return nil, err
	}
	records := make([]repository.NodeExecutionRecord, 0)
	for {
		page, err := service.persistence.ListNodeExecutions(
			ctx, companyID, workflowExecutionID, request)
		if err != nil {
			return nil, fmt.Errorf("list partial recovery node executions: %w", err)
		}
		records = append(records, page.Items()...)
		next, exists := page.Next()
		if !exists {
			return records, nil
		}
		request, err = repository.NewPageRequest(repository.MaximumPageLimit, next)
		if err != nil {
			return nil, err
		}
	}
}

func validatePartialRecoverySource(
	builtGraph graph.Graph,
	topologicalOrder []workflow.NodeID,
	records []repository.NodeExecutionRecord,
	companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID,
) (map[workflow.NodeID]repository.NodeExecutionRecord, map[workflow.NodeID]bool, error) {
	if len(records) != len(topologicalOrder) {
		return nil, nil, ErrPartialRecoveryUnsafe
	}
	byNode := make(map[workflow.NodeID]repository.NodeExecutionRecord, len(records))
	for _, record := range records {
		nodeDefinition, exists := builtGraph.Node(record.NodeID())
		if !record.IsValid() ||
			record.CompanyID() != companyID ||
			record.WorkflowExecutionID() != workflowExecutionID ||
			!exists ||
			record.PluginType() != nodeDefinition.PluginType() ||
			record.PluginVersion() != nodeDefinition.PluginVersion() {
			return nil, nil, ErrPartialRecoveryUnsafe
		}
		if _, exists := byNode[record.NodeID()]; exists {
			return nil, nil, ErrPartialRecoveryUnsafe
		}
		switch record.Status() {
		case execution.NodeExecutionStatusSucceeded,
			execution.NodeExecutionStatusFailed,
			execution.NodeExecutionStatusTimedOut,
			execution.NodeExecutionStatusSkipped:
		default:
			return nil, nil, ErrPartialRecoveryUnsupported
		}
		byNode[record.NodeID()] = record
	}
	downstream := make(map[workflow.NodeID]bool, len(records))
	boundaryCount := 0
	for _, nodeID := range topologicalOrder {
		record, exists := byNode[nodeID]
		if !exists {
			return nil, nil, ErrPartialRecoveryUnsafe
		}
		for _, incoming := range builtGraph.IncomingEdges(nodeID) {
			source := byNode[incoming.SourceNodeID()]
			if source.Status() == execution.NodeExecutionStatusFailed ||
				source.Status() == execution.NodeExecutionStatusTimedOut ||
				downstream[incoming.SourceNodeID()] {
				downstream[nodeID] = true
			}
		}
		switch record.Status() {
		case execution.NodeExecutionStatusSucceeded:
			if downstream[nodeID] {
				return nil, nil, ErrPartialRecoveryUnsafe
			}
		case execution.NodeExecutionStatusFailed, execution.NodeExecutionStatusTimedOut:
			if downstream[nodeID] {
				return nil, nil, ErrPartialRecoveryUnsafe
			}
			for _, incoming := range builtGraph.IncomingEdges(nodeID) {
				if byNode[incoming.SourceNodeID()].Status() != execution.NodeExecutionStatusSucceeded {
					return nil, nil, ErrPartialRecoveryUnsafe
				}
			}
			boundaryCount++
		case execution.NodeExecutionStatusSkipped:
			if !downstream[nodeID] {
				return nil, nil, ErrPartialRecoveryUnsafe
			}
		}
	}
	if boundaryCount == 0 {
		return nil, nil, ErrPartialRecoveryUnsupported
	}
	return byNode, downstream, nil
}

func buildRecoveryNodePlans(
	recovery repository.WorkflowExecutionRecord,
	topologicalOrder []workflow.NodeID,
	sourceByNode map[workflow.NodeID]repository.NodeExecutionRecord,
	downstream map[workflow.NodeID]bool,
	recoveryAt time.Time,
) ([]repository.RecoveryNodePlan, map[workflow.NodeID]repository.RecoveryNodePlan, error) {
	plans := make([]repository.RecoveryNodePlan, 0, len(topologicalOrder))
	byNode := make(map[workflow.NodeID]repository.RecoveryNodePlan, len(topologicalOrder))
	for _, nodeID := range topologicalOrder {
		source := sourceByNode[nodeID]
		params := repository.NodeExecutionRecordParams{
			ID:                  execution.NodeExecutionID(fmt.Sprintf("%s/node/%s", recovery.ID(), nodeID)),
			WorkflowExecutionID: recovery.ID(), CompanyID: recovery.CompanyID(),
			NodeID: nodeID, PluginType: source.PluginType(), PluginVersion: source.PluginVersion(),
			Attempt: 1, CreatedAt: recoveryAt, UpdatedAt: recoveryAt, LockVersion: 0,
		}
		if retryPolicy, exists := source.RetryPolicy(); exists {
			params.RetryPolicy = retryPolicy
		}
		var disposition repository.RecoveryNodeDisposition
		switch source.Status() {
		case execution.NodeExecutionStatusSucceeded:
			params.Status = execution.NodeExecutionStatusSucceeded
			params.Attempt = source.Attempt()
			params.ReadyAt = recoveryAt
			params.StartedAt = recoveryAt
			params.FinishedAt = recoveryAt
			if summary, exists := source.InputSummary(); exists {
				params.InputSummary = summary.Bytes()
			}
			if summary, exists := source.OutputSummary(); exists {
				params.OutputSummary = summary.Bytes()
			}
			disposition = repository.RecoveryNodePreserved
		case execution.NodeExecutionStatusFailed, execution.NodeExecutionStatusTimedOut:
			params.Status = execution.NodeExecutionStatusQueued
			params.ReadyAt = recoveryAt
			params.QueuedAt = recoveryAt
			disposition = repository.RecoveryNodeScheduled
		case execution.NodeExecutionStatusSkipped:
			if !downstream[nodeID] {
				return nil, nil, ErrPartialRecoveryUnsafe
			}
			params.Status = execution.NodeExecutionStatusPending
			disposition = repository.RecoveryNodeReset
		default:
			return nil, nil, ErrPartialRecoveryUnsupported
		}
		record, err := repository.NewNodeExecutionRecord(params)
		if err != nil {
			return nil, nil, fmt.Errorf("create recovery node %s: %w", nodeID, err)
		}
		plan, err := repository.NewRecoveryNodePlan(
			source.ID(), record, source.Status(), source.Attempt(), disposition)
		if err != nil {
			return nil, nil, err
		}
		plans = append(plans, plan)
		byNode[nodeID] = plan
	}
	return plans, byNode, nil
}

func (service PartialRecoveryService) copyRecoveryContext(
	ctx context.Context,
	companyID workflow.CompanyID,
	sourceExecutionID execution.WorkflowExecutionID,
	recoveryExecutionID execution.WorkflowExecutionID,
	recoveryAt time.Time,
) ([]repository.AsyncContextVariable, error) {
	sourceVariables, err := service.persistence.ListAsyncContextVariables(
		ctx, companyID, sourceExecutionID)
	if err != nil {
		return nil, fmt.Errorf("list partial recovery context: %w", err)
	}
	result := make([]repository.AsyncContextVariable, 0, len(sourceVariables))
	for _, source := range sourceVariables {
		variable, err := repository.NewAsyncContextVariable(
			repository.AsyncContextVariableParams{
				CompanyID: companyID, WorkflowExecutionID: recoveryExecutionID,
				Key: source.Key(), Value: source.Value(),
				CreatedAt: recoveryAt, UpdatedAt: recoveryAt, Version: 1,
			})
		if err != nil {
			return nil, fmt.Errorf("copy partial recovery context: %w", err)
		}
		result = append(result, variable)
	}
	return result, nil
}

func (service PartialRecoveryService) copyRecoveryInputs(
	ctx context.Context,
	builtGraph graph.Graph,
	companyID workflow.CompanyID,
	sourceExecutionID execution.WorkflowExecutionID,
	recoveryExecutionID execution.WorkflowExecutionID,
	plans []repository.RecoveryNodePlan,
	planByNode map[workflow.NodeID]repository.RecoveryNodePlan,
	recoveryAt time.Time,
) ([]repository.AsyncNodeInput, error) {
	result := make([]repository.AsyncNodeInput, 0)
	for _, targetPlan := range plans {
		if targetPlan.Disposition() == repository.RecoveryNodePreserved {
			continue
		}
		inputs, err := service.persistence.ListAsyncNodeInputs(
			ctx, companyID, sourceExecutionID, targetPlan.SourceNodeExecutionID())
		if err != nil {
			return nil, fmt.Errorf("list partial recovery inputs for %s: %w",
				targetPlan.NodeExecution().NodeID(), err)
		}
		for _, sourceInput := range inputs {
			sourcePlan, exists := planByNode[sourceInput.SourceNodeID()]
			if !exists || !isSafePersistedRecoveryInput(
				builtGraph, companyID, sourceExecutionID,
				sourceInput, sourcePlan, targetPlan) {
				return nil, ErrPartialRecoveryUnsafe
			}
			copied, err := repository.NewAsyncNodeInput(repository.AsyncNodeInputParams{
				CompanyID: companyID, WorkflowExecutionID: recoveryExecutionID,
				TargetNodeExecutionID: targetPlan.NodeExecution().ID(),
				SourceNodeExecutionID: sourcePlan.NodeExecution().ID(),
				TargetNodeID:          sourceInput.TargetNodeID(),
				SourceNodeID:          sourceInput.SourceNodeID(), EdgeID: sourceInput.EdgeID(),
				SourceOutputPort: sourceInput.SourceOutputPort(),
				TargetInputPort:  sourceInput.TargetInputPort(),
				SourceAttempt:    sourceInput.SourceAttempt(),
				Payload:          sourceInput.Payload(), CreatedAt: recoveryAt,
			})
			if err != nil {
				return nil, fmt.Errorf("copy partial recovery input: %w", err)
			}
			result = append(result, copied)
		}
	}
	return result, nil
}

func isSafePersistedRecoveryInput(
	builtGraph graph.Graph,
	companyID workflow.CompanyID,
	sourceExecutionID execution.WorkflowExecutionID,
	sourceInput repository.AsyncNodeInput,
	sourcePlan repository.RecoveryNodePlan,
	targetPlan repository.RecoveryNodePlan,
) bool {
	if !sourceInput.IsValid() ||
		sourceInput.CompanyID() != companyID ||
		sourceInput.WorkflowExecutionID() != sourceExecutionID ||
		targetPlan.Disposition() == repository.RecoveryNodePreserved ||
		sourceInput.TargetNodeExecutionID() != targetPlan.SourceNodeExecutionID() ||
		sourcePlan.Disposition() != repository.RecoveryNodePreserved ||
		sourceInput.SourceNodeExecutionID() != sourcePlan.SourceNodeExecutionID() ||
		sourceInput.SourceNodeID() != sourcePlan.NodeExecution().NodeID() ||
		sourceInput.TargetNodeID() != targetPlan.NodeExecution().NodeID() ||
		sourceInput.SourceAttempt() != sourcePlan.NodeExecution().Attempt() {
		return false
	}
	edge, exists := builtGraph.Edge(sourceInput.EdgeID())
	return exists &&
		edge.ID() == sourceInput.EdgeID() &&
		edge.SourceNodeID() == sourceInput.SourceNodeID() &&
		edge.TargetNodeID() == sourceInput.TargetNodeID() &&
		edge.SourceOutputPort() == sourceInput.SourceOutputPort() &&
		edge.TargetInputPort() == sourceInput.TargetInputPort()
}

func (service PartialRecoveryService) buildRecoveryOutboxes(
	definition workflow.WorkflowDefinition,
	recovery repository.WorkflowExecutionRecord,
	plans []repository.RecoveryNodePlan,
	correlationID string,
	sourceExecutionID execution.WorkflowExecutionID,
	recoveryAt time.Time,
) ([]repository.OutboxMessage, error) {
	nodeDefinitions := make(map[workflow.NodeID]workflow.NodeDefinition, len(definition.Nodes()))
	for _, node := range definition.Nodes() {
		nodeDefinitions[node.ID()] = node
	}
	input, err := runtime.NewNodeInput(nil)
	if err != nil {
		return nil, err
	}
	causationID := deterministicRecoveryCausationID(sourceExecutionID)
	outboxes := make([]repository.OutboxMessage, 0)
	for _, plan := range plans {
		if plan.Disposition() != repository.RecoveryNodeScheduled {
			continue
		}
		node := plan.NodeExecution()
		definition, exists := nodeDefinitions[node.NodeID()]
		if !exists {
			return nil, ErrPartialRecoveryUnsafe
		}
		metadata, err := messaging.NewMessageMetadata(messaging.MessageMetadataParams{
			MessageID: asyncNodeCommandMessageID(recovery.ID(), node.ID(), node.Attempt()),
			CreatedAt: recoveryAt, CompanyID: recovery.CompanyID(),
			WorkflowID: recovery.WorkflowID(), WorkflowExecutionID: recovery.ID(),
			NodeID: node.NodeID(), NodeExecutionID: node.ID(), Attempt: node.Attempt(),
			CorrelationID: correlationID, CausationID: causationID,
		})
		if err != nil {
			return nil, err
		}
		command, err := messaging.NewNodeCommand(metadata, definition, input, nil, nil)
		if err != nil {
			return nil, err
		}
		message, err := service.coordinator.BuildNodeCommandOutbox(command)
		if err != nil {
			return nil, err
		}
		outboxes = append(outboxes, message)
	}
	return outboxes, nil
}

func buildRecoveryTimeline(
	recovery repository.WorkflowExecutionRecord,
	sourceExecutionID execution.WorkflowExecutionID,
	recoveryAt time.Time,
) ([]repository.TimelineEntry, error) {
	metadata, err := json.Marshal(map[string]any{
		"recoveryKind":      "PARTIAL",
		"sourceExecutionId": sourceExecutionID.String(),
	})
	if err != nil {
		return nil, err
	}
	correlationID, _ := recovery.CorrelationID()
	event, err := repository.NewExecutionEventDraft(repository.ExecutionEventDraftParams{
		ID:                  repository.ExecutionEventID(fmt.Sprintf("%s/event/%020d", recovery.ID(), 1)),
		WorkflowExecutionID: recovery.ID(), CompanyID: recovery.CompanyID(),
		Type:          repository.ExecutionEventTypeWorkflowStarted,
		NewStatus:     execution.WorkflowExecutionStatusRunning.String(),
		CorrelationID: correlationID,
		CausationID:   deterministicRecoveryCausationID(sourceExecutionID),
		SafeMessage:   "Partial recovery execution started",
		Metadata:      metadata, CreatedAt: recoveryAt,
	})
	if err != nil {
		return nil, err
	}
	eventEntry, err := repository.NewEventTimelineEntry(event)
	if err != nil {
		return nil, err
	}
	log, err := repository.NewExecutionLogDraft(repository.ExecutionLogDraftParams{
		ID:                  repository.ExecutionLogID(fmt.Sprintf("%s/log/%020d", recovery.ID(), 2)),
		WorkflowExecutionID: recovery.ID(), CompanyID: recovery.CompanyID(),
		Level:    repository.ExecutionLogLevelInfo,
		Message:  "Partial recovery execution started",
		Metadata: metadata, CreatedAt: recoveryAt,
	})
	if err != nil {
		return nil, err
	}
	logEntry, err := repository.NewLogTimelineEntry(log)
	if err != nil {
		return nil, err
	}
	return []repository.TimelineEntry{eventEntry, logEntry}, nil
}

func marshalRecoveryPlan(
	sourceExecutionID execution.WorkflowExecutionID,
	plans []repository.RecoveryNodePlan,
) ([]byte, error) {
	type recoveryPlanNode struct {
		NodeID        string `json:"nodeId"`
		SourceStatus  string `json:"sourceStatus"`
		Disposition   string `json:"disposition"`
		SourceAttempt int16  `json:"sourceAttempt"`
	}
	nodes := make([]recoveryPlanNode, 0, len(plans))
	for _, plan := range plans {
		nodes = append(nodes, recoveryPlanNode{
			NodeID:        plan.NodeExecution().NodeID().String(),
			SourceStatus:  plan.SourceStatus().String(),
			Disposition:   string(plan.Disposition()),
			SourceAttempt: plan.SourceAttempt(),
		})
	}
	return json.Marshal(map[string]any{
		"kind":              "PARTIAL",
		"sourceExecutionId": sourceExecutionID.String(),
		"nodes":             nodes,
	})
}

func deterministicRecoveryExecutionID(
	companyID workflow.CompanyID,
	sourceExecutionID execution.WorkflowExecutionID,
	idempotencyKey string,
) execution.WorkflowExecutionID {
	digest := sha256.Sum256([]byte(
		"partial-recovery|" + companyID.String() + "|" +
			sourceExecutionID.String() + "|" + idempotencyKey))
	return execution.WorkflowExecutionID("recovery-" + hex.EncodeToString(digest[:]))
}

func deterministicRecoveryCausationID(
	sourceExecutionID execution.WorkflowExecutionID,
) string {
	digest := sha256.Sum256([]byte("partial-recovery-causation|" + sourceExecutionID.String()))
	return "recovery-cause-" + hex.EncodeToString(digest[:])
}

func partialRecoveryOutcome(record repository.PartialRecoveryRecord) PartialRecoveryOutcome {
	return PartialRecoveryOutcome{
		SourceExecutionID:   record.SourceWorkflowExecutionID(),
		RecoveryExecutionID: record.RecoveryWorkflowExecutionID(),
		Status:              execution.WorkflowExecutionStatusRunning,
		PreservedNodeCount:  record.PreservedNodeCount(),
		ScheduledNodeCount:  record.ScheduledNodeCount(),
		ResetNodeCount:      record.ResetNodeCount(),
		CreatedAt:           record.CreatedAt(),
	}
}
