package graph

import workflow "miletos-go/internal/features/workflow"

type visitState uint8

const (
	visitStateUnvisited visitState = iota
	visitStateVisiting
	visitStateVisited
)

func (g Graph) CyclePath() ([]workflow.NodeID, bool) {
	states := make(map[workflow.NodeID]visitState, len(g.nodeIDs))
	stack := make([]workflow.NodeID,
		0, len(g.nodeIDs))
	stackIndexes := make(map[workflow.NodeID]int, len(g.nodeIDs))
	for _, nodeID := range g.nodeIDs {
		if states[nodeID] != visitStateUnvisited {
			continue
		}
		cyclePath, found := findCycleFromNode(g,
			nodeID, states, &stack,
			stackIndexes)
		if found {
			return canonicalizeCyclePath(cyclePath), true
		}
	}
	return nil, false
}
func findCycleFromNode(g Graph,
	nodeID workflow.NodeID, states map[workflow.NodeID]visitState, stack *[]workflow.NodeID,
	stackIndexes map[workflow.NodeID]int) ([]workflow.NodeID, bool) {
	states[nodeID] = visitStateVisiting
	stackIndexes[nodeID] = len(*stack)
	*stack = append(*stack,
		nodeID)
	for _, edge := range g.outgoingEdgesByNode[nodeID] {
		targetNodeID := edge.TargetNodeID()
		switch states[targetNodeID] {
		case visitStateUnvisited:
			cyclePath, found := findCycleFromNode(
				g, targetNodeID, states,
				stack, stackIndexes)
			if found {
				return cyclePath, true
			}
		case visitStateVisiting:
			startIndex := stackIndexes[targetNodeID]
			cyclePath := cloneNodeIDs((*stack)[startIndex:])
			cyclePath = append(cyclePath,
				targetNodeID)
			return cyclePath, true
		}
	}
	delete(stackIndexes,
		nodeID)
	*stack = (*stack)[:len(*stack)-1]
	states[nodeID] = visitStateVisited
	return nil, false
}
func canonicalizeCyclePath(
	path []workflow.NodeID) []workflow.NodeID {
	if len(path) < 2 {
		return cloneNodeIDs(path)
	}
	closedPath := path
	if path[0] != path[len(path)-1] {
		closedPath = append(
			cloneNodeIDs(path), path[0])
	}
	cycleNodes := closedPath[:len(closedPath)-1]
	if len(cycleNodes) == 0 {
		return nil
	}
	smallestIndex := 0
	for index := 1; index < len(cycleNodes); index++ {
		if cycleNodes[index].String() < cycleNodes[smallestIndex].String() {
			smallestIndex = index
		}
	}
	canonical := make([]workflow.NodeID,
		0, len(cycleNodes)+1)
	canonical = append(canonical,
		cycleNodes[smallestIndex:]...)
	canonical = append(
		canonical, cycleNodes[:smallestIndex]...)
	canonical = append(canonical, canonical[0])
	return canonical
}
