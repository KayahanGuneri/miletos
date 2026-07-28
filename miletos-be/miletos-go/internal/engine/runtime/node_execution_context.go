package runtime

import (
	"context"
	"fmt"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
	"sort"
	"time"
)

type IncomingEdge struct {
	id               workflow.EdgeID
	sourceNodeID     workflow.NodeID
	sourceOutputPort string
	targetNodeID     workflow.NodeID
	targetInputPort  string
	cache            *EdgeCache
}

func (edge IncomingEdge) ID() workflow.EdgeID {
	return edge.id
}
func (edge IncomingEdge) SourceNodeID() workflow.NodeID { return edge.sourceNodeID }
func (edge IncomingEdge) SourceOutputPort() string {
	return edge.sourceOutputPort
}
func (edge IncomingEdge) TargetNodeID() workflow.NodeID {
	return edge.targetNodeID
}
func (edge IncomingEdge) TargetInputPort() string { return edge.targetInputPort }
func (edge IncomingEdge) Cache() *EdgeCache {
	return edge.cache
}

type NodeExecutionContext struct {
	executionContext *ExecutionContext
	nodeExecutionID  execution.NodeExecutionID
	nodeID           workflow.NodeID
	startedAt        time.Time
	incomingEdges    map[workflow.EdgeID]IncomingEdge
	incomingOrder    []workflow.EdgeID
}

