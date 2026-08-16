package execution

import (
	"context"
	"errors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
	"miletos-go/internal/shared/requestcontext"
)

type GRPCService struct {
	runtimev1.UnimplementedExecutionServiceServer
	service  *ExecutionService
	recovery *RecoveryService
	queries  *ExecutionQueryService
}

type ErrorReason string

const (
	ErrorReasonInvalidWorkflowDefinition    ErrorReason = "INVALID_WORKFLOW_DEFINITION"
	ErrorReasonInvalidExecutionRequest      ErrorReason = "INVALID_EXECUTION_REQUEST"
	ErrorReasonTriggerRuntimeUnavailable    ErrorReason = "TRIGGER_RUNTIME_UNAVAILABLE"
	ErrorReasonInternalServerError          ErrorReason = "INTERNAL_SERVER_ERROR"
	ErrorReasonIdempotencyKeyReused         ErrorReason = "IDEMPOTENCY_KEY_REUSED"
	ErrorReasonIdempotencyRequestInProgress ErrorReason = "IDEMPOTENCY_REQUEST_IN_PROGRESS"
	ErrorReasonRecoveryNotSupported         ErrorReason = "RECOVERY_NOT_SUPPORTED"
	ErrorReasonInitialVariablesNotAccepted  ErrorReason = "INITIAL_VARIABLES_NOT_ACCEPTED"
	ErrorReasonInvalidExecutionOrigin       ErrorReason = "INVALID_EXECUTION_ORIGIN"
	ErrorReasonWorkflowValidationFailed     ErrorReason = "WORKFLOW_VALIDATION_FAILED"
)

func NewGRPCService(
	service *ExecutionService,
	recovery *RecoveryService,
	queries *ExecutionQueryService,
) *GRPCService {
	return &GRPCService{
		service:  service,
		recovery: recovery,
		queries:  queries,
	}
}

func (service *GRPCService) ValidateWorkflow(
	ctx context.Context,
	request *runtimev1.ValidateWorkflowRequest,
) (*emptypb.Empty, error) {
	if request == nil || request.Definition == nil {
		return nil, status.Error(codes.InvalidArgument, "workflow definition is required")
	}
	definition := mapWorkflowFromGRPC(request.GetDefinition(), requestcontext.CompanyID(ctx))
	if err := service.service.Validate(definition); err != nil {
		return nil, mapGRPCError(err)
	}
	return &emptypb.Empty{}, nil
}

func (service *GRPCService) ExecuteSync(
	ctx context.Context,
	request *runtimev1.ExecuteRequest,
) (*runtimev1.ExecutionResponse, error) {
	if requestcontext.IdempotencyKey(ctx) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key is required")
	}
	workflow, startInput, err := mapGRPCExecuteRequest(
		request, requestcontext.CompanyID(ctx),
	)
	if err != nil {
		return nil, err
	}
	outcome, err := service.service.ExecuteManual(
		ctx,
		workflow,
		startInput,
		requestcontext.CorrelationID(ctx),
		requestcontext.IdempotencyKey(ctx),
		manualExecutionFingerprint(workflow, startInput, "SYNC"),
		"SYNC",
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCManualExecutionBatch(outcome, requestcontext.RequestID(ctx)), nil
}

func (service *GRPCService) ExecuteAsync(
	ctx context.Context,
	request *runtimev1.ExecuteRequest,
) (*runtimev1.ExecutionResponse, error) {
	if requestcontext.IdempotencyKey(ctx) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key is required")
	}
	workflow, startInput, err := mapGRPCExecuteRequest(
		request, requestcontext.CompanyID(ctx),
	)
	if err != nil {
		return nil, err
	}
	outcome, err := service.service.ExecuteManual(
		ctx,
		workflow,
		startInput,
		requestcontext.CorrelationID(ctx),
		requestcontext.IdempotencyKey(ctx),
		manualExecutionFingerprint(workflow, startInput, "ASYNC"),
		"ASYNC",
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCManualExecutionBatch(outcome, requestcontext.RequestID(ctx)), nil
}

