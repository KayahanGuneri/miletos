package plugin

import runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"

func mapGRPCPluginList(registrations []NodeRegistration) *runtimev1.ListPluginsResponse {
	items := make([]*runtimev1.Plugin, 0, len(registrations))
	for _, registration := range registrations {
		items = append(items, mapGRPCPlugin(registration))
	}
	return &runtimev1.ListPluginsResponse{Items: items, Count: uint32(len(items))}
}

func mapGRPCPlugin(registration NodeRegistration) *runtimev1.Plugin {
	definition := registration.Definition
	declaresHTTPWebhook := declaresExecutionSource(registration, "HTTP_WEBHOOK")
	return &runtimev1.Plugin{
		Type: definition.Type, Version: definition.Version,
		DisplayName: definition.DisplayName, Description: definition.Description,
		InputMode:               definition.InputMode,
		AcceptsInitialVariables: CanReceiveEntryInput(definition) || declaresHTTPWebhook,
		InputPorts:              mapGRPCPorts(definition.InputPorts),
		OutputPorts:             mapGRPCPorts(definition.OutputPorts),
		InputEdgeConstraint:     mapGRPCEdgeConstraint(definition.InputEdgeConstraint),
		OutputEdgeConstraint:    mapGRPCEdgeConstraint(definition.OutputEdgeConstraint),
		AllowedRootOrigins:      append([]string(nil), registration.AllowedExecutionSources...),
		ContextProvider:         contextProviderName(declaresHTTPWebhook),
	}
}

func mapGRPCPorts(ports []Port) []*runtimev1.Port {
	result := make([]*runtimev1.Port, 0, len(ports))
	for _, port := range ports {
		result = append(result, &runtimev1.Port{
			Name: port.Name, DisplayName: port.DisplayName, Description: port.Description,
		})
	}
	return result
}

func mapGRPCEdgeConstraint(constraint EdgeConstraint) *runtimev1.EdgeConstraint {
	result := &runtimev1.EdgeConstraint{
		Minimum: uint32(constraint.Minimum), Unlimited: constraint.Maximum == nil,
	}
	if constraint.Maximum != nil {
		maximum := uint32(*constraint.Maximum)
		result.Maximum = &maximum
	}
	return result
}
