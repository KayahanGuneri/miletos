package http_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"miletos-go/internal/context-provider/http-trigger"
	"miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/health"
	runtimehttp "miletos-go/internal/shared/http"
	"miletos-go/internal/shared/security"
)

const routerToken = "router-internal-service-token-value-32"

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	healthController := &health.Controller{}
	pluginController := plugin.NewController(registry)
	executionController := execution.NewExecutionController(nil, nil, nil)
	publicController := httptrigger.NewPublicController(nil)
	validator, err := security.NewInternalTokenValidator(routerToken)
	if err != nil {
		t.Fatalf("NewInternalTokenValidator() error = %v", err)
	}
	return runtimehttp.NewRouter(
		logger,
		security.NewAuthentication(validator),
		runtimehttp.RouteHandlers{
			Health:                 healthController.Get,
			PublicTrigger:          publicController.Invoke,
			ListPlugins:            pluginController.List,
			ListExecutions:         executionController.List,
			ExecuteSync:            executionController.ExecuteSync,
			ExecuteAsync:           executionController.ExecuteAsync,
			GetExecution:           executionController.Get,
			GetExecutionDefinition: executionController.GetDefinition,
			GetExecutionNodes:      executionController.GetNodes,
			GetExecutionEvents:     executionController.GetEvents,
			GetExecutionLogs:       executionController.GetLogs,
			GetExecutionErrors:     executionController.GetErrors,
			RecoverExecution:       executionController.Recover,
		},
	)
}

func TestRouterHealthAndPluginRoutes(t *testing.T) {
	router := testRouter(t)
	t.Run("health", func(t *testing.T) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
		if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
		}
	})
	t.Run("plugins require authentication", func(t *testing.T) {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})
	t.Run("plugins", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/plugins", nil)
		request.Header.Set("Authorization", "Bearer "+routerToken)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", response.Code, response.Body)
		}
	})
}

func TestRouterWiresProtectedExecutionRoutes(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/executions/sync"},
		{method: http.MethodPost, path: "/api/v1/executions/async"},
		{method: http.MethodPost, path: "/api/v1/executions/exec-1/recover"},
		{method: http.MethodGet, path: "/api/v1/executions"},
		{method: http.MethodGet, path: "/api/v1/executions/exec-1"},
		{method: http.MethodGet, path: "/api/v1/executions/exec-1/definition"},
		{method: http.MethodGet, path: "/api/v1/executions/exec-1/nodes"},
		{method: http.MethodGet, path: "/api/v1/executions/exec-1/events"},
		{method: http.MethodGet, path: "/api/v1/executions/exec-1/logs"},
		{method: http.MethodGet, path: "/api/v1/executions/exec-1/errors"},
	}
	router := testRouter(t)
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, nil)
			request.Header.Set("Authorization", "Bearer "+routerToken)
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			var document map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if document["code"] != "INVALID_COMPANY_CONTEXT" {
				t.Fatalf("body = %#v", document)
			}
		})
	}
}

func TestRouterReturnsJSONNotFoundAndMethodNotAllowed(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
		code   string
	}{
		{
			name: "not found", method: http.MethodGet, path: "/missing",
			status: http.StatusNotFound, code: "NOT_FOUND",
		},
		{
			name: "method not allowed", method: http.MethodPut, path: "/health",
			status: http.StatusMethodNotAllowed, code: "METHOD_NOT_ALLOWED",
		},
	}
	router := testRouter(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status || response.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
			}
			var document map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if document["code"] != test.code {
				t.Fatalf("body = %#v", document)
			}
		})
	}
}

func TestNewServerUsesRuntimeTimeouts(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	server := runtimehttp.NewServer("127.0.0.1:9090", handler)

	if server.Addr != "127.0.0.1:9090" || server.Handler == nil {
		t.Fatalf("server = %#v", server)
	}
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 ||
		server.WriteTimeout <= 0 || server.IdleTimeout <= 0 {
		t.Fatalf("server timeouts = %#v", server)
	}
}
