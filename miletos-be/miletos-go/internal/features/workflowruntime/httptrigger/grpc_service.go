package httptrigger

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/workflow"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
	"miletos-go/internal/shared/requestcontext"
)

type GRPCService struct {
	runtimev1.UnimplementedHTTPTriggerServiceServer
	service *Service
}

func NewGRPCService(service *Service) *GRPCService {
	return &GRPCService{service: service}
}

func (grpcService *GRPCService) CreateHTTPTrigger(
	ctx context.Context,
	request *runtimev1.CreateHTTPTriggerRequest,
) (*runtimev1.CreateHTTPTriggerResponse, error) {
	if request == nil || request.Definition == nil {
		return nil, status.Error(codes.InvalidArgument, "workflow definition is required")
	}
	created, err := grpcService.service.Create(ctx, CreateRequest{
		CompanyID:     requestcontext.CompanyID(ctx),
		Workflow:      mapTriggerWorkflow(request.Definition, requestcontext.CompanyID(ctx)),
		TriggerNodeID: request.GetTriggerNodeId(),
		Method:        request.GetHttpMethod(),
		ResolvedMode:  request.GetResolvedMode(),
	})
	if err != nil {
		return nil, mapTriggerGRPCError(err)
	}
	return &runtimev1.CreateHTTPTriggerResponse{
		Trigger:   mapTriggerBinding(created.Binding),
		PublicUrl: created.PublicURL,
	}, nil
}

func (grpcService *GRPCService) GetHTTPTrigger(
	ctx context.Context,
	request *runtimev1.GetHTTPTriggerRequest,
) (*runtimev1.HTTPTriggerResponse, error) {
	binding, err := grpcService.service.Get(
		ctx, requestcontext.CompanyID(ctx), request.GetTriggerId(),
	)
	if err != nil {
		return nil, mapTriggerGRPCError(err)
	}
	return mapTriggerBinding(binding), nil
}

func (grpcService *GRPCService) DisableHTTPTrigger(
	ctx context.Context,
	request *runtimev1.DisableHTTPTriggerRequest,
) (*runtimev1.HTTPTriggerResponse, error) {
	binding, err := grpcService.service.Disable(
		ctx, requestcontext.CompanyID(ctx), request.GetTriggerId(),
	)
	if err != nil {
		return nil, mapTriggerGRPCError(err)
	}
	return mapTriggerBinding(binding), nil
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

func mapTriggerBinding(binding Binding) *runtimev1.HTTPTriggerResponse {
	return &runtimev1.HTTPTriggerResponse{
		TriggerId:        binding.ID,
		WorkflowId:       binding.WorkflowID,
		WorkflowRevision: binding.WorkflowRevision,
		SnapshotId:       binding.SnapshotID,
		TriggerNodeId:    binding.TriggerNodeID,
		HttpMethod:       binding.Method,
		Status:           binding.Status,
		ResolvedMode:     binding.ResolvedMode,
		CreatedAt:        grpcserver.Time(binding.CreatedAt),
		UpdatedAt:        grpcserver.Time(binding.UpdatedAt),
		DisabledAt:       grpcserver.OptionalTime(binding.DisabledAt),
	}
}

func mapTriggerGRPCError(err error) error {
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, "the requested HTTP trigger was not found")
	case errors.Is(err, ErrInvalidTrigger),
		errors.Is(err, ErrMethodNotAllowed):
		return status.Error(codes.InvalidArgument, "HTTP trigger request is invalid")
	case errors.Is(err, ErrPublicURLUnavailable),
		errors.Is(err, ErrBindingInvariant):
		return status.Error(codes.FailedPrecondition, "HTTP trigger service is unavailable")
	default:
		return status.Error(codes.Internal, "an unexpected internal error occurred")
	}
}
