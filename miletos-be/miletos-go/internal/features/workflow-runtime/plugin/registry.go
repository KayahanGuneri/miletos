package plugin

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type NodeConfigurationValidator func(map[string]any) error

type OutputRoutingMode string

const (
	OutputRoutingBroadcast OutputRoutingMode = "BROADCAST"
	OutputRoutingExplicit  OutputRoutingMode = "EXPLICIT"
)

type NodeHandler func(ctx *Context) error

type NodeRegistration struct {
	Key                     string
	DisplayName             string
	Description             string
	InputMode               string
	InputPorts              []Port
	OutputPorts             []Port
	InputEdgeConstraint     EdgeConstraint
	OutputEdgeConstraint    EdgeConstraint
	RoutingMode             OutputRoutingMode
	Validator               NodeConfigurationValidator
	AllowedExecutionSources []string
	ContextProvider         string
	Handler                 NodeHandler
}

func CanReceiveEntryInput(registration NodeRegistration) bool {
	if len(registration.InputPorts) == 0 {
		return false
	}
	constraint := registration.InputEdgeConstraint
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
	return &NodeRegistry{registrations: make(map[string]NodeRegistration)}
}

func (registry *NodeRegistry) RegisterNode(registration NodeRegistration) error {
	key := strings.TrimSpace(registration.Key)
	if key == "" {
		return fmt.Errorf("node key must not be empty")
	}
	if registration.Handler == nil {
		return fmt.Errorf("handler for node %q must not be nil", key)
	}
	if registration.RoutingMode == "" {
		registration.RoutingMode = OutputRoutingBroadcast
	}
	if registration.RoutingMode != OutputRoutingBroadcast &&
		registration.RoutingMode != OutputRoutingExplicit {
		return fmt.Errorf("output routing mode for node %q is invalid", key)
	}
	if registration.InputMode == "" {
		registration.InputMode = NodeInputSingle
	}
	registration.Key = key
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	if _, exists := registry.registrations[key]; exists {
		return fmt.Errorf("node %q is already registered", key)
	}
	registry.registrations[key] = cloneRegistration(registration)
	return nil
}

func (registry *NodeRegistry) Get(nodeType string) (NodeRegistration, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	registration, exists := registry.registrations[strings.TrimSpace(nodeType)]
	return cloneRegistration(registration), exists
}

func (registry *NodeRegistry) Has(nodeType string) bool {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	_, exists := registry.registrations[strings.TrimSpace(nodeType)]
	return exists
}

func (registry *NodeRegistry) CanStartFrom(nodeType string, source string) bool {
	registration, exists := registry.Get(nodeType)
	return exists && allowsExecutionSource(registration, source)
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

func (registry *NodeRegistry) DeclaresExecutionSource(nodeType string, source string) bool {
	registration, exists := registry.Get(nodeType)
	return exists && declaresExecutionSource(registration, source)
}

func (registry *NodeRegistry) ValidateConfiguration(
	nodeType string,
	configuration map[string]any,
) error {
	registration, exists := registry.Get(nodeType)
	if !exists {
		return fmt.Errorf("%w: node %q", ErrNodeRegistrationNotFound, nodeType)
	}
	if registration.Validator == nil {
		return nil
	}
	return registration.Validator(configuration)
}

func (registry *NodeRegistry) Registrations() []NodeRegistration {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	result := make([]NodeRegistration, 0, len(registry.registrations))
	for _, registration := range registry.registrations {
		result = append(result, cloneRegistration(registration))
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Key < result[right].Key
	})
	return result
}

func cloneRegistration(registration NodeRegistration) NodeRegistration {
	registration.InputPorts = append([]Port(nil), registration.InputPorts...)
	registration.OutputPorts = append([]Port(nil), registration.OutputPorts...)
	registration.InputEdgeConstraint.Maximum = cloneMaximum(
		registration.InputEdgeConstraint.Maximum,
	)
	registration.OutputEdgeConstraint.Maximum = cloneMaximum(
		registration.OutputEdgeConstraint.Maximum,
	)
	registration.AllowedExecutionSources = append(
		[]string(nil), registration.AllowedExecutionSources...,
	)
	return registration
}

func cloneMaximum(maximum *uint) *uint {
	if maximum == nil {
		return nil
	}
	cloned := *maximum
	return &cloned
}
