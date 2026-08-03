package plugin

import (
	"net/http"

	httpresponse "miletos-go/internal/shared/http/response"
)

type Controller struct {
	registry *NodeRegistry
}

func NewController(registry *NodeRegistry) *Controller {
	return &Controller{registry: registry}
}

func (controller *Controller) List(writer http.ResponseWriter, _ *http.Request) {
	registrations := controller.registry.Registrations()
	response := mapPluginList(registrations)
	httpresponse.WriteJSON(writer, http.StatusOK, response)
}
