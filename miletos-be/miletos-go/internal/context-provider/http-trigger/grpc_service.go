package httptrigger

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/workflow"
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
	created, err := grpcService.service.Create(
		ctx, mapCreateRequest(request, requestcontext.CompanyID(ctx)),
	)
	if err != nil {
		return nil, mapTriggerGRPCError(err)
	}
	return mapCreatedTrigger(created), nil
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

func (grpcService *GRPCService) GetActiveHTTPTriggerByWorkflow(
	ctx context.Context,
	request *runtimev1.GetActiveHTTPTriggerByWorkflowRequest,
) (*runtimev1.HTTPTriggerResponse, error) {
	if request == nil || request.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow id is required")
	}
	if request.GetTriggerNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "trigger node id is required")
	}
	binding, err := grpcService.service.GetActiveByWorkflowAndNode(
		ctx, requestcontext.CompanyID(ctx), request.GetWorkflowId(), request.GetTriggerNodeId(),
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

func mapTriggerGRPCError(err error) error {
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, "the requested HTTP trigger was not found")
	case errors.Is(err, workflow.ErrInvalidWorkflow):
		return execution.WorkflowValidationGRPCStatus(err)
	case errors.Is(err, ErrInvalidTrigger),
		errors.Is(err, ErrMethodNotAllowed):
		return status.Error(codes.InvalidArgument, "HTTP trigger request is invalid")
	case errors.Is(err, ErrBindingInvariant),
		errors.Is(err, repository.ErrStateTransition):
		return status.Error(codes.FailedPrecondition, "HTTP trigger state is invalid")
	case errors.Is(err, ErrPublicURLUnavailable):
		return status.Error(codes.FailedPrecondition, "HTTP trigger service is unavailable")
	default:
		return status.Error(codes.Internal, "an unexpected internal error occurred")
	}
}
