package execution

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/workflow"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
)

func mapGRPCExecuteRequest(
	request *runtimev1.ExecuteRequest,
	companyID string,
) (workflow.Workflow, map[string]any, error) {
	if request == nil || request.Definition == nil {
		return workflow.Workflow{}, nil, status.Error(
			codes.InvalidArgument, "workflow definition is required",
		)
	}
	definition := mapWorkflowFromGRPC(request.Definition, companyID)
	var startInput map[string]any
	if request.InitialVariables != nil {
		startInput = request.InitialVariables.AsMap()
	}
	return definition, startInput, nil
}

func mapWorkflowFromGRPC(
	definition *runtimev1.WorkflowDefinition,
	companyID string,
) workflow.Workflow {
	return MapWorkflowFromGRPC(definition, companyID)
}

func MapWorkflowFromGRPC(
	definition *runtimev1.WorkflowDefinition,
	companyID string,
) workflow.Workflow {
	mappedWorkflow := workflow.Workflow{
		ID:        definition.GetWorkflowId(),
		CompanyID: companyID,
		Name:      definition.GetName(),
		Revision:  definition.GetWorkflowRevision(),
		Nodes:     make([]workflow.WorkflowNode, 0, len(definition.GetNodes())),
		Edges:     make([]workflow.Edge, 0, len(definition.GetEdges())),
	}
	if definition.Metadata != nil {
		mappedWorkflow.Metadata = definition.Metadata.AsMap()
	}
	for _, node := range definition.GetNodes() {
		mapped := workflow.WorkflowNode{
			ID:          node.GetNodeId(),
			DisplayName: node.GetDisplayName(),
			Type:        node.GetPluginType(),
			Version:     node.GetPluginVersion(),
		}
		if node.Configuration != nil {
			mapped.Configuration = node.Configuration.AsMap()
		}
		if node.Position != nil {
			mapped.Position = &workflow.NodePosition{
				X: node.Position.X, Y: node.Position.Y,
			}
		}
		mappedWorkflow.Nodes = append(mappedWorkflow.Nodes, mapped)
	}
	for _, edge := range definition.GetEdges() {
		mappedWorkflow.Edges = append(mappedWorkflow.Edges, workflow.Edge{
			ID:               edge.GetEdgeId(),
			SourceNodeID:     edge.GetSourceNodeId(),
			SourceOutputPort: edge.GetSourceOutputPort(),
			TargetNodeID:     edge.GetTargetNodeId(),
			TargetInputPort:  edge.GetTargetInputPort(),
		})
	}
	return mappedWorkflow
}

func mapGRPCWorkflow(definition workflow.Workflow) *runtimev1.WorkflowDefinition {
	result := &runtimev1.WorkflowDefinition{
		WorkflowId:       definition.ID,
		Name:             definition.Name,
		WorkflowRevision: definition.Revision,
		Metadata:         grpcserver.Struct(definition.Metadata),
		Nodes:            make([]*runtimev1.WorkflowNode, 0, len(definition.Nodes)),
		Edges:            make([]*runtimev1.WorkflowEdge, 0, len(definition.Edges)),
	}
	for _, node := range definition.Nodes {
		mapped := &runtimev1.WorkflowNode{
			NodeId:        node.ID,
			DisplayName:   &node.DisplayName,
			PluginType:    node.Type,
			PluginVersion: node.Version,
			Configuration: grpcserver.Struct(node.Configuration),
		}
		if node.Position != nil {
			mapped.Position = &runtimev1.NodePosition{
				X: node.Position.X, Y: node.Position.Y,
			}
		}
		result.Nodes = append(result.Nodes, mapped)
	}
	for _, edge := range definition.Edges {
		result.Edges = append(result.Edges, &runtimev1.WorkflowEdge{
			EdgeId:           edge.ID,
			SourceNodeId:     edge.SourceNodeID,
			SourceOutputPort: edge.SourceOutputPort,
			TargetNodeId:     edge.TargetNodeID,
			TargetInputPort:  edge.TargetInputPort,
		})
	}
	return result
}

func mapGRPCExecutionOutcome(outcome ExecutionOutcome, requestID string) *runtimev1.ExecutionResponse {
	execution := redactExecution(outcome.Execution)
	return &runtimev1.ExecutionResponse{
		ExecutionId:      execution.ID,
		WorkflowId:       execution.WorkflowID,
		WorkflowRevision: execution.WorkflowRevision,
		SnapshotId:       execution.SnapshotID,
		Mode:             execution.Mode,
		Status:           string(execution.Status),
		ExecutionOrigin:  string(execution.Origin),
		CorrelationId:    execution.CorrelationID,
		RequestId:        requestID,
		CreatedAt:        grpcserver.Time(execution.CreatedAt),
		StartedAt:        grpcserver.OptionalTime(execution.StartedAt),
		FinishedAt:       grpcserver.OptionalTime(execution.FinishedAt),
		ScheduledRoots:   uint32(outcome.ScheduledEntryNodes),
		Replayed:         outcome.Replayed,
		TerminalOutputs:  grpcserver.Struct(execution.TerminalOutputs),
		FailureSummary:   grpcserver.Struct(execution.Failure),
		ExecutionIds:     []string{execution.ID},
		ExecutionCount:   1,
	}
}

