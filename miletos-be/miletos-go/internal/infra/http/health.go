package api

import (
	"net/http"

	healthfeature "miletos-go/internal/features/health"
	"miletos-go/internal/infra/http/transport"
)

const healthStatusUp = "UP"

type Handler struct {
	serviceName string
	version     string
}

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

func NewHandler(serviceName string, version string) Handler {
	return Handler{serviceName: serviceName, version: version}
}

func (handler Handler) Health(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	_ = writeJSON(
		writer,
		http.StatusOK,
		HealthResponse{
			Status:  healthStatusUp,
			Service: handler.serviceName,
			Version: handler.version,
		},
	)
}

func (handler Handler) NotFound(
	writer http.ResponseWriter,
	request *http.Request,
) {
	writeAPIError(
		writer,
		request,
		newAPIError(
			http.StatusNotFound,
			errorCodeNotFound,
			"The requested resource was not found.",
		),
	)
}

type HealthHandler struct {
	readiness healthfeature.ReadinessService
}

func NewHealthHandler(readiness healthfeature.ReadinessService) HealthHandler {
	return HealthHandler{readiness: readiness}
}

func (handler HealthHandler) Ready(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if !transport.RequireMethod(writer, request, http.MethodGet) {
		return
	}
	report := handler.readiness.Check(request.Context())
	status := http.StatusOK
	if report.Status != healthfeature.StatusReady {
		status = http.StatusServiceUnavailable
	}
	_ = writeJSON(writer, status, report)
}
