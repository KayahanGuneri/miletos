package httptrigger

import (
	"miletos-go/internal/features/workflow-runtime/workflow"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
)

func mapCreateRequest(
	request *runtimev1.CreateHTTPTriggerRequest,
	companyID string,
) CreateRequest {
	return CreateRequest{
		CompanyID:     companyID,
		Workflow:      mapTriggerWorkflow(request.Definition, companyID),
		TriggerNodeID: request.GetTriggerNodeId(),
		Method:        request.GetHttpMethod(),
		ResolvedMode:  ResolvedMode(request.GetResolvedMode()),
	}
}

func mapTriggerWorkflow(
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
			ID:      node.GetNodeId(),
			Type:    node.GetPluginType(),
			Version: node.GetPluginVersion(),
		}
		if node.Configuration != nil {
			mapped.Configuration = node.Configuration.AsMap()
		}
		if node.Position != nil {
			mapped.Position = &workflow.NodePosition{X: node.Position.X, Y: node.Position.Y}
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

func mapCreatedTrigger(created CreatedBinding) *runtimev1.CreateHTTPTriggerResponse {
	return &runtimev1.CreateHTTPTriggerResponse{
		Trigger:   mapTriggerBinding(created.Binding),
		PublicUrl: created.PublicURL,
	}
}

func mapTriggerBinding(binding Binding) *runtimev1.HTTPTriggerResponse {
	return &runtimev1.HTTPTriggerResponse{
		TriggerId:        binding.ID,
		WorkflowId:       binding.WorkflowID,
		WorkflowRevision: binding.WorkflowRevision,
		SnapshotId:       binding.SnapshotID,
		TriggerNodeId:    binding.TriggerNodeID,
		HttpMethod:       binding.Method,
		Status:           string(binding.Status),
		ResolvedMode:     string(binding.ResolvedMode),
		CreatedAt:        grpcserver.Time(binding.CreatedAt),
		UpdatedAt:        grpcserver.Time(binding.UpdatedAt),
		DisabledAt:       grpcserver.OptionalTime(binding.DisabledAt),
	}
}
