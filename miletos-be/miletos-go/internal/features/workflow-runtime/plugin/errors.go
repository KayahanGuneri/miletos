package plugin

import "errors"

var (
	ErrNodeRegistrationNotFound  = errors.New("node registration not found")
	ErrScenarioStartUnavailable  = errors.New("node scenario-start lifecycle is unavailable")
	ErrRunUnavailable            = errors.New("node run lifecycle is unavailable")
	ErrWorkflowDefinitionInvalid = errors.New("workflow definition is invalid")
	ErrWorkflowNotFound          = errors.New("referenced workflow was not found")
	ErrWorkflowNotCallable       = errors.New("referenced workflow is not callable")
	ErrWorkflowSelfReference     = errors.New("workflow cannot reference itself")
	ErrWorkflowCycleDetected     = errors.New("workflow invocation cycle detected")
	ErrWorkflowReturnMissing     = errors.New("referenced workflow has no workflow result")
	ErrWorkflowReturnAmbiguous   = errors.New("referenced workflow has multiple workflow results")
)

type NodeError struct {
	Category string
	Code     string
	Message  string
	CanRetry bool
	Cause    error
}

func (nodeError *NodeError) Error() string {
	return nodeError.Message
}

func (nodeError *NodeError) Unwrap() error {
	return nodeError.Cause
}
