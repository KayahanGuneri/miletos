package execution

import (
	"time"

	"miletos-go/internal/features/workflow"
)

type NodeExecution struct {
	id                  NodeExecutionID
	workflowExecutionID WorkflowExecutionID
	nodeID              workflow.NodeID
	status              NodeExecutionStatus
	createdAt           time.Time
	startedAt           time.Time
	finishedAt          time.Time
}

func NewNodeExecution(
	id NodeExecutionID, workflowExecutionID WorkflowExecutionID, nodeID workflow.NodeID,
	createdAt time.Time) (NodeExecution, error) {
	normalizedID, err := NewNodeExecutionID(
		id.String())
	if err != nil {
		return NodeExecution{}, err
	}
	normalizedWorkflowExecutionID, err := NewWorkflowExecutionID(workflowExecutionID.String())
	if err != nil {
		return NodeExecution{}, err
	}
	normalizedNodeID, err := workflow.NewNodeID(nodeID.String())
	if err != nil {
		return NodeExecution{}, err
	}
	if err := validateCreatedAt(createdAt); err != nil {
		return NodeExecution{}, err
	}
	return NodeExecution{id: normalizedID, workflowExecutionID: normalizedWorkflowExecutionID,
		nodeID: normalizedNodeID, status: NodeExecutionStatusPending, createdAt: createdAt,
	}, nil
}
func (execution NodeExecution) ID() NodeExecutionID { return execution.id }
func (execution NodeExecution) WorkflowExecutionID() WorkflowExecutionID {
	return execution.workflowExecutionID
}
func (execution NodeExecution) NodeID() workflow.NodeID {
	return execution.nodeID
}
func (execution NodeExecution) Status() NodeExecutionStatus { return execution.status }
func (execution NodeExecution) CreatedAt() time.Time {
	return execution.createdAt
}
func (execution NodeExecution) StartedAt() (time.Time, bool) {
	if execution.startedAt.IsZero() {
		return time.Time{}, false
	}
	return execution.startedAt, true
}
func (execution NodeExecution) FinishedAt() (time.Time, bool) {
	if execution.finishedAt.IsZero() {
		return time.Time{}, false
	}
	return execution.finishedAt, true
}
func (execution NodeExecution) IsTerminal() bool { return execution.status.IsTerminal() }
func (execution *NodeExecution) MarkReady(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusReady,
		at)
}
func (execution *NodeExecution) Queue(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusQueued,
		at)
}
func (execution *NodeExecution) Start(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusRunning,
		at)
}
func (execution *NodeExecution) Succeed(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusSucceeded,
		at)
}
func (execution *NodeExecution) ScheduleRetry(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusRetryPending,
		at)
}
func (execution *NodeExecution) ResumeRetry(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusRunning,
		at)
}
func (execution *NodeExecution) Fail(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusFailed,
		at)
}
func (execution *NodeExecution) Skip(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusSkipped,
		at)
}
func (execution *NodeExecution) Cancel(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusCancelled,
		at)
}
func (execution *NodeExecution) Timeout(at time.Time,
) error {
	return execution.transitionTo(NodeExecutionStatusTimedOut,
		at)
}
func (execution *NodeExecution) transitionTo(target NodeExecutionStatus,
	at time.Time) error {
	if execution == nil {
		return newValidationError("nodeExecution", "must not be nil")
	}
	if !isNodeTransitionAllowed(execution.status, target) {
		return &TransitionError{Entity: "node execution",
			CurrentStatus: execution.status.String(), TargetStatus: target.String()}
	}
	if err := validateTransitionTimestamp(
		at, execution.createdAt, execution.startedAt,
		target.IsTerminal() ||
			execution.status == NodeExecutionStatusRetryPending ||
			target == NodeExecutionStatusRetryPending); err != nil {
		return err
	}
	nextStartedAt := execution.startedAt
	nextFinishedAt := execution.finishedAt
	if target == NodeExecutionStatusRunning &&
		nextStartedAt.IsZero() {
		nextStartedAt = at
	}
	if target.IsTerminal() {
		nextFinishedAt = at
	}
	execution.status = target
	execution.startedAt = nextStartedAt
	execution.finishedAt = nextFinishedAt
	return nil
}
func isNodeTransitionAllowed(current NodeExecutionStatus, target NodeExecutionStatus,
) bool {
	switch current {
	case NodeExecutionStatusPending:
		return target == NodeExecutionStatusReady || target == NodeExecutionStatusSkipped || target == NodeExecutionStatusCancelled
	case NodeExecutionStatusReady:
		return target == NodeExecutionStatusQueued ||
			target == NodeExecutionStatusRunning || target == NodeExecutionStatusSkipped || target == NodeExecutionStatusCancelled ||
			target == NodeExecutionStatusTimedOut
	case NodeExecutionStatusQueued:
		return target == NodeExecutionStatusRunning || target == NodeExecutionStatusFailed || target == NodeExecutionStatusCancelled ||
			target == NodeExecutionStatusTimedOut
	case NodeExecutionStatusRunning:
		return target == NodeExecutionStatusSucceeded || target == NodeExecutionStatusRetryPending ||
			target == NodeExecutionStatusFailed || target == NodeExecutionStatusCancelled ||
			target == NodeExecutionStatusTimedOut
	case NodeExecutionStatusRetryPending:
		return target == NodeExecutionStatusRunning ||
			target == NodeExecutionStatusFailed ||
			target == NodeExecutionStatusCancelled ||
			target == NodeExecutionStatusTimedOut
	default:
		return false
	}
}

type NodeExecutionStatus string

const (
	NodeExecutionStatusPending      NodeExecutionStatus = "PENDING"
	NodeExecutionStatusReady        NodeExecutionStatus = "READY"
	NodeExecutionStatusQueued       NodeExecutionStatus = "QUEUED"
	NodeExecutionStatusRunning      NodeExecutionStatus = "RUNNING"
	NodeExecutionStatusRetryPending NodeExecutionStatus = "RETRY_PENDING"
	NodeExecutionStatusSucceeded    NodeExecutionStatus = "SUCCEEDED"
	NodeExecutionStatusFailed       NodeExecutionStatus = "FAILED"
	NodeExecutionStatusSkipped      NodeExecutionStatus = "SKIPPED"
	NodeExecutionStatusCancelled    NodeExecutionStatus = "CANCELLED"
	NodeExecutionStatusTimedOut     NodeExecutionStatus = "TIMED_OUT"
)

func (
	status NodeExecutionStatus) String() string {
	return string(status)
}
func (
	status NodeExecutionStatus) IsValid() bool {
	switch status {
	case NodeExecutionStatusPending, NodeExecutionStatusReady, NodeExecutionStatusQueued,
		NodeExecutionStatusRunning, NodeExecutionStatusRetryPending, NodeExecutionStatusSucceeded, NodeExecutionStatusFailed,
		NodeExecutionStatusSkipped, NodeExecutionStatusCancelled, NodeExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
func (status NodeExecutionStatus,
) IsTerminal() bool {
	switch status {
	case NodeExecutionStatusSucceeded,
		NodeExecutionStatusFailed, NodeExecutionStatusSkipped, NodeExecutionStatusCancelled,
		NodeExecutionStatusTimedOut:
		return true
	default:
		return false
	}
}
func (
	status NodeExecutionStatus) CanTransitionTo(target NodeExecutionStatus,
) bool {
	if !status.IsValid() || !target.IsValid() {
		return false
	}
	return isNodeTransitionAllowed(status, target)
}
