package triggerlifecycle

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	crontrigger "miletos-go/internal/context-provider/cron-trigger"
	httptrigger "miletos-go/internal/context-provider/http-trigger"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
	"miletos-go/internal/shared/requestcontext"
)

type GRPCService struct {
	runtimev1.UnimplementedTriggerLifecycleServiceServer
	http *httptrigger.Service
	cron *crontrigger.Service
}

func NewGRPCService(http *httptrigger.Service, cron *crontrigger.Service) *GRPCService {
	return &GRPCService{http: http, cron: cron}
}
func (service *GRPCService) DisableWorkflowTriggers(ctx context.Context, request *runtimev1.DisableWorkflowTriggersRequest) (*runtimev1.DisableWorkflowTriggersResponse, error) {
	if request == nil || strings.TrimSpace(request.GetWorkflowId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow ID is required")
	}
	companyID := requestcontext.CompanyID(ctx)
	httpCount, err := service.http.DisableWorkflow(ctx, companyID, request.GetWorkflowId())
	if err != nil {
		return nil, status.Error(codes.Internal, "runtime trigger lifecycle operation failed")
	}
	cronCount, err := service.cron.DisableWorkflow(ctx, companyID, request.GetWorkflowId())
	if err != nil {
		return nil, status.Error(codes.Internal, "runtime trigger lifecycle operation failed")
	}
	return &runtimev1.DisableWorkflowTriggersResponse{DisabledHttpTriggerCount: uint32(httpCount), DisabledCronTriggerCount: uint32(cronCount)}, nil
}
