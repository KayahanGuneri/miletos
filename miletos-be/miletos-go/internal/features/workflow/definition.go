package workflow

import "fmt"

type WorkflowDefinition struct {
	id        WorkflowID
	companyID CompanyID
	name      string
	revision  uint64
	nodes     []NodeDefinition
	edges     []EdgeDefinition
	metadata  JSONObject
}

func NewWorkflowDefinition(id WorkflowID, companyID CompanyID,
	name string, revision uint64, nodes []NodeDefinition,
	edges []EdgeDefinition, metadata []byte) (WorkflowDefinition, error) {
	normalizedID, err := NewWorkflowID(id.String())
	if err != nil {
		return WorkflowDefinition{}, err
	}
	normalizedCompanyID, err := NewCompanyID(
		companyID.String())
	if err != nil {
		return WorkflowDefinition{}, err
	}
	normalizedName, err := normalizeRequiredString("name", name)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	if revision == 0 {
		return WorkflowDefinition{}, newValidationError("revision", "must be greater than zero")
	}
	normalizedNodes := make([]NodeDefinition, len(nodes))
	for index, node := range nodes {
		normalizedNode, err := node.normalized()
		if err != nil {
			return WorkflowDefinition{}, newValidationError(
				fmt.Sprintf("nodes[%d]", index), err.Error())
		}
		normalizedNodes[index] = normalizedNode
	}
	normalizedEdges := make(
		[]EdgeDefinition, len(edges))
	for index, edge := range edges {
		normalizedEdge, err := edge.normalized()
		if err != nil {
			return WorkflowDefinition{}, newValidationError(fmt.Sprintf("edges[%d]", index),
				err.Error())
		}
		normalizedEdges[index] = normalizedEdge
	}
	metadataObject, err := newJSONObject("metadata",
		metadata)
	if err != nil {
		return WorkflowDefinition{}, err
	}
	return WorkflowDefinition{id: normalizedID, companyID: normalizedCompanyID,
		name: normalizedName, revision: revision, nodes: normalizedNodes,
		edges: normalizedEdges, metadata: metadataObject}, nil
}
func (definition WorkflowDefinition) ID() WorkflowID {
	return definition.id
}
func (definition WorkflowDefinition) CompanyID() CompanyID { return definition.companyID }
func (definition WorkflowDefinition) Name() string {
	return definition.name
}
func (definition WorkflowDefinition) Revision() uint64 {
	return definition.revision
}
func (definition WorkflowDefinition) Nodes() []NodeDefinition {
	nodes := make([]NodeDefinition,
		len(definition.nodes))
	for index, node := range definition.nodes {
		nodes[index] = node.clone()
	}
	return nodes
}
func (definition WorkflowDefinition) Edges() []EdgeDefinition {
	edges := make(
		[]EdgeDefinition, len(definition.edges))
	for index, edge := range definition.edges {
		edges[index] = edge.clone()
	}
	return edges
}
func (definition WorkflowDefinition) Metadata() JSONObject {
	return JSONObject{raw: definition.metadata.Bytes()}
}
