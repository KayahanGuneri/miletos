package triggerlifecycle

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	crontrigger "miletos-go/internal/context-provider/cron-trigger"
	dataarrival "miletos-go/internal/context-provider/data-arrival"
	httptrigger "miletos-go/internal/context-provider/http-trigger"
	"miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/workflow"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
	"miletos-go/internal/shared/requestcontext"
)

type GRPCService struct {
	runtimev1.UnimplementedTriggerLifecycleServiceServer
	http    *httptrigger.Service
	cron    *crontrigger.Service
	sources *dataarrival.Service
}

func NewGRPCService(
	http *httptrigger.Service,
	cron *crontrigger.Service,
	sources *dataarrival.Service,
) *GRPCService {
	return &GRPCService{http: http, cron: cron, sources: sources}
}

func (service *GRPCService) ActivateWorkflowSources(
	ctx context.Context,
	request *runtimev1.ValidateWorkflowRequest,
) (*emptypb.Empty, error) {
	if request == nil || request.GetDefinition() == nil {
		return nil, execution.StatusWithErrorInfo(
			codes.InvalidArgument,
			string(execution.ErrorReasonInvalidWorkflowDefinition),
			execution.ErrorReasonInvalidWorkflowDefinition,
		)
	}
	definition := execution.MapWorkflowFromGRPC(
		request.GetDefinition(), requestcontext.CompanyID(ctx),
	)
	if err := service.sources.Activate(ctx, definition); err != nil {
		return nil, mapLifecycleGRPCError(err, codes.FailedPrecondition)
	}
	return &emptypb.Empty{}, nil
}

func (service *GRPCService) DisableWorkflowTriggers(
	ctx context.Context,
	request *runtimev1.DisableWorkflowTriggersRequest,
) (*runtimev1.DisableWorkflowTriggersResponse, error) {
	if request == nil || strings.TrimSpace(request.GetWorkflowId()) == "" {
		return nil, execution.StatusWithErrorInfo(
			codes.InvalidArgument,
			string(execution.ErrorReasonInvalidExecutionRequest),
			execution.ErrorReasonInvalidExecutionRequest,
		)
	}
	companyID := requestcontext.CompanyID(ctx)
	httpCount, err := service.http.DisableWorkflow(ctx, companyID, request.GetWorkflowId())
	if err != nil {
		return nil, mapLifecycleGRPCError(err, codes.Internal)
	}
	cronCount, err := service.cron.DisableWorkflow(ctx, companyID, request.GetWorkflowId())
	if err != nil {
		return nil, mapLifecycleGRPCError(err, codes.Internal)
	}
	if _, err := service.sources.DisableWorkflow(ctx, companyID, request.GetWorkflowId()); err != nil {
		return nil, mapLifecycleGRPCError(err, codes.Internal)
	}
	return &runtimev1.DisableWorkflowTriggersResponse{
		DisabledHttpTriggerCount: uint32(httpCount),
		DisabledCronTriggerCount: uint32(cronCount),
	}, nil
}

func mapLifecycleGRPCError(err error, fallback codes.Code) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return status.FromContextError(err).Err()
	case errors.Is(err, workflow.ErrInvalidWorkflow):
		return execution.WorkflowValidationGRPCStatus(err)
	case errors.Is(err, execution.ErrAsyncUnavailable):
		return execution.StatusWithErrorInfo(
			codes.FailedPrecondition,
			string(execution.ErrorReasonTriggerRuntimeUnavailable),
			execution.ErrorReasonTriggerRuntimeUnavailable,
		)
	default:
		reason := execution.ErrorReasonTriggerRuntimeUnavailable
		if fallback == codes.Internal {
			reason = execution.ErrorReasonInternalServerError
		}
		return execution.StatusWithErrorInfo(fallback, string(reason), reason)
	}
}
