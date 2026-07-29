package model

import "time"

type Workflow struct {
	ID        string         `json:"id"`
	CompanyID string         `json:"companyId"`
	Name      string         `json:"name"`
	Revision  uint64         `json:"revision"`
	Nodes     []Node         `json:"nodes"`
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

type WorkflowSnapshot struct {
	ID        string    `json:"snapshotId"`
	Workflow  Workflow  `json:"definition"`
	CreatedAt time.Time `json:"createdAt"`
}
