package plugin

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type OutputRoutingMode string

const (
	OutputRoutingBroadcast OutputRoutingMode = "BROADCAST"
	OutputRoutingExplicit  OutputRoutingMode = "EXPLICIT"
)

type NodeHandler func(ctx *Context) error
type NodeValidator func(configuration map[string]any) error

type RecordMaterializer func(
	ctx context.Context,
	companyID string,
	configuration map[string]any,
) ([]map[string]any, error)

type RecordSource struct {
	Materialize RecordMaterializer
}

type RecordSourceRelation string

const (
	RecordSourceRelationUnsupported RecordSourceRelation = "UNSUPPORTED"
	RecordSourceRelationDriving     RecordSourceRelation = "DRIVING"
	RecordSourceRelationPassive     RecordSourceRelation = "PASSIVE"
)

type RecordSourceRelationClassifier func(
	configuration map[string]any,
	sourceNodeID string,
) RecordSourceRelation

type NodeRegistration struct {
	Key                     string
	InputMode               string
	TriggerInputMode        TriggerInputMode
	InputPorts              []Port
	OutputPorts             []Port
	InputEdgeConstraint     EdgeConstraint
	OutputEdgeConstraint    EdgeConstraint
	RoutingMode             OutputRoutingMode
	Validator               NodeValidator
	AllowedExecutionSources []string
	ConnectionRestrictions  []ConnectionRestriction
	DataArrivalSource       *DataArrivalSource
	RecordSource            *RecordSource
	RecordSourceRelation    RecordSourceRelationClassifier
	ContextProvider         string
	ProvidesWorkflowResult  bool
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
	if registration.TriggerInputMode == "" {
		registration.TriggerInputMode = TriggerInputAll
	}
	if registration.TriggerInputMode != TriggerInputAll &&
		registration.TriggerInputMode != TriggerInputAvailable {
		return fmt.Errorf("trigger input mode for node %q is invalid", key)
	}
	for _, port := range append(
		append([]Port(nil), registration.InputPorts...), registration.OutputPorts...,
	) {
		if port.EdgeConstraint != nil && port.EdgeConstraint.Maximum != nil &&
			port.EdgeConstraint.Minimum > *port.EdgeConstraint.Maximum {
			return fmt.Errorf("edge constraint for node %q port %q is invalid", key, port.Name)
		}
	}
	for _, restriction := range registration.ConnectionRestrictions {
		if err := validateConnectionRestriction(key, restriction); err != nil {
			return err
		}
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

func (registry *NodeRegistry) ConnectionRestricted(
	fromType string,
	toType string,
	targetInputPort string,
) bool {
	from, fromExists := registry.Get(fromType)
	to, toExists := registry.Get(toType)
	if !fromExists || !toExists {
		return false
	}
	for _, restriction := range from.ConnectionRestrictions {
		if connectionRestrictionMatches(
			restriction,
			fromType,
			toType,
			targetInputPort,
			to.InputPorts,
		) {
			return true
		}
	}
	return false
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
	clonePortConstraints(registration.InputPorts)
	clonePortConstraints(registration.OutputPorts)
	registration.InputEdgeConstraint.Maximum = cloneMaximum(
		registration.InputEdgeConstraint.Maximum,
	)
	registration.OutputEdgeConstraint.Maximum = cloneMaximum(
		registration.OutputEdgeConstraint.Maximum,
	)
	registration.AllowedExecutionSources = append(
		[]string(nil), registration.AllowedExecutionSources...,
	)
	registration.ConnectionRestrictions = append(
		[]ConnectionRestriction(nil), registration.ConnectionRestrictions...,
	)
	if registration.DataArrivalSource != nil {
		cloned := *registration.DataArrivalSource
		registration.DataArrivalSource = &cloned
	}
	if registration.RecordSource != nil {
		cloned := *registration.RecordSource
		registration.RecordSource = &cloned
	}
	return registration
}

func clonePortConstraints(ports []Port) {
	for index := range ports {
		if ports[index].EdgeConstraint == nil {
			continue
		}
		cloned := *ports[index].EdgeConstraint
		cloned.Maximum = cloneMaximum(cloned.Maximum)
		ports[index].EdgeConstraint = &cloned
	}
}

func validateConnectionRestriction(
	registrationKey string,
	restriction ConnectionRestriction,
) error {
	if strings.TrimSpace(restriction.From) != registrationKey {
		return fmt.Errorf("connection restriction source for node %q must match its key", registrationKey)
	}
	if strings.TrimSpace(restriction.To) == "" {
		return fmt.Errorf("connection restriction target for node %q must not be empty", registrationKey)
	}
	switch restriction.Selector {
	case ConnectionRestrictionPrimary, ConnectionRestrictionNotPrimary:
		if restriction.Position != 0 {
			return fmt.Errorf("named connection restriction for node %q must not declare a position", registrationKey)
		}
	case ConnectionRestrictionPosition:
		if restriction.Position == 0 {
			return fmt.Errorf("numeric connection restriction for node %q must use a 1-based position", registrationKey)
		}
	default:
		return fmt.Errorf("connection restriction selector for node %q is invalid", registrationKey)
	}
	return nil
}

func connectionRestrictionMatches(
	restriction ConnectionRestriction,
	fromType string,
	toType string,
	targetInputPort string,
	targetInputPorts []Port,
) bool {
	if restriction.From != fromType || restriction.To != toType {
		return false
	}
	switch restriction.Selector {
	case ConnectionRestrictionPrimary:
		return len(targetInputPorts) > 0 && targetInputPorts[0].Name == targetInputPort
	case ConnectionRestrictionNotPrimary:
		if len(targetInputPorts) < 2 {
			return false
		}
		for _, port := range targetInputPorts[1:] {
			if port.Name == targetInputPort {
				return true
			}
		}
	case ConnectionRestrictionPosition:
		index := restriction.Position - 1
		return index < uint(len(targetInputPorts)) && targetInputPorts[index].Name == targetInputPort
	}
	return false
}

func cloneMaximum(maximum *uint) *uint {
	if maximum == nil {
		return nil
	}
	cloned := *maximum
	return &cloned
}
