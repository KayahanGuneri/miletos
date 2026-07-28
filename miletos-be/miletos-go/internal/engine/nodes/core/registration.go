package core

import (
	"fmt"

	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/workflow"
)

var _ runtime.NodeExecutor = staticInputExecutor{}
var _ runtime.NodeExecutor = passThroughExecutor{}
var _ runtime.NodeExecutor = delayExecutor{}
var _ runtime.NodeExecutor = terminalExecutor{}

func DefaultExecutorRegistrations(limits runtime.RuntimeLimits,
) ([]runtime.ExecutorRegistration, error) {
	return ExecutorRegistrations(limits,
		TimerWaiter{})
}
func ExecutorRegistrations(limits runtime.RuntimeLimits,
	waiter Waiter) ([]runtime.ExecutorRegistration, error) {
	if !limits.IsValid() {
		return nil, fmt.Errorf("runtime limits must be valid")
	}
	if waiter == nil {
		return nil, fmt.Errorf("delay waiter must not be nil")
	}
	specifications := []struct {
		pluginType workflow.PluginType
		executor   runtime.NodeExecutor
	}{
		{pluginType: DelayPluginType, executor: delayExecutor{waiter: waiter}},
		{pluginType: PassThroughPluginType, executor: passThroughExecutor{}},
		{pluginType: StaticInputPluginType, executor: staticInputExecutor{
			maximumInlinePayloadBytes: limits.MaximumInlinePayloadBytes(),
		}},
		{pluginType: TerminalPluginType, executor: terminalExecutor{}},
	}
	registrations := make([]runtime.ExecutorRegistration,
		0, len(specifications))
	for _, specification := range specifications {
		identity, err := plugin.NewPluginIdentity(
			specification.pluginType, CorePluginVersion)
		if err != nil {
			return nil, fmt.Errorf("create core executor identity: %w",
				err)
		}
		registration, err := runtime.NewExecutorRegistration(
			identity, specification.executor)
		if err != nil {
			return nil, fmt.Errorf("create core executor registration for %s: %w",
				identity.String(), err)
		}
		registrations = append(
			registrations, registration)
	}
	return registrations, nil
}
