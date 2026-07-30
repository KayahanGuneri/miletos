package workflow

import "time"

type Workflow struct {
	ID        string         `json:"id"`
	CompanyID string         `json:"companyId"`
	Name      string         `json:"name"`
	Revision  uint64         `json:"revision"`
	Nodes     []WorkflowNode `json:"nodes"`
	Edges     []Edge         `json:"edges"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Edge struct {
	ID               string `json:"id"`
	SourceNodeID     string `json:"sourceNodeId"`
	SourceOutputPort string `json:"sourceOutputPort"`
	TargetNodeID     string `json:"targetNodeId"`
	TargetInputPort  string `json:"targetInputPort"`
}

type WorkflowNode struct {
	ID            string         `json:"id"`
	Type          string         `json:"pluginType"`
	Version       string         `json:"pluginVersion"`
	Configuration map[string]any `json:"configuration,omitempty"`
	Position      *NodePosition  `json:"position,omitempty"`
}

type NodePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type WorkflowSnapshot struct {
	ID        string    `json:"snapshotId"`
	Workflow  Workflow  `json:"definition"`
	CreatedAt time.Time `json:"createdAt"`
}
