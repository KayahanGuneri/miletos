package plugin

import (
	"net/http"

	httpresponse "miletos-go/internal/shared/http/response"
)

type Controller struct {
	registry *NodeRegistry
}

func NewController(registry *NodeRegistry) *Controller {
	return &Controller{registry: registry}
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

type pluginResponse struct {
	Type                    string                       `json:"type"`
	Version                 string                       `json:"version"`
	DisplayName             string                       `json:"displayName"`
	Description             string                       `json:"description,omitempty"`
	InputMode               string                       `json:"inputMode"`
	AcceptsInitialVariables bool                         `json:"acceptsInitialVariables"`
	InputPorts              []pluginPortResponse         `json:"inputPorts"`
	OutputPorts             []pluginPortResponse         `json:"outputPorts"`
	InputEdgeConstraint     pluginEdgeConstraintResponse `json:"inputEdgeConstraint"`
	OutputEdgeConstraint    pluginEdgeConstraintResponse `json:"outputEdgeConstraint"`
}

func (controller *Controller) List(writer http.ResponseWriter, _ *http.Request) {
	definitions := controller.registry.Definitions()
	items := make([]pluginResponse, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, mapPlugin(definition))
	}
	httpresponse.WriteJSON(writer, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func mapPlugin(definition NodeDefinition) pluginResponse {
	return pluginResponse{
		Type: definition.Type, Version: definition.Version,
		DisplayName: definition.DisplayName, Description: definition.Description,
		InputMode:               definition.InputMode,
		AcceptsInitialVariables: definition.AcceptsInitialVariables,
		InputPorts:              mapPorts(definition.InputPorts), OutputPorts: mapPorts(definition.OutputPorts),
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
	}
}

func mapPorts(ports []Port) []pluginPortResponse {
	result := make([]pluginPortResponse, 0, len(ports))
	for _, port := range ports {
		result = append(result, pluginPortResponse{
			Name: port.Name, DisplayName: port.DisplayName, Description: port.Description,
		})
	}
	return result
}
