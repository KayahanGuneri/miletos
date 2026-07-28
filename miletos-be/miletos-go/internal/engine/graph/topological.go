package graph

import (
	"container/heap"

	"miletos-go/internal/features/workflow"
)

type nodeIDHeap []workflow.NodeID

func (h nodeIDHeap) Len() int {
	return len(h)
}
func (h nodeIDHeap) Less(
	left int, right int) bool {
	return h[left].String() < h[right].String()
}
func (h nodeIDHeap) Swap(left int,
	right int) {
	h[left], h[right] = h[right], h[left]
}
func (h *nodeIDHeap) Push(
	value any) {
	*h = append(
		*h, value.(workflow.NodeID))
}
func (h *nodeIDHeap) Pop() any {
	current := *h
	lastIndex := len(current) - 1
	value := current[lastIndex]
	*h = current[:lastIndex]
	return value
}
func (g Graph) TopologicalOrder() ([]workflow.NodeID, bool) {
	inDegree := make(map[workflow.NodeID]int,
		len(g.nodeIDs))
	readyNodes := make(nodeIDHeap, 0,
		len(g.nodeIDs))
	for _, nodeID := range g.nodeIDs {
		inDegree[nodeID] = len(g.incomingEdgesByNode[nodeID])
		if inDegree[nodeID] == 0 {
			readyNodes = append(readyNodes, nodeID)
		}
	}
	heap.Init(&readyNodes)
	order := make([]workflow.NodeID, 0,
		len(g.nodeIDs))
	for readyNodes.Len() > 0 {
		currentNodeID := heap.Pop(&readyNodes).(workflow.NodeID)
		order = append(
			order, currentNodeID)
		for _, edge := range g.outgoingEdgesByNode[currentNodeID] {
			targetNodeID := edge.TargetNodeID()
			inDegree[targetNodeID]--
			if inDegree[targetNodeID] == 0 {
				heap.Push(&readyNodes, targetNodeID)
			}
		}
	}
	if len(order) != len(g.nodeIDs) {
		return nil, false
	}
	return cloneNodeIDs(order), true
}