func (service *GRPCService) ListExecutions(
	ctx context.Context,
	request *runtimev1.ListExecutionsRequest,
) (*runtimev1.ExecutionPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 100)
	if err != nil {
		return nil, err
	}
	statusFilter := model.ExecutionStatus("")
	if request.GetStatus() != "" {
		statusFilter, err = ParseExecutionStatus(request.GetStatus())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}
	page, err := service.queries.List(
		ctx,
		requestcontext.CompanyID(ctx),
		request.GetWorkflowId(),
		statusFilter,
		request.GetCursor(),
		limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCExecutionPage(page), nil
}

func (service *GRPCService) GetExecution(
	ctx context.Context,
	request *runtimev1.GetExecutionRequest,
) (*runtimev1.ExecutionSummary, error) {
	execution, err := service.queries.Get(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCExecution(execution), nil
}

func (service *GRPCService) GetExecutionDefinition(
	ctx context.Context,
	request *runtimev1.GetExecutionRequest,
) (*runtimev1.ExecutionDefinition, error) {
	snapshot, err := service.queries.Definition(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCExecutionDefinition(request.GetExecutionId(), snapshot), nil
}

func (service *GRPCService) ListExecutionNodes(
	ctx context.Context,
	request *runtimev1.ListExecutionResourceRequest,
) (*runtimev1.NodeExecutionPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 200)
	if err != nil {
		return nil, err
	}
	page, err := service.queries.Nodes(
		ctx,
		requestcontext.CompanyID(ctx),
		request.GetExecutionId(),
		request.GetCursor(),
		limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCNodeExecutionPage(page), nil
}

func (service *GRPCService) ListExecutionEvents(
	ctx context.Context,
	request *runtimev1.ListExecutionResourceRequest,
) (*runtimev1.ExecutionEventPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 200)
	if err != nil {
		return nil, err
	}
	page, err := service.queries.Events(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
		request.GetCursor(), limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCEventPage(page), nil
}

func (service *GRPCService) ListExecutionLogs(
	ctx context.Context,
	request *runtimev1.ListExecutionResourceRequest,
) (*runtimev1.ExecutionLogPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 200)
	if err != nil {
		return nil, err
	}
	page, err := service.queries.Logs(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
		request.GetCursor(), limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCLogPage(page), nil
}

func (service *GRPCService) ListExecutionErrors(
	ctx context.Context,
	request *runtimev1.ListExecutionResourceRequest,
) (*runtimev1.ExecutionErrorPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 200)
	if err != nil {
		return nil, err
	}
	page, err := service.queries.Errors(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
		request.GetCursor(), limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCExecutionErrorPage(page), nil
}

func (service *GRPCService) RecoverExecution(
	ctx context.Context,
	request *runtimev1.RecoverExecutionRequest,
) (*runtimev1.RecoveryResponse, error) {
	if requestcontext.IdempotencyKey(ctx) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key is required")
	}
	companyID := requestcontext.CompanyID(ctx)
	outcome, err := service.recovery.Recover(
		ctx,
		companyID,
		request.GetExecutionId(),
		requestcontext.IdempotencyKey(ctx),
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCRecoveryOutcome(outcome), nil
}

func grpcLimit(raw uint32, maximum int) (int, error) {
	if raw == 0 {
		return 50, nil
	}
	if raw > uint32(maximum) {
		return 0, status.Errorf(
			codes.InvalidArgument, "limit must be between 1 and %d", maximum,
		)
	}
	return int(raw), nil
}

func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	var nodeError *plugin.NodeError
	if errors.As(err, &nodeError) {
		switch nodeError.Category {
		case "VALIDATION":
			return status.Error(codes.InvalidArgument, nodeError.Message)
		case "EXECUTION":
			if nodeError.CanRetry {
				return status.Error(codes.Unavailable, nodeError.Message)
			}
			return status.Error(codes.FailedPrecondition, nodeError.Message)
		default:
			return status.Error(codes.Internal, nodeError.Message)
		}
	}
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, "the requested resource was not found")
	case errors.Is(err, repository.ErrIdempotencyConflict):
		return statusWithErrorInfo(
			codes.AlreadyExists,
			"idempotency key was already used for a different request",
			ErrorReasonIdempotencyKeyReused,
		)
	case errors.Is(err, ErrIdempotencyRequestInProgress):
		return statusWithErrorInfo(
			codes.FailedPrecondition,
			"a request with this idempotency key is still in progress",
			ErrorReasonIdempotencyRequestInProgress,
		)
	case errors.Is(err, ErrRecoveryUnsupported),
		errors.Is(err, repository.ErrRecoveryConflict):
		return statusWithErrorInfo(
			codes.FailedPrecondition,
			"the execution cannot be recovered in its current state",
			ErrorReasonRecoveryNotSupported,
		)
	case errors.Is(err, ErrAsyncUnavailable):
		return status.Error(codes.Unavailable, "workflow execution is unavailable")
	case errors.Is(err, ErrStartInputNotAccepted):
		return validationStatus([]*runtimev1.ValidationIssue{{
			Code:   string(ErrorReasonInitialVariablesNotAccepted),
			Field:  "initialVariables",
			Reason: "no workflow entry node accepts the supplied initial variables",
		}})
	case errors.Is(err, ErrInvalidExecutionOrigin):
		return statusWithErrorInfo(
			codes.InvalidArgument,
			"execution origin is incompatible with the workflow",
			ErrorReasonInvalidExecutionOrigin,
		)
	case errors.Is(err, ErrAmbiguousRecordSources):
		return statusWithErrorInfo(
			codes.InvalidArgument,
			"multiple record sources require an explicit correlation contract",
			ErrorReasonInvalidExecutionOrigin,
		)
	case errors.Is(err, workflow.ErrInvalidWorkflow):
		return WorkflowValidationGRPCStatus(err)
	default:
		return status.Error(codes.Internal, "an unexpected internal error occurred")
	}
}

