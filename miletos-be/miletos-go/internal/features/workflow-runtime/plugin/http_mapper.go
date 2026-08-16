package plugin

type pluginPortResponse struct {
	Name           string                        `json:"name"`
	DisplayName    string                        `json:"displayName,omitempty"`
	Description    string                        `json:"description,omitempty"`
	EdgeConstraint *pluginEdgeConstraintResponse `json:"edgeConstraint,omitempty"`
}

type pluginEdgeConstraintResponse struct {
	Minimum   uint  `json:"minimum"`
	Maximum   *uint `json:"maximum,omitempty"`
	Unlimited bool  `json:"unlimited"`
}

type pluginConnectionRestrictionResponse struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Selector string `json:"selector"`
	Position uint   `json:"position,omitempty"`
}

type pluginResponse struct {
	Type                    string                                `json:"type"`
	Version                 string                                `json:"version"`
	DisplayName             string                                `json:"displayName"`
	Description             string                                `json:"description,omitempty"`
	InputMode               string                                `json:"inputMode"`
	AcceptsInitialVariables bool                                  `json:"acceptsInitialVariables"`
	InputPorts              []pluginPortResponse                  `json:"inputPorts"`
	OutputPorts             []pluginPortResponse                  `json:"outputPorts"`
	InputEdgeConstraint     pluginEdgeConstraintResponse          `json:"inputEdgeConstraint"`
	OutputEdgeConstraint    pluginEdgeConstraintResponse          `json:"outputEdgeConstraint"`
	AllowedRootOrigins      []string                              `json:"allowedRootOrigins,omitempty"`
	ContextProvider         string                                `json:"contextProvider,omitempty"`
	ConnectionRestrictions  []pluginConnectionRestrictionResponse `json:"connectionRestrictions,omitempty"`
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
	return pluginResponse{
		Type: registration.Key, Version: "v1",
		DisplayName:             registration.Key,
		InputMode:               registration.InputMode,
		AcceptsInitialVariables: CanReceiveEntryInput(registration) && allowsExecutionSource(registration, "MANUAL_DIRECT"),
		InputPorts:              mapPorts(registration.InputPorts),
		OutputPorts:             mapPorts(registration.OutputPorts),
		InputEdgeConstraint:     mapEdgeConstraint(registration.InputEdgeConstraint),
		OutputEdgeConstraint:    mapEdgeConstraint(registration.OutputEdgeConstraint),
		AllowedRootOrigins:      append([]string(nil), registration.AllowedExecutionSources...),
		ContextProvider:         registration.ContextProvider,
		ConnectionRestrictions:  mapConnectionRestrictions(registration.ConnectionRestrictions),
	}
}

func mapPorts(ports []Port) []pluginPortResponse {
	result := make([]pluginPortResponse, 0, len(ports))
	for _, port := range ports {
		mapped := pluginPortResponse{
			Name: port.Name, DisplayName: port.Name,
		}
		if port.EdgeConstraint != nil {
			constraint := mapEdgeConstraint(*port.EdgeConstraint)
			mapped.EdgeConstraint = &constraint
		}
		result = append(result, mapped)
	}
	return result
}

func mapConnectionRestrictions(
	restrictions []ConnectionRestriction,
) []pluginConnectionRestrictionResponse {
	result := make([]pluginConnectionRestrictionResponse, 0, len(restrictions))
	for _, restriction := range restrictions {
		result = append(result, pluginConnectionRestrictionResponse{
			From: restriction.From, To: restriction.To,
			Selector: string(restriction.Selector), Position: restriction.Position,
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
