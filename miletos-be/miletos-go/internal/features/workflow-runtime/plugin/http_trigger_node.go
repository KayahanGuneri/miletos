package plugin

import "strings"

func httpTriggerRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.http-trigger",
		InputMode:               NodeInputSingle,
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(0),
		OutputEdgeConstraint:    EdgeConstraint{Minimum: 1},
		RoutingMode:             OutputRoutingExplicit,
		Validator:               validateHTTPTrigger,
		AllowedExecutionSources: []string{"HTTP_WEBHOOK"},
		ContextProvider:         "http-trigger",
		Handler:                 httpTriggerHandler,
	}
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

func httpTrigger(nodeContext *Context) (any, error) {
	input := nodeContext.Payload
	if err := validateHTTPTriggerPayload(input); err != nil {
		return nil, err
	}
	payload := input.(map[string]any)
	downstreamPayload := input
	if body, present := payload["body"]; present {
		downstreamPayload = body
	}
	for index := 0; index < nodeContext.Access.GetOutputEdgeCount(); index++ {
		if err := nodeContext.Access.PushEdge(index, downstreamPayload); err != nil {
			return nil, err
		}
	}
	return downstreamPayload, nil
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

func httpTriggerHandler(ctx *Context) error {
	createdBinding := HTTPBinding{}
	ctx.Lifecycles.OnScenarioStart(func() error {
		if err := validateHTTPTrigger(ctx.configuration); err != nil {
			return err
		}
		method := HTTPMethod(strings.ToUpper(strings.TrimSpace(
			ctx.configuration["method"].(string),
		)))
		binding, err := ctx.Infra.HTTP.CreateHTTPURL(method)
		if err != nil {
			return err
		}
		createdBinding = binding
		return nil
	})
	if err := ctx.Infra.HTTP.OnRequest(func(currentIdentity string, payload any) (any, error) {
		if createdBinding.Identity == "" || currentIdentity != createdBinding.Identity {
			return nil, nil
		}
		ctx.Payload = payload
		return httpTrigger(ctx)
	}); err != nil {
		return err
	}
	ctx.Lifecycles.OnRun(func() (any, error) {
		return httpTrigger(ctx)
	})
	return nil
}
