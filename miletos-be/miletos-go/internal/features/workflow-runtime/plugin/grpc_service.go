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
	return mapGRPCPluginList(service.registry.Registrations()), nil
}
