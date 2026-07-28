package plugin

import (
	"fmt"
	"sort"

	"miletos-go/internal/features/workflow"
)

type DuplicatePluginError struct {
	Identity PluginIdentity
}

func (e *DuplicatePluginError) Error() string {
	if e == nil || !e.Identity.IsValid() {
		return "plugin registry contains a duplicate descriptor"
	}
	return fmt.Sprintf(
		"plugin registry contains duplicate descriptor %s", e.Identity.String())
}

type Registry struct {
	descriptorsByIdentity map[PluginIdentity]Descriptor
	pluginTypes           map[workflow.PluginType]struct{}
	descriptors           []Descriptor
}

func NewRegistry(
	descriptors []Descriptor) (Registry, error) {
	descriptorsByIdentity := make(
		map[PluginIdentity]Descriptor, len(descriptors))
	pluginTypes := make(map[workflow.PluginType]struct{},
		len(descriptors))
	normalizedDescriptors := make([]Descriptor, 0,
		len(descriptors))
	for index, descriptor := range descriptors {
		normalizedDescriptor, err := descriptor.normalized()
		if err != nil {
			return Registry{}, newValidationError(fmt.Sprintf("descriptors[%d]",
				index), err.Error(),
			)
		}
		identity := normalizedDescriptor.Identity()
		if _, exists := descriptorsByIdentity[identity]; exists {
			return Registry{}, &DuplicatePluginError{Identity: identity}
		}
		storedDescriptor := normalizedDescriptor.clone()
		descriptorsByIdentity[identity] = storedDescriptor
		pluginTypes[identity.Type()] = struct{}{}
		normalizedDescriptors = append(normalizedDescriptors,
			storedDescriptor.clone())
	}
	sort.SliceStable(normalizedDescriptors,
		func(left int, right int,
		) bool {
			return normalizedDescriptors[left].Identity().
				Less(normalizedDescriptors[right].Identity())
		})
	return Registry{descriptorsByIdentity: descriptorsByIdentity,
		pluginTypes: pluginTypes, descriptors: normalizedDescriptors}, nil
}
func (registry Registry) Lookup(identity PluginIdentity,
) (Descriptor, bool) {
	normalizedIdentity, err := NewPluginIdentity(identity.Type(),
		identity.Version())
	if err != nil {
		return Descriptor{}, false
	}
	descriptor, exists := registry.descriptorsByIdentity[normalizedIdentity]
	if !exists {
		return Descriptor{}, false
	}
	return descriptor.clone(), true
}
func (registry Registry) LookupByTypeAndVersion(pluginType workflow.PluginType, version workflow.PluginVersion,
) (Descriptor, bool) {
	identity, err := NewPluginIdentity(pluginType,
		version)
	if err != nil {
		return Descriptor{}, false
	}
	return registry.Lookup(identity)
}
func (registry Registry) HasType(pluginType workflow.PluginType) bool {
	normalizedType, err := workflow.NewPluginType(pluginType.String())
	if err != nil {
		return false
	}
	_, exists := registry.pluginTypes[normalizedType]
	return exists
}
func (registry Registry) List() []Descriptor {
	return cloneDescriptors(registry.descriptors)
}
func (registry Registry) Len() int { return len(registry.descriptors) }
func (registry Registry) IsEmpty() bool {
	return registry.Len() == 0
}
func cloneDescriptors(
	descriptors []Descriptor) []Descriptor {
	if len(descriptors) == 0 {
		return nil
	}
	cloned := make([]Descriptor, len(descriptors))
	for index, descriptor := range descriptors {
		cloned[index] = descriptor.clone()
	}
	return cloned
}
