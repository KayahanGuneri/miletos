package graph

import workflow "miletos-go/internal/features/workflow"

func unreachableNodeIDs(
	built Graph) []workflow.NodeID {
	if len(built.nodeIDs) == 0 {
		return nil
	}
	reachable := make(map[workflow.NodeID]struct{}, len(built.nodeIDs))
	queue := cloneNodeIDs(
		built.roots)
	for _, rootNodeID := range queue {
		reachable[rootNodeID] = struct{}{}
	}
	for index := 0; index < len(queue); index++ {
		currentNodeID := queue[index]
		for _, edge := range built.outgoingEdgesByNode[currentNodeID] {
			targetNodeID := edge.TargetNodeID()
			if _, exists := reachable[targetNodeID]; exists {
				continue
			}
			reachable[targetNodeID] = struct{}{}
			queue = append(queue, targetNodeID)
		}
	}
	unreachable := make([]workflow.NodeID,
		0)
	for _, nodeID := range built.nodeIDs {
		if _, exists := reachable[nodeID]; exists {
			continue
		}
		unreachable = append(
			unreachable, nodeID)
	}
	return unreachable
}
