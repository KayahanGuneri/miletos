package workflow

import (
	"context"
	"errors"
	"fmt"

	"miletos-go/internal/features/workflow-runtime/plugin"
)

var ErrInvalidWorkflow = errors.New("workflow is invalid")

type WorkflowValidationError struct {
	Issues []ValidationIssue
}

func (validationError *WorkflowValidationError) Error() string {
	return "workflow definition failed validation"
}

func (validationError *WorkflowValidationError) Unwrap() error {
	return ErrInvalidWorkflow
}

type WorkflowService struct {
	workflows *WorkflowRepository
	registry  *plugin.NodeRegistry
}

func NewWorkflowService(
	workflows *WorkflowRepository,
	registry *plugin.NodeRegistry,
) *WorkflowService {
	return &WorkflowService{workflows: workflows, registry: registry}
}

func (service *WorkflowService) CreateWorkflow(
	ctx context.Context,
	workflow Workflow,
) (WorkflowSnapshot, error) {
	if err := service.Validate(workflow); err != nil {
		return WorkflowSnapshot{}, err
	}
	snapshot, err := service.workflows.Save(ctx, workflow)
	if err != nil {
		return WorkflowSnapshot{}, fmt.Errorf("create workflow: %w", err)
	}
	return snapshot, nil
}

func (service *WorkflowService) Validate(workflow Workflow) error {
	if err := ValidateWorkflowDefinition(
		workflow,
		service.registry.Definition,
		service.registry.ValidateConfiguration,
	); err != nil {
		var validationError *WorkflowDefinitionValidationError
		if errors.As(err, &validationError) {
			return &WorkflowValidationError{
				Issues: validationError.Issues,
			}
		}
		return fmt.Errorf("%w: %v", ErrInvalidWorkflow, err)
	}
	return nil
}
