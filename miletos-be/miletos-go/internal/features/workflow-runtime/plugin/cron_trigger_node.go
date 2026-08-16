package plugin

import (
	"strings"

	"miletos-go/internal/features/workflow-runtime/cronexpr"
)

func cronTriggerRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.cron-trigger",
		InputMode:               NodeInputSingle,
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(0),
		OutputEdgeConstraint:    EdgeConstraint{Minimum: 1},
		RoutingMode:             OutputRoutingBroadcast,
		Validator:               validateCronTrigger,
		AllowedExecutionSources: []string{"CRON"},
		ContextProvider:         "cron-trigger",
		Handler:                 cronTriggerHandler,
	}
}

func cronTriggerHandler(ctx *Context) error {
	ctx.Lifecycles.OnScenarioStart(func() error {
		schedule, err := resolveCronTrigger(ctx.configuration)
		if err != nil {
			return err
		}
		return ctx.Infra.Cron.CreateCronTrigger(schedule.Expression, schedule.Timezone)
	})
	ctx.Lifecycles.OnRun(func() (any, error) {
		return cronTrigger(ctx)
	})
	return nil
}

func validateCronTrigger(configuration map[string]any) error {
	_, err := resolveCronTrigger(configuration)
	return err
}

func resolveCronTrigger(configuration map[string]any) (cronexpr.Schedule, error) {
	expression, ok := configuration["expression"].(string)
	if !ok || strings.TrimSpace(expression) == "" {
		return cronexpr.Schedule{}, &NodeError{
			Category: "VALIDATION",
			Code:     "CRON_TRIGGER_EXPRESSION_REQUIRED",
			Message:  "configuration.expression is required",
		}
	}
	timezone, _ := configuration["timezone"].(string)
	schedule, err := cronexpr.Parse(expression, timezone)
	if err != nil {
		code, ok := cronexpr.ValidationCode(err)
		if !ok {
			return cronexpr.Schedule{}, &NodeError{
				Category: "INTERNAL",
				Code:     "CRON_TRIGGER_UNEXPECTED",
				Message:  "Cron trigger configuration could not be validated.",
			}
		}
		return cronexpr.Schedule{}, &NodeError{
			Category: "VALIDATION",
			Code:     "CRON_TRIGGER_" + code,
			Message:  "Cron trigger configuration is invalid.",
		}
	}
	return schedule, nil
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
