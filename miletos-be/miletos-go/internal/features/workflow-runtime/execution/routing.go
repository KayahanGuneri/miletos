package execution

import (
	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/workflow"
)

type nodeOutcome struct {
	status     model.NodeStatus
	skipReason model.SkipReason
	output     any
	routing    model.NodeRoutingOutcome
}

type routeReadiness struct {
	ready        bool
	inactive     bool
	failed       bool
	edgePayloads map[string]any
}

func resolveNodeRoutes(
	definition workflow.Workflow,
	nodeID string,
	outcomes map[string]nodeOutcome,
) routeReadiness {
	incoming := 0
	active := 0
	undecided := false
	failed := false
	edgePayloads := make(map[string]any)
	for _, edge := range definition.Edges {
		if edge.TargetNodeID != nodeID {
			continue
		}
		incoming++
		predecessor := outcomes[edge.SourceNodeID]
		switch predecessor.status {
		case model.NodeSucceeded:
			if predecessor.routing.Explicit {
				payload, selected := predecessor.routing.EdgePayloads[edge.ID]
				if selected {
					edgePayloads[edge.ID] = payload
					active++
				}
				continue
			}
			edgePayloads[edge.ID] = predecessor.output
			active++
		case model.NodeSkipped:
			if predecessor.skipReason == model.SkipReasonDependencyFailed {
				failed = true
			}
			continue
		case model.NodeFailed, model.NodeTimedOut, model.NodeCancelled:
			failed = true
		default:
			undecided = true
		}
	}
	if incoming == 0 {
		return routeReadiness{ready: true, edgePayloads: edgePayloads}
	}
	if failed {
		return routeReadiness{failed: true, edgePayloads: edgePayloads}
	}
	if undecided {
		return routeReadiness{edgePayloads: edgePayloads}
	}
	if active == 0 {
		return routeReadiness{inactive: true, edgePayloads: edgePayloads}
	}
	return routeReadiness{ready: true, edgePayloads: edgePayloads}
}

func nodeOutcomes(states map[string]model.NodeExecution) map[string]nodeOutcome {
	outcomes := make(map[string]nodeOutcome, len(states))
	for nodeID, state := range states {
		outcomes[nodeID] = nodeOutcome{
			status: state.Status, skipReason: model.SkipReason(nodeSkipReason(state)),
			output: state.OutputPayload, routing: state.Routing,
		}
	}
	return outcomes
}

func nodeSkipReason(state model.NodeExecution) string {
	if state.Status != model.NodeSkipped || state.Failure == nil {
		return ""
	}
	reason, _ := state.Failure["skipReason"].(string)
	return reason
}
