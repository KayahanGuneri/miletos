package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"miletos-go/internal/features/workflowruntime/execution/model"
	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/workflow"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
	"miletos-go/internal/shared/requestcontext"
)

type GRPCService struct {
	runtimev1.UnimplementedExecutionServiceServer
	service    *ExecutionService
	recovery   *RecoveryService
	workflows  *workflow.WorkflowRepository
	executions *repository.ExecutionRepository
}

func NewGRPCService(
	service *ExecutionService,
	recovery *RecoveryService,
	workflows *workflow.WorkflowRepository,
	executions *repository.ExecutionRepository,
) *GRPCService {
	return &GRPCService{
		service:    service,
		recovery:   recovery,
		workflows:  workflows,
		executions: executions,
	}
}

func (service *GRPCService) ExecuteSync(
	ctx context.Context,
	request *runtimev1.ExecuteRequest,
) (*runtimev1.ExecutionResponse, error) {
	if requestcontext.IdempotencyKey(ctx) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key is required")
	}
	workflow, variables, err := mapGRPCExecuteRequest(
		request, requestcontext.CompanyID(ctx),
	)
	if err != nil {
		return nil, err
	}
	outcome, err := service.service.ExecuteSync(
		ctx,
		workflow,
		variables,
		requestcontext.CorrelationID(ctx),
		requestcontext.IdempotencyKey(ctx),
		grpcFingerprint(request, "SYNC"),
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCExecutionOutcome(outcome), nil
}

func (service *GRPCService) ExecuteAsync(
	ctx context.Context,
	request *runtimev1.ExecuteRequest,
) (*runtimev1.ExecutionResponse, error) {
	if requestcontext.IdempotencyKey(ctx) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency key is required")
	}
	workflow, variables, err := mapGRPCExecuteRequest(
		request, requestcontext.CompanyID(ctx),
	)
	if err != nil {
		return nil, err
	}
	outcome, err := service.service.ExecuteAsync(
		ctx,
		workflow,
		variables,
		requestcontext.CorrelationID(ctx),
		requestcontext.IdempotencyKey(ctx),
		grpcFingerprint(request, "ASYNC"),
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return mapGRPCExecutionOutcome(outcome), nil
}

func (service *GRPCService) ListExecutions(
	ctx context.Context,
	request *runtimev1.ListExecutionsRequest,
) (*runtimev1.ExecutionPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 100)
	if err != nil {
		return nil, err
	}
	page, err := service.executions.List(
		ctx,
		requestcontext.CompanyID(ctx),
		request.GetWorkflowId(),
		request.GetStatus(),
		request.GetCursor(),
		limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	items := make([]*runtimev1.ExecutionSummary, 0, len(page.Items))
	for _, execution := range page.Items {
		items = append(items, mapGRPCExecution(execution))
	}
	return &runtimev1.ExecutionPage{
		Items:   items,
		Cursor:  page.Next,
		HasNext: page.HasNext,
	}, nil
}

