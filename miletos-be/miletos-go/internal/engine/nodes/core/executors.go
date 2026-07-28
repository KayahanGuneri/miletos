package core

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/workflow"
)

const (
	failureCodeInvalidConfiguration = "INVALID_NODE_CONFIGURATION"
	failureCodeInvalidInput         = "INVALID_NODE_INPUT"
)

type staticInputExecutor struct{ maximumInlinePayloadBytes int }

func (executor staticInputExecutor) Execute(nodeContext *runtime.NodeExecutionContext,
	input runtime.NodeInput, configuration workflow.JSONObject) (runtime.NodeResult, error) {
	if err := validateExecutionContext(nodeContext); err != nil {
		return runtime.NodeResult{}, err
	}
	if !input.IsValid() {
		return newValidationFailureResult(failureCodeInvalidInput,
			"Node input is invalid", map[string]string{"reason": "input contract validation failed"})
	}
	if !input.IsEmpty() {
		return newValidationFailureResult(
			failureCodeInvalidInput, "Static input node must not receive input payloads", map[string]string{
				"pluginType": StaticInputPluginType.String()})
	}
	issues := validateStaticInputConfiguration(
		configuration)
	if len(issues) > 0 {
		return newConfigurationFailureResult(StaticInputPluginType, issues)
	}
	decoded, decodeIssue := decodeStrictConfiguration[staticInputConfiguration](configuration)
	if decodeIssue != nil {
		return newConfigurationFailureResult(
			StaticInputPluginType, []plugin.ConfigurationIssue{*decodeIssue})
	}
	payload, err := runtime.NewInlinePayload(runtime.ContentTypeApplicationJSON,
		bytes.Clone(bytes.TrimSpace(decoded.Value)),
		nil, executor.maximumInlinePayloadBytes)
	if err != nil {
		return newValidationFailureResult(failureCodeInvalidConfiguration,
			"Static input payload exceeds the runtime inline payload limit", map[string]string{"pluginType": StaticInputPluginType.String(),
				"field": "value"})
	}
	return runtime.NewNodeSuccessResult(
		map[string][]runtime.Payload{OutputPortName: {payload}}, runtime.ContextChanges{},
	)
}

type passThroughExecutor struct{}

func (passThroughExecutor) Execute(
	nodeContext *runtime.NodeExecutionContext, input runtime.NodeInput, configuration workflow.JSONObject,
) (runtime.NodeResult, error) {
	if err := validateExecutionContext(nodeContext); err != nil {
		return runtime.NodeResult{}, err
	}
	issues := validatePassThroughConfiguration(configuration)
	if len(issues) > 0 {
		return newConfigurationFailureResult(
			PassThroughPluginType, issues)
	}
	payload, failureResult, failed, err :=
		singleInputPayload(input, PassThroughPluginType)
	if err != nil {
		return runtime.NodeResult{}, err
	}
	if failed {
		return failureResult, nil
	}
	return newForwardingResult(payload)
}

type delayExecutor struct{ waiter Waiter }

func (executor delayExecutor) Execute(nodeContext *runtime.NodeExecutionContext,
	input runtime.NodeInput, configuration workflow.JSONObject) (runtime.NodeResult, error) {
	if err := validateExecutionContext(nodeContext); err != nil {
		return runtime.NodeResult{}, err
	}
	issues := validateDelayConfiguration(configuration)
	if len(issues) > 0 {
		return newConfigurationFailureResult(DelayPluginType,
			issues)
	}
	payload, failureResult, failed, err := singleInputPayload(
		input, DelayPluginType)
	if err != nil {
		return runtime.NodeResult{}, err
	}
	if failed {
		return failureResult, nil
	}
	decoded, decodeIssue :=
		decodeStrictConfiguration[delayConfiguration](configuration)
	if decodeIssue != nil {
		return newConfigurationFailureResult(DelayPluginType,
			[]plugin.ConfigurationIssue{*decodeIssue},
		)
	}
	duration, err := time.ParseDuration(strings.TrimSpace(decoded.Delay))
	if err != nil {
		return newConfigurationFailureResult(DelayPluginType,
			[]plugin.ConfigurationIssue{{Field: "delay",
				Reason: "must be a valid duration"}},
		)
	}
	if executor.waiter == nil {
		return runtime.NodeResult{}, fmt.Errorf("delay waiter must not be nil")
	}
	if err := executor.waiter.Wait(nodeContext.Context(), duration); err != nil {
		return runtime.NodeResult{}, err
	}
	if err := validateExecutionContext(nodeContext); err != nil {
		return runtime.NodeResult{}, err
	}
	return newForwardingResult(payload)
}

