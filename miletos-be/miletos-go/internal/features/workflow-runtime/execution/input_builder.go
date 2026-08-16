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
	edgePayloads := make(map[string]any)
	for _, edge := range definition.Edges {
		if edge.TargetNodeID == nodeID {
			edgePayloads[edge.ID] = outputs[edge.SourceNodeID]
		}
	}
	return buildNodeInputFromEdges(definition, nodeID, edgePayloads, registry)
}

func buildNodeInputFromEdges(
	definition workflow.Workflow,
	nodeID string,
	edgePayloads map[string]any,
	registry *plugin.NodeRegistry,
) any {
	node, _ := findWorkflowNode(definition, nodeID)
	registration, _ := registry.Get(node.Type)
	if registration.InputMode == plugin.NodeInputMulti {
		inputs := make([]any, 0)
		for edgeOrder, edge := range definition.Edges {
			if edge.TargetNodeID != nodeID {
				continue
			}
			value, active := edgePayloads[edge.ID]
			if !active {
				continue
			}
			inputs = append(inputs, map[string]any{
				"edgeId":           edge.ID,
				"edgeOrder":        edgeOrder,
				"sourceNodeId":     edge.SourceNodeID,
				"sourceOutputPort": edge.SourceOutputPort,
				"targetInputPort":  edge.TargetInputPort,
				"value":            value,
			})
		}
		return map[string]any{"inputs": inputs}
	}
	values := make(map[string]any)
	for _, edge := range definition.Edges {
		if edge.TargetNodeID != nodeID {
			continue
		}
		value, active := edgePayloads[edge.ID]
		if active {
			values[edge.TargetInputPort] = value
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
	edgePayloads map[string]any,
	registry *plugin.NodeRegistry,
	startInput any,
	startNodeID string,
	origin model.ExecutionOrigin,
) any {
	payload := buildNodeInputFromEdges(definition, nodeID, edgePayloads, registry)
	if startInput == nil || len(workflow.Predecessors(definition, nodeID)) != 0 {
		return payload
	}
	if startNodeID != "" && nodeID != startNodeID {
		return payload
	}
	node, exists := findWorkflowNode(definition, nodeID)
	if !exists {
		return payload
	}
	switch origin {
	case model.ExecutionOriginManualDirect:
		registration, registered := registry.Get(node.Type)
		if registered && plugin.CanReceiveEntryInput(registration) {
			return startInput
		}
	case model.ExecutionOriginHTTPWebhook, model.ExecutionOriginCron, model.ExecutionOriginDataArrival:
		if registry.DeclaresExecutionSource(node.Type, string(origin)) {
			return startInput
		}
	}
	return payload
}