func mapGRPCManualExecutionBatch(
	batch ManualExecutionBatch,
	requestID string,
) *runtimev1.ExecutionResponse {
	response := &runtimev1.ExecutionResponse{
		WorkflowId:       batch.WorkflowID,
		WorkflowRevision: batch.WorkflowRevision,
		SnapshotId:       batch.SnapshotID,
		Mode:             batch.Mode,
		Status:           batch.Status,
		ExecutionOrigin:  string(model.ExecutionOriginManualDirect),
		CorrelationId:    batch.CorrelationID,
		RequestId:        requestID,
		ScheduledRoots:   uint32(batch.ScheduledEntryNodes),
		Replayed:         batch.Replayed,
		ExecutionIds:     make([]string, 0, len(batch.Executions)),
		ExecutionCount:   uint32(len(batch.Executions)),
	}
	for _, execution := range batch.Executions {
		response.ExecutionIds = append(response.ExecutionIds, execution.ID)
	}
	if len(batch.Executions) != 1 {
		return response
	}
	single := mapGRPCExecutionOutcome(
		ExecutionOutcome{
			Execution: batch.Executions[0], ScheduledEntryNodes: batch.ScheduledEntryNodes,
			Replayed: batch.Replayed,
		},
		requestID,
	)
	return single
}

func mapGRPCExecutionPage(page model.Page[model.Execution]) *runtimev1.ExecutionPage {
	items := make([]*runtimev1.ExecutionSummary, 0, len(page.Items))
	for _, execution := range page.Items {
		items = append(items, mapGRPCExecution(execution))
	}
	return &runtimev1.ExecutionPage{Items: items, Cursor: page.Next, HasNext: page.HasNext}
}

func mapGRPCExecution(execution model.Execution) *runtimev1.ExecutionSummary {
	execution = redactExecution(execution)
	return &runtimev1.ExecutionSummary{
		ExecutionId:      execution.ID,
		WorkflowId:       execution.WorkflowID,
		WorkflowRevision: execution.WorkflowRevision,
		SnapshotId:       execution.SnapshotID,
		Mode:             execution.Mode,
		Status:           string(execution.Status),
		ExecutionOrigin:  string(execution.Origin),
		CorrelationId:    execution.CorrelationID,
		CreatedAt:        grpcserver.Time(execution.CreatedAt),
		ValidatingAt:     grpcserver.OptionalTime(execution.ValidatingAt),
		QueuedAt:         grpcserver.OptionalTime(execution.QueuedAt),
		StartedAt:        grpcserver.OptionalTime(execution.StartedAt),
		FinishedAt:       grpcserver.OptionalTime(execution.FinishedAt),
		UpdatedAt:        grpcserver.Time(execution.UpdatedAt),
		TerminalOutputs:  grpcserver.Struct(execution.TerminalOutputs),
		FailureSummary:   grpcserver.Struct(execution.Failure),
		IsStalled:        execution.IsStalled,
	}
}

func mapGRPCExecutionDefinition(
	executionID string,
	snapshot workflow.WorkflowSnapshot,
) *runtimev1.ExecutionDefinition {
	return &runtimev1.ExecutionDefinition{
		ExecutionId:      executionID,
		SnapshotId:       snapshot.ID,
		WorkflowId:       snapshot.Workflow.ID,
		WorkflowRevision: snapshot.Workflow.Revision,
		WorkflowName:     snapshot.Workflow.Name,
		Definition:       mapGRPCWorkflow(snapshot.Workflow),
		CreatedAt:        grpcserver.Time(snapshot.CreatedAt),
	}
}

func mapGRPCNodeExecutionPage(page model.Page[model.NodeExecution]) *runtimev1.NodeExecutionPage {
	items := make([]*runtimev1.NodeExecution, 0, len(page.Items))
	for _, node := range page.Items {
		items = append(items, mapGRPCNodeExecution(node))
	}
	return &runtimev1.NodeExecutionPage{Items: items, Cursor: page.Next, HasNext: page.HasNext}
}

