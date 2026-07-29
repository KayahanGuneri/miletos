package model

import "time"

type Node struct {
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

type NodeExecution struct {
	ID          string         `json:"nodeExecutionId"`
	ExecutionID string         `json:"workflowExecutionId"`
	CompanyID   string         `json:"-"`
	NodeID      string         `json:"nodeId"`
	Type        string         `json:"pluginType"`
	Version     string         `json:"pluginVersion"`
	Status      string         `json:"status"`
	Attempt     int            `json:"attempt"`
	CreatedAt   time.Time      `json:"createdAt"`
	ReadyAt     *time.Time     `json:"readyAt,omitempty"`
	QueuedAt    *time.Time     `json:"queuedAt,omitempty"`
	StartedAt   *time.Time     `json:"startedAt,omitempty"`
	FinishedAt  *time.Time     `json:"finishedAt,omitempty"`
	NextAttemptAt *time.Time   `json:"nextAttemptAt,omitempty"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	Input       map[string]any `json:"inputSummary,omitempty"`
	Output      map[string]any `json:"outputSummary,omitempty"`
	Failure     map[string]any `json:"failureSummary,omitempty"`
}

type NodeDefinition struct {
	Type                 string
	Version              string
	DisplayName          string
	Description          string
	InputPorts           []Port
	OutputPorts          []Port
	InputEdgeConstraint  EdgeConstraint
	OutputEdgeConstraint EdgeConstraint
}

type Port struct {
	Name        string
	DisplayName string
	Description string
}

type EdgeConstraint struct {
	Minimum   uint
	Maximum   *uint
	Unlimited bool
}
