package graph

import workflow "miletos-go/internal/features/workflow"

type Graph struct {
	nodeDefinitions     []workflow.NodeDefinition
	edgeDefinitions     []workflow.EdgeDefinition
	nodesByID           map[workflow.NodeID]workflow.NodeDefinition
	edgesByID           map[workflow.EdgeID]workflow.EdgeDefinition
	incomingEdgesByNode map[workflow.NodeID][]workflow.EdgeDefinition
	outgoingEdgesByNode map[workflow.NodeID][]workflow.EdgeDefinition
	nodeIDs             []workflow.NodeID
	roots               []workflow.NodeID
	terminals           []workflow.NodeID
}

func (g Graph) Node(
	id workflow.NodeID) (workflow.NodeDefinition, bool) {
	node, exists := g.nodesByID[id]
	return node, exists
}
func (g Graph) Edge(id workflow.EdgeID) (workflow.EdgeDefinition, bool) {
	edge, exists := g.edgesByID[id]
	return edge, exists
}
func (g Graph) Nodes() []workflow.NodeDefinition {
	return cloneNodeDefinitions(
		g.nodeDefinitions)
}
func (g Graph) Edges() []workflow.EdgeDefinition {
	return cloneEdgeDefinitions(
		g.edgeDefinitions)
}
func (g Graph) IncomingEdges(nodeID workflow.NodeID,
) []workflow.EdgeDefinition {
	return cloneEdgeDefinitions(g.incomingEdgesByNode[nodeID])
}
func (g Graph) OutgoingEdges(nodeID workflow.NodeID) []workflow.EdgeDefinition {
	return cloneEdgeDefinitions(g.outgoingEdgesByNode[nodeID])
}
func (g Graph) Roots() []workflow.NodeID {
	return cloneNodeIDs(g.roots)
}
func (g Graph) Terminals() []workflow.NodeID {
	return cloneNodeIDs(g.terminals)
}
func cloneNodeDefinitions(
	definitions []workflow.NodeDefinition) []workflow.NodeDefinition {
	if len(definitions) == 0 {
		return nil
	}
	cloned := make([]workflow.NodeDefinition, len(definitions))
	copy(cloned,
		definitions)
	return cloned
}
func cloneEdgeDefinitions(definitions []workflow.EdgeDefinition) []workflow.EdgeDefinition {
	if len(definitions) == 0 {
		return nil
	}
	cloned := make([]workflow.EdgeDefinition,
		len(definitions))
	copy(
		cloned, definitions)
	return cloned
}
func cloneNodeIDs(identifiers []workflow.NodeID,
) []workflow.NodeID {
	if len(identifiers) == 0 {
		return nil
	}
	cloned := make(
		[]workflow.NodeID, len(identifiers))
	copy(cloned, identifiers)
	return cloned
}
