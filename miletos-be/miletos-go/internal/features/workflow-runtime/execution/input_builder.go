package execution

import (
	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

func BuildNodeInput(
	definition workflow.Workflow,
	nodeID string,
	outputs map[string]any,
	registry *plugin.NodeRegistry,
) any {
	node, _ := findWorkflowNode(definition, nodeID)
	descriptor, _ := registry.Definition(node.Type, node.Version)
	if descriptor.InputMode == plugin.NodeInputMulti {
		inputs := make([]any, 0)
		for _, edge := range definition.Edges {
			if edge.TargetNodeID != nodeID {
				continue
			}
			inputs = append(inputs, map[string]any{
				"edgeId":           edge.ID,
				"sourceNodeId":     edge.SourceNodeID,
				"sourceOutputPort": edge.SourceOutputPort,
				"targetInputPort":  edge.TargetInputPort,
				"value":            normalizeOutput(outputs[edge.SourceNodeID]),
			})
		}
		return map[string]any{"inputs": inputs}
	}
	values := make(map[string]any)
	for _, edge := range definition.Edges {
		if edge.TargetNodeID == nodeID {
			values[edge.TargetInputPort] = normalizeOutput(
				outputs[edge.SourceNodeID],
			)
		}
	}
	if len(values) == 1 {
		for _, value := range values {
			return value
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func buildExecutionNodeInput(
	definition workflow.Workflow,
	nodeID string,
	outputs map[string]any,
	registry *plugin.NodeRegistry,
	startInput map[string]any,
	origin model.ExecutionOrigin,
) any {
	payload := BuildNodeInput(definition, nodeID, outputs, registry)
	if startInput == nil || len(workflow.Predecessors(definition, nodeID)) != 0 {
		return payload
	}
	node, exists := findWorkflowNode(definition, nodeID)
	if !exists {
		return payload
	}
	switch origin {
	case model.ExecutionOriginHTTPWebhook:
		return startInput
	case model.ExecutionOriginManualDirect:
		descriptor, registered := registry.Definition(node.Type, node.Version)
		if registered && plugin.CanReceiveEntryInput(descriptor) {
			return startInput
		}
	}
	return payload
}

func normalizeOutput(output any) any {
	summary, ok := output.(map[string]any)
	if !ok {
		return output
	}
	if value, exists := summary["value"]; exists && len(summary) == 1 {
		return value
	}
	return summary
}
