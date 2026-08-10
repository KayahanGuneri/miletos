package crontrigger

import (
	"miletos-go/internal/features/workflow-runtime/execution"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
)

func mapCreateRequest(request *runtimev1.CreateCronTriggerRequest, companyID string) CreateRequest {
	return CreateRequest{
		CompanyID:     companyID,
		Workflow:      execution.MapWorkflowFromGRPC(request.GetDefinition(), companyID),
		TriggerNodeID: request.GetTriggerNodeId(),
		Expression:    request.GetCronExpression(),
		Timezone:      request.GetTimezone(),
		ResolvedMode:  ResolvedMode(request.GetResolvedMode()),
	}
}

func mapBinding(binding Binding) *runtimev1.CronTriggerResponse {
	return &runtimev1.CronTriggerResponse{
		TriggerId:        binding.ID,
		WorkflowId:       binding.WorkflowID,
		WorkflowRevision: binding.WorkflowRevision,
		SnapshotId:       binding.SnapshotID,
		TriggerNodeId:    binding.TriggerNodeID,
		CronExpression:   binding.Expression,
		Timezone:         binding.Timezone,
		Status:           string(binding.Status),
		NextFireAt:       grpcserver.Time(binding.NextFireAt),
		LastScheduledAt:  grpcserver.OptionalTime(binding.LastScheduledAt),
		LastFiredAt:      grpcserver.OptionalTime(binding.LastFiredAt),
		CreatedAt:        grpcserver.Time(binding.CreatedAt),
		UpdatedAt:        grpcserver.Time(binding.UpdatedAt),
		DisabledAt:       grpcserver.OptionalTime(binding.DisabledAt),
	}
}
