package workflow

import "strings"

type CompanyID string
type WorkflowID string
type NodeID string
type EdgeID string
type PluginType string
type PluginVersion string

func NewCompanyID(value string) (CompanyID, error) {
	normalized, err := normalizeRequiredString("companyID", value)
	return CompanyID(normalized), err
}

func (id CompanyID) String() string { return string(id) }

func NewWorkflowID(value string) (WorkflowID, error) {
	normalized, err := normalizeRequiredString("workflowID", value)
	return WorkflowID(normalized), err
}

func (id WorkflowID) String() string { return string(id) }

func NewNodeID(value string) (NodeID, error) {
	normalized, err := normalizeRequiredString("nodeID", value)
	return NodeID(normalized), err
}

func (id NodeID) String() string { return string(id) }

func NewEdgeID(value string) (EdgeID, error) {
	normalized, err := normalizeRequiredString("edgeID", value)
	return EdgeID(normalized), err
}

func (id EdgeID) String() string { return string(id) }

func NewPluginType(value string) (PluginType, error) {
	normalized, err := normalizeRequiredString("pluginType", value)
	return PluginType(normalized), err
}

func (pluginType PluginType) String() string { return string(pluginType) }

func NewPluginVersion(value string) (PluginVersion, error) {
	normalized, err := normalizeRequiredString("pluginVersion", value)
	return PluginVersion(normalized), err
}

func (version PluginVersion) String() string { return string(version) }

func normalizeRequiredString(field string, value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(field, "must not be empty")
	}
	return normalized, nil
}
