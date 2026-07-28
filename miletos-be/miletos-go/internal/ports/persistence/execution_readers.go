package repository

import (
	"context"

	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type WorkflowSnapshotRepository interface {
	Create(ctx context.Context, snapshot DefinitionSnapshot) error
	GetByID(
		ctx context.Context,
		companyID workflow.CompanyID,
		snapshotID DefinitionSnapshotID,
	) (DefinitionSnapshot, error)
}

type WorkflowExecutionReader interface {
	GetWorkflowExecution(
		ctx context.Context,
		companyID workflow.CompanyID,
		workflowExecutionID execution.WorkflowExecutionID,
	) (WorkflowExecutionRecord, error)
	ListWorkflowExecutions(
		ctx context.Context,
		companyID workflow.CompanyID,
		filter WorkflowExecutionFilter,
		pageRequest PageRequest,
	) (Page[WorkflowExecutionRecord], error)
}

type NodeExecutionReader interface {
	ListNodeExecutions(
		ctx context.Context,
		companyID workflow.CompanyID,
		workflowExecutionID execution.WorkflowExecutionID,
		pageRequest PageRequest,
	) (Page[NodeExecutionRecord], error)
}

type ExecutionEventReader interface {
	ListExecutionEvents(
		ctx context.Context,
		companyID workflow.CompanyID,
		workflowExecutionID execution.WorkflowExecutionID,
		pageRequest PageRequest,
	) (Page[ExecutionEventRecord], error)
}

type ExecutionLogReader interface {
	ListExecutionLogs(
		ctx context.Context,
		companyID workflow.CompanyID,
		workflowExecutionID execution.WorkflowExecutionID,
		pageRequest PageRequest,
	) (Page[ExecutionLogRecord], error)
}

type ExecutionErrorReader interface {
	ListExecutionErrors(
		ctx context.Context,
		companyID workflow.CompanyID,
		workflowExecutionID execution.WorkflowExecutionID,
		pageRequest PageRequest,
	) (Page[ExecutionErrorRecord], error)
}
