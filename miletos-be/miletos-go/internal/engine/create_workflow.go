package engine

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/engine/modules"
	"miletos-go/internal/model"
	"miletos-go/internal/repository"
)

var ErrInvalidWorkflow = errors.New("workflow is invalid")

type WorkflowValidationError struct {
	Issues []modules.ValidationIssue
}

func (validationError *WorkflowValidationError) Error() string {
	return "workflow definition failed validation"
}

func (validationError *WorkflowValidationError) Unwrap() error {
	return ErrInvalidWorkflow
}

type WorkflowService struct {
	workflows *repository.WorkflowRepository
	registry  *NodeRegistry
}

func NewWorkflowService(
	workflows *repository.WorkflowRepository,
	registry *NodeRegistry,
) *WorkflowService {
	return &WorkflowService{workflows: workflows, registry: registry}
}

func (service *WorkflowService) CreateWorkflow(
	ctx context.Context,
	workflow model.Workflow,
) (model.WorkflowSnapshot, error) {
	if err := modules.ValidateWorkflowDefinition(
		workflow,
		service.registry.Definition,
		service.registry.HasType,
		service.registry.ValidateConfiguration,
	); err != nil {
		var validationError *modules.WorkflowValidationError
		if errors.As(err, &validationError) {
			return model.WorkflowSnapshot{}, &WorkflowValidationError{
				Issues: validationError.Issues,
			}
		}
		return model.WorkflowSnapshot{}, fmt.Errorf("%w: %v", ErrInvalidWorkflow, err)
	}
	snapshot, err := service.workflows.Save(ctx, workflow)
	if err != nil {
		return model.WorkflowSnapshot{}, fmt.Errorf("create workflow: %w", err)
	}
	return snapshot, nil
}
