package execution

import (
	"context"
	"errors"
	"strings"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

var allowedExecutionStatuses = map[model.ExecutionStatus]struct{}{
	model.ExecutionCreated: {}, model.ExecutionValidating: {},
	model.ExecutionRejected: {}, model.ExecutionQueued: {},
	model.ExecutionRunning: {}, model.ExecutionSucceeded: {},
	model.ExecutionFailed: {}, model.ExecutionCancelled: {},
	model.ExecutionTimedOut: {},
}

var ErrInvalidObservationResource = errors.New("observation resource is invalid")

func ParseExecutionStatus(raw string) (model.ExecutionStatus, error) {
	status := model.ExecutionStatus(strings.TrimSpace(raw))
	if _, exists := allowedExecutionStatuses[status]; !exists {
		return "", errors.New("execution status filter is invalid")
	}
	return status, nil
}

type ExecutionQueryService struct {
	executions   *repository.ExecutionRepository
	workflows    *workflow.WorkflowRepository
	registry     *plugin.NodeRegistry
	observations map[model.ObservationResource]observationStrategy
}

type observationStrategy func(
	context.Context, string, string, string, int,
) (any, error)

func NewExecutionQueryService(
	executions *repository.ExecutionRepository,
	workflows *workflow.WorkflowRepository,
	registry *plugin.NodeRegistry,
) *ExecutionQueryService {
	service := &ExecutionQueryService{
		executions: executions, workflows: workflows, registry: registry,
	}
	service.observations = map[model.ObservationResource]observationStrategy{
		model.ObservationNodes:  service.listNodes,
		model.ObservationEvents: service.listEvents,
		model.ObservationLogs:   service.listLogs,
		model.ObservationErrors: service.listErrors,
	}
	return service
}

func (service *ExecutionQueryService) List(
	ctx context.Context, companyID, workflowID string, status model.ExecutionStatus,
	after string, limit int,
) (model.Page[model.Execution], error) {
	page, err := service.executions.List(ctx, companyID, workflowID, status, after, limit)
	if err != nil {
		return model.Page[model.Execution]{}, err
	}
	for index := range page.Items {
		page.Items[index] = redactExecution(page.Items[index])
	}
	return page, nil
}

func (service *ExecutionQueryService) Get(
	ctx context.Context, companyID, executionID string,
) (model.Execution, error) {
	execution, err := service.executions.FindByID(ctx, companyID, executionID)
	if err != nil {
		return model.Execution{}, err
	}
	return redactExecution(execution), nil
}

func redactExecution(execution model.Execution) model.Execution {
	execution.TerminalOutputs = redactObject(execution.TerminalOutputs)
	execution.Failure = redactObject(execution.Failure)
	return execution
}

func (service *ExecutionQueryService) Definition(
	ctx context.Context, companyID, executionID string,
) (workflow.WorkflowSnapshot, error) {
	snapshot, err := service.workflows.FindByExecutionID(ctx, companyID, executionID)
	if err != nil {
		return workflow.WorkflowSnapshot{}, err
	}
	for index := range snapshot.Workflow.Nodes {
		snapshot.Workflow.Nodes[index].Configuration = redactObject(
			snapshot.Workflow.Nodes[index].Configuration,
		)
	}
	return snapshot, nil
}

func (service *ExecutionQueryService) Nodes(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.NodeExecution], error) {
	if _, err := service.executions.FindByID(ctx, companyID, executionID); err != nil {
		return model.Page[model.NodeExecution]{}, err
	}
	return service.loadNodes(ctx, companyID, executionID, after, limit)
}

func (service *ExecutionQueryService) loadNodes(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.NodeExecution], error) {
	page, err := service.executions.ListNodes(ctx, companyID, executionID, after, limit)
	if err != nil {
		return model.Page[model.NodeExecution]{}, err
	}
	snapshot, err := service.workflows.FindByExecutionID(ctx, companyID, executionID)
	if err != nil {
		return model.Page[model.NodeExecution]{}, err
	}
	configurationByNode := make(map[string]map[string]any, len(snapshot.Workflow.Nodes))
	for _, node := range snapshot.Workflow.Nodes {
		configurationByNode[node.ID] = node.Configuration
	}
	for index := range page.Items {
		page.Items[index], err = decodeNodeExecutionOutcome(
			page.Items[index], service.registry,
		)
		if err != nil {
			return model.Page[model.NodeExecution]{}, err
		}
		page.Items[index].Configuration = redactObject(configurationByNode[page.Items[index].NodeID])
		page.Items[index].Input = redactObject(page.Items[index].Input)
		page.Items[index].Output = redactObject(page.Items[index].Output)
		page.Items[index].Failure = redactObject(page.Items[index].Failure)
	}
	return page, nil
}

func (service *ExecutionQueryService) Observations(
	ctx context.Context, companyID, executionID string,
	resource model.ObservationResource, after string, limit int,
) (any, error) {
	if _, err := service.executions.FindByID(ctx, companyID, executionID); err != nil {
		return nil, err
	}
	strategy, exists := service.observations[resource]
	if !exists {
		return nil, ErrInvalidObservationResource
	}
	return strategy(ctx, companyID, executionID, after, limit)
}

func (service *ExecutionQueryService) Events(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionEvent], error) {
	if _, err := service.executions.FindByID(ctx, companyID, executionID); err != nil {
		return model.Page[model.ExecutionEvent]{}, err
	}
	return service.executions.ListEvents(ctx, companyID, executionID, after, limit)
}

func (service *ExecutionQueryService) Logs(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionLog], error) {
	if _, err := service.executions.FindByID(ctx, companyID, executionID); err != nil {
		return model.Page[model.ExecutionLog]{}, err
	}
	return service.executions.ListLogs(ctx, companyID, executionID, after, limit)
}

func (service *ExecutionQueryService) Errors(
	ctx context.Context, companyID, executionID, after string, limit int,
) (model.Page[model.ExecutionError], error) {
	if _, err := service.executions.FindByID(ctx, companyID, executionID); err != nil {
		return model.Page[model.ExecutionError]{}, err
	}
	return service.executions.ListErrors(ctx, companyID, executionID, after, limit)
}

func (service *ExecutionQueryService) listEvents(
	ctx context.Context, companyID, executionID, after string, limit int,
) (any, error) {
	return service.executions.ListEvents(ctx, companyID, executionID, after, limit)
}

func (service *ExecutionQueryService) listNodes(
	ctx context.Context, companyID, executionID, after string, limit int,
) (any, error) {
	return service.loadNodes(ctx, companyID, executionID, after, limit)
}

func (service *ExecutionQueryService) listLogs(
	ctx context.Context, companyID, executionID, after string, limit int,
) (any, error) {
	return service.executions.ListLogs(ctx, companyID, executionID, after, limit)
}

func (service *ExecutionQueryService) listErrors(
	ctx context.Context, companyID, executionID, after string, limit int,
) (any, error) {
	return service.executions.ListErrors(ctx, companyID, executionID, after, limit)
}
