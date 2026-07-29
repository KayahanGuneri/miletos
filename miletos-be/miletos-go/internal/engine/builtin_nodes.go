package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"miletos-go/internal/model"
)

func RegisterBuiltinNodes(registry *NodeRegistry) error {
	definitions := []struct {
		definition model.NodeDefinition
		handler    NodeHandler
		validator  NodeConfigurationValidator
	}{
		{builtinDefinition("core.static-input", "Static Input", "Provides configured static content to a workflow", false, true), staticInput, validateStaticInput},
		{builtinDefinition("core.pass-through", "Pass Through", "Forwards one incoming payload without changing it", true, true), passThrough, nil},
		{builtinDefinition("core.delay", "Delay", "Delays forwarding one incoming payload", true, true), delay, validateDelay},
		{builtinDefinition("core.terminal", "Terminal", "Receives final workflow output", true, false), terminal, nil},
	}
	for _, builtin := range definitions {
		if err := registry.defineValidated(
			builtin.definition,
			builtin.handler,
			builtin.validator,
		); err != nil {
			return err
		}
	}
	return nil
}

func builtinDefinition(nodeType, displayName, description string, input, output bool) model.NodeDefinition {
	definition := model.NodeDefinition{
		Type:        nodeType,
		Version:     "v1",
		DisplayName: displayName,
		Description: description,
		InputEdgeConstraint: model.EdgeConstraint{
			Minimum: boolUint(input),
		},
		OutputEdgeConstraint: model.EdgeConstraint{
			Minimum: 0,
		},
	}
	if input {
		maximum := uint(1)
		definition.InputPorts = []model.Port{{
			Name: "input", DisplayName: "Input", Description: "Receives an incoming payload",
		}}
		definition.InputEdgeConstraint.Maximum = &maximum
	} else {
		maximum := uint(0)
		definition.InputEdgeConstraint.Maximum = &maximum
	}
	if output {
		definition.OutputPorts = []model.Port{{
			Name: "output", DisplayName: "Output", Description: "Provides an outgoing payload",
		}}
		definition.OutputEdgeConstraint.Unlimited = true
	} else {
		maximum := uint(0)
		definition.OutputEdgeConstraint.Maximum = &maximum
	}
	return definition
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

func boolUint(value bool) uint {
	if value {
		return 1
	}
	return 0
}

func staticInput(_ context.Context, parameters map[string]any, input any) (any, error) {
	if input != nil {
		if values, ok := input.(map[string]any); !ok || len(values) > 0 {
			return nil, fmt.Errorf("static input node must not receive input")
		}
	}
	value, exists := parameters["value"]
	if !exists {
		return nil, fmt.Errorf("static input configuration requires value")
	}
	return value, nil
}

func passThrough(_ context.Context, _ map[string]any, input any) (any, error) {
	if input == nil {
		return nil, fmt.Errorf("pass-through node requires input")
	}
	return input, nil
}

func delay(ctx context.Context, parameters map[string]any, input any) (any, error) {
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

func terminal(_ context.Context, _ map[string]any, input any) (any, error) {
	if input == nil {
		return nil, fmt.Errorf("terminal node requires input")
	}
	return input, nil
}
