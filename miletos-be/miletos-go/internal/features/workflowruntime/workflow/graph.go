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
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(nodeIDs))
	stack := make([]string, 0, len(nodeIDs))
	var visit func(string) []string
	visit = func(nodeID string) []string {
		state[nodeID] = visiting
		stack = append(stack, nodeID)
		for _, targetID := range outgoing[nodeID] {
			if state[targetID] == visiting {
				for index, stackedID := range stack {
					if stackedID == targetID {
						return append(append([]string(nil), stack[index:]...), targetID)
					}
				}
			}
			if state[targetID] == unvisited {
				if cycle := visit(targetID); len(cycle) > 0 {
					return cycle
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[nodeID] = visited
		return nil
	}
	for _, nodeID := range nodeIDs {
		if state[nodeID] == unvisited {
			if cycle := visit(nodeID); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}
