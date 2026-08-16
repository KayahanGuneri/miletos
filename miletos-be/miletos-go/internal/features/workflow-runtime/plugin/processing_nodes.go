package plugin

func RegisterDataProcessingNodes(registry *NodeRegistry) error {
	registrations := []NodeRegistration{
		mergeRegistration(),
		mapRegistration(),
		ifRegistration(),
		filterRegistration(),
		subflowRegistration(),
		subflowReturnRegistration(),
	}
	for _, registration := range registrations {
		if err := registry.RegisterNode(registration); err != nil {
			return err
		}
	}
	return nil
}
