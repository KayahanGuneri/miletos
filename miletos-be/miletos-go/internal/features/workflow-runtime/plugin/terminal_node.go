package plugin

func terminalRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                     "core.terminal",
		InputMode:               NodeInputSingle,
		InputPorts:              standardInputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(1),
		OutputEdgeConstraint:    fixedEdgeConstraint(0),
		RoutingMode:             OutputRoutingBroadcast,
		AllowedExecutionSources: []string{"MANUAL_DIRECT"},
		Handler:                 terminalNodeHandler,
	}
}

func terminalNodeHandler(nodeContext *Context) error {
	nodeContext.Lifecycles.OnRun(func() (any, error) {
		return terminal(nodeContext)
	})
	return nil
}

func terminal(nodeContext *Context) (any, error) {
	return objectPayload(
		nodeContext.Payload,
		"TERMINAL_INPUT_INVALID",
		"Terminal requires a JSON object payload.",
	)
}
