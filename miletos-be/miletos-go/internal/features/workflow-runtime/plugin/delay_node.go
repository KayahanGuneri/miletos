package plugin

import (
	"fmt"
	"strings"
	"time"
)

func delayRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.delay",
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

func delay(nodeContext *Context) (any, error) {
	ctx := nodeContext.runtime
	parameters := nodeContext.configuration
	input := nodeContext.Payload
	if _, err := objectPayload(
		input, "DELAY_INPUT_INVALID", "Delay requires a JSON object payload.",
	); err != nil {
		return nil, err
	}
	if err := validateDelay(parameters); err != nil {
		return nil, err
	}
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
