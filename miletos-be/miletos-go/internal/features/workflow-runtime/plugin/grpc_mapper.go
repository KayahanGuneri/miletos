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
	return &runtimev1.Plugin{
		Type: registration.Key, Version: "v1",
		DisplayName:             registration.Key,
		InputMode:               registration.InputMode,
		AcceptsInitialVariables: CanReceiveEntryInput(registration) && allowsExecutionSource(registration, "MANUAL_DIRECT"),
		InputPorts:              mapGRPCPorts(registration.InputPorts),
		OutputPorts:             mapGRPCPorts(registration.OutputPorts),
		InputEdgeConstraint:     mapGRPCEdgeConstraint(registration.InputEdgeConstraint),
		OutputEdgeConstraint:    mapGRPCEdgeConstraint(registration.OutputEdgeConstraint),
		AllowedRootOrigins:      append([]string(nil), registration.AllowedExecutionSources...),
		ContextProvider:         registration.ContextProvider,
		ConnectionRestrictions:  mapGRPCConnectionRestrictions(registration.ConnectionRestrictions),
	}
}

func mapGRPCPorts(ports []Port) []*runtimev1.Port {
	result := make([]*runtimev1.Port, 0, len(ports))
	for _, port := range ports {
		mapped := &runtimev1.Port{
			Name: port.Name, DisplayName: port.Name,
		}
		if port.EdgeConstraint != nil {
			mapped.EdgeConstraint = mapGRPCEdgeConstraint(*port.EdgeConstraint)
		}
		result = append(result, mapped)
	}
	return result
}

func mapGRPCConnectionRestrictions(
	restrictions []ConnectionRestriction,
) []*runtimev1.ConnectionRestriction {
	result := make([]*runtimev1.ConnectionRestriction, 0, len(restrictions))
	for _, restriction := range restrictions {
		result = append(result, &runtimev1.ConnectionRestriction{
			From: restriction.From, To: restriction.To,
			Selector: string(restriction.Selector), Position: uint32(restriction.Position),
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
