package execution

import (
	"fmt"
	"sync"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

type nodeAccess struct {
	edges     []workflow.Edge
	inputs    map[string]plugin.InputNodeData
	explicit  bool
	mutex     sync.Mutex
	emissions map[string]any
	closed    bool
}

func newNodeAccess(
	definition workflow.Workflow,
	nodeID string,
	routingMode plugin.OutputRoutingMode,
) *nodeAccess {
	edges := make([]workflow.Edge, 0)
	inputNodeIDs := make(map[string]struct{})
	for _, edge := range definition.Edges {
		if edge.SourceNodeID == nodeID {
			edges = append(edges, edge)
		}
		if edge.TargetNodeID == nodeID {
			inputNodeIDs[edge.SourceNodeID] = struct{}{}
		}
	}
	inputs := make(map[string]plugin.InputNodeData, len(inputNodeIDs))
	for _, node := range definition.Nodes {
		if _, connected := inputNodeIDs[node.ID]; !connected {
			continue
		}
		configuration := make(map[string]any, len(node.Configuration))
		for key, value := range node.Configuration {
			configuration[key] = value
		}
		inputs[node.ID] = plugin.InputNodeData{
			ID: node.ID, PluginType: node.Type, Configuration: configuration,
		}
	}
	return &nodeAccess{
		edges:     edges,
		inputs:    inputs,
		explicit:  routingMode == plugin.OutputRoutingExplicit,
		emissions: make(map[string]any),
	}
}

func (access *nodeAccess) GetInputNode(nodeID string) (plugin.InputNodeData, bool) {
	input, exists := access.inputs[nodeID]
	if !exists {
		return plugin.InputNodeData{}, false
	}
	configuration := make(map[string]any, len(input.Configuration))
	for key, value := range input.Configuration {
		configuration[key] = value
	}
	input.Configuration = configuration
	return input, true
}

func NewNodeAccess(
	definition workflow.Workflow,
	nodeID string,
	routingMode plugin.OutputRoutingMode,
) plugin.Access {
	return newNodeAccess(definition, nodeID, routingMode)
}

type NodeAccessCapture struct {
	*nodeAccess
}

func NewNodeAccessCapture(
	definition workflow.Workflow,
	nodeID string,
	routingMode plugin.OutputRoutingMode,
) *NodeAccessCapture {
	return &NodeAccessCapture{
		nodeAccess: newNodeAccess(definition, nodeID, routingMode),
	}
}

func (capture *NodeAccessCapture) Outcome() model.NodeRoutingOutcome {
	return capture.outcome()
}

func (access *nodeAccess) GetOutputEdgeCount() int {
	return len(access.edges)
}

func (access *nodeAccess) GetEdgeData(index int) (plugin.EdgeData, bool) {
	if index < 0 || index >= len(access.edges) {
		return plugin.EdgeData{}, false
	}
	edge := access.edges[index]
	return plugin.EdgeData{
		ID: edge.ID, SourceOutputPort: edge.SourceOutputPort,
		TargetNodeID: edge.TargetNodeID, TargetInputPort: edge.TargetInputPort,
	}, true
}

func (access *nodeAccess) PushEdge(index int, payload any) error {
	if !access.explicit {
		return fmt.Errorf("push edge requires explicit output routing")
	}
	if index < 0 || index >= len(access.edges) {
		return fmt.Errorf("output edge index %d is out of range", index)
	}
	edgeID := access.edges[index].ID
	access.mutex.Lock()
	defer access.mutex.Unlock()
	if access.closed {
		return fmt.Errorf("output routing is closed")
	}
	if _, exists := access.emissions[edgeID]; exists {
		return fmt.Errorf("output edge %q was already selected", edgeID)
	}
	access.emissions[edgeID] = payload
	return nil
}

func (access *nodeAccess) outcome() model.NodeRoutingOutcome {
	result := model.NodeRoutingOutcome{Explicit: access.explicit}
	if !access.explicit {
		return result
	}
	access.mutex.Lock()
	defer access.mutex.Unlock()
	access.closed = true
	result.EdgePayloads = make(map[string]any, len(access.emissions))
	for edgeID, payload := range access.emissions {
		result.EdgePayloads[edgeID] = payload
	}
	return result
}

func (access *nodeAccess) discard() {
	access.mutex.Lock()
	defer access.mutex.Unlock()
	access.closed = true
	access.emissions = nil
}

func decodeNodeExecutionOutcome(
	node model.NodeExecution,
	registry *plugin.NodeRegistry,
) (model.NodeExecution, error) {
	node = decodePersistedSummaries(node)
	if registry == nil {
		return node, nil
	}
	registration, exists := registry.Get(node.Type)
	if !exists {
		return node, nil
	}
	if registration.RoutingMode != plugin.OutputRoutingExplicit {
		return node, nil
	}
	node.Routing.Explicit = true
	if node.Status != model.NodeSucceeded {
		return node, nil
	}
	if node.Routing.EdgePayloads == nil {
		return model.NodeExecution{}, fmt.Errorf(
			"explicit routing outcome for node %q is invalid", node.NodeID,
		)
	}
	return node, nil
}

func decodePersistedSummaries(node model.NodeExecution) model.NodeExecution {
	if node.Input != nil {
		node.InputPayload = model.DecodePersistedValue(node.Input)
		node.Input = model.PublicSummary(node.InputPayload)
	}
	if node.Output == nil {
		return node
	}
	if node.Output["format"] == model.PersistedRoutedOutputFormat {
		outputEnvelope, outputExists := node.Output["output"].(map[string]any)
		edgePayloads, routesExist := node.Output["edgePayloads"].(map[string]any)
		if outputExists && routesExist {
			node.OutputPayload = model.DecodePersistedValue(outputEnvelope)
			node.Output = model.PublicSummary(node.OutputPayload)
			node.Routing.Explicit = true
			node.Routing.EdgePayloads = model.DecodePersistedEdgePayloads(edgePayloads)
			return node
		}
	}
	node.OutputPayload = model.DecodePersistedValue(node.Output)
	node.Output = model.PublicSummary(node.OutputPayload)
	return node
}
