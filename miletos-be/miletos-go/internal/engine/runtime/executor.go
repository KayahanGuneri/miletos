package runtime

import (
	"fmt"
	"sort"

	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/features/workflow"
)

type DuplicateExecutorError struct {
	Identity plugin.PluginIdentity
}

func (e *DuplicateExecutorError) Error() string {
	if e == nil || !e.Identity.IsValid() {
		return "executor registry contains a duplicate executor"
	}
	return fmt.Sprintf("executor registry contains duplicate executor %s",
		e.Identity.String())
}

type ExecutorRegistration struct {
	identity plugin.PluginIdentity
	executor NodeExecutor
}

func NewExecutorRegistration(identity plugin.PluginIdentity, executor NodeExecutor,
) (ExecutorRegistration, error) {
	normalizedIdentity, err := plugin.NewPluginIdentity(identity.Type(),
		identity.Version())
	if err != nil {
		return ExecutorRegistration{}, newValidationError("executor.identity", err.Error())
	}
	if executor == nil {
		return ExecutorRegistration{}, newValidationError("executor",
			"must not be nil")
	}
	return ExecutorRegistration{identity: normalizedIdentity,
		executor: executor}, nil
}
func (registration ExecutorRegistration) Identity() plugin.PluginIdentity {
	return registration.identity
}
func (registration ExecutorRegistration) Executor() NodeExecutor {
	return registration.executor
}

type ExecutorRegistry struct {
	executorsByIdentity map[plugin.PluginIdentity]NodeExecutor
	identities          []plugin.PluginIdentity
}

func NewExecutorRegistry(registrations []ExecutorRegistration) (ExecutorRegistry, error) {
	executorsByIdentity := make(map[plugin.PluginIdentity]NodeExecutor, len(registrations))
	identities := make(
		[]plugin.PluginIdentity, 0, len(registrations),
	)
	for index, registration := range registrations {
		normalizedRegistration, err := NewExecutorRegistration(registration.identity, registration.executor)
		if err != nil {
			return ExecutorRegistry{}, newValidationError(fmt.Sprintf("registrations[%d]",
				index), err.Error(),
			)
		}
		identity := normalizedRegistration.Identity()
		if _, exists := executorsByIdentity[identity]; exists {
			return ExecutorRegistry{}, &DuplicateExecutorError{Identity: identity}
		}
		executorsByIdentity[identity] =
			normalizedRegistration.Executor()
		identities = append(
			identities, identity)
	}
	sort.SliceStable(
		identities, func(left int, right int) bool {
			return identities[left].Less(
				identities[right])
		},
	)
	return ExecutorRegistry{
		executorsByIdentity: executorsByIdentity, identities: identities}, nil
}
func (registry ExecutorRegistry) Lookup(
	identity plugin.PluginIdentity) (NodeExecutor, bool) {
	normalizedIdentity, err := plugin.NewPluginIdentity(
		identity.Type(), identity.Version())
	if err != nil {
		return nil, false
	}
	executor, exists := registry.executorsByIdentity[normalizedIdentity]
	return executor, exists
}
func (registry ExecutorRegistry) LookupByTypeAndVersion(pluginType workflow.PluginType,
	version workflow.PluginVersion) (NodeExecutor, bool) {
	identity, err := plugin.NewPluginIdentity(
		pluginType, version)
	if err != nil {
		return nil, false
	}
	return registry.Lookup(identity)
}
func (registry ExecutorRegistry) Identities() []plugin.PluginIdentity {
	if len(registry.identities) == 0 {
		return nil
	}
	return append([]plugin.PluginIdentity(nil), registry.identities...,
	)
}
func (registry ExecutorRegistry) Len() int { return len(registry.executorsByIdentity) }
func (registry ExecutorRegistry) IsEmpty() bool {
	return registry.Len() == 0
}

type NodeExecutor interface {
	Execute(nodeContext *NodeExecutionContext, input NodeInput,
		configuration workflow.JSONObject) (NodeResult, error)
}

type NodeExecutorFunc func(nodeContext *NodeExecutionContext,
	input NodeInput, configuration workflow.JSONObject) (NodeResult, error)

func (executor NodeExecutorFunc) Execute(nodeContext *NodeExecutionContext,
	input NodeInput, configuration workflow.JSONObject) (NodeResult, error) {
	if executor == nil {
		return NodeResult{}, newValidationError("nodeExecutor",
			"must not be nil")
	}
	return executor(nodeContext,
		input, configuration)
}
