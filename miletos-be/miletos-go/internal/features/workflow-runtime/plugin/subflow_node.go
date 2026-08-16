package plugin

import (
	"errors"
	"strings"
)

func subflowRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                  "core.subflow",
		InputMode:            NodeInputSingle,
		InputPorts:           standardInputPorts(),
		OutputPorts:          standardOutputPorts(),
		InputEdgeConstraint:  fixedEdgeConstraint(1),
		OutputEdgeConstraint: EdgeConstraint{},
		RoutingMode:          OutputRoutingBroadcast,
		Validator:            validateSubflow,
		Handler:              onRunHandler(subflowNode),
	}
}

func validateSubflow(configuration map[string]any) error {
	if strings.TrimSpace(configWorkflowID(configuration)) == "" {
		return validationNodeError("SUBFLOW_WORKFLOW_REQUIRED", "configuration.workflowId is required")
	}
	return nil
}

func subflowNode(nodeContext *Context) (any, error) {
	if err := validateSubflow(nodeContext.configuration); err != nil {
		return nil, err
	}
	workflowID := configWorkflowID(nodeContext.configuration)
	result, err := nodeContext.Infra.Workflow.ExecuteByID(workflowID, nodeContext.Payload)
	if err != nil {
		return nil, mapSubflowExecutionError(err)
	}
	if !result.Succeeded {
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     "SUBFLOW_EXECUTION_FAILED",
			Message:  "The selected child workflow failed.",
		}
	}
	return result.TerminalOutputs, nil
}

func mapSubflowExecutionError(err error) error {
	switch {
	case errors.Is(err, ErrWorkflowSelfReference):
		return &NodeError{
			Category: "VALIDATION",
			Code:     "SUBFLOW_SELF_REFERENCE",
			Message:  "A workflow cannot select itself as a Subflow.",
		}
	case errors.Is(err, ErrWorkflowCycleDetected):
		return &NodeError{
			Category: "VALIDATION",
			Code:     "SUBFLOW_CYCLE_DETECTED",
			Message:  "The selected workflow would create a Subflow cycle.",
		}
	case errors.Is(err, ErrWorkflowNotFound):
		return &NodeError{
			Category: "VALIDATION",
			Code:     "SUBFLOW_WORKFLOW_NOT_FOUND",
			Message:  "The selected workflow does not exist or is inaccessible.",
		}
	case errors.Is(err, ErrWorkflowNotCallable):
		return &NodeError{
			Category: "VALIDATION",
			Code:     "SUBFLOW_WORKFLOW_NOT_CALLABLE",
			Message:  "The selected workflow cannot be executed as a Subflow.",
		}
	case errors.Is(err, ErrWorkflowReturnMissing):
		return &NodeError{
			Category: "VALIDATION",
			Code:     "SUBFLOW_RETURN_REQUIRED",
			Message:  "The selected workflow must contain a Subflow Return node.",
		}
	case errors.Is(err, ErrWorkflowReturnAmbiguous):
		return &NodeError{
			Category: "VALIDATION",
			Code:     "SUBFLOW_RETURN_AMBIGUOUS",
			Message:  "The selected workflow must contain exactly one Subflow Return node.",
		}
	case errors.Is(err, ErrCapabilityUnavailable):
		return &NodeError{
			Category: "EXECUTION",
			Code:     "SUBFLOW_EXECUTION_FAILED",
			Message:  "Workflow infrastructure is unavailable.",
		}
	default:
		return &NodeError{
			Category: "EXECUTION",
			Code:     "SUBFLOW_EXECUTION_FAILED",
			Message:  "The selected child workflow could not be executed.",
			Cause:    err,
		}
	}
}

func configWorkflowID(configuration map[string]any) string {
	value, _ := configuration["workflowId"].(string)
	return strings.TrimSpace(value)
}
