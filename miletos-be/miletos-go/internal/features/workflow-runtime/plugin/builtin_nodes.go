package plugin

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/cronexpr"
)

func RegisterBuiltinNodes(registry *NodeRegistry) error {
	registrations := []struct {
		registration NodeRegistration
	}{
		{builtinRegistration(builtinRegistrationSpec{
			Type:        "core.static-input",
			DisplayName: "Static Input",
			Description: "Provides configured static content to a workflow",
			Input:       false,
			Output:      true,
			OnRun:       staticInput,
			Validator:   validateStaticInput,
		})},
		{builtinRegistration(builtinRegistrationSpec{
			Type:        "core.pass-through",
			DisplayName: "Pass Through",
			Description: "Forwards one incoming payload without changing it",
			Input:       true,
			Output:      true,
			OnRun:       passThrough,
		})},
		{builtinRegistration(builtinRegistrationSpec{
			Type:        "core.delay",
			DisplayName: "Delay",
			Description: "Delays forwarding one incoming payload",
			Input:       true,
			Output:      true,
			OnRun:       delay,
			Validator:   validateDelay,
		})},
		{builtinRegistration(builtinRegistrationSpec{
			Type:        "core.terminal",
			DisplayName: "Terminal",
			Description: "Receives final workflow output",
			Input:       true,
			Output:      false,
			OnRun:       terminal,
		})},
		{outputRegistration()},
		{joinRegistration()},
		{httpTriggerRegistration()},
		{cronTriggerRegistration()},
	}
	for _, builtin := range registrations {
		if err := registry.RegisterNode(builtin.registration); err != nil {
			return err
		}
	}
	return nil
}

type builtinRegistrationSpec struct {
	Type        string
	DisplayName string
	Description string
	Input       bool
	Output      bool
	RoutingMode OutputRoutingMode
	OnRun       RunHandler
	Validator   NodeConfigurationValidator
}

func builtinRegistration(spec builtinRegistrationSpec) NodeRegistration {
	return NodeRegistration{
		Definition: builtinDefinition(
			spec.Type,
			spec.DisplayName,
			spec.Description,
			spec.Input,
			spec.Output,
		),
		Lifecycles:              NodeLifecycles{OnRun: spec.OnRun},
		OutputRoutingMode:       spec.RoutingMode,
		Validator:               spec.Validator,
		AllowedExecutionSources: []string{"MANUAL_DIRECT"},
	}
}

func builtinDefinition(nodeType, displayName, description string, input, output bool) NodeDefinition {
	definition := NodeDefinition{
		Type:        nodeType,
		Version:     "v1",
		DisplayName: displayName,
		Description: description,
		InputMode:   NodeInputSingle,
		InputEdgeConstraint: EdgeConstraint{
			Minimum: boolUint(input),
		},
		OutputEdgeConstraint: EdgeConstraint{
			Minimum: 0,
		},
	}
	if input {
		maximum := uint(1)
		definition.InputPorts = []Port{{
			Name: "input", DisplayName: "Input", Description: "Receives an incoming payload",
		}}
		definition.InputEdgeConstraint.Maximum = &maximum
	} else {
		maximum := uint(0)
		definition.InputEdgeConstraint.Maximum = &maximum
	}
	if output {
		definition.OutputPorts = []Port{{
			Name: "output", DisplayName: "Output", Description: "Provides an outgoing payload",
		}}
	} else {
		maximum := uint(0)
		definition.OutputEdgeConstraint.Maximum = &maximum
	}
	return definition
}

func outputRegistration() NodeRegistration {
	registration := builtinRegistration(builtinRegistrationSpec{
		Type:        "core.output",
		DisplayName: "Output",
		Description: "Represents a generic workflow output boundary and returns its input unchanged",
		Input:       true,
		Output:      false,
		OnRun:       terminal,
	})
	registration.AllowedExecutionSources = nil
	return registration
}

