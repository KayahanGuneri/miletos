package graph

import (
	"sort"

	"miletos-go/internal/features/workflow"
)

func Build(
	definition workflow.WorkflowDefinition) Graph {
	nodes := definition.Nodes()
	edges := definition.Edges()
	sortNodeDefinitions(nodes)
	sortEdgeDefinitions(edges)
	result := Graph{
		nodeDefinitions: nodes, edgeDefinitions: edges, nodesByID: make(
			map[workflow.NodeID]workflow.NodeDefinition, len(nodes)),
		edgesByID: make(map[workflow.EdgeID]workflow.EdgeDefinition, len(edges)), incomingEdgesByNode: make(map[workflow.NodeID][]workflow.EdgeDefinition,
			len(nodes)), outgoingEdgesByNode: make(
			map[workflow.NodeID][]workflow.EdgeDefinition, len(nodes)),
	}
	indexNodes(
		&result, nodes)
	indexEdges(&result, edges)
	detectRootsAndTerminals(&result)
	return result
}
func indexNodes(
	result *Graph, nodes []workflow.NodeDefinition) {
	for _, node := range nodes {
		nodeID := node.ID()
		if _, exists := result.nodesByID[nodeID]; exists {
			continue
		}
		result.nodesByID[nodeID] = node
		result.nodeIDs = append(
			result.nodeIDs, nodeID)
		result.incomingEdgesByNode[nodeID] = nil
		result.outgoingEdgesByNode[nodeID] = nil
	}
}
func indexEdges(result *Graph, edges []workflow.EdgeDefinition,
) {
	for _, edge := range edges {
		edgeID := edge.ID()
		if _, exists := result.edgesByID[edgeID]; !exists {
			result.edgesByID[edgeID] = edge
		}
		sourceNodeID := edge.SourceNodeID()
		targetNodeID := edge.TargetNodeID()
		_, sourceExists := result.nodesByID[sourceNodeID]
		_, targetExists := result.nodesByID[targetNodeID]
		if !sourceExists || !targetExists {
			continue
		}
		if sourceNodeID == targetNodeID {
			continue
		}
		result.outgoingEdgesByNode[sourceNodeID] = append(result.outgoingEdgesByNode[sourceNodeID],
			edge)
		result.incomingEdgesByNode[targetNodeID] = append(result.incomingEdgesByNode[targetNodeID], edge)
	}
}
func detectRootsAndTerminals(result *Graph,
) {
	for _, nodeID := range result.nodeIDs {
		if len(result.incomingEdgesByNode[nodeID]) == 0 {
			result.roots = append(result.roots, nodeID)
		}
		if len(result.outgoingEdgesByNode[nodeID]) == 0 {
			result.terminals = append(result.terminals,
				nodeID)
		}
	}
}
func sortNodeDefinitions(nodes []workflow.NodeDefinition) {
	sort.SliceStable(nodes, func(
		left int, right int) bool {
		return nodes[left].ID().String() < nodes[right].ID().String()
	},
	)
}
func sortEdgeDefinitions(edges []workflow.EdgeDefinition) {
	sort.SliceStable(edges, func(
		left int, right int) bool {
		return edges[left].ID().String() < edges[right].ID().String()
	},
	)
}
