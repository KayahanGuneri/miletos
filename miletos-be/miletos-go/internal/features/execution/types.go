package execution

import "strings"

type WorkflowExecutionID string
type NodeExecutionID string

func NewWorkflowExecutionID(value string) (WorkflowExecutionID, error) {
	normalized, err := normalizeRequiredString("workflowExecutionID", value)
	if err != nil {
		return "", err
	}
	return WorkflowExecutionID(normalized), nil
}
func (id WorkflowExecutionID) String() string {
	return string(id)
}
func NewNodeExecutionID(value string) (NodeExecutionID, error) {
	normalized, err := normalizeRequiredString("nodeExecutionID", value)
	if err != nil {
		return "", err
	}
	return NodeExecutionID(normalized), nil
}
func (id NodeExecutionID) String() string {
	return string(id)
}
func normalizeRequiredString(field string, value string,
) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(field, "must not be empty")
	}
	return normalized, nil
}

type ExecutionMode string

const (
	ExecutionModeSync  ExecutionMode = "SYNC"
	ExecutionModeAsync ExecutionMode = "ASYNC"
)

func ParseExecutionMode(value string,
) (ExecutionMode, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch ExecutionMode(normalized) {
	case ExecutionModeSync:
		return ExecutionModeSync, nil
	case ExecutionModeAsync:
		return ExecutionModeAsync, nil
	default:
		return "", &InvalidModeError{Value: value}
	}
}
func (mode ExecutionMode) String() string {
	return string(mode)
}
func (mode ExecutionMode) IsValid() bool {
	switch mode {
	case ExecutionModeSync, ExecutionModeAsync:
		return true
	default:
		return false
	}
}
