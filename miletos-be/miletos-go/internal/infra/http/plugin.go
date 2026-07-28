package api

import (
	engineplugin "miletos-go/internal/engine/plugin"
	"miletos-go/internal/infra/http/transport"
	"net/http"
)

type pluginCatalog interface {
	List() []engineplugin.Descriptor
	Len() int
}

// RegistryCatalog is kept as a compatibility alias.
// engineplugin.Registry already satisfies Catalog directly, so no wrapper state is needed.
type RegistryCatalog = engineplugin.Registry

func NewRegistryCatalog(registry engineplugin.Registry) RegistryCatalog { return registry }

type PluginPortResponse struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
}
type PluginEdgeConstraintResponse struct {
	Minimum   uint  `json:"minimum"`
	Maximum   *uint `json:"maximum,omitempty"`
	Unlimited bool  `json:"unlimited"`
}
type PluginPolicyResponse struct {
	DefaultCapacity  uint   `json:"defaultCapacity"`
	OverflowStrategy string `json:"overflowStrategy"`
}
type PluginDescriptor struct {
	Type                 string                       `json:"type"`
	Version              string                       `json:"version"`
	DisplayName          string                       `json:"displayName"`
	Description          string                       `json:"description,omitempty"`
	InputPorts           []PluginPortResponse         `json:"inputPorts"`
	OutputPorts          []PluginPortResponse         `json:"outputPorts"`
	InputEdgeConstraint  PluginEdgeConstraintResponse `json:"inputEdgeConstraint"`
	OutputEdgeConstraint PluginEdgeConstraintResponse `json:"outputEdgeConstraint"`
	QueuePolicy          PluginPolicyResponse         `json:"queuePolicy"`
	CachePolicy          PluginPolicyResponse         `json:"cachePolicy"`
	Distribution         string                       `json:"distribution"`
}
type PluginListResponse struct {
	Items []PluginDescriptor `json:"items"`
	Count int                `json:"count"`
}
type PluginHandler struct{ catalog pluginCatalog }

func NewPluginHandler(catalog pluginCatalog) PluginHandler { return PluginHandler{catalog: catalog} }
func (handler PluginHandler) Plugins(writer http.ResponseWriter, request *http.Request) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	if handler.catalog == nil {
		transport.WriteRequestAPIError(writer, request, transport.NewAPIError(http.StatusServiceUnavailable, transport.ErrorCodeServiceUnavailable, "Plugin catalog is unavailable."))
		return
	}
	items := make([]PluginDescriptor, 0, handler.catalog.Len())
	for _, descriptor := range handler.catalog.List() {
		items = append(items, mapPluginDescriptor(descriptor))
	}
	_ = transport.WriteJSON(writer, http.StatusOK, PluginListResponse{Items: items, Count: len(items)})
}
func mapPluginDescriptor(
	descriptor engineplugin.Descriptor) PluginDescriptor {
	identity := descriptor.Identity()
	metadata := descriptor.Metadata()
	return PluginDescriptor{
		Type: identity.Type().String(), Version: identity.Version().String(), DisplayName: metadata.DisplayName(),
		Description: metadata.Description(), InputPorts: mapPluginPorts(
			descriptor.InputPorts()),
		OutputPorts:         mapPluginPorts(descriptor.OutputPorts()),
		InputEdgeConstraint: mapPluginEdgeConstraint(descriptor.InputEdgeConstraint()), OutputEdgeConstraint: mapPluginEdgeConstraint(
			descriptor.OutputEdgeConstraint()),
		QueuePolicy: mapPluginPolicy(descriptor.QueuePolicy().DefaultCapacity(),
			descriptor.QueuePolicy().OverflowStrategy().
				String()),
		CachePolicy: mapPluginPolicy(descriptor.CachePolicy().DefaultCapacity(),
			descriptor.CachePolicy().OverflowStrategy().
				String()),
		Distribution: descriptor.Distribution().String(),
	}
}
func mapPluginPorts(ports []engineplugin.Port) []PluginPortResponse {
	if len(ports) == 0 {
		return []PluginPortResponse{}
	}
	responses := make([]PluginPortResponse,
		0, len(ports))
	for _, port := range ports {
		responses = append(
			responses, PluginPortResponse{Name: port.Name(),
				DisplayName: port.DisplayName(), Description: port.Description()},
		)
	}
	return responses
}
func mapPluginEdgeConstraint(constraint engineplugin.EdgeConstraint) PluginEdgeConstraintResponse {
	maximum, hasMaximum := constraint.Maximum()
	var maximumValue *uint
	if hasMaximum {
		value := maximum
		maximumValue = &value
	}
	return PluginEdgeConstraintResponse{Minimum: constraint.Minimum(),
		Maximum: maximumValue, Unlimited: constraint.IsUnlimited()}
}
func mapPluginPolicy(
	defaultCapacity uint, overflowStrategy string) PluginPolicyResponse {
	return PluginPolicyResponse{DefaultCapacity: defaultCapacity, OverflowStrategy: overflowStrategy}
}