func mapGRPCNodeExecution(node model.NodeExecution) *runtimev1.NodeExecution {
	return &runtimev1.NodeExecution{
		NodeExecutionId: node.ID,
		ExecutionId:     node.ExecutionID,
		NodeId:          node.NodeID,
		PluginType:      node.Type,
		PluginVersion:   node.Version,
		Configuration:   grpcserver.Struct(node.Configuration),
		Status:          string(node.Status),
		Attempt:         uint32(node.Attempt),
		CreatedAt:       grpcserver.Time(node.CreatedAt),
		ReadyAt:         grpcserver.OptionalTime(node.ReadyAt),
		QueuedAt:        grpcserver.OptionalTime(node.QueuedAt),
		StartedAt:       grpcserver.OptionalTime(node.StartedAt),
		FinishedAt:      grpcserver.OptionalTime(node.FinishedAt),
		NextAttemptAt:   grpcserver.OptionalTime(node.NextAttemptAt),
		UpdatedAt:       grpcserver.Time(node.UpdatedAt),
		InputSummary:    grpcserver.Struct(node.Input),
		OutputSummary:   grpcserver.Struct(node.Output),
		FailureSummary:  grpcserver.Struct(node.Failure),
	}
}

func mapGRPCEventPage(page model.Page[model.ExecutionEvent]) *runtimev1.ExecutionEventPage {
	items := make([]*runtimev1.ExecutionEvent, 0, len(page.Items))
	for _, event := range page.Items {
		items = append(items, mapGRPCEvent(event))
	}
	return &runtimev1.ExecutionEventPage{Items: items, Cursor: page.Next, HasNext: page.HasNext}
}

func mapGRPCEvent(event model.ExecutionEvent) *runtimev1.ExecutionEvent {
	return &runtimev1.ExecutionEvent{
		EventId:         event.ID,
		ExecutionId:     event.ExecutionID,
		NodeExecutionId: event.NodeExecutionID,
		SequenceNumber:  event.Sequence,
		Type:            event.Type,
		PreviousStatus:  event.PreviousStatus,
		NewStatus:       event.NewStatus,
		CorrelationId:   event.CorrelationID,
		CausationId:     event.CausationID,
		SafeMessage:     event.Message,
		Metadata:        grpcserver.Struct(event.Metadata),
		CreatedAt:       grpcserver.Time(event.CreatedAt),
	}
}

func mapGRPCLogPage(page model.Page[model.ExecutionLog]) *runtimev1.ExecutionLogPage {
	items := make([]*runtimev1.ExecutionLog, 0, len(page.Items))
	for _, entry := range page.Items {
		items = append(items, mapGRPCLog(entry))
	}
	return &runtimev1.ExecutionLogPage{Items: items, Cursor: page.Next, HasNext: page.HasNext}
}

func mapGRPCLog(entry model.ExecutionLog) *runtimev1.ExecutionLog {
	return &runtimev1.ExecutionLog{
		LogId:           entry.ID,
		ExecutionId:     entry.ExecutionID,
		NodeExecutionId: entry.NodeExecutionID,
		SequenceNumber:  entry.Sequence,
		Level:           entry.Level,
		Message:         entry.Message,
		Metadata:        grpcserver.Struct(entry.Metadata),
		CreatedAt:       grpcserver.Time(entry.CreatedAt),
	}
}

func mapGRPCExecutionErrorPage(
	page model.Page[model.ExecutionError],
) *runtimev1.ExecutionErrorPage {
	items := make([]*runtimev1.ExecutionError, 0, len(page.Items))
	for _, entry := range page.Items {
		items = append(items, mapGRPCExecutionError(entry))
	}
	return &runtimev1.ExecutionErrorPage{Items: items, Cursor: page.Next, HasNext: page.HasNext}
}

func mapGRPCExecutionError(entry model.ExecutionError) *runtimev1.ExecutionError {
	return &runtimev1.ExecutionError{
		ErrorId:         entry.ID,
		ExecutionId:     entry.ExecutionID,
		NodeExecutionId: entry.NodeExecutionID,
		RelatedEventId:  entry.RelatedEventID,
		Category:        string(entry.Category),
		Code:            string(entry.Code),
		SafeMessage:     entry.Message,
		Retryable:       entry.Retryable,
		Details:         grpcserver.Struct(entry.Details),
		CreatedAt:       grpcserver.Time(entry.CreatedAt),
	}
}

func mapGRPCRecoveryOutcome(outcome RecoveryOutcome) *runtimev1.RecoveryResponse {
	return &runtimev1.RecoveryResponse{
		SourceExecutionId:   outcome.SourceExecutionID,
		RecoveryExecutionId: outcome.RecoveryExecutionID,
		Status:              string(outcome.Status),
		PreservedNodeCount:  uint32(outcome.PreservedNodeCount),
		ScheduledNodeCount:  uint32(outcome.ScheduledNodeCount),
		ResetNodeCount:      uint32(outcome.ResetNodeCount),
		CreatedAt:           grpcserver.Time(outcome.CreatedAt),
		Replayed:            outcome.Replayed,
	}
}

func manualExecutionFingerprint(
	definition workflow.Workflow,
	startInput map[string]any,
	mode string,
) string {
	return Fingerprint(map[string]any{
		"workflowId":       definition.ID,
		"workflowRevision": definition.Revision,
		"origin":           string(model.ExecutionOriginManualDirect),
		"mode":             mode,
		"initialVariables": startInput,
	})
}
