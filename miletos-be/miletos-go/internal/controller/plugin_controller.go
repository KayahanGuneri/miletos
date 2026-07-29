package controller

import (
	"net/http"

	"miletos-go/internal/engine"
	"miletos-go/internal/model"
)

type PluginController struct {
	registry *engine.NodeRegistry
}

func NewPluginController(registry *engine.NodeRegistry) *PluginController {
	return &PluginController{registry: registry}
}

type pluginPortResponse struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
}

type pluginEdgeConstraintResponse struct {
	Minimum   uint  `json:"minimum"`
	Maximum   *uint `json:"maximum,omitempty"`
	Unlimited bool  `json:"unlimited"`
}

type pluginPolicyResponse struct {
	DefaultCapacity  uint   `json:"defaultCapacity"`
	OverflowStrategy string `json:"overflowStrategy"`
}

type pluginResponse struct {
	Type                 string                       `json:"type"`
	Version              string                       `json:"version"`
	DisplayName          string                       `json:"displayName"`
	Description          string                       `json:"description,omitempty"`
	InputPorts           []pluginPortResponse         `json:"inputPorts"`
	OutputPorts          []pluginPortResponse         `json:"outputPorts"`
	InputEdgeConstraint  pluginEdgeConstraintResponse `json:"inputEdgeConstraint"`
	OutputEdgeConstraint pluginEdgeConstraintResponse `json:"outputEdgeConstraint"`
	QueuePolicy          pluginPolicyResponse         `json:"queuePolicy"`
	CachePolicy          pluginPolicyResponse         `json:"cachePolicy"`
	Distribution         string                       `json:"distribution"`
}

func (controller *PluginController) List(writer http.ResponseWriter, _ *http.Request) {
	definitions := controller.registry.Definitions()
	items := make([]pluginResponse, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, mapPlugin(definition))
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func mapPlugin(definition model.NodeDefinition) pluginResponse {
	return pluginResponse{
		Type: definition.Type, Version: definition.Version,
		DisplayName: definition.DisplayName, Description: definition.Description,
		InputPorts: mapPorts(definition.InputPorts), OutputPorts: mapPorts(definition.OutputPorts),
		InputEdgeConstraint: pluginEdgeConstraintResponse{
			Minimum:   definition.InputEdgeConstraint.Minimum,
			Maximum:   definition.InputEdgeConstraint.Maximum,
			Unlimited: definition.InputEdgeConstraint.Unlimited,
		},
		OutputEdgeConstraint: pluginEdgeConstraintResponse{
			Minimum:   definition.OutputEdgeConstraint.Minimum,
			Maximum:   definition.OutputEdgeConstraint.Maximum,
			Unlimited: definition.OutputEdgeConstraint.Unlimited,
		},
		QueuePolicy: pluginPolicyResponse{
			DefaultCapacity: 64, OverflowStrategy: "DROP_OLDEST",
		},
		CachePolicy: pluginPolicyResponse{
			DefaultCapacity: 1, OverflowStrategy: "DROP_OLDEST",
		},
		Distribution: "DISTRIBUTABLE",
	}
}

func mapPorts(ports []model.Port) []pluginPortResponse {
	result := make([]pluginPortResponse, 0, len(ports))
	for _, port := range ports {
		result = append(result, pluginPortResponse{
			Name: port.Name, DisplayName: port.DisplayName, Description: port.Description,
		})
	}
	return result
}
