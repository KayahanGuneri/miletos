package engine

import (
	"fmt"
	"strings"

	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "engine validation failed"
	}
	if e.Field == "" && e.Reason == "" {
		return "engine validation failed"
	}
	if e.Field == "" {
		return fmt.Sprintf("engine validation failed: %s", e.Reason)
	}
	if e.Reason == "" {
		return fmt.Sprintf("engine validation failed for %s", e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Reason)
}

func newValidationError(field string, reason string) error {
	return &ValidationError{Field: field, Reason: reason}
}

func normalizeRequiredString(field string, value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(field, "must not be empty")
	}
	return normalized, nil
}

type ExecutionRequest struct {
	workflowExecutionID execution.WorkflowExecutionID
	definition          workflow.WorkflowDefinition
	correlationID       string
	initialVariables    runtime.ContextChanges
	mode                execution.ExecutionMode
	initialized         bool
}

func NewExecutionRequest(
	workflowExecutionID execution.WorkflowExecutionID,
	definition workflow.WorkflowDefinition,
	correlationID string,
	initialVariables map[string]runtime.RuntimeValue,
) (ExecutionRequest, error) {
	return NewExecutionRequestWithMode(
		execution.ExecutionModeSync,
		workflowExecutionID,
		definition,
		correlationID,
		initialVariables,
	)
}

// NewExecutionRequestWithMode creates a request for either synchronous or
// asynchronous execution while preserving the original synchronous constructor
// as the backwards-compatible default.
func NewExecutionRequestWithMode(
	mode execution.ExecutionMode,
	workflowExecutionID execution.WorkflowExecutionID,
	definition workflow.WorkflowDefinition,
	correlationID string,
	initialVariables map[string]runtime.RuntimeValue,
) (ExecutionRequest, error) {
	if !mode.IsValid() {
		return ExecutionRequest{}, newValidationError("mode", "must be valid")
	}
	normalizedExecutionID, err := execution.NewWorkflowExecutionID(
		workflowExecutionID.String(),
	)
	if err != nil {
		return ExecutionRequest{}, newValidationError("workflowExecutionID", err.Error())
	}
	normalizedDefinition, err := normalizeWorkflowDefinition(definition)
	if err != nil {
		return ExecutionRequest{}, newValidationError("definition", err.Error())
	}
	normalizedCorrelationID, err := normalizeRequiredString("correlationID", correlationID)
	if err != nil {
		return ExecutionRequest{}, err
	}
	normalizedInitialVariables, err := runtime.NewContextChanges(initialVariables, nil)
	if err != nil {
		return ExecutionRequest{}, newValidationError("initialVariables", err.Error())
	}
	return ExecutionRequest{
		workflowExecutionID: normalizedExecutionID,
		definition:          normalizedDefinition,
		correlationID:       normalizedCorrelationID,
		initialVariables:    normalizedInitialVariables,
		mode:                mode,
		initialized:         true,
	}, nil
}

func NewAsyncExecutionRequest(
	workflowExecutionID execution.WorkflowExecutionID,
	definition workflow.WorkflowDefinition,
	correlationID string,
	initialVariables map[string]runtime.RuntimeValue,
) (ExecutionRequest, error) {
	return NewExecutionRequestWithMode(
		execution.ExecutionModeAsync,
		workflowExecutionID,
		definition,
		correlationID,
		initialVariables,
	)
}

func (request ExecutionRequest) WorkflowExecutionID() execution.WorkflowExecutionID {
	return request.workflowExecutionID
}

func (request ExecutionRequest) Definition() workflow.WorkflowDefinition {
	return request.definition
}

func (request ExecutionRequest) CorrelationID() string {
	return request.correlationID
}

func (request ExecutionRequest) Mode() execution.ExecutionMode {
	if request.mode == "" {
		return execution.ExecutionModeSync
	}
	return request.mode
}

func (request ExecutionRequest) InitialVariables() map[string]runtime.RuntimeValue {
	return request.initialVariables.SetValues()
}

func (request ExecutionRequest) IsValid() bool {
	if !request.initialized || !request.initialVariables.IsValid() {
		return false
	}
	_, err := NewExecutionRequestWithMode(
		request.Mode(),
		request.workflowExecutionID,
		request.definition,
		request.correlationID,
		request.initialVariables.SetValues(),
	)
	return err == nil
}

func normalizeWorkflowDefinition(
	definition workflow.WorkflowDefinition,
) (workflow.WorkflowDefinition, error) {
	return workflow.NewWorkflowDefinition(
		definition.ID(),
		definition.CompanyID(),
		definition.Name(),
		definition.Revision(),
		definition.Nodes(),
		definition.Edges(),
		definition.Metadata().Bytes(),
	)
}
