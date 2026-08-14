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

type NodeConfigurationValidator func(map[string]any) error

type OutputRoutingMode string

const (
	OutputRoutingBroadcast OutputRoutingMode = "BROADCAST"
	OutputRoutingExplicit  OutputRoutingMode = "EXPLICIT"
)

type NodeRegistration struct {
	Definition              NodeDefinition
	Lifecycles              NodeLifecycles
	OutputRoutingMode       OutputRoutingMode
	Validator               NodeConfigurationValidator
	AllowedExecutionSources []string
	ContextProvider         string
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

func (registry *NodeRegistry) RegisterNode(registration NodeRegistration) error {
	return registry.register(registration)
}

func (registry *NodeRegistry) GetLifecycles(name string) (NodeLifecycles, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	registration, exists := registry.registrations[strings.TrimSpace(name)]
	return registration.Lifecycles, exists
}

func (registry *NodeRegistry) GetOutputRoutingMode(
	name string,
) (OutputRoutingMode, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	registration, exists := registry.registrations[strings.TrimSpace(name)]
	return registration.OutputRoutingMode, exists
}

func (registry *NodeRegistry) StartScenario(
	runtime context.Context,
	nodeType string,
	scenario ScenarioStartContext,
	configuration map[string]any,
	infrastructure Infrastructure,
) error {
	lifecycles, exists := registry.GetLifecycles(nodeType)
	if !exists {
		return fmt.Errorf(
			"%w: node %q",
			ErrNodeRegistrationNotFound, nodeType,
		)
	}
	if lifecycles.OnScenarioStart == nil {
		return fmt.Errorf(
			"%w: node %q",
			ErrScenarioStartUnavailable, nodeType,
		)
	}
	return lifecycles.OnScenarioStart(&Context{
		Runtime:       runtime,
		Scenario:      &scenario,
		Configuration: configuration,
		Infra:         infrastructure,
	})
}

func (registry *NodeRegistry) Definition(
	nodeType string,
) (NodeDefinition, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	registration, exists := registry.registrations[strings.TrimSpace(nodeType)]
	return cloneDefinition(registration.Definition), exists
}

func (registry *NodeRegistry) CanStartFrom(
	nodeType string,
	source string,
) bool {
	registry.mutex.RLock()
	registration, exists := registry.registrations[strings.TrimSpace(nodeType)]
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
	source string,
) bool {
	registry.mutex.RLock()
	registration, exists := registry.registrations[strings.TrimSpace(nodeType)]
	registry.mutex.RUnlock()
	return exists && declaresExecutionSource(registration, source)
}

func (registry *NodeRegistry) ValidateConfiguration(
	nodeType string,
	configuration map[string]any,
) error {
	registry.mutex.RLock()
	validator := registry.registrations[strings.TrimSpace(nodeType)].Validator
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
		return result[left].Definition.Type < result[right].Definition.Type
	})
	return result
}

func (registry *NodeRegistry) register(registration NodeRegistration) error {
	definition := registration.Definition
	name := strings.TrimSpace(definition.Type)
	if name == "" {
		return fmt.Errorf("node name must not be empty")
	}
	if registration.Lifecycles.OnRun == nil {
		return fmt.Errorf("on-run lifecycle for node %q must not be nil", name)
	}
	if registration.OutputRoutingMode == "" {
		registration.OutputRoutingMode = OutputRoutingBroadcast
	}
	if registration.OutputRoutingMode != OutputRoutingBroadcast &&
		registration.OutputRoutingMode != OutputRoutingExplicit {
		return fmt.Errorf("output routing mode for node %q is invalid", name)
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
	if _, exists := registry.registrations[name]; exists {
		return fmt.Errorf("node %q is already defined", name)
	}
	registration.Definition = definition
	registry.registrations[name] = cloneRegistration(registration)
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
