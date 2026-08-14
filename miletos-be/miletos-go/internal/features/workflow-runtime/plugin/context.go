package plugin

import "context"

type ScenarioStartContext struct {
	CompanyID        string
	WorkflowID       string
	WorkflowRevision uint64
	NodeID           string
}

type HTTPScenarioStartRequest struct {
	Scenario ScenarioStartContext
	Method   string
}

type HTTPInfrastructure interface {
	CreateTrigger(context.Context, HTTPScenarioStartRequest) error
}

type EdgeData struct {
	ID               string
	SourceOutputPort string
	TargetNodeID     string
	TargetInputPort  string
}

type Access interface {
	GetOutputEdgeCount() int
	GetEdgeData(index int) (EdgeData, bool)
	PushEdge(index int, payload any) error
}

type Infrastructure struct {
	HTTP HTTPInfrastructure
}

type Context struct {
	Runtime       context.Context
	Execution     *NodeExecutionContext
	Scenario      *ScenarioStartContext
	Configuration map[string]any
	Payload       any
	Access        Access
	Infra         Infrastructure
}

type ScenarioStartHandler func(*Context) error

type RunHandler func(*Context) (any, error)

type NodeLifecycles struct {
	OnScenarioStart ScenarioStartHandler
	OnRun           RunHandler
}