func NewNodeExecutionContext(executionContext *ExecutionContext,
	nodeExecution execution.NodeExecution, incomingEdgeIDs []workflow.EdgeID) (*NodeExecutionContext, error) {
	if executionContext == nil {
		return nil, newValidationError("executionContext",
			"must not be nil")
	}
	normalizedNodeExecutionID, err := execution.NewNodeExecutionID(
		nodeExecution.ID().String())
	if err != nil {
		return nil, newValidationError("nodeExecution.id", err.Error())
	}
	normalizedWorkflowExecutionID, err := execution.NewWorkflowExecutionID(nodeExecution.WorkflowExecutionID().String())
	if err != nil {
		return nil, newValidationError(
			"nodeExecution.workflowExecutionID", err.Error())
	}
	if normalizedWorkflowExecutionID !=
		executionContext.WorkflowExecutionID() {
		return nil, newValidationError("nodeExecution.workflowExecutionID",
			"must match the parent execution context")
	}
	normalizedNodeID, err := workflow.NewNodeID(nodeExecution.NodeID().String())
	if err != nil {
		return nil, newValidationError(
			"nodeExecution.nodeID", err.Error())
	}
	if nodeExecution.Status() !=
		execution.NodeExecutionStatusRunning {
		return nil, newValidationError("nodeExecution.status",
			"must be RUNNING")
	}
	startedAt, started := nodeExecution.StartedAt()
	if !started || startedAt.IsZero() {
		return nil, newValidationError("nodeExecution.startedAt", "must exist for a running node execution")
	}
	incomingEdges, incomingOrder, err := buildAuthorizedIncomingEdges(executionContext,
		normalizedNodeID, incomingEdgeIDs)
	if err != nil {
		return nil, err
	}
	return &NodeExecutionContext{executionContext: executionContext,
		nodeExecutionID: normalizedNodeExecutionID, nodeID: normalizedNodeID,
		startedAt: startedAt, incomingEdges: incomingEdges,
		incomingOrder: incomingOrder}, nil
}
func (nodeContext *NodeExecutionContext) Context() context.Context {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return nil
	}
	return nodeContext.executionContext.Context()
}
func (nodeContext *NodeExecutionContext) Done() <-chan struct{} {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return nil
	}
	return nodeContext.executionContext.Done()
}
func (nodeContext *NodeExecutionContext) Err() error {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return nil
	}
	return nodeContext.executionContext.Err()
}
func (nodeContext *NodeExecutionContext) Deadline() (time.Time,
	bool) {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return time.Time{}, false
	}
	return nodeContext.executionContext.Deadline()
}
func (nodeContext *NodeExecutionContext) NodeExecutionID() execution.NodeExecutionID {
	if nodeContext == nil {
		return ""
	}
	return nodeContext.nodeExecutionID
}
func (nodeContext *NodeExecutionContext) NodeID() workflow.NodeID {
	if nodeContext == nil {
		return ""
	}
	return nodeContext.nodeID
}
func (nodeContext *NodeExecutionContext) StartedAt() time.Time {
	if nodeContext == nil {
		return time.Time{}
	}
	return nodeContext.startedAt
}
func (nodeContext *NodeExecutionContext) WorkflowExecutionID() execution.WorkflowExecutionID {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return ""
	}
	return nodeContext.executionContext.WorkflowExecutionID()
}
func (nodeContext *NodeExecutionContext) CompanyID() workflow.CompanyID {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return ""
	}
	return nodeContext.executionContext.CompanyID()
}
func (nodeContext *NodeExecutionContext) WorkflowID() workflow.WorkflowID {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return ""
	}
	return nodeContext.executionContext.WorkflowID()
}
func (nodeContext *NodeExecutionContext) Mode() execution.ExecutionMode {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return ""
	}
	return nodeContext.executionContext.Mode()
}
func (nodeContext *NodeExecutionContext) CorrelationID() string {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return ""
	}
	return nodeContext.executionContext.CorrelationID()
}
func (nodeContext *NodeExecutionContext) ProtectedMetadata() map[string]string {
	if nodeContext == nil ||
		nodeContext.executionContext == nil {
		return nil
	}
	return nodeContext.executionContext.ProtectedMetadata()
}
func (nodeContext *NodeExecutionContext) Variable(key string,
) (RuntimeValue, bool, error) {
	if nodeContext == nil || nodeContext.executionContext == nil {
		return RuntimeValue{}, false, newValidationError("nodeExecutionContext", "must not be nil")
	}
	return nodeContext.executionContext.Variable(key)
}
func (nodeContext *NodeExecutionContext) VariablesSnapshot() map[string]RuntimeValue {
	if nodeContext == nil || nodeContext.executionContext == nil {
		return nil
	}
	return nodeContext.executionContext.VariablesSnapshot()
}
func (nodeContext *NodeExecutionContext) IncomingEdge(id workflow.EdgeID) (IncomingEdge, bool, error) {
	if nodeContext == nil {
		return IncomingEdge{}, false, newValidationError("nodeExecutionContext",
			"must not be nil")
	}
	normalizedID, err := workflow.NewEdgeID(id.String())
	if err != nil {
		return IncomingEdge{}, false, newValidationError(
			"edgeID", err.Error())
	}
	edge, authorized :=
		nodeContext.incomingEdges[normalizedID]
	if !authorized {
		return IncomingEdge{}, false, newAccessError("incoming edge", normalizedID.String(),
			"is not authorized for this node execution")
	}
	return edge, true, nil
}
func (nodeContext *NodeExecutionContext) IncomingEdges() []IncomingEdge {
	if nodeContext == nil ||
		len(nodeContext.incomingOrder) == 0 {
		return nil
	}
	edges := make([]IncomingEdge,
		0, len(nodeContext.incomingOrder))
	for _, edgeID := range nodeContext.incomingOrder {
		edges = append(
			edges, nodeContext.incomingEdges[edgeID])
	}
	return edges
}
func buildAuthorizedIncomingEdges(
	executionContext *ExecutionContext, nodeID workflow.NodeID, incomingEdgeIDs []workflow.EdgeID,
) (map[workflow.EdgeID]IncomingEdge, []workflow.EdgeID,
	error) {
	if len(incomingEdgeIDs) == 0 {
		return map[workflow.EdgeID]IncomingEdge{}, nil, nil
	}
	incomingEdges := make(
		map[workflow.EdgeID]IncomingEdge, len(incomingEdgeIDs))
	order := make([]workflow.EdgeID,
		0, len(incomingEdgeIDs))
	for index, edgeID := range incomingEdgeIDs {
		field := fmt.Sprintf(
			"incomingEdgeIDs[%d]", index)
		normalizedEdgeID, err := workflow.NewEdgeID(edgeID.String())
		if err != nil {
			return nil, nil, newValidationError(
				field, err.Error())
		}
		if _, exists := incomingEdges[normalizedEdgeID]; exists {
			return nil, nil, newValidationError("incomingEdgeIDs", fmt.Sprintf(
				"contains duplicate edge ID %q", normalizedEdgeID.String()),
			)
		}
		edgeRuntime, exists, err := executionContext.Edge(normalizedEdgeID)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			return nil, nil, newValidationError(field,
				"is not registered in the execution context")
		}
		if edgeRuntime.TargetNodeID() != nodeID {
			return nil, nil, newValidationError(
				field, "must target the current node")
		}
		incomingEdges[normalizedEdgeID] =
			newIncomingEdge(edgeRuntime)
		order = append(
			order, normalizedEdgeID)
	}
	sort.Slice(
		order, func(left int, right int) bool {
			return order[left].String() <
				order[right].String()
		})
	return incomingEdges, order, nil
}
func newIncomingEdge(edgeRuntime *EdgeRuntime,
) IncomingEdge {
	return IncomingEdge{id: edgeRuntime.ID(),
		sourceNodeID: edgeRuntime.SourceNodeID(), sourceOutputPort: edgeRuntime.SourceOutputPort(), targetNodeID: edgeRuntime.TargetNodeID(),
		targetInputPort: edgeRuntime.TargetInputPort(), cache: edgeRuntime.Cache()}
}
