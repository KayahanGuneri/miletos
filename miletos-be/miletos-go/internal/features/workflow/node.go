package workflow

import "math"

type NodePosition struct {
	x float64
	y float64
}

func NewNodePosition(x float64,
	y float64) (NodePosition, error) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return NodePosition{}, newValidationError("position.x", "must be finite")
	}
	if math.IsNaN(y) || math.IsInf(y, 0) {
		return NodePosition{}, newValidationError("position.y",
			"must be finite")
	}
	return NodePosition{x: x,
		y: y}, nil
}
func (position NodePosition) X() float64 {
	return position.x
}
func (position NodePosition) Y() float64 {
	return position.y
}

type NodeDefinition struct {
	id            NodeID
	pluginType    PluginType
	pluginVersion PluginVersion
	configuration JSONObject
	position      NodePosition
	hasPosition   bool
}

func NewNodeDefinition(id NodeID, pluginType PluginType,
	pluginVersion PluginVersion, configuration []byte, position *NodePosition,
) (NodeDefinition, error) {
	normalizedID, err := NewNodeID(id.String())
	if err != nil {
		return NodeDefinition{}, err
	}
	normalizedPluginType, err := NewPluginType(pluginType.String())
	if err != nil {
		return NodeDefinition{}, err
	}
	normalizedPluginVersion, err := NewPluginVersion(pluginVersion.String())
	if err != nil {
		return NodeDefinition{}, err
	}
	configurationObject, err := newJSONObject(
		"configuration", configuration)
	if err != nil {
		return NodeDefinition{}, err
	}
	var storedPosition NodePosition
	hasPosition := false
	if position != nil {
		normalizedPosition, err := NewNodePosition(
			position.X(), position.Y())
		if err != nil {
			return NodeDefinition{}, err
		}
		storedPosition = normalizedPosition
		hasPosition = true
	}
	return NodeDefinition{
		id: normalizedID, pluginType: normalizedPluginType, pluginVersion: normalizedPluginVersion,
		configuration: configurationObject, position: storedPosition, hasPosition: hasPosition,
	}, nil
}
func (definition NodeDefinition) ID() NodeID { return definition.id }
func (definition NodeDefinition) PluginType() PluginType {
	return definition.pluginType
}
func (definition NodeDefinition) PluginVersion() PluginVersion {
	return definition.pluginVersion
}
func (definition NodeDefinition) Configuration() JSONObject {
	return JSONObject{raw: definition.configuration.Bytes()}
}
func (definition NodeDefinition) Position() (NodePosition, bool) {
	if !definition.hasPosition {
		return NodePosition{}, false
	}
	return definition.position, true
}
func (definition NodeDefinition) clone() NodeDefinition {
	cloned := definition
	cloned.configuration = JSONObject{raw: definition.configuration.Bytes()}
	return cloned
}
func (definition NodeDefinition) normalized() (NodeDefinition, error) {
	var position *NodePosition
	if definition.hasPosition {
		copiedPosition := definition.position
		position = &copiedPosition
	}
	return NewNodeDefinition(definition.id,
		definition.pluginType, definition.pluginVersion, definition.configuration.Bytes(),
		position)
}
