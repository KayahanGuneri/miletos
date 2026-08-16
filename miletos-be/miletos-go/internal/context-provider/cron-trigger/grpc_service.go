package crontrigger

import (
	"context"
	"errors"

	"miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/workflow"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
	"miletos-go/internal/shared/requestcontext"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GRPCService struct {
	runtimev1.UnimplementedCronTriggerServiceServer
	service *Service
}

func NewGRPCService(service *Service) *GRPCService { return &GRPCService{service: service} }

func (server *GRPCService) CreateCronTrigger(ctx context.Context, request *runtimev1.CreateCronTriggerRequest) (*runtimev1.CreateCronTriggerResponse, error) {
	if request == nil || request.Definition == nil {
		return nil, status.Error(codes.InvalidArgument, "workflow definition is required")
	}
	binding, err := server.service.Create(ctx, mapCreateRequest(request, requestcontext.CompanyID(ctx)))
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return &runtimev1.CreateCronTriggerResponse{Trigger: mapBinding(binding)}, nil
}
func (server *GRPCService) GetCronTrigger(ctx context.Context, request *runtimev1.GetCronTriggerRequest) (*runtimev1.CronTriggerResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	binding, err := server.service.Get(ctx, requestcontext.CompanyID(ctx), request.GetTriggerId())
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapBinding(binding), nil
}
func (server *GRPCService) GetActiveCronTriggerByWorkflow(
	ctx context.Context,
	request *runtimev1.GetActiveCronTriggerByWorkflowRequest,
) (*runtimev1.CronTriggerResponse, error) {
	if request == nil || request.GetWorkflowId() == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow id is required")
	}
	if request.GetTriggerNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "trigger node id is required")
	}
	binding, err := server.service.GetActiveByWorkflowAndNode(
		ctx, requestcontext.CompanyID(ctx), request.GetWorkflowId(), request.GetTriggerNodeId(),
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapBinding(binding), nil
}
func (server *GRPCService) DisableCronTrigger(ctx context.Context, request *runtimev1.DisableCronTriggerRequest) (*runtimev1.CronTriggerResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	binding, err := server.service.Disable(ctx, requestcontext.CompanyID(ctx), request.GetTriggerId())
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapBinding(binding), nil
}
func mapGRPCError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, "the requested cron trigger was not found")
	case errors.Is(err, workflow.ErrInvalidWorkflow):
		return execution.WorkflowValidationGRPCStatus(err)
	case errors.Is(err, ErrInvalidTrigger):
		return status.Error(codes.InvalidArgument, "cron trigger request is invalid")
	case errors.Is(err, repository.ErrStateTransition),
		errors.Is(err, ErrBindingInvariant):
		return status.Error(codes.FailedPrecondition, "cron trigger state is invalid")
	case errors.Is(err, execution.ErrAsyncUnavailable),
		errors.Is(err, ErrSchedulerUnavailable):
		return status.Error(codes.FailedPrecondition, "cron trigger service is unavailable")
	default:
		return status.Error(codes.Internal, "an unexpected internal error occurred")
	}
}
