package controller

import (
	"encoding/json"
	"net/http"
)

type HealthController struct{}

func (HealthController) Get(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(map[string]string{"status": "UP"})
}
