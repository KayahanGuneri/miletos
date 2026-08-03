//go:build integration

package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"miletos-go/internal/context-provider/http-trigger"
	execution "miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/execution/queue"
	executionrepository "miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	workflowfeature "miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/health"
	"miletos-go/internal/shared/database"
	runtimehttp "miletos-go/internal/shared/http"
	"miletos-go/internal/shared/requestcontext"
	"miletos-go/internal/shared/security"
)

type routerQueue struct{}

func (routerQueue) Push(context.Context, string, string, []byte) error {
	return nil
}

func (routerQueue) Consume(
	context.Context,
	string,
	func(context.Context, []byte) queue.RecordResult,
) error {
	return nil
}

func integrationRouter(t *testing.T) http.Handler {
	t.Helper()
	connectionURL := strings.TrimSpace(os.Getenv("MILETOS_TEST_DATABASE_URL"))
	if connectionURL == "" {
		t.Skip("MILETOS_TEST_DATABASE_URL is required for router integration tests")
	}
	configuration, err := pgxpool.ParseConfig(connectionURL)
	if err != nil {
		t.Fatalf("MILETOS_TEST_DATABASE_URL is invalid")
	}
	if !safeRouterTestDatabase(configuration.ConnConfig.Host, configuration.ConnConfig.Database) {
		t.Skipf(
			"MILETOS_TEST_DATABASE_URL is not explicitly test-safe (host %q)",
			configuration.ConnConfig.Host,
		)
	}
	t.Logf("using test PostgreSQL host %q", configuration.ConnConfig.Host)
	configuration.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(context.Background(), configuration)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("connect to integration database: %v", err)
	}
	if _, err := pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS workflow_runtime CASCADE"); err != nil {
		pool.Close()
		t.Fatalf("reset integration schema: %v", err)
	}
	_, helperFile, _, _ := runtime.Caller(0)
	files, err := filepath.Glob(filepath.Join(
		filepath.Dir(helperFile), "..", "..", "..", ".local", "migrations", "*.sql",
	))
	if err != nil || len(files) != 10 {
		pool.Close()
		t.Fatalf("expected ten local migration files")
	}
	sort.Strings(files)
	for _, file := range files {
		contents, err := os.ReadFile(file)
		if err != nil {
			pool.Close()
			t.Fatalf("read migration %s: %v", filepath.Base(file), err)
		}
		up := strings.SplitN(string(contents), "-- +goose Down", 2)[0]
		if _, err := pool.Exec(context.Background(), up); err != nil {
			pool.Close()
			t.Fatalf("apply migration %s: %v", filepath.Base(file), err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS workflow_runtime CASCADE")
		pool.Close()
	})

	client, err := database.Open(context.Background(), connectionURL)
	if err != nil {
		t.Fatalf("open GORM router integration database: %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close GORM router integration database: %v", err)
		}
	})
	workflows := workflowfeature.NewWorkflowRepository(client)
	executions := executionrepository.NewExecutionRepository(client)
	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		t.Fatalf("RegisterBuiltinNodes() error = %v", err)
	}
	if err := registry.DefineNode("test.failure", func(
		context.Context, plugin.NodeExecutionContext, map[string]any, any,
	) (any, error) {
		return nil, errors.New("controlled integration failure")
	}); err != nil {
		t.Fatalf("DefineNode(test.failure) error = %v", err)
	}
	var nodeQueue queue.Queue = routerQueue{}
	scheduler := execution.NewScheduler(
		workflows, executions, nodeQueue, "commands", registry,
	)
	processor, err := execution.NewNodeProcessor(
		workflows, executions, registry, scheduler, nodeQueue, "commands", 1, time.Millisecond,
	)
	if err != nil {
		t.Fatalf("NewNodeProcessor() error = %v", err)
	}
	workflowService := workflowfeature.NewWorkflowService(workflows, registry)
	executionService := execution.NewExecutionService(
		workflowService, executions, scheduler, processor, true,
	)
	recoveryService := execution.NewRecoveryService(
		workflows, executions, workflowService, scheduler,
	)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	healthController := &health.Controller{}
	pluginController := plugin.NewController(registry)
	executionController := execution.NewExecutionController(
		executionService, recoveryService,
		execution.NewExecutionQueryService(executions, workflows),
	)
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

func safeRouterTestDatabase(host, database string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	database = strings.ToLower(strings.TrimSpace(database))
	if strings.Contains(host, "prod") || strings.Contains(host, "stag") ||
		strings.Contains(database, "prod") || strings.Contains(database, "stag") {
		return false
	}
	localHost := host == "localhost" || host == "127.0.0.1" || host == "::1"
	testHost := strings.Contains(host, "test")
	return (localHost || testHost) && strings.Contains(database, "test")
}

func routerRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body any,
	idempotencyKey string,
) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Authorization", "Bearer "+routerToken)
	request.Header.Set(requestcontext.HeaderCompanyID, "company-1")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func successfulWorkflowRequest() map[string]any {
	return map[string]any{
		"definition": map[string]any{
			"id": "workflow-success", "name": "Successful", "revision": 1,
			"nodes": []map[string]any{
				{
					"id": "root", "pluginType": "core.static-input",
					"pluginVersion": "v1", "configuration": map[string]any{"value": "payload"},
				},
				{
					"id": "terminal", "pluginType": "core.terminal",
					"pluginVersion": "v1",
				},
			},
			"edges": []map[string]any{{
				"id": "edge-1", "sourceNodeId": "root", "sourceOutputPort": "output",
				"targetNodeId": "terminal", "targetInputPort": "input",
			}},
		},
	}
}

func TestRouterExecutionContractsIntegration(t *testing.T) {
	router := integrationRouter(t)
	syncResponse := routerRequest(
		t, router, http.MethodPost, "/api/v1/executions/sync",
		successfulWorkflowRequest(), "sync-key",
	)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", syncResponse.Code, syncResponse.Body)
	}
	var syncDocument map[string]any
	if err := json.Unmarshal(syncResponse.Body.Bytes(), &syncDocument); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	executionID, _ := syncDocument["executionId"].(string)
	if executionID == "" || syncDocument["status"] != "SUCCEEDED" {
		t.Fatalf("sync response = %#v", syncDocument)
	}

	getRoutes := []string{
		"/api/v1/executions",
		"/api/v1/executions/" + executionID,
		"/api/v1/executions/" + executionID + "/definition",
		"/api/v1/executions/" + executionID + "/nodes",
		"/api/v1/executions/" + executionID + "/events",
		"/api/v1/executions/" + executionID + "/logs",
		"/api/v1/executions/" + executionID + "/errors",
	}
	for _, path := range getRoutes {
		response := routerRequest(t, router, http.MethodGet, path, nil, "")
		if response.Code != http.StatusOK ||
			response.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("%s status=%d headers=%v body=%s",
				path, response.Code, response.Header(), response.Body)
		}
	}

	asyncResponse := routerRequest(
		t, router, http.MethodPost, "/api/v1/executions/async",
		successfulWorkflowRequest(), "async-key",
	)
	if asyncResponse.Code != http.StatusAccepted {
		t.Fatalf("async status=%d body=%s", asyncResponse.Code, asyncResponse.Body)
	}
}

func TestRouterRecoveryContractIntegration(t *testing.T) {
	router := integrationRouter(t)
	failingRequest := map[string]any{
		"definition": map[string]any{
			"id": "workflow-failure", "name": "Failure", "revision": 1,
			"nodes": []map[string]any{{
				"id": "failure", "pluginType": "test.failure", "pluginVersion": "v1",
			}},
			"edges": []map[string]any{},
		},
	}
	syncResponse := routerRequest(
		t, router, http.MethodPost, "/api/v1/executions/sync", failingRequest, "failure-key",
	)
	if syncResponse.Code != http.StatusOK {
		t.Fatalf("failed sync status=%d body=%s", syncResponse.Code, syncResponse.Body)
	}
	var source map[string]any
	if err := json.Unmarshal(syncResponse.Body.Bytes(), &source); err != nil {
		t.Fatalf("decode failed sync response: %v", err)
	}
	sourceID, _ := source["executionId"].(string)
	if sourceID == "" || source["status"] != "FAILED" {
		t.Fatalf("failed sync response = %#v", source)
	}

	recoveryResponse := routerRequest(
		t, router, http.MethodPost,
		"/api/v1/executions/"+sourceID+"/recover", nil, "recovery-key",
	)
	if recoveryResponse.Code != http.StatusAccepted {
		t.Fatalf("recovery status=%d body=%s", recoveryResponse.Code, recoveryResponse.Body)
	}
	var recovery map[string]any
	if err := json.Unmarshal(recoveryResponse.Body.Bytes(), &recovery); err != nil {
		t.Fatalf("decode recovery response: %v", err)
	}
	if recovery["sourceExecutionId"] != sourceID || recovery["recoveryExecutionId"] == "" {
		t.Fatalf("recovery response = %#v", recovery)
	}
}
