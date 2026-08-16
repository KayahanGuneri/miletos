package plugin

type InputNodeRuntime struct {
	Database       DatabaseInfrastructure
	InputDirectory string
	Secrets        SecretDecryptor
}

func RegisterInputSourceNodes(registry *NodeRegistry, runtime InputNodeRuntime) error {
	registrations := []NodeRegistration{
		fileInputRegistration(runtime),
		excelInputRegistration(runtime),
		databaseInputRegistration(runtime),
	}
	for _, registration := range registrations {
		if err := registry.RegisterNode(registration); err != nil {
			return err
		}
	}
	return nil
}
