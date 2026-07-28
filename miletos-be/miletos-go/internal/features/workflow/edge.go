package workflow

type EdgeDefinition struct {
	id               EdgeID
	sourceNodeID     NodeID
	sourceOutputPort string
	targetNodeID     NodeID
	targetInputPort  string
}

func NewEdgeDefinition(id EdgeID,
	sourceNodeID NodeID, sourceOutputPort string, targetNodeID NodeID,
	targetInputPort string) (EdgeDefinition, error) {
	normalizedID, err := NewEdgeID(id.String())
	if err != nil {
		return EdgeDefinition{}, err
	}
	normalizedSourceNodeID, err := NewNodeID(sourceNodeID.String())
	if err != nil {
		return EdgeDefinition{}, newValidationError(
			"sourceNodeID", err.Error())
	}
	normalizedSourceOutputPort, err := normalizeRequiredString(
		"sourceOutputPort", sourceOutputPort)
	if err != nil {
		return EdgeDefinition{}, err
	}
	normalizedTargetNodeID, err := NewNodeID(targetNodeID.String())
	if err != nil {
		return EdgeDefinition{}, newValidationError(
			"targetNodeID", err.Error())
	}
	normalizedTargetInputPort, err := normalizeRequiredString(
		"targetInputPort", targetInputPort)
	if err != nil {
		return EdgeDefinition{}, err
	}
	return EdgeDefinition{id: normalizedID,
		sourceNodeID: normalizedSourceNodeID, sourceOutputPort: normalizedSourceOutputPort, targetNodeID: normalizedTargetNodeID,
		targetInputPort: normalizedTargetInputPort}, nil
}
func (definition EdgeDefinition) ID() EdgeID {
	return definition.id
}
func (definition EdgeDefinition) SourceNodeID() NodeID {
	return definition.sourceNodeID
}
func (definition EdgeDefinition) SourceOutputPort() string { return definition.sourceOutputPort }
func (definition EdgeDefinition) TargetNodeID() NodeID {
	return definition.targetNodeID
}
func (definition EdgeDefinition) TargetInputPort() string {
	return definition.targetInputPort
}
func (definition EdgeDefinition) clone() EdgeDefinition { return definition }
func (definition EdgeDefinition) normalized() (EdgeDefinition, error) {
	return NewEdgeDefinition(
		definition.id, definition.sourceNodeID, definition.sourceOutputPort,
		definition.targetNodeID, definition.targetInputPort)
}
