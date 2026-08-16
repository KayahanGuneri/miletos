package execution

import (
	"context"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/plugin"
)

type WorkflowInfrastructure struct {
	service *ExecutionService
}

type scopedWorkflowInfrastructure struct {
	ctx              context.Context
	companyID        string
	parentWorkflowID string
	service          *ExecutionService
}

func NewWorkflowInfrastructure(service *ExecutionService) *WorkflowInfrastructure {
	return &WorkflowInfrastructure{service: service}
}

func (infrastructure *WorkflowInfrastructure) ForContext(
	ctx context.Context,
	companyID string,
	workflowID string,
) plugin.WorkflowInfrastructure {
	return &scopedWorkflowInfrastructure{
		ctx:              ctx,
		companyID:        companyID,
		parentWorkflowID: workflowID,
		service:          infrastructure.service,
	}
}

func (infrastructure *scopedWorkflowInfrastructure) ExecuteByID(
	workflowID string,
	startInput any,
) (plugin.WorkflowExecutionResult, error) {
	if infrastructure.service == nil {
		return plugin.WorkflowExecutionResult{}, plugin.ErrCapabilityUnavailable
	}
	outcome, err := infrastructure.service.ExecuteReferenced(
		infrastructure.ctx,
		infrastructure.companyID,
		infrastructure.parentWorkflowID,
		workflowID,
		startInput,
	)
	if err != nil {
		return plugin.WorkflowExecutionResult{}, err
	}
	if outcome.Execution.Status != model.ExecutionSucceeded {
		return plugin.WorkflowExecutionResult{Succeeded: false}, nil
	}
	output, err := referencedWorkflowOutput(
		infrastructure.ctx,
		infrastructure.service,
		outcome.Execution,
	)
	if err != nil {
		return plugin.WorkflowExecutionResult{}, err
	}
	return plugin.WorkflowExecutionResult{
		Succeeded:       true,
		TerminalOutputs: output,
	}, nil
}

func referencedWorkflowOutput(
	ctx context.Context,
	service *ExecutionService,
	execution model.Execution,
) (any, error) {
	if service == nil || service.executions == nil || service.scheduler == nil {
		return nil, plugin.ErrCapabilityUnavailable
	}
	states, err := service.executions.ListAllNodes(ctx, execution.CompanyID, execution.ID)
	if err != nil {
		return nil, err
	}
	outputs, err := workflowReturnOutputs(states, service.scheduler.registry)
	if err != nil {
		return nil, err
	}
	if len(outputs) == 0 {
		return nil, plugin.ErrWorkflowReturnMissing
	}
	if len(outputs) > 1 {
		return nil, plugin.ErrWorkflowReturnAmbiguous
	}
	for _, output := range outputs {
		return output, nil
	}
	return nil, plugin.ErrWorkflowReturnMissing
}

func workflowReturnOutputs(
	states []model.NodeExecution,
	registry *plugin.NodeRegistry,
) (map[string]any, error) {
	if registry == nil {
		return nil, nil
	}
	outputs := make(map[string]any)
	for _, state := range states {
		decoded, decodeErr := decodeNodeExecutionOutcome(state, registry)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if decoded.Status != model.NodeSucceeded {
			continue
		}
		registration, exists := registry.Get(decoded.Type)
		if !exists || !registration.ProvidesWorkflowResult {
			continue
		}
		if decoded.OutputPayload != nil {
			outputs[decoded.NodeID] = decoded.OutputPayload
			continue
		}
		outputs[decoded.NodeID] = decoded.Output
	}
	return outputs, nil
}
