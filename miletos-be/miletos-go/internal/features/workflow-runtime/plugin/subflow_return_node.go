package plugin

func subflowReturnRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                    "core.subflow-return",
		InputMode:              NodeInputSingle,
		InputPorts:             standardInputPorts(),
		InputEdgeConstraint:    fixedEdgeConstraint(1),
		OutputEdgeConstraint:   fixedEdgeConstraint(0),
		RoutingMode:            OutputRoutingBroadcast,
		ProvidesWorkflowResult: true,
		Handler:                subflowReturnNodeHandler,
	}
}

func subflowReturnNodeHandler(nodeContext *Context) error {
	nodeContext.Lifecycles.OnRun(func() (any, error) {
		return subflowReturnNode(nodeContext)
	})
	return nil
}

func subflowReturnNode(nodeContext *Context) (any, error) {
	if nodeContext.Payload == nil {
		return nil, validationNodeError(
			"SUBFLOW_RETURN_INPUT_REQUIRED",
			"Subflow Return input is required.",
		)
	}
	return nodeContext.Payload, nil
}
