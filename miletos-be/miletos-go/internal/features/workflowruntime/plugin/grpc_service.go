package plugin

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
)

type GRPCService struct {
	runtimev1.UnimplementedPluginServiceServer
	registry *NodeRegistry
}

func NewGRPCService(registry *NodeRegistry) *GRPCService {
	return &GRPCService{registry: registry}
}

func (service *GRPCService) ListPlugins(
	ctx context.Context,
	_ *emptypb.Empty,
) (*runtimev1.ListPluginsResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	definitions := service.registry.Definitions()
	items := make([]*runtimev1.Plugin, 0, len(definitions))
	for _, definition := range definitions {
		items = append(items, mapGRPCPlugin(definition))
	}
	return &runtimev1.ListPluginsResponse{
		Items: items,
		Count: uint32(len(items)),
	}, nil
}

func mapGRPCPlugin(definition NodeDefinition) *runtimev1.Plugin {
	return &runtimev1.Plugin{
		Type:                    definition.Type,
		Version:                 definition.Version,
		DisplayName:             definition.DisplayName,
		Description:             definition.Description,
		InputMode:               definition.InputMode,
		AcceptsInitialVariables: definition.AcceptsInitialVariables,
		InputPorts:              mapGRPCPorts(definition.InputPorts),
		OutputPorts:             mapGRPCPorts(definition.OutputPorts),
		InputEdgeConstraint:     mapGRPCEdgeConstraint(definition.InputEdgeConstraint),
		OutputEdgeConstraint:    mapGRPCEdgeConstraint(definition.OutputEdgeConstraint),
	}
}

func mapGRPCPorts(ports []Port) []*runtimev1.Port {
	result := make([]*runtimev1.Port, 0, len(ports))
	for _, port := range ports {
		result = append(result, &runtimev1.Port{
			Name:        port.Name,
			DisplayName: port.DisplayName,
			Description: port.Description,
		})
	}
	return result
}

func mapGRPCEdgeConstraint(constraint EdgeConstraint) *runtimev1.EdgeConstraint {
	result := &runtimev1.EdgeConstraint{
		Minimum:   uint32(constraint.Minimum),
		Unlimited: constraint.Unlimited,
	}
	if constraint.Maximum != nil {
		maximum := uint32(*constraint.Maximum)
		result.Maximum = &maximum
	}
	return result
}