func joinRegistration() NodeRegistration {
	registration := builtinRegistration(builtinRegistrationSpec{
		Type:        "core.join",
		DisplayName: "Join",
		Description: "Aggregates every predecessor payload in immutable workflow edge order",
		Input:       true,
		Output:      true,
		OnRun:       join,
	})
	definition := registration.Definition
	definition.InputMode = NodeInputMulti
	definition.InputEdgeConstraint.Minimum = 2
	definition.InputEdgeConstraint.Maximum = nil
	registration.Definition = definition
	registration.AllowedExecutionSources = nil
	return registration
}

func httpTriggerRegistration() NodeRegistration {
	registration := builtinRegistration(builtinRegistrationSpec{
		Type:        "core.http-trigger",
		DisplayName: "HTTP Trigger",
		Description: "Receives one authenticated inbound webhook request and forwards its normalized payload",
		Input:       false,
		Output:      true,
		OnRun:       httpTrigger,
		Validator:   validateHTTPTrigger,
	})
	definition := registration.Definition
	definition.OutputEdgeConstraint.Minimum = 1
	registration.Definition = definition
	registration.AllowedExecutionSources = []string{"HTTP_WEBHOOK"}
	registration.ContextProvider = "http-trigger"
	registration.Lifecycles.OnScenarioStart = httpTriggerScenarioStart
	return registration
}

func cronTriggerRegistration() NodeRegistration {
	registration := builtinRegistration(builtinRegistrationSpec{
		Type:        "core.cron-trigger",
		DisplayName: "Cron Trigger",
		Description: "Fires on a five-field cron schedule and forwards scheduler metadata",
		Input:       false,
		Output:      true,
		OnRun:       cronTrigger,
		Validator:   validateCronTrigger,
	})
	definition := registration.Definition
	definition.OutputEdgeConstraint.Minimum = 1
	registration.Definition = definition
	registration.AllowedExecutionSources = []string{"CRON"}
	registration.ContextProvider = "cron-trigger"
	return registration
}

func validateStaticInput(configuration map[string]any) error {
	value, exists := configuration["value"]
	if !exists {
		return fmt.Errorf("configuration.value is required")
	}
	if _, err := json.Marshal(value); err != nil {
		return fmt.Errorf("configuration.value must be JSON-compatible")
	}
	return nil
}

func validateDelay(configuration map[string]any) error {
	rawDelay, ok := configuration["delay"].(string)
	if !ok || strings.TrimSpace(rawDelay) == "" {
		return fmt.Errorf("configuration.delay must be a nonblank duration string")
	}
	wait, err := time.ParseDuration(rawDelay)
	if err != nil || wait <= 0 || wait > 24*time.Hour {
		return fmt.Errorf("configuration.delay must be greater than zero and no greater than 24h")
	}
	return nil
}

func validateHTTPTrigger(configuration map[string]any) error {
	method, ok := configuration["method"].(string)
	if !ok {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "HTTP_TRIGGER_METHOD_INVALID",
			Message:  "configuration.method must be a supported HTTP method",
		}
	}
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return nil
	default:
		return &NodeError{
			Category: "VALIDATION",
			Code:     "HTTP_TRIGGER_METHOD_INVALID",
			Message:  "configuration.method must be GET, POST, PUT, PATCH, or DELETE",
		}
	}
}

func validateCronTrigger(configuration map[string]any) error {
	expression, ok := configuration["expression"].(string)
	if !ok || strings.TrimSpace(expression) == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "CRON_TRIGGER_EXPRESSION_REQUIRED",
			Message:  "configuration.expression is required",
		}
	}
	timezone, _ := configuration["timezone"].(string)
	if _, err := cronexpr.Parse(expression, timezone); err != nil {
		code, ok := cronexpr.ValidationCode(err)
		if !ok {
			return &NodeError{
				Category: "INTERNAL",
				Code:     "CRON_TRIGGER_UNEXPECTED",
				Message:  "Cron trigger configuration could not be validated.",
			}
		}
		return &NodeError{
			Category: "VALIDATION",
			Code:     "CRON_TRIGGER_" + code,
			Message:  "Cron trigger configuration is invalid.",
		}
	}
	return nil
}