type terminalExecutor struct{}

func (terminalExecutor) Execute(nodeContext *runtime.NodeExecutionContext, input runtime.NodeInput,
	configuration workflow.JSONObject) (runtime.NodeResult, error) {
	if err := validateExecutionContext(
		nodeContext); err != nil {
		return runtime.NodeResult{}, err
	}
	issues := validateTerminalConfiguration(
		configuration)
	if len(issues) > 0 {
		return newConfigurationFailureResult(TerminalPluginType, issues)
	}
	payload, failureResult, failed, err := singleInputPayload(input,
		TerminalPluginType)
	if err != nil {
		return runtime.NodeResult{}, err
	}
	if failed {
		return failureResult, nil
	}
	return runtime.NewTerminalNodeSuccessResult(payload,
		runtime.ContextChanges{})
}
func validateExecutionContext(nodeContext *runtime.NodeExecutionContext,
) error {
	if nodeContext == nil {
		return fmt.Errorf(
			"node execution context must not be nil")
	}
	ctx := nodeContext.Context()
	if ctx == nil {
		return fmt.Errorf("node execution context must contain a parent context")
	}
	return ctx.Err()
}
func singleInputPayload(
	input runtime.NodeInput, pluginType workflow.PluginType) (
	runtime.Payload, runtime.NodeResult, bool,
	error) {
	if !input.IsValid() {
		failureResult, err := newValidationFailureResult(failureCodeInvalidInput, "Node input is invalid",
			map[string]string{"pluginType": pluginType.String(), "reason": "input contract validation failed"})
		return runtime.Payload{}, failureResult, true,
			err
	}
	if input.PortCount() != 1 {
		failureResult, err := newValidationFailureResult(failureCodeInvalidInput,
			"Node requires exactly one populated input port", map[string]string{"pluginType": pluginType.String()})
		return runtime.Payload{}, failureResult, true,
			err
	}
	payloads, exists, err := input.Payloads(InputPortName)
	if err != nil {
		return runtime.Payload{}, runtime.NodeResult{},
			false, err
	}
	if !exists {
		failureResult, failureErr :=
			newValidationFailureResult(failureCodeInvalidInput, "Required input port is missing",
				map[string]string{"pluginType": pluginType.String(), "inputPort": InputPortName})
		return runtime.Payload{}, failureResult, true,
			failureErr
	}
	if len(payloads) != 1 || input.TotalPayloadCount() != 1 {
		failureResult, failureErr :=
			newValidationFailureResult(failureCodeInvalidInput, "Node requires exactly one input payload",
				map[string]string{"pluginType": pluginType.String(), "inputPort": InputPortName})
		return runtime.Payload{}, failureResult, true,
			failureErr
	}
	return payloads[0], runtime.NodeResult{}, false,
		nil
}
func newForwardingResult(payload runtime.Payload) (runtime.NodeResult, error) {
	return runtime.NewNodeSuccessResult(map[string][]runtime.Payload{OutputPortName: {
		payload}},
		runtime.ContextChanges{})
}
func newConfigurationFailureResult(pluginType workflow.PluginType,
	issues []plugin.ConfigurationIssue) (runtime.NodeResult, error) {
	details := map[string]string{
		"pluginType": pluginType.String()}
	if len(issues) > 0 {
		details["field"] = issues[0].Field
		details["reason"] = issues[0].Reason
	}
	return newValidationFailureResult(
		failureCodeInvalidConfiguration, "Node configuration is invalid", details,
	)
}
func newValidationFailureResult(code string, message string,
	details map[string]string) (runtime.NodeResult, error) {
	failure, err := runtime.NewRuntimeFailure(
		runtime.FailureCategoryValidation, code, message,
		false, details)
	if err != nil {
		return runtime.NodeResult{}, fmt.Errorf("create node validation failure: %w",
			err)
	}
	result, err := runtime.NewNodeFailureResult(failure)
	if err != nil {
		return runtime.NodeResult{}, fmt.Errorf(
			"create failed node result: %w", err)
	}
	return result, nil
}
