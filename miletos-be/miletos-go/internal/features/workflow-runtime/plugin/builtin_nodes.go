package plugin

import (
	"context"
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
		{builtinRegistration("core.static-input", "Static Input", "Provides configured static content to a workflow", false, true, staticInput, validateStaticInput)},
		{builtinRegistration("core.pass-through", "Pass Through", "Forwards one incoming payload without changing it", true, true, passThrough, nil)},
		{builtinRegistration("core.delay", "Delay", "Delays forwarding one incoming payload", true, true, delay, validateDelay)},
		{builtinRegistration("core.terminal", "Terminal", "Receives final workflow output", true, false, terminal, nil)},
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

func builtinRegistration(
	nodeType, displayName, description string,
	input, output bool,
	handler NodeHandler,
	validator NodeConfigurationValidator,
) NodeRegistration {
	return NodeRegistration{
		Definition:              builtinDefinition(nodeType, displayName, description, input, output),
		Handler:                 handler,
		Validator:               validator,
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
	registration := builtinRegistration(
		"core.output",
		"Output",
		"Represents a generic workflow output boundary and returns its input unchanged",
		true,
		false,
		terminal,
		nil,
	)
	registration.AllowedExecutionSources = nil
	return registration
}

func joinRegistration() NodeRegistration {
	registration := builtinRegistration(
		"core.join",
		"Join",
		"Aggregates every predecessor payload in immutable workflow edge order",
		true,
		true,
		join,
		nil,
	)
	definition := registration.Definition
	definition.InputMode = NodeInputMulti
	definition.InputEdgeConstraint.Minimum = 2
	definition.InputEdgeConstraint.Maximum = nil
	registration.Definition = definition
	registration.AllowedExecutionSources = nil
	return registration
}

func httpTriggerRegistration() NodeRegistration {
	registration := builtinRegistration(
		"core.http-trigger",
		"HTTP Trigger",
		"Receives one authenticated inbound webhook request and forwards its normalized payload",
		false,
		true,
		httpTrigger,
		validateHTTPTrigger,
	)
	definition := registration.Definition
	definition.OutputEdgeConstraint.Minimum = 1
	registration.Definition = definition
	registration.AllowedExecutionSources = []string{"HTTP_WEBHOOK"}
	registration.ContextProvider = "http-trigger"
	return registration
}

func cronTriggerRegistration() NodeRegistration {
	registration := builtinRegistration(
		"core.cron-trigger",
		"Cron Trigger",
		"Fires on a five-field cron schedule and forwards scheduler metadata",
		false,
		true,
		cronTrigger,
		validateCronTrigger,
	)
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

func staticInput(_ context.Context, _ NodeExecutionContext, parameters map[string]any, input any) (any, error) {
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

func join(_ context.Context, _ NodeExecutionContext, _ map[string]any, input any) (any, error) {
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

func httpTrigger(_ context.Context, _ NodeExecutionContext, _ map[string]any, input any) (any, error) {
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

func cronTrigger(_ context.Context, _ NodeExecutionContext, _ map[string]any, input any) (any, error) {
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

func passThrough(_ context.Context, _ NodeExecutionContext, _ map[string]any, input any) (any, error) {
	if input == nil {
		return nil, fmt.Errorf("pass-through node requires input")
	}
	return input, nil
}

func delay(ctx context.Context, _ NodeExecutionContext, parameters map[string]any, input any) (any, error) {
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

func terminal(_ context.Context, _ NodeExecutionContext, _ map[string]any, input any) (any, error) {
	if input == nil {
		return nil, fmt.Errorf("terminal node requires input")
	}
	return input, nil
}
