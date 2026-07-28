package executionfeature

import (
	"context"
	"fmt"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

type ExecutionQueryApplication interface {
	GetExecution(context.Context, workflow.CompanyID, execution.WorkflowExecutionID) (repository.WorkflowExecutionRecord, error)
	ListExecutions(context.Context, workflow.CompanyID, repository.WorkflowExecutionFilter, repository.PageRequest) (repository.Page[repository.WorkflowExecutionRecord], error)
	GetExecutionDefinition(context.Context, workflow.CompanyID, execution.WorkflowExecutionID) (repository.DefinitionSnapshot, error)
	ListExecutionNodes(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, repository.PageRequest) (repository.Page[repository.NodeExecutionRecord], error)
	ListExecutionEvents(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, repository.PageRequest) (repository.Page[repository.ExecutionEventRecord], error)
	ListExecutionLogs(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, repository.PageRequest) (repository.Page[repository.ExecutionLogRecord], error)
	ListExecutionErrors(context.Context, workflow.CompanyID, execution.WorkflowExecutionID, repository.PageRequest) (repository.Page[repository.ExecutionErrorRecord], error)
}

// RepositoryExecutionQueries centralizes the read side of the execution feature.
// It replaces six one-purpose application structs that all wrapped the same store.
type executionQueryStore interface {
	repository.WorkflowExecutionReader
	repository.WorkflowSnapshotRepository
	repository.NodeExecutionReader
	repository.ExecutionEventReader
	repository.ExecutionLogReader
	repository.ExecutionErrorReader
}

type RepositoryExecutionQueries struct {
	store executionQueryStore
}

func NewRepositoryExecutionQueries(store executionQueryStore) (RepositoryExecutionQueries, error) {
	if store == nil {
		return RepositoryExecutionQueries{}, fmt.Errorf("execution query store must not be nil")
	}
	return RepositoryExecutionQueries{store: store}, nil
}

func (queries RepositoryExecutionQueries) GetExecution(ctx context.Context,
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID) (repository.WorkflowExecutionRecord, error) {
	if ctx == nil {
		return repository.WorkflowExecutionRecord{}, fmt.Errorf("execution query context must not be nil")
	}
	return queries.store.GetWorkflowExecution(ctx, companyID,
		workflowExecutionID)
}

func (queries RepositoryExecutionQueries) ListExecutions(ctx context.Context,
	companyID workflow.CompanyID, filter repository.WorkflowExecutionFilter, pageRequest repository.PageRequest,
) (repository.Page[repository.WorkflowExecutionRecord], error) {
	if ctx == nil {
		return repository.Page[repository.WorkflowExecutionRecord]{}, fmt.Errorf(
			"execution query context must not be nil")
	}
	return queries.store.ListWorkflowExecutions(ctx,
		companyID, filter, pageRequest,
	)
}

func (queries RepositoryExecutionQueries) GetExecutionDefinition(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID) (repository.DefinitionSnapshot, error) {
	if ctx == nil {
		return repository.DefinitionSnapshot{}, fmt.Errorf("execution definition query context must not be nil")
	}
	executionRecord, err := queries.store.GetWorkflowExecution(
		ctx, companyID, workflowExecutionID,
	)
	if err != nil {
		return repository.DefinitionSnapshot{}, err
	}
	return queries.store.GetByID(
		ctx, companyID, executionRecord.SnapshotID(),
	)
}

func (queries RepositoryExecutionQueries) ListExecutionNodes(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest) (repository.Page[repository.NodeExecutionRecord], error) {
	if ctx == nil {
		return repository.Page[repository.NodeExecutionRecord]{}, fmt.Errorf("execution nodes query context must not be nil")
	}
	if _, err := queries.store.GetWorkflowExecution(ctx, companyID,
		workflowExecutionID); err != nil {
		return repository.Page[repository.NodeExecutionRecord]{}, err
	}
	return queries.store.ListNodeExecutions(
		ctx, companyID, workflowExecutionID,
		pageRequest)
}

func (queries RepositoryExecutionQueries) ListExecutionEvents(ctx context.Context,
	companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest,
) (repository.Page[repository.ExecutionEventRecord], error) {
	if ctx == nil {
		return repository.Page[repository.ExecutionEventRecord]{}, fmt.Errorf(
			"execution events query context must not be nil")
	}
	if _, err := queries.store.GetWorkflowExecution(ctx,
		companyID, workflowExecutionID); err != nil {
		return repository.Page[repository.ExecutionEventRecord]{}, err
	}
	return queries.store.ListExecutionEvents(ctx, companyID,
		workflowExecutionID, pageRequest)
}

func (queries RepositoryExecutionQueries) ListExecutionLogs(
	ctx context.Context, companyID workflow.CompanyID, workflowExecutionID execution.WorkflowExecutionID,
	pageRequest repository.PageRequest) (repository.Page[repository.ExecutionLogRecord], error) {
	if ctx == nil {
		return repository.Page[repository.ExecutionLogRecord]{}, fmt.Errorf("execution logs query context must not be nil")
	}
	if _, err := queries.store.GetWorkflowExecution(
		ctx, companyID, workflowExecutionID,
	); err != nil {
		return repository.Page[repository.ExecutionLogRecord]{}, err
	}
	return queries.store.ListExecutionLogs(ctx,
		companyID, workflowExecutionID, pageRequest,
	)
}

func (queries RepositoryExecutionQueries) ListExecutionErrors(ctx context.Context, companyID workflow.CompanyID,
	workflowExecutionID execution.WorkflowExecutionID, pageRequest repository.PageRequest) (repository.Page[repository.ExecutionErrorRecord], error) {
	if ctx == nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, fmt.Errorf("execution errors query context must not be nil")
	}
	if _, err := queries.store.GetWorkflowExecution(ctx, companyID,
		workflowExecutionID); err != nil {
		return repository.Page[repository.ExecutionErrorRecord]{}, err
	}
	return queries.store.ListExecutionErrors(
		ctx, companyID, workflowExecutionID,
		pageRequest)
}
