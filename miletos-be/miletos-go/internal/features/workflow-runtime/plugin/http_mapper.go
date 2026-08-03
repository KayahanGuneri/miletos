package plugin

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
	AllowedRootOrigins      []string                     `json:"allowedRootOrigins,omitempty"`
	ContextProvider         string                       `json:"contextProvider,omitempty"`
}

type pluginListResponse struct {
	Items []pluginResponse `json:"items"`
	Count int              `json:"count"`
}

func mapPluginList(registrations []NodeRegistration) pluginListResponse {
	items := make([]pluginResponse, 0, len(registrations))
	for _, registration := range registrations {
		items = append(items, mapPlugin(registration))
	}
	return pluginListResponse{Items: items, Count: len(items)}
}

func mapPlugin(registration NodeRegistration) pluginResponse {
	definition := registration.Definition
	declaresHTTPWebhook := declaresExecutionSource(registration, "HTTP_WEBHOOK")
	return pluginResponse{
		Type: definition.Type, Version: definition.Version,
		DisplayName: definition.DisplayName, Description: definition.Description,
		InputMode:               definition.InputMode,
		AcceptsInitialVariables: CanReceiveEntryInput(definition) || declaresHTTPWebhook,
		InputPorts:              mapPorts(definition.InputPorts),
		OutputPorts:             mapPorts(definition.OutputPorts),
		InputEdgeConstraint:     mapEdgeConstraint(definition.InputEdgeConstraint),
		OutputEdgeConstraint:    mapEdgeConstraint(definition.OutputEdgeConstraint),
		AllowedRootOrigins:      append([]string(nil), registration.AllowedExecutionSources...),
		ContextProvider:         contextProviderName(declaresHTTPWebhook),
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

func mapEdgeConstraint(constraint EdgeConstraint) pluginEdgeConstraintResponse {
	return pluginEdgeConstraintResponse{
		Minimum:   constraint.Minimum,
		Maximum:   constraint.Maximum,
		Unlimited: constraint.Maximum == nil,
	}
}

func contextProviderName(allowsHTTPWebhook bool) string {
	if allowsHTTPWebhook {
		return "http-trigger"
	}
	return ""
}