func (service *GRPCService) GetExecution(
	ctx context.Context,
	request *runtimev1.GetExecutionRequest,
) (*runtimev1.ExecutionSummary, error) {
	execution, err := service.executions.FindByID(
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
	snapshot, err := service.workflows.FindByExecutionID(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return &runtimev1.ExecutionDefinition{
		ExecutionId:      request.GetExecutionId(),
		SnapshotId:       snapshot.ID,
		WorkflowId:       snapshot.Workflow.ID,
		WorkflowRevision: snapshot.Workflow.Revision,
		WorkflowName:     snapshot.Workflow.Name,
		Definition:       mapGRPCWorkflow(snapshot.Workflow),
		CreatedAt:        grpcserver.Time(snapshot.CreatedAt),
	}, nil
}

func (service *GRPCService) ListExecutionNodes(
	ctx context.Context,
	request *runtimev1.ListExecutionResourceRequest,
) (*runtimev1.NodeExecutionPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 200)
	if err != nil {
		return nil, err
	}
	page, err := service.executions.ListNodes(
		ctx,
		requestcontext.CompanyID(ctx),
		request.GetExecutionId(),
		request.GetCursor(),
		limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	items := make([]*runtimev1.NodeExecution, 0, len(page.Items))
	for _, node := range page.Items {
		items = append(items, mapGRPCNodeExecution(node))
	}
	return &runtimev1.NodeExecutionPage{
		Items: items, Cursor: page.Next, HasNext: page.HasNext,
	}, nil
}

func (service *GRPCService) ListExecutionEvents(
	ctx context.Context,
	request *runtimev1.ListExecutionResourceRequest,
) (*runtimev1.ExecutionEventPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 200)
	if err != nil {
		return nil, err
	}
	page, err := service.executions.ListEvents(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
		request.GetCursor(), limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	items := make([]*runtimev1.ExecutionEvent, 0, len(page.Items))
	for _, event := range page.Items {
		items = append(items, mapGRPCEvent(event))
	}
	return &runtimev1.ExecutionEventPage{
		Items: items, Cursor: page.Next, HasNext: page.HasNext,
	}, nil
}

func (service *GRPCService) ListExecutionLogs(
	ctx context.Context,
	request *runtimev1.ListExecutionResourceRequest,
) (*runtimev1.ExecutionLogPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 200)
	if err != nil {
		return nil, err
	}
	page, err := service.executions.ListLogs(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
		request.GetCursor(), limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	items := make([]*runtimev1.ExecutionLog, 0, len(page.Items))
	for _, entry := range page.Items {
		items = append(items, mapGRPCLog(entry))
	}
	return &runtimev1.ExecutionLogPage{
		Items: items, Cursor: page.Next, HasNext: page.HasNext,
	}, nil
}

func (service *GRPCService) ListExecutionErrors(
	ctx context.Context,
	request *runtimev1.ListExecutionResourceRequest,
) (*runtimev1.ExecutionErrorPage, error) {
	limit, err := grpcLimit(request.GetLimit(), 200)
	if err != nil {
		return nil, err
	}
	page, err := service.executions.ListErrors(
		ctx, requestcontext.CompanyID(ctx), request.GetExecutionId(),
		request.GetCursor(), limit,
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	items := make([]*runtimev1.ExecutionError, 0, len(page.Items))
	for _, entry := range page.Items {
		items = append(items, mapGRPCExecutionError(entry))
	}
	return &runtimev1.ExecutionErrorPage{
		Items: items, Cursor: page.Next, HasNext: page.HasNext,
	}, nil
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
		grpcFingerprint(map[string]string{
			"companyId":   companyID,
			"executionId": request.GetExecutionId(),
		}, "RECOVER"),
	)
	if err != nil {
		return nil, mapGRPCError(err)
	}
	return &runtimev1.RecoveryResponse{
		SourceExecutionId:   outcome.SourceExecutionID,
		RecoveryExecutionId: outcome.RecoveryExecutionID,
		Status:              outcome.Status,
		PreservedNodeCount:  uint32(outcome.PreservedNodeCount),
		ScheduledNodeCount:  uint32(outcome.ScheduledNodeCount),
		ResetNodeCount:      uint32(outcome.ResetNodeCount),
		CreatedAt:           grpcserver.Time(outcome.CreatedAt),
		Replayed:            outcome.Replayed,
	}, nil
}

func mapGRPCExecuteRequest(
	request *runtimev1.ExecuteRequest,
	companyID string,
) (workflow.Workflow, map[string]any, error) {
	if request == nil || request.Definition == nil {
		return workflow.Workflow{}, nil, status.Error(
			codes.InvalidArgument, "workflow definition is required",
		)
	}
	workflow := mapWorkflowFromGRPC(request.Definition, companyID)
	var variables map[string]any
	if request.InitialVariables != nil {
		variables = request.InitialVariables.AsMap()
	}
	return workflow, variables, nil
}

func mapWorkflowFromGRPC(
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

func mapGRPCWorkflow(workflow workflow.Workflow) *runtimev1.WorkflowDefinition {
	result := &runtimev1.WorkflowDefinition{
		WorkflowId:       workflow.ID,
		Name:             workflow.Name,
		WorkflowRevision: workflow.Revision,
		Metadata:         grpcserver.Struct(workflow.Metadata),
		Nodes:            make([]*runtimev1.WorkflowNode, 0, len(workflow.Nodes)),
		Edges:            make([]*runtimev1.WorkflowEdge, 0, len(workflow.Edges)),
	}
	for _, node := range workflow.Nodes {
		mapped := &runtimev1.WorkflowNode{
			NodeId:        node.ID,
			PluginType:    node.Type,
			PluginVersion: node.Version,
			Configuration: grpcserver.Struct(node.Configuration),
		}
		if node.Position != nil {
			mapped.Position = &runtimev1.NodePosition{
				X: node.Position.X, Y: node.Position.Y,
			}
		}
		result.Nodes = append(result.Nodes, mapped)
	}
	for _, edge := range workflow.Edges {
		result.Edges = append(result.Edges, &runtimev1.WorkflowEdge{
			EdgeId:           edge.ID,
			SourceNodeId:     edge.SourceNodeID,
			SourceOutputPort: edge.SourceOutputPort,
			TargetNodeId:     edge.TargetNodeID,
			TargetInputPort:  edge.TargetInputPort,
		})
	}
	return result
}

func mapGRPCExecutionOutcome(outcome ExecutionOutcome) *runtimev1.ExecutionResponse {
	execution := outcome.Execution
	return &runtimev1.ExecutionResponse{
		ExecutionId:      execution.ID,
		WorkflowId:       execution.WorkflowID,
		WorkflowRevision: execution.WorkflowRevision,
		SnapshotId:       execution.SnapshotID,
		Mode:             execution.Mode,
		Status:           execution.Status,
		ExecutionOrigin:  execution.Origin,
		CorrelationId:    execution.CorrelationID,
		CreatedAt:        grpcserver.Time(execution.CreatedAt),
		StartedAt:        grpcserver.OptionalTime(execution.StartedAt),
		FinishedAt:       grpcserver.OptionalTime(execution.FinishedAt),
		ScheduledRoots:   uint32(outcome.ScheduledRoots),
		Replayed:         outcome.Replayed,
		TerminalOutputs:  grpcserver.Struct(execution.TerminalOutputs),
		FailureSummary:   grpcserver.Struct(execution.Failure),
	}
}

func mapGRPCExecution(execution model.Execution) *runtimev1.ExecutionSummary {
	return &runtimev1.ExecutionSummary{
		ExecutionId:      execution.ID,
		WorkflowId:       execution.WorkflowID,
		WorkflowRevision: execution.WorkflowRevision,
		SnapshotId:       execution.SnapshotID,
		Mode:             execution.Mode,
		Status:           execution.Status,
		ExecutionOrigin:  execution.Origin,
		CorrelationId:    execution.CorrelationID,
		CreatedAt:        grpcserver.Time(execution.CreatedAt),
		ValidatingAt:     grpcserver.OptionalTime(execution.ValidatingAt),
		QueuedAt:         grpcserver.OptionalTime(execution.QueuedAt),
		StartedAt:        grpcserver.OptionalTime(execution.StartedAt),
		FinishedAt:       grpcserver.OptionalTime(execution.FinishedAt),
		UpdatedAt:        grpcserver.Time(execution.UpdatedAt),
		TerminalOutputs:  grpcserver.Struct(execution.TerminalOutputs),
		FailureSummary:   grpcserver.Struct(execution.Failure),
		IsStalled:        execution.IsStalled,
	}
}

func mapGRPCNodeExecution(node model.NodeExecution) *runtimev1.NodeExecution {
	return &runtimev1.NodeExecution{
		NodeExecutionId: node.ID,
		ExecutionId:     node.ExecutionID,
		NodeId:          node.NodeID,
		PluginType:      node.Type,
		PluginVersion:   node.Version,
		Status:          node.Status,
		Attempt:         uint32(node.Attempt),
		CreatedAt:       grpcserver.Time(node.CreatedAt),
		ReadyAt:         grpcserver.OptionalTime(node.ReadyAt),
		QueuedAt:        grpcserver.OptionalTime(node.QueuedAt),
		StartedAt:       grpcserver.OptionalTime(node.StartedAt),
		FinishedAt:      grpcserver.OptionalTime(node.FinishedAt),
		NextAttemptAt:   grpcserver.OptionalTime(node.NextAttemptAt),
		UpdatedAt:       grpcserver.Time(node.UpdatedAt),
		InputSummary:    grpcserver.Struct(node.Input),
		OutputSummary:   grpcserver.Struct(node.Output),
		FailureSummary:  grpcserver.Struct(node.Failure),
	}
}

func mapGRPCEvent(event model.ExecutionEvent) *runtimev1.ExecutionEvent {
	return &runtimev1.ExecutionEvent{
		EventId:         event.ID,
		ExecutionId:     event.ExecutionID,
		NodeExecutionId: event.NodeExecutionID,
		SequenceNumber:  event.Sequence,
		Type:            event.Type,
		PreviousStatus:  event.PreviousStatus,
		NewStatus:       event.NewStatus,
		CorrelationId:   event.CorrelationID,
		CausationId:     event.CausationID,
		SafeMessage:     event.Message,
		Metadata:        grpcserver.Struct(event.Metadata),
		CreatedAt:       grpcserver.Time(event.CreatedAt),
	}
}

func mapGRPCLog(entry model.ExecutionLog) *runtimev1.ExecutionLog {
	return &runtimev1.ExecutionLog{
		LogId:           entry.ID,
		ExecutionId:     entry.ExecutionID,
		NodeExecutionId: entry.NodeExecutionID,
		SequenceNumber:  entry.Sequence,
		Level:           entry.Level,
		Message:         entry.Message,
		Metadata:        grpcserver.Struct(entry.Metadata),
		CreatedAt:       grpcserver.Time(entry.CreatedAt),
	}
}

func mapGRPCExecutionError(entry model.ExecutionError) *runtimev1.ExecutionError {
	return &runtimev1.ExecutionError{
		ErrorId:         entry.ID,
		ExecutionId:     entry.ExecutionID,
		NodeExecutionId: entry.NodeExecutionID,
		RelatedEventId:  entry.RelatedEventID,
		Category:        entry.Category,
		Code:            entry.Code,
		SafeMessage:     entry.Message,
		Retryable:       entry.Retryable,
		Details:         grpcserver.Struct(entry.Details),
		CreatedAt:       grpcserver.Time(entry.CreatedAt),
	}
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

func grpcFingerprint(value any, mode string) string {
	encoded, _ := json.Marshal(map[string]any{"request": value, "mode": mode})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func mapGRPCError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return status.Error(codes.NotFound, "the requested resource was not found")
	case errors.Is(err, repository.ErrIdempotencyConflict):
		return statusWithErrorInfo(
			codes.AlreadyExists,
			"idempotency key was already used for a different request",
			"IDEMPOTENCY_KEY_REUSED",
		)
	case errors.Is(err, ErrRecoveryUnsupported),
		errors.Is(err, repository.ErrRecoveryConflict):
		return statusWithErrorInfo(
			codes.FailedPrecondition,
			"the execution cannot be recovered in its current state",
			"RECOVERY_NOT_SUPPORTED",
		)
	case errors.Is(err, ErrAsyncUnavailable):
		return status.Error(codes.Unavailable, "workflow execution is unavailable")
	case errors.Is(err, ErrInitialVariablesNotAccepted):
		return validationStatus([]*runtimev1.ValidationIssue{{
			Code:   "INITIAL_VARIABLES_NOT_ACCEPTED",
			Field:  "initialVariables",
			Reason: "no workflow root accepts the supplied initial variables",
		}})
	case errors.Is(err, ErrInvalidExecutionOrigin):
		return statusWithErrorInfo(
			codes.InvalidArgument,
			"execution origin is incompatible with the workflow",
			"INVALID_EXECUTION_ORIGIN",
		)
	case errors.Is(err, workflow.ErrInvalidWorkflow):
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
	default:
		return status.Error(codes.Internal, "an unexpected internal error occurred")
	}
}

func validationStatus(issues []*runtimev1.ValidationIssue) error {
	base := status.New(codes.InvalidArgument, "workflow definition failed validation")
	errorInfo := &errdetails.ErrorInfo{
		Reason: "WORKFLOW_VALIDATION_FAILED",
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

func statusWithErrorInfo(code codes.Code, message string, reason string) error {
	base := status.New(code, message)
	withDetails, err := base.WithDetails(&errdetails.ErrorInfo{
		Reason: reason,
		Domain: "miletos.runtime",
	})
	if err != nil {
		return base.Err()
	}
	return withDetails.Err()
}
