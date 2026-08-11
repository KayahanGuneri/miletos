package httptrigger

import (
	"miletos-go/internal/features/workflow-runtime/execution"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
)

func mapCreateRequest(
	request *runtimev1.CreateHTTPTriggerRequest,
	companyID string,
) CreateRequest {
	return CreateRequest{
		CompanyID:     companyID,
		Workflow:      execution.MapWorkflowFromGRPC(request.Definition, companyID),
		TriggerNodeID: request.GetTriggerNodeId(),
		Method:        request.GetHttpMethod(),
		ResolvedMode:  ResolvedMode(request.GetResolvedMode()),
	}
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
