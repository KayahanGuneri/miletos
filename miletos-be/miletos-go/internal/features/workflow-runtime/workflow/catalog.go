package workflow

import (
	"encoding/json"
	"fmt"
)

const CatalogStatusActive = "ACTIVE"

type CatalogWorkflow struct {
	Workflow Workflow
	Status   string
}

type catalogWorkflowRecord struct {
	ID             int64  `gorm:"column:id"`
	CompanyID      int64  `gorm:"column:company_id"`
	Name           string `gorm:"column:name"`
	Status         string `gorm:"column:status"`
	Revision       uint64 `gorm:"column:revision"`
	DefinitionJSON []byte `gorm:"column:definition_json"`
}

type catalogDefinitionJSON struct {
	Nodes    []catalogNodeJSON `json:"nodes"`
	Edges    []catalogEdgeJSON `json:"edges"`
	Metadata map[string]any    `json:"metadata"`
}

type catalogNodeJSON struct {
	NodeID        string         `json:"nodeId"`
	PluginType    string         `json:"pluginType"`
	PluginVersion string         `json:"pluginVersion"`
	Configuration map[string]any `json:"configuration"`
	Position      *NodePosition  `json:"position"`
}

type catalogEdgeJSON struct {
	EdgeID           string `json:"edgeId"`
	SourceNodeID     string `json:"sourceNodeId"`
	SourceOutputPort string `json:"sourceOutputPort"`
	TargetNodeID     string `json:"targetNodeId"`
	TargetInputPort  string `json:"targetInputPort"`
}

func parseCatalogDefinition(
	definitionJSON []byte,
	companyID string,
	workflowID string,
	name string,
	revision uint64,
) (Workflow, error) {
	var definition catalogDefinitionJSON
	if err := json.Unmarshal(definitionJSON, &definition); err != nil {
		return Workflow{}, fmt.Errorf("decode catalog workflow definition: %w", err)
	}
	parsed := Workflow{
		ID:        workflowID,
		CompanyID: companyID,
		Name:      name,
		Revision:  revision,
		Nodes:     make([]WorkflowNode, 0, len(definition.Nodes)),
		Edges:     make([]Edge, 0, len(definition.Edges)),
		Metadata:  definition.Metadata,
	}
	for _, node := range definition.Nodes {
		parsed.Nodes = append(parsed.Nodes, WorkflowNode{
			ID:            node.NodeID,
			Type:          node.PluginType,
			Version:       node.PluginVersion,
			Configuration: node.Configuration,
			Position:      node.Position,
		})
	}
	for _, edge := range definition.Edges {
		parsed.Edges = append(parsed.Edges, Edge{
			ID:               edge.EdgeID,
			SourceNodeID:     edge.SourceNodeID,
			SourceOutputPort: edge.SourceOutputPort,
			TargetNodeID:     edge.TargetNodeID,
			TargetInputPort:  edge.TargetInputPort,
		})
	}
	return parsed, nil
}
