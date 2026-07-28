package engine

import (
	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/execution"
	sharedclock "miletos-go/internal/shared/clock"
)

type EngineDependencies struct {
	pluginRegistry    plugin.Registry
	executorRegistry  runtime.ExecutorRegistry
	runtimeLimits     runtime.RuntimeLimits
	clock             sharedclock.Clock
	lifecycleRecorder LifecycleRecorder
	retryWaiter       RetryWaiter
	retryPolicy       execution.RetryPolicy
	hasRetryPolicy    bool
	initialized       bool
}

func NewEngineDependencies(pluginRegistry plugin.Registry, executorRegistry runtime.ExecutorRegistry,
	runtimeLimits runtime.RuntimeLimits, clock sharedclock.Clock) (EngineDependencies, error) {
	if !runtimeLimits.IsValid() {
		return EngineDependencies{}, newValidationError("runtimeLimits",
			"must be valid")
	}
	if clock == nil {
		return EngineDependencies{}, newValidationError(
			"clock", "must not be nil")
	}
	return EngineDependencies{
		pluginRegistry: pluginRegistry, executorRegistry: executorRegistry, runtimeLimits: runtimeLimits,
		clock: clock, lifecycleRecorder: NoopLifecycleRecorder{},
		retryWaiter: SystemRetryWaiter{}, initialized: true,
	}, nil
}
func (dependencies EngineDependencies) PluginRegistry() plugin.Registry {
	return dependencies.pluginRegistry
}
func (dependencies EngineDependencies) ExecutorRegistry() runtime.ExecutorRegistry {
	return dependencies.executorRegistry
}
func (dependencies EngineDependencies) RuntimeLimits() runtime.RuntimeLimits {
	return dependencies.runtimeLimits
}
func (dependencies EngineDependencies) Clock() sharedclock.Clock {
	return dependencies.clock
}
func (dependencies EngineDependencies) LifecycleRecorder() LifecycleRecorder {
	return dependencies.lifecycleRecorder
}
func (dependencies EngineDependencies) RetryWaiter() RetryWaiter {
	return dependencies.retryWaiter
}
func (dependencies EngineDependencies) RetryPolicy() (execution.RetryPolicy, bool) {
	if !dependencies.hasRetryPolicy {
		return execution.RetryPolicy{}, false
	}
	return dependencies.retryPolicy, true
}
func (dependencies EngineDependencies) WithLifecycleRecorder(
	recorder LifecycleRecorder) (EngineDependencies, error) {
	if !dependencies.IsValid() {
		return EngineDependencies{}, newValidationError("dependencies", "must be valid")
	}
	if recorder == nil {
		return EngineDependencies{}, newValidationError("lifecycleRecorder",
			"must not be nil")
	}
	updated := dependencies
	updated.lifecycleRecorder = recorder
	return updated, nil
}
func (dependencies EngineDependencies) WithRetryWaiter(
	waiter RetryWaiter,
) (EngineDependencies, error) {
	if !dependencies.IsValid() {
		return EngineDependencies{}, newValidationError("dependencies", "must be valid")
	}
	if waiter == nil {
		return EngineDependencies{}, newValidationError(
			"retryWaiter", "must not be nil")
	}
	updated := dependencies
	updated.retryWaiter = waiter
	return updated, nil
}
func (dependencies EngineDependencies) WithRetryPolicy(
	policy execution.RetryPolicy,
) (EngineDependencies, error) {
	if !dependencies.IsValid() {
		return EngineDependencies{}, newValidationError("dependencies", "must be valid")
	}
	if !policy.IsValid() {
		return EngineDependencies{}, newValidationError(
			"retryPolicy", "must be valid")
	}
	updated := dependencies
	updated.retryPolicy = policy
	updated.hasRetryPolicy = true
	return updated, nil
}
func (dependencies EngineDependencies,
) IsValid() bool {
	return dependencies.initialized && dependencies.runtimeLimits.IsValid() &&
		dependencies.clock != nil &&
		dependencies.lifecycleRecorder != nil &&
		dependencies.retryWaiter != nil &&
		(!dependencies.hasRetryPolicy || dependencies.retryPolicy.IsValid())
}