// WorkflowValidationGRPCStatus maps workflow.ErrInvalidWorkflow to a gRPC
// InvalidArgument status that carries the original ValidationIssue details.
func WorkflowValidationGRPCStatus(err error) error {
	var validationError *workflow.WorkflowValidationError
	issues := make([]*runtimev1.ValidationIssue, 0)
	if errors.As(err, &validationError) {
		for _, issue := range validationError.Issues {
			issues = append(issues, &runtimev1.ValidationIssue{
				Code:          issue.Code,
				Field:         issue.Field,
				Reason:        issue.Reason,
				NodeId:        issue.NodeID,
				EdgeId:        issue.EdgeID,
				PluginType:    issue.PluginType,
				PluginVersion: issue.PluginVersion,
				Expected:      issue.Expected,
				Actual:        issue.Actual,
				CyclePath:     issue.CyclePath,
			})
		}
	}
	return validationStatus(issues)
}

func validationStatus(issues []*runtimev1.ValidationIssue) error {
	base := status.New(codes.InvalidArgument, "workflow definition failed validation")
	errorInfo := &errdetails.ErrorInfo{
		Reason: string(ErrorReasonWorkflowValidationFailed),
		Domain: "miletos.runtime",
	}
	badRequest := &errdetails.BadRequest{}
	for _, issue := range issues {
		badRequest.FieldViolations = append(
			badRequest.FieldViolations,
			&errdetails.BadRequest_FieldViolation{
				Field:       issue.Field,
				Description: issue.Reason,
			},
		)
	}
	withDetails, detailErr := base.WithDetails(errorInfo)
	if detailErr != nil {
		return base.Err()
	}
	for _, issue := range issues {
		withDetails, detailErr = withDetails.WithDetails(issue)
		if detailErr != nil {
			return base.Err()
		}
	}
	withDetails, detailErr = withDetails.WithDetails(badRequest)
	if detailErr != nil {
		return base.Err()
	}
	return withDetails.Err()
}

func StatusWithErrorInfo(code codes.Code, message string, reason ErrorReason) error {
	return statusWithErrorInfo(code, message, reason)
}

func statusWithErrorInfo(code codes.Code, message string, reason ErrorReason) error {
	base := status.New(code, message)
	withDetails, err := base.WithDetails(&errdetails.ErrorInfo{
		Reason: string(reason),
		Domain: "miletos.runtime",
	})
	if err != nil {
		return base.Err()
	}
	return withDetails.Err()
}
