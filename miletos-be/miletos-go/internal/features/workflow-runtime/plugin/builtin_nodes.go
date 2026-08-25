package plugin

func RegisterBuiltinNodes(registry *NodeRegistry) error {
	registrations := []NodeRegistration{
		delayRegistration(),
		terminalRegistration(),
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

func standardInputPorts() []Port {
	return []Port{{Name: "input"}}
}

func standardOutputPorts() []Port {
	return []Port{{Name: "output"}}
}

func fixedEdgeConstraint(count uint) EdgeConstraint {
	maximum := count
	return EdgeConstraint{Minimum: count, Maximum: &maximum}
}
