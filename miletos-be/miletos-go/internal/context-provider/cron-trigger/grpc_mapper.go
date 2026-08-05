package crontrigger

import (
	"miletos-go/internal/features/workflow-runtime/workflow"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
)

func mapCreateRequest(request *runtimev1.CreateCronTriggerRequest, companyID string) CreateRequest {
	return CreateRequest{CompanyID: companyID, Workflow: mapWorkflow(request.GetDefinition(), companyID),
		TriggerNodeID: request.GetTriggerNodeId(), Expression: request.GetCronExpression(),
		Timezone: request.GetTimezone(), ResolvedMode: ResolvedMode(request.GetResolvedMode())}
}

func mapWorkflow(definition *runtimev1.WorkflowDefinition, companyID string) workflow.Workflow {
	mapped := workflow.Workflow{ID: definition.GetWorkflowId(), CompanyID: companyID, Name: definition.GetName(), Revision: definition.GetWorkflowRevision()}
	if definition.Metadata != nil {
		mapped.Metadata = definition.Metadata.AsMap()
	}
	for _, node := range definition.GetNodes() {
		item := workflow.WorkflowNode{ID: node.GetNodeId(), Type: node.GetPluginType(), Version: node.GetPluginVersion()}
		if node.Configuration != nil {
			item.Configuration = node.Configuration.AsMap()
		}
		if node.Position != nil {
			item.Position = &workflow.NodePosition{X: node.Position.X, Y: node.Position.Y}
		}
		mapped.Nodes = append(mapped.Nodes, item)
	}
	for _, edge := range definition.GetEdges() {
		mapped.Edges = append(mapped.Edges, workflow.Edge{ID: edge.GetEdgeId(), SourceNodeID: edge.GetSourceNodeId(), SourceOutputPort: edge.GetSourceOutputPort(), TargetNodeID: edge.GetTargetNodeId(), TargetInputPort: edge.GetTargetInputPort()})
	}
	return mapped
}

func mapBinding(binding Binding) *runtimev1.CronTriggerResponse {
	return &runtimev1.CronTriggerResponse{TriggerId: binding.ID, WorkflowId: binding.WorkflowID, WorkflowRevision: binding.WorkflowRevision,
		SnapshotId: binding.SnapshotID, TriggerNodeId: binding.TriggerNodeID, CronExpression: binding.Expression, Timezone: binding.Timezone,
		Status: string(binding.Status), NextFireAt: grpcserver.Time(binding.NextFireAt), LastScheduledAt: grpcserver.OptionalTime(binding.LastScheduledAt),
		LastFiredAt: grpcserver.OptionalTime(binding.LastFiredAt), CreatedAt: grpcserver.Time(binding.CreatedAt), UpdatedAt: grpcserver.Time(binding.UpdatedAt), DisabledAt: grpcserver.OptionalTime(binding.DisabledAt)}
}
