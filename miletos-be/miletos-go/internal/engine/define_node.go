package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"miletos-go/internal/model"
)

type NodeHandler func(context.Context, map[string]any, any) (any, error)

type NodeConfigurationValidator func(map[string]any) error

type NodeRegistry struct {
	mutex       sync.RWMutex
	handlers    map[string]NodeHandler
	definitions map[string]model.NodeDefinition
	validators  map[string]NodeConfigurationValidator
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		handlers:    make(map[string]NodeHandler),
		definitions: make(map[string]model.NodeDefinition),
		validators:  make(map[string]NodeConfigurationValidator),
	}
}

func (registry *NodeRegistry) DefineNode(name string, handler NodeHandler) error {
	return registry.define(model.NodeDefinition{
		Type:        strings.TrimSpace(name),
		Version:     "v1",
		DisplayName: strings.TrimSpace(name),
		InputPorts: []model.Port{{Name: "input", DisplayName: "Input"}},
		OutputPorts: []model.Port{{Name: "output", DisplayName: "Output"}},
		InputEdgeConstraint: model.EdgeConstraint{
			Minimum: 0, Unlimited: true,
		},
		OutputEdgeConstraint: model.EdgeConstraint{
			Minimum: 0, Unlimited: true,
		},
	}, handler)
}

func (registry *NodeRegistry) Get(name string) (NodeHandler, bool) {
	return registry.GetVersion(name, "v1")
}

func (registry *NodeRegistry) GetVersion(name, version string) (NodeHandler, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	handler, exists := registry.handlers[definitionKey(name, version)]
	return handler, exists
}

func (registry *NodeRegistry) IsDefined(name string) bool {
	_, exists := registry.Get(name)
	return exists
}

func (registry *NodeRegistry) Definition(
	nodeType string,
	version string,
) (model.NodeDefinition, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	definition, exists := registry.definitions[definitionKey(nodeType, version)]
	return definition, exists
}

func (registry *NodeRegistry) HasType(nodeType string) bool {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	prefix := strings.TrimSpace(nodeType) + "\x00"
	for key := range registry.definitions {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (registry *NodeRegistry) ValidateConfiguration(node model.Node) error {
	registry.mutex.RLock()
	validator := registry.validators[definitionKey(node.Type, node.Version)]
	registry.mutex.RUnlock()
	if validator == nil {
		return nil
	}
	return validator(node.Configuration)
}

func (registry *NodeRegistry) Definitions() []model.NodeDefinition {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	result := make([]model.NodeDefinition, 0, len(registry.definitions))
	for _, definition := range registry.definitions {
		result = append(result, definition)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Type == result[right].Type {
			return result[left].Version < result[right].Version
		}
		return result[left].Type < result[right].Type
	})
	return result
}

func (registry *NodeRegistry) define(definition model.NodeDefinition, handler NodeHandler) error {
	return registry.defineValidated(definition, handler, nil)
}

func (registry *NodeRegistry) defineValidated(
	definition model.NodeDefinition,
	handler NodeHandler,
	validator NodeConfigurationValidator,
) error {
	name := strings.TrimSpace(definition.Type)
	if name == "" {
		return fmt.Errorf("node name must not be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler for node %q must not be nil", name)
	}
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	definition.Type = name
	definition.Version = strings.TrimSpace(definition.Version)
	if definition.Version == "" {
		definition.Version = "v1"
	}
	key := definitionKey(name, definition.Version)
	if _, exists := registry.handlers[key]; exists {
		return fmt.Errorf("node %q version %q is already defined", name, definition.Version)
	}
	registry.handlers[key] = handler
	registry.definitions[key] = definition
	if validator != nil {
		registry.validators[key] = validator
	}
	return nil
}

func definitionKey(nodeType, version string) string {
	normalizedVersion := strings.TrimSpace(version)
	if normalizedVersion == "" {
		normalizedVersion = "v1"
	}
	return strings.TrimSpace(nodeType) + "\x00" + normalizedVersion
}
