package plugin

import (
	"net/http"
	"time"
)

type OutputNodeRuntime struct {
	HTTPClient *http.Client
}

func RegisterOutputDestinationNodes(registry *NodeRegistry, runtime OutputNodeRuntime) error {
	if runtime.HTTPClient == nil {
		runtime.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	registrations := []NodeRegistration{
		restOutputRegistration(runtime.HTTPClient),
		databaseOutputRegistration(),
		csvOutputRegistration(),
	}
	for _, registration := range registrations {
		if err := registry.RegisterNode(registration); err != nil {
			return err
		}
	}
	return nil
}
