package workflow

import (
	"fmt"
	"sort"
)

func TopologicalOrder(workflow Workflow) ([]string, error) {
	incoming := make(map[string]int, len(workflow.Nodes))
	outgoing := make(map[string][]string, len(workflow.Nodes))
	for _, node := range workflow.Nodes {
		incoming[node.ID] = 0
	}
	for _, edge := range workflow.Edges {
		outgoing[edge.SourceNodeID] = append(outgoing[edge.SourceNodeID], edge.TargetNodeID)
		incoming[edge.TargetNodeID]++
	}
	ready := make([]string, 0)
	for nodeID, count := range incoming {
		if count == 0 {
			ready = append(ready, nodeID)
		}
	}
	sort.Strings(ready)

	order := make([]string, 0, len(workflow.Nodes))
	for len(ready) > 0 {
		nodeID := ready[0]
		ready = ready[1:]
		order = append(order, nodeID)
		for _, targetID := range outgoing[nodeID] {
			incoming[targetID]--
			if incoming[targetID] == 0 {
				ready = append(ready, targetID)
				sort.Strings(ready)
			}
		}
	}
	if len(order) != len(workflow.Nodes) {
		return nil, fmt.Errorf("workflow graph contains a cycle")
	}
	return order, nil
}

func Predecessors(workflow Workflow, nodeID string) []string {
	result := make([]string, 0)
	for _, edge := range workflow.Edges {
		if edge.TargetNodeID == nodeID {
			result = append(result, edge.SourceNodeID)
		}
	}
	sort.Strings(result)
	return result
}

func Roots(definition Workflow) []WorkflowNode {
	roots := make([]WorkflowNode, 0)
	for _, node := range definition.Nodes {
		if len(Predecessors(definition, node.ID)) == 0 {
			roots = append(roots, node)
		}
	}
	return roots
}

func Downstream(workflow Workflow, startingNodeIDs []string) map[string]bool {
	outgoing := make(map[string][]string)
	for _, edge := range workflow.Edges {
		outgoing[edge.SourceNodeID] = append(outgoing[edge.SourceNodeID], edge.TargetNodeID)
	}
	result := make(map[string]bool)
	queue := append([]string(nil), startingNodeIDs...)
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if result[nodeID] {
			continue
		}
		result[nodeID] = true
		queue = append(queue, outgoing[nodeID]...)
	}
	return result
}

func FindCyclePath(workflow Workflow) []string {
	outgoing := make(map[string][]string, len(workflow.Nodes))
	nodeIDs := make([]string, 0, len(workflow.Nodes))
	for _, node := range workflow.Nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	for _, edge := range workflow.Edges {
		outgoing[edge.SourceNodeID] = append(outgoing[edge.SourceNodeID], edge.TargetNodeID)
	}
	sort.Strings(nodeIDs)
	for nodeID := range outgoing {
		sort.Strings(outgoing[nodeID])
	}
	detector := cycleDetector{
		outgoing:   outgoing,
		state:      make(map[string]visitState, len(nodeIDs)),
		stack:      make([]string, 0, len(nodeIDs)),
		stackIndex: make(map[string]int, len(nodeIDs)),
	}
	for _, nodeID := range nodeIDs {
		if detector.state[nodeID] == nodeUnvisited {
			if cycle := detector.visit(nodeID); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}

type visitState uint8

const (
	nodeUnvisited visitState = iota
	nodeVisiting
	nodeVisited
)

type cycleDetector struct {
	outgoing   map[string][]string
	state      map[string]visitState
	stack      []string
	stackIndex map[string]int
}

func (detector *cycleDetector) visit(nodeID string) []string {
	detector.state[nodeID] = nodeVisiting
	detector.stackIndex[nodeID] = len(detector.stack)
	detector.stack = append(detector.stack, nodeID)
	for _, targetID := range detector.outgoing[nodeID] {
		if detector.state[targetID] == nodeVisiting {
			start := detector.stackIndex[targetID]
			cycle := append([]string(nil), detector.stack[start:]...)
			return append(cycle, targetID)
		}
		if detector.state[targetID] != nodeUnvisited {
			continue
		}
		if cycle := detector.visit(targetID); len(cycle) > 0 {
			return cycle
		}
	}
	detector.stack = detector.stack[:len(detector.stack)-1]
	delete(detector.stackIndex, nodeID)
	detector.state[nodeID] = nodeVisited
	return nil
}
