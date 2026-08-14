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
	for _, edge := range definition.Edges {
		if edge.SourceNodeID == nodeID {
			edges = append(edges, edge)
		}
	}
	return &nodeAccess{
		edges:     edges,
		explicit:  routingMode == plugin.OutputRoutingExplicit,
		emissions: make(map[string]any),
	}
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
	if registry == nil {
		return node, nil
	}
	mode, exists := registry.GetOutputRoutingMode(node.Type)
	if !exists {
		return node, nil
	}
	if mode != plugin.OutputRoutingExplicit {
		return node, nil
	}
	node.Routing.Explicit = true
	if node.Status != model.NodeSucceeded {
		return node, nil
	}
	if node.Output["format"] != model.PersistedRoutedOutputFormat {
		return model.NodeExecution{}, fmt.Errorf(
			"explicit routing outcome for node %q is invalid", node.NodeID,
		)
	}
	output, outputExists := node.Output["output"].(map[string]any)
	edgePayloads, routesExist := node.Output["edgePayloads"].(map[string]any)
	if !outputExists || !routesExist {
		return model.NodeExecution{}, fmt.Errorf(
			"explicit routing outcome for node %q is incomplete", node.NodeID,
		)
	}
	node.Output = output
	node.Routing.EdgePayloads = edgePayloads
	return node, nil
}
