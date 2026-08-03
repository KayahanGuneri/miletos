package plugin

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type NodeExecutionContext struct {
	CompanyID     string
	WorkflowID    string
	ExecutionID   string
	NodeID        string
	CorrelationID string
	Source        string
}

type NodeHandler func(
	context.Context,
	NodeExecutionContext,
	map[string]any,
	any,
) (any, error)

type NodeConfigurationValidator func(map[string]any) error

type NodeRegistration struct {
	Definition              NodeDefinition
	Handler                 NodeHandler
	Validator               NodeConfigurationValidator
	AllowedExecutionSources []string
}

func CanReceiveEntryInput(definition NodeDefinition) bool {
	if len(definition.InputPorts) == 0 {
		return false
	}
	constraint := definition.InputEdgeConstraint
	if constraint.Minimum != 0 {
		return false
	}
	if constraint.Maximum == nil {
		return true
	}
	return *constraint.Maximum > 0
}

type NodeRegistry struct {
	mutex         sync.RWMutex
	registrations map[string]NodeRegistration
}

func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		registrations: make(map[string]NodeRegistration),
	}
}

func (registry *NodeRegistry) DefineNode(name string, handler NodeHandler) error {
	return registry.register(NodeRegistration{
		Definition: NodeDefinition{
			Type:        strings.TrimSpace(name),
			Version:     "v1",
			DisplayName: strings.TrimSpace(name),
			InputMode:   NodeInputSingle,
			InputPorts:  []Port{{Name: "input", DisplayName: "Input"}},
			OutputPorts: []Port{{Name: "output", DisplayName: "Output"}},
			InputEdgeConstraint: EdgeConstraint{
				Minimum: 0,
			},
			OutputEdgeConstraint: EdgeConstraint{
				Minimum: 0,
			},
		},
		Handler: handler,
	})
}

func (registry *NodeRegistry) Register(
	definition NodeDefinition,
	handler NodeHandler,
	validator NodeConfigurationValidator,
) error {
	return registry.register(NodeRegistration{
		Definition: definition,
		Handler:    handler,
		Validator:  validator,
	})
}

func (registry *NodeRegistry) RegisterNode(registration NodeRegistration) error {
	return registry.register(registration)
}

func (registry *NodeRegistry) Get(name string) (NodeHandler, bool) {
	return registry.GetVersion(name, "v1")
}

func (registry *NodeRegistry) GetVersion(name, version string) (NodeHandler, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	registration, exists := registry.registrations[definitionKey(name, version)]
	return registration.Handler, exists
}

func (registry *NodeRegistry) Definition(
	nodeType string,
	version string,
) (NodeDefinition, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	registration, exists := registry.registrations[definitionKey(nodeType, version)]
	return cloneDefinition(registration.Definition), exists
}

func (registry *NodeRegistry) CanStartFrom(
	nodeType string,
	version string,
	source string,
) bool {
	registry.mutex.RLock()
	registration, exists := registry.registrations[definitionKey(nodeType, version)]
	registry.mutex.RUnlock()
	if !exists {
		return false
	}
	return allowsExecutionSource(registration, source)
}

func allowsExecutionSource(registration NodeRegistration, source string) bool {
	if len(registration.AllowedExecutionSources) == 0 {
		return true
	}
	for _, allowed := range registration.AllowedExecutionSources {
		if allowed == source {
			return true
		}
	}
	return false
}

func declaresExecutionSource(registration NodeRegistration, source string) bool {
	for _, allowed := range registration.AllowedExecutionSources {
		if allowed == source {
			return true
		}
	}
	return false
}

func (registry *NodeRegistry) DeclaresExecutionSource(
	nodeType string,
	version string,
	source string,
) bool {
	registry.mutex.RLock()
	registration, exists := registry.registrations[definitionKey(nodeType, version)]
	registry.mutex.RUnlock()
	return exists && declaresExecutionSource(registration, source)
}

func (registry *NodeRegistry) HasType(nodeType string) bool {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	prefix := strings.TrimSpace(nodeType) + "\x00"
	for key := range registry.registrations {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func (registry *NodeRegistry) ValidateConfiguration(
	nodeType string,
	version string,
	configuration map[string]any,
) error {
	registry.mutex.RLock()
	validator := registry.registrations[definitionKey(nodeType, version)].Validator
	registry.mutex.RUnlock()
	if validator == nil {
		return nil
	}
	return validator(configuration)
}

func (registry *NodeRegistry) Registrations() []NodeRegistration {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	result := make([]NodeRegistration, 0, len(registry.registrations))
	for _, registration := range registry.registrations {
		result = append(result, cloneRegistration(registration))
	}
	sort.Slice(result, func(left, right int) bool {
		leftDefinition := result[left].Definition
		rightDefinition := result[right].Definition
		if leftDefinition.Type == rightDefinition.Type {
			return leftDefinition.Version < rightDefinition.Version
		}
		return leftDefinition.Type < rightDefinition.Type
	})
	return result
}

func (registry *NodeRegistry) register(registration NodeRegistration) error {
	definition := registration.Definition
	name := strings.TrimSpace(definition.Type)
	if name == "" {
		return fmt.Errorf("node name must not be empty")
	}
	if registration.Handler == nil {
		return fmt.Errorf("handler for node %q must not be nil", name)
	}
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	definition.Type = name
	if definition.InputMode == "" {
		definition.InputMode = NodeInputSingle
	}
	definition.Version = strings.TrimSpace(definition.Version)
	if definition.Version == "" {
		definition.Version = "v1"
	}
	key := definitionKey(name, definition.Version)
	if _, exists := registry.registrations[key]; exists {
		return fmt.Errorf("node %q version %q is already defined", name, definition.Version)
	}
	registration.Definition = definition
	registry.registrations[key] = cloneRegistration(registration)
	return nil
}

func cloneRegistration(registration NodeRegistration) NodeRegistration {
	registration.Definition = cloneDefinition(registration.Definition)
	registration.AllowedExecutionSources = append(
		[]string(nil), registration.AllowedExecutionSources...,
	)
	return registration
}

func cloneDefinition(definition NodeDefinition) NodeDefinition {
	definition.InputPorts = append([]Port(nil), definition.InputPorts...)
	definition.OutputPorts = append([]Port(nil), definition.OutputPorts...)
	definition.InputEdgeConstraint.Maximum = cloneMaximum(
		definition.InputEdgeConstraint.Maximum,
	)
	definition.OutputEdgeConstraint.Maximum = cloneMaximum(
		definition.OutputEdgeConstraint.Maximum,
	)
	return definition
}

func cloneMaximum(maximum *uint) *uint {
	if maximum == nil {
		return nil
	}
	cloned := *maximum
	return &cloned
}

func definitionKey(nodeType, version string) string {
	normalizedVersion := strings.TrimSpace(version)
	if normalizedVersion == "" {
		normalizedVersion = "v1"
	}
	return strings.TrimSpace(nodeType) + "\x00" + normalizedVersion
}
