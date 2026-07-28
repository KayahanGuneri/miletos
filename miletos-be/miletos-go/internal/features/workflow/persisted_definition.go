package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type persistedWorkflowDefinition struct {
	ID        string                  `json:"id"`
	CompanyID string                  `json:"companyId"`
	Name      string                  `json:"name"`
	Revision  uint64                  `json:"revision"`
	Nodes     []persistedWorkflowNode `json:"nodes"`
	Edges     []persistedWorkflowEdge `json:"edges"`
	Metadata  json.RawMessage         `json:"metadata"`
}
type persistedWorkflowNode struct {
	ID            string                     `json:"id"`
	PluginType    string                     `json:"pluginType"`
	PluginVersion string                     `json:"pluginVersion"`
	Configuration json.RawMessage            `json:"configuration"`
	Position      *persistedWorkflowPosition `json:"position,omitempty"`
}
type persistedWorkflowPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type persistedWorkflowEdge struct {
	ID               string `json:"id"`
	SourceNodeID     string `json:"sourceNodeId"`
	SourceOutputPort string `json:"sourceOutputPort"`
	TargetNodeID     string `json:"targetNodeId"`
	TargetInputPort  string `json:"targetInputPort"`
}

func DecodePersistedDefinition(encoded []byte) (WorkflowDefinition, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var document persistedWorkflowDefinition
	if err := decoder.Decode(&document); err != nil {
		return WorkflowDefinition{}, fmt.Errorf("decode persisted workflow definition: %w", err)
	}
	if err := ensurePersistedDefinitionEOF(decoder); err != nil {
		return WorkflowDefinition{}, err
	}
	nodes := make([]NodeDefinition, 0, len(document.Nodes))
	for index, item := range document.Nodes {
		var position *NodePosition
		if item.Position != nil {
			value, err := NewNodePosition(item.Position.X, item.Position.Y)
			if err != nil {
				return WorkflowDefinition{}, fmt.Errorf("decode persisted node %d position: %w", index, err)
			}
			position = &value
		}
		node, err := NewNodeDefinition(
			NodeID(item.ID), PluginType(item.PluginType), PluginVersion(item.PluginVersion),
			item.Configuration, position)
		if err != nil {
			return WorkflowDefinition{}, fmt.Errorf("decode persisted node %d: %w", index, err)
		}
		nodes = append(nodes, node)
	}
	edges := make([]EdgeDefinition, 0, len(document.Edges))
	for index, item := range document.Edges {
		edge, err := NewEdgeDefinition(
			EdgeID(item.ID), NodeID(item.SourceNodeID), item.SourceOutputPort,
			NodeID(item.TargetNodeID), item.TargetInputPort)
		if err != nil {
			return WorkflowDefinition{}, fmt.Errorf("decode persisted edge %d: %w", index, err)
		}
		edges = append(edges, edge)
	}
	return NewWorkflowDefinition(WorkflowID(document.ID), CompanyID(document.CompanyID),
		document.Name, document.Revision, nodes,
		edges, document.Metadata)
}
func ensurePersistedDefinitionEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode persisted workflow definition trailing content: %w", err)
	}
	return fmt.Errorf("persisted workflow definition contains multiple JSON values")
}