func boolUint(value bool) uint {
	if value {
		return 1
	}
	return 0
}

func staticInput(nodeContext *Context) (any, error) {
	parameters := nodeContext.Configuration
	input := nodeContext.Payload
	if input != nil {
		if values, ok := input.(map[string]any); !ok || len(values) > 0 {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     "STATIC_INPUT_UNEXPECTED_INPUT",
				Message:  "Static input does not accept runtime input.",
			}
		}
	}
	value, exists := parameters["value"]
	if !exists {
		return nil, fmt.Errorf("static input configuration requires value")
	}
	return value, nil
}

func join(nodeContext *Context) (any, error) {
	input := nodeContext.Payload
	payload, ok := input.(map[string]any)
	if !ok {
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     "JOIN_INPUT_INVALID",
			Message:  "Join requires a deterministic multi-input payload.",
		}
	}
	inputs, ok := payload["inputs"].([]any)
	if !ok || len(inputs) < 2 {
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     "JOIN_INPUT_INVALID",
			Message:  "Join requires at least two predecessor inputs.",
		}
	}
	return payload, nil
}

func httpTrigger(nodeContext *Context) (any, error) {
	input := nodeContext.Payload
	payload, ok := input.(map[string]any)
	if !ok {
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     "HTTP_TRIGGER_INPUT_INVALID",
			Message:  "HTTP trigger input is missing or invalid.",
		}
	}
	for _, field := range []string{"method", "path", "requestId", "correlationId"} {
		if value, ok := payload[field].(string); !ok || strings.TrimSpace(value) == "" {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     "HTTP_TRIGGER_INPUT_INVALID",
				Message:  "HTTP trigger input is missing required request metadata.",
			}
		}
	}
	return payload, nil
}

func cronTrigger(nodeContext *Context) (any, error) {
	input := nodeContext.Payload
	payload, ok := input.(map[string]any)
	if !ok {
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     "CRON_TRIGGER_INPUT_INVALID",
			Message:  "Cron trigger input is missing or invalid.",
		}
	}
	for _, field := range []string{"triggerId", "cronExpression", "timezone", "scheduledAt", "firedAt"} {
		if value, ok := payload[field].(string); !ok || strings.TrimSpace(value) == "" {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     "CRON_TRIGGER_INPUT_INVALID",
				Message:  "Cron trigger input is missing required scheduler metadata.",
			}
		}
	}
	return payload, nil
}

func passThrough(nodeContext *Context) (any, error) {
	input := nodeContext.Payload
	if input == nil {
		return nil, fmt.Errorf("pass-through node requires input")
	}
	return input, nil
}

func delay(nodeContext *Context) (any, error) {
	ctx := nodeContext.Runtime
	parameters := nodeContext.Configuration
	input := nodeContext.Payload
	rawDelay, ok := parameters["delay"].(string)
	if !ok || strings.TrimSpace(rawDelay) == "" {
		return nil, fmt.Errorf("delay node configuration requires delay")
	}
	wait, err := time.ParseDuration(rawDelay)
	if err != nil || wait <= 0 || wait > 24*time.Hour {
		return nil, fmt.Errorf("delay must be a positive duration no greater than 24h")
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return input, nil
	}
}

func terminal(nodeContext *Context) (any, error) {
	input := nodeContext.Payload
	if input == nil {
		return nil, fmt.Errorf("terminal node requires input")
	}
	return input, nil
}

func httpTriggerScenarioStart(nodeContext *Context) error {
	if err := validateHTTPTrigger(nodeContext.Configuration); err != nil {
		return err
	}
	method := strings.ToUpper(strings.TrimSpace(
		nodeContext.Configuration["method"].(string),
	))
	return nodeContext.Infra.HTTP.CreateTrigger(
		nodeContext.Runtime,
		HTTPScenarioStartRequest{
			Scenario: *nodeContext.Scenario,
			Method:   method,
		},
	)
}
