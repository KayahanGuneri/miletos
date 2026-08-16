package plugin

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/features/workflow-runtime/cronexpr"
)

func RegisterBuiltinNodes(registry *NodeRegistry) error {
	registrations := []NodeRegistration{
		staticInputRegistration(),
		passThroughRegistration(),
		delayRegistration(),
		terminalRegistration(),
		outputRegistration(),
		joinRegistration(),
		httpTriggerRegistration(),
		cronTriggerRegistration(),
	}
	for _, registration := range registrations {
		if err := registry.RegisterNode(registration); err != nil {
			return err
		}
	}
	return nil
}

func staticInputRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.static-input",
		DisplayName:             "Static Input",
		Description:             "Provides configured static content to a workflow",
		InputMode:               NodeInputSingle,
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(0),
		OutputEdgeConstraint:    EdgeConstraint{},
		RoutingMode:             OutputRoutingBroadcast,
		Validator:               validateStaticInput,
		AllowedExecutionSources: []string{"MANUAL_DIRECT"},
		Handler:                 onRunHandler(staticInput),
	}
}

func passThroughRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.pass-through",
		DisplayName:             "Pass Through",
		Description:             "Forwards one incoming payload without changing it",
		InputMode:               NodeInputSingle,
		InputPorts:              standardInputPorts(),
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(1),
		OutputEdgeConstraint:    EdgeConstraint{},
		RoutingMode:             OutputRoutingBroadcast,
		AllowedExecutionSources: []string{"MANUAL_DIRECT"},
		Handler:                 onRunHandler(passThrough),
	}
}

func delayRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.delay",
		DisplayName:             "Delay",
		Description:             "Delays forwarding one incoming payload",
		InputMode:               NodeInputSingle,
		InputPorts:              standardInputPorts(),
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(1),
		OutputEdgeConstraint:    EdgeConstraint{},
		RoutingMode:             OutputRoutingBroadcast,
		Validator:               validateDelay,
		AllowedExecutionSources: []string{"MANUAL_DIRECT"},
		Handler:                 onRunHandler(delay),
	}
}

func terminalRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.terminal",
		DisplayName:             "Terminal",
		Description:             "Receives final workflow output",
		InputMode:               NodeInputSingle,
		InputPorts:              standardInputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(1),
		OutputEdgeConstraint:    fixedEdgeConstraint(0),
		RoutingMode:             OutputRoutingBroadcast,
		AllowedExecutionSources: []string{"MANUAL_DIRECT"},
		Handler:                 onRunHandler(terminal),
	}
}

func onRunHandler(run func(*Context) (any, error)) NodeHandler {
	return func(ctx *Context) error {
		ctx.Lifecycles.OnRun(func() (any, error) {
			return run(ctx)
		})
		return nil
	}
}

func outputRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                  "core.output",
		DisplayName:          "Output",
		Description:          "Represents a generic workflow output boundary and returns its input unchanged",
		InputMode:            NodeInputSingle,
		InputPorts:           standardInputPorts(),
		InputEdgeConstraint:  fixedEdgeConstraint(1),
		OutputEdgeConstraint: fixedEdgeConstraint(0),
		RoutingMode:          OutputRoutingBroadcast,
		Handler:              onRunHandler(terminal),
	}
}

func joinRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                  "core.join",
		DisplayName:          "Join",
		Description:          "Aggregates every predecessor payload in immutable workflow edge order",
		InputMode:            NodeInputMulti,
		InputPorts:           standardInputPorts(),
		OutputPorts:          standardOutputPorts(),
		InputEdgeConstraint:  EdgeConstraint{Minimum: 2},
		OutputEdgeConstraint: EdgeConstraint{},
		RoutingMode:          OutputRoutingBroadcast,
		Handler:              onRunHandler(join),
	}
}

func httpTriggerRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.http-trigger",
		DisplayName:             "HTTP Trigger",
		Description:             "Receives one authenticated inbound webhook request and forwards its normalized payload",
		InputMode:               NodeInputSingle,
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(0),
		OutputEdgeConstraint:    EdgeConstraint{Minimum: 1},
		RoutingMode:             OutputRoutingBroadcast,
		Validator:               validateHTTPTrigger,
		AllowedExecutionSources: []string{"HTTP_WEBHOOK"},
		ContextProvider:         "http-trigger",
		Handler:                 httpTriggerHandler,
	}
}

func cronTriggerRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.cron-trigger",
		DisplayName:             "Cron Trigger",
		Description:             "Fires on a five-field cron schedule and forwards scheduler metadata",
		InputMode:               NodeInputSingle,
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(0),
		OutputEdgeConstraint:    EdgeConstraint{Minimum: 1},
		RoutingMode:             OutputRoutingBroadcast,
		Validator:               validateCronTrigger,
		AllowedExecutionSources: []string{"CRON"},
		ContextProvider:         "cron-trigger",
		Handler:                 onRunHandler(cronTrigger),
	}
}

func standardInputPorts() []Port {
	return []Port{{
		Name: "input", DisplayName: "Input", Description: "Receives an incoming payload",
	}}
}

func standardOutputPorts() []Port {
	return []Port{{
		Name: "output", DisplayName: "Output", Description: "Provides an outgoing payload",
	}}
}

func fixedEdgeConstraint(count uint) EdgeConstraint {
	maximum := count
	return EdgeConstraint{Minimum: count, Maximum: &maximum}
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

func staticInput(nodeContext *Context) (any, error) {
	parameters := nodeContext.configuration
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
	if err := validateHTTPTriggerPayload(input); err != nil {
		return nil, err
	}
	return input, nil
}

func validateHTTPTriggerPayload(input any) error {
	payload, ok := input.(map[string]any)
	if !ok {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "HTTP_TRIGGER_INPUT_INVALID",
			Message:  "HTTP trigger input is missing or invalid.",
		}
	}
	for _, field := range []string{"method", "path", "requestId", "correlationId"} {
		if value, ok := payload[field].(string); !ok || strings.TrimSpace(value) == "" {
			return &NodeError{
				Category: "VALIDATION",
				Code:     "HTTP_TRIGGER_INPUT_INVALID",
				Message:  "HTTP trigger input is missing required request metadata.",
			}
		}
	}
	return nil
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
	ctx := nodeContext.runtime
	parameters := nodeContext.configuration
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

func httpTriggerHandler(ctx *Context) error {
	ctx.Lifecycles.OnScenarioStart(func() error {
		if err := validateHTTPTrigger(ctx.configuration); err != nil {
			return err
		}
		method := HTTPMethod(strings.ToUpper(strings.TrimSpace(
			ctx.configuration["method"].(string),
		)))
		_, err := ctx.Infra.HTTP.CreateHTTPURL(method)
		return err
	})
	if err := ctx.Infra.HTTP.OnRequest(func(currentURL string, payload any) error {
		_ = currentURL
		return validateHTTPTriggerPayload(payload)
	}); err != nil {
		return err
	}
	ctx.Lifecycles.OnRun(func() (any, error) {
		return httpTrigger(ctx)
	})
	return nil
}
