package api

import (
	bytes "bytes"
	context "context"
	json "encoding/json"
	errors "errors"
	io "io"
	slog "log/slog"
	engine "miletos-go/internal/engine"
	core "miletos-go/internal/engine/nodes/core"
	plugin "miletos-go/internal/engine/plugin"
	execution "miletos-go/internal/features/execution"
	executionfeature "miletos-go/internal/features/execution/application"
	healthfeature "miletos-go/internal/features/health"
	executionhttp "miletos-go/internal/infra/http/execution"
	openapi "miletos-go/internal/infra/http/openapi"
	http "net/http"
	httptest "net/http/httptest"
	os "os"
	strings "strings"
	testing "testing"
	time "time"
)

const (
	errorCodeInvalidExecutionRequest      = "INVALID_EXECUTION_REQUEST"
	errorCodeWorkflowValidationFailed     = "WORKFLOW_VALIDATION_FAILED"
	errorCodeExecutionUnavailable         = "EXECUTION_UNAVAILABLE"
	errorCodeInvalidIdempotencyKey        = "INVALID_IDEMPOTENCY_KEY"
	errorCodeIdempotencyKeyReused         = "IDEMPOTENCY_KEY_REUSED"
	errorCodeIdempotencyRequestInProgress = "IDEMPOTENCY_REQUEST_IN_PROGRESS"
	errorCodeExecutionQueryUnavailable    = "EXECUTION_QUERY_UNAVAILABLE"
	errorCodeInvalidExecutionQuery        = "INVALID_EXECUTION_QUERY"
	errorCodeInvalidExecutionID           = "INVALID_EXECUTION_ID"
)

type executionApplicationStub struct {
	execute func(
		context.Context,
		engine.ExecutionRequest,
	) (
		executionfeature.ExecutionOutcome,
		error,
	)

	asyncEnabled bool
}

func (
	stub executionApplicationStub,
) Execute(
	ctx context.Context,
	request engine.ExecutionRequest,
) (
	executionfeature.ExecutionOutcome,
	error,
) {
	if stub.execute == nil {
		return executionfeature.ExecutionOutcome{},
			nil
	}

	return stub.execute(
		ctx,
		request,
	)
}

func (
	stub executionApplicationStub,
) AsyncEnabled() bool {
	return stub.asyncEnabled
}

func TestExecuteSyncMapsTrustedRequestToExecutionService(
	t *testing.T,
) {
	createdAt :=
		time.Date(
			2026,
			time.July,
			20,
			7,
			0,
			0,
			0,
			time.UTC,
		)

	startedAt :=
		createdAt.Add(
			time.Second,
		)

	finishedAt :=
		startedAt.Add(
			time.Second,
		)

	application :=
		executionApplicationStub{
			execute: func(
				_ context.Context,
				request engine.ExecutionRequest,
			) (
				executionfeature.ExecutionOutcome,
				error,
			) {
				if request.Mode() !=
					execution.ExecutionModeSync {

					t.Fatalf(
						"mode = %q, want %q",
						request.Mode(),
						execution.ExecutionModeSync,
					)
				}

				if request.
					Definition().
					CompanyID().
					String() !=
					"company-sync-test" {

					t.Fatalf(
						"company ID = %q",
						request.
							Definition().
							CompanyID().
							String(),
					)
				}

				if request.
					Definition().
					ID().
					String() !=
					"workflow-sync-smoke" {

					t.Fatalf(
						"workflow ID = %q",
						request.
							Definition().
							ID().
							String(),
					)
				}

				if request.
					CorrelationID() !=
					"corr-sync-test" {

					t.Fatalf(
						"correlation ID = %q",
						request.
							CorrelationID(),
					)
				}

				if !strings.HasPrefix(
					request.
						WorkflowExecutionID().
						String(),
					"exec_",
				) {
					t.Fatalf(
						"execution ID = %q",
						request.
							WorkflowExecutionID().
							String(),
					)
				}

				variables :=
					request.
						InitialVariables()

				value, exists :=
					variables["requestValue"]

				if !exists {
					t.Fatal(
						"requestValue initial variable is missing",
					)
				}

				if value.String() !=
					`"hello"` {

					t.Fatalf(
						"requestValue = %q",
						value.String(),
					)
				}

				return executionfeature.ExecutionOutcome{
						ExecutionID: request.
							WorkflowExecutionID().
							String(),

						WorkflowID: request.
							Definition().
							ID().
							String(),

						WorkflowRevision: request.
							Definition().
							Revision(),

						Mode: "SYNC",

						Status: "SUCCEEDED",

						CorrelationID: request.
							CorrelationID(),

						CreatedAt: createdAt,

						StartedAt: &startedAt,

						FinishedAt: &finishedAt,
					},
					nil
			},
		}

	router :=
		newExecutionTestRouter(
			t,
			application,
		)

	request :=
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/executions/sync",
			strings.NewReader(
				validSyncExecutionRequestJSON(),
			),
		)

	setExecutionTestHeaders(
		request,
		true,
		true,
	)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusOK {

		t.Fatalf(
			"status code = %d, want %d\nbody=%s",
			response.Code,
			http.StatusOK,
			response.Body.String(),
		)
	}

	var body executionhttp.ExecutionResponse

	if err :=
		json.NewDecoder(
			response.Body,
		).Decode(
			&body,
		); err != nil {

		t.Fatalf(
			"decode execution response: %v",
			err,
		)
	}

	if body.Status !=
		"SUCCEEDED" {

		t.Fatalf(
			"status = %q",
			body.Status,
		)
	}

	if body.Mode !=
		"SYNC" {

		t.Fatalf(
			"mode = %q",
			body.Mode,
		)
	}

	if body.CorrelationID !=
		"corr-sync-test" {

		t.Fatalf(
			"correlationId = %q",
			body.CorrelationID,
		)
	}
}

func TestExecuteSyncRequiresAuthentication(
	t *testing.T,
) {
	router :=
		newExecutionTestRouter(
			t,
			executionApplicationStub{},
		)

	request :=
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/executions/sync",
			strings.NewReader(
				validSyncExecutionRequestJSON(),
			),
		)

	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	request.Header.Set(
		HeaderCompanyID,
		"company-sync-test",
	)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusUnauthorized {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusUnauthorized,
		)
	}
}

func TestExecuteSyncRequiresTrustedCompanyContext(
	t *testing.T,
) {
	router :=
		newExecutionTestRouter(
			t,
			executionApplicationStub{},
		)

	request :=
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/executions/sync",
			strings.NewReader(
				validSyncExecutionRequestJSON(),
			),
		)

	setExecutionTestHeaders(
		request,
		true,
		false,
	)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusBadRequest {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusBadRequest,
		)
	}

	body :=
		decodeAPIErrorForTest(
			t,
			response,
		)

	if body.Code !=
		errorCodeMissingCompanyContext {

		t.Fatalf(
			"error code = %q",
			body.Code,
		)
	}
}

func TestExecuteSyncRejectsBodyCompanyOverride(
	t *testing.T,
) {
	router :=
		newExecutionTestRouter(
			t,
			executionApplicationStub{},
		)

	body :=
		`{` +
			`"companyId":"other-company",` +
			`"definition":{` +
			`"id":"workflow-sync-smoke",` +
			`"name":"Sync Smoke",` +
			`"revision":1,` +
			`"nodes":[],` +
			`"edges":[],` +
			`"metadata":{}` +
			`}` +
			`}`

	request :=
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/executions/sync",
			strings.NewReader(
				body,
			),
		)

	setExecutionTestHeaders(
		request,
		true,
		true,
	)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusBadRequest {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusBadRequest,
		)
	}

	errorBody :=
		decodeAPIErrorForTest(
			t,
			response,
		)

	if errorBody.Code !=
		errorCodeInvalidJSON {

		t.Fatalf(
			"error code = %q, want %q",
			errorBody.Code,
			errorCodeInvalidJSON,
		)
	}
}

func TestExecuteSyncMapsRejectedWorkflowTo422(
	t *testing.T,
) {
	application :=
		executionApplicationStub{
			execute: func(
				context.Context,
				engine.ExecutionRequest,
			) (
				executionfeature.ExecutionOutcome,
				error,
			) {
				return executionfeature.ExecutionOutcome{
						ExecutionID: "exec_rejected",

						WorkflowID: "workflow-sync-smoke",

						WorkflowRevision: 1,

						Mode: "SYNC",

						Status: "REJECTED",

						CorrelationID: "corr-sync-test",

						CreatedAt: time.Now().
							UTC(),

						Rejected: true,

						ValidationDetails: []executionfeature.ValidationDetail{
							{
								Code: "PLUGIN_NOT_FOUND",

								NodeID: "source",

								PluginType: "missing.plugin",

								PluginVersion: "v1",

								Reason: "plugin is not registered",
							},
						},
					},
					nil
			},
		}

	router :=
		newExecutionTestRouter(
			t,
			application,
		)

	request :=
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/executions/sync",
			strings.NewReader(
				validSyncExecutionRequestJSON(),
			),
		)

	setExecutionTestHeaders(
		request,
		true,
		true,
	)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusUnprocessableEntity {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusUnprocessableEntity,
		)
	}

	body :=
		decodeAPIErrorForTest(
			t,
			response,
		)

	if body.Code !=
		errorCodeWorkflowValidationFailed {

		t.Fatalf(
			"error code = %q",
			body.Code,
		)
	}

	if len(body.Details) !=
		1 {

		t.Fatalf(
			"details length = %d, want 1",
			len(body.Details),
		)
	}

	if body.Details[0].Code !=
		"PLUGIN_NOT_FOUND" {

		t.Fatalf(
			"detail code = %q",
			body.Details[0].Code,
		)
	}
}

func TestExecuteSyncRejectsOversizedBody(
	t *testing.T,
) {
	router :=
		newExecutionTestRouter(
			t,
			executionApplicationStub{},
		)

	request :=
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/executions/sync",
			strings.NewReader(
				strings.Repeat(
					"x",
					int(
						defaultExecutionRequestBodyLimit+
							1,
					),
				),
			),
		)

	setExecutionTestHeaders(
		request,
		true,
		true,
	)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusRequestEntityTooLarge {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusRequestEntityTooLarge,
		)
	}
}

func newExecutionTestRouter(
	t *testing.T,
	application executionfeature.ExecutionApplication,
) http.Handler {
	t.Helper()

	authenticator, err :=
		NewInternalAuthenticator(
			testInternalServiceToken,
		)

	if err != nil {
		t.Fatalf(
			"create authenticator: %v",
			err,
		)
	}

	logger :=
		slog.New(
			slog.NewTextHandler(
				io.Discard,
				nil,
			),
		)

	handler :=
		NewHandler(
			"miletos-go",
			"development",
		)

	executionHandler :=
		executionhttp.NewHandler(application, nil)

	return NewRouterWithOptions(
		handler,
		RouterOptions{
			ExecutionHandler: executionHandler,
			Logger:           logger,

			HandlerTimeout: time.Second,

			Authenticator: &authenticator,
		},
	)
}

func setExecutionTestHeaders(
	request *http.Request,
	authenticated bool,
	withCompany bool,
) {
	request.Header.Set(
		"Content-Type",
		"application/json",
	)

	request.Header.Set(
		HeaderCorrelationID,
		"corr-sync-test",
	)

	if authenticated {
		request.Header.Set(
			"Authorization",
			"Bearer "+
				testInternalServiceToken,
		)
	}

	if withCompany {
		request.Header.Set(
			HeaderCompanyID,
			"company-sync-test",
		)
	}
}

func validSyncExecutionRequestJSON() string {
	return `{
		"definition": {
			"id": "workflow-sync-smoke",
			"name": "Sync Smoke",
			"revision": 1,
			"nodes": [
				{
					"id": "source",
					"pluginType": "core.static-input",
					"pluginVersion": "v1",
					"configuration": {
						"value": {
							"message": "hello"
						}
					}
				},
				{
					"id": "terminal",
					"pluginType": "core.terminal",
					"pluginVersion": "v1",
					"configuration": {}
				}
			],
			"edges": [
				{
					"id": "edge-1",
					"sourceNodeId": "source",
					"sourceOutputPort": "output",
					"targetNodeId": "terminal",
					"targetInputPort": "input"
				}
			],
			"metadata": {}
		},
		"initialVariables": {
			"requestValue": "hello"
		}
	}`
}

func TestAPIErrorStringFormatting(
	t *testing.T,
) {
	var nilError *APIError

	if nilError.Error() !=
		"api error" {

		t.Fatalf(
			"nil Error() = %q",
			nilError.Error(),
		)
	}

	tests :=
		[]struct {
			apiError APIError
			expected string
		}{
			{
				apiError: APIError{
					Code: "CODE",

					Message: "Message",
				},

				expected: "CODE: Message",
			},
			{
				apiError: APIError{
					Message: "Message",
				},

				expected: "Message",
			},
			{
				apiError: APIError{
					Code: "CODE",
				},

				expected: "CODE",
			},
		}

	for _, test := range tests {

		if actual :=
			test.apiError.
				Error(); actual !=
			test.expected {

			t.Fatalf(
				"Error() = %q, want %q",
				actual,
				test.expected,
			)
		}
	}
}

func TestRegistryPluginCatalogLenUsesRegistrySize(
	t *testing.T,
) {
	descriptors, err :=
		core.CoreDescriptors()

	if err != nil {
		t.Fatalf(
			"CoreDescriptors() returned an unexpected error: %v",
			err,
		)
	}

	registry, err :=
		plugin.NewRegistry(
			descriptors,
		)

	if err != nil {
		t.Fatalf(
			"NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	catalog :=
		NewRegistryPluginCatalog(
			registry,
		)

	if catalog.Len() !=
		4 {

		t.Fatalf(
			"catalog length = %d, want 4",
			catalog.Len(),
		)
	}
}

func TestContextAccessorsRejectMissingValues(
	t *testing.T,
) {
	ctx :=
		context.Background()

	if _, exists :=
		RequestIDFromContext(
			ctx,
		); exists {

		t.Fatal(
			"empty context contains request ID",
		)
	}

	if _, exists :=
		CorrelationIDFromContext(
			ctx,
		); exists {

		t.Fatal(
			"empty context contains correlation ID",
		)
	}

	if isInternalAuthenticated(
		ctx,
	) {
		t.Fatal(
			"empty context is internally authenticated",
		)
	}

	if _, exists :=
		CompanyIDFromContext(
			ctx,
		); exists {

		t.Fatal(
			"empty context contains company ID",
		)
	}
}

func TestHealthReturnsServiceStatus(
	t *testing.T,
) {
	router := newTestRouter()

	request := httptest.NewRequest(
		http.MethodGet,
		"/health",
		nil,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusOK {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}

	if actual :=
		response.Header().
			Get("Content-Type"); actual != "application/json" {

		t.Fatalf(
			"Content-Type = %q, want %q",
			actual,
			"application/json",
		)
	}

	if response.Header().
		Get(HeaderRequestID) == "" {

		t.Fatal(
			"X-Request-ID must be returned",
		)
	}

	if response.Header().
		Get(HeaderCorrelationID) == "" {

		t.Fatal(
			"X-Correlation-ID must be returned",
		)
	}

	var body HealthResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {

		t.Fatalf(
			"response body is not valid JSON: %v",
			err,
		)
	}

	expected := HealthResponse{
		Status:  "UP",
		Service: "miletos-go",
		Version: "development",
	}

	if body != expected {
		t.Fatalf(
			"response body = %#v, want %#v",
			body,
			expected,
		)
	}
}

func TestHealthRejectsUnsupportedMethod(
	t *testing.T,
) {
	router := newTestRouter()

	request := httptest.NewRequest(
		http.MethodPost,
		"/health",
		nil,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusMethodNotAllowed {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusMethodNotAllowed,
		)
	}

	if actual :=
		response.Header().
			Get("Allow"); actual != http.MethodGet {

		t.Fatalf(
			"Allow header = %q, want %q",
			actual,
			http.MethodGet,
		)
	}

	body := decodeAPIErrorForTest(
		t,
		response,
	)

	if body.Code !=
		errorCodeMethodNotAllowed {

		t.Fatalf(
			"error code = %q, want %q",
			body.Code,
			errorCodeMethodNotAllowed,
		)
	}
}

func TestUnknownRouteReturnsNotFound(
	t *testing.T,
) {
	router := newTestRouter()

	request := httptest.NewRequest(
		http.MethodGet,
		"/unknown",
		nil,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusNotFound {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusNotFound,
		)
	}

	body := decodeAPIErrorForTest(
		t,
		response,
	)

	if body.Code !=
		errorCodeNotFound {

		t.Fatalf(
			"error code = %q, want %q",
			body.Code,
			errorCodeNotFound,
		)
	}
}

func TestHealthSubpathReturnsNotFound(
	t *testing.T,
) {
	router := newTestRouter()

	request := httptest.NewRequest(
		http.MethodGet,
		"/health/details",
		nil,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusNotFound {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusNotFound,
		)
	}
}

func newTestRouter() http.Handler {
	logger := slog.New(
		slog.NewTextHandler(
			io.Discard,
			nil,
		),
	)

	return NewRouterWithOptions(
		NewHandler(
			"miletos-go",
			"development",
		),

		RouterOptions{
			Logger: logger,

			HandlerTimeout: time.Second,
		},
	)
}

func decodeAPIErrorForTest(
	t *testing.T,
	response *httptest.ResponseRecorder,
) APIErrorResponse {
	t.Helper()

	var body APIErrorResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {

		t.Fatalf(
			"response body is not valid JSON: %v",
			err,
		)
	}

	if body.Timestamp == "" {
		t.Fatal(
			"error response timestamp must not be empty",
		)
	}

	if body.Status !=
		response.Code {

		t.Fatalf(
			"error response status = %d, want %d",
			body.Status,
			response.Code,
		)
	}

	if body.RequestID == "" {
		t.Fatal(
			"error response requestId must not be empty",
		)
	}

	if body.CorrelationID == "" {
		t.Fatal(
			"error response correlationId must not be empty",
		)
	}

	return body
}

const testInternalServiceToken = "0123456789abcdef0123456789abcdef"

func TestInternalAuthenticationAndTrustedCompanyContext(
	t *testing.T,
) {
	authenticator, err :=
		NewInternalAuthenticator(
			testInternalServiceToken,
		)

	if err != nil {
		t.Fatalf(
			"create authenticator: %v",
			err,
		)
	}

	finalHandler := http.HandlerFunc(
		func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			companyID, exists :=
				CompanyIDFromContext(
					request.Context(),
				)

			if !exists {
				t.Fatal(
					"company ID must exist in trusted context",
				)
			}

			if companyID.String() !=
				"550e8400-e29b-41d4-a716-446655440000" {

				t.Fatalf(
					"company ID = %q",
					companyID.String(),
				)
			}

			writer.WriteHeader(
				http.StatusNoContent,
			)
		},
	)

	handler := Chain(
		finalHandler,

		RequestIDMiddleware(),

		authenticator.Middleware,

		TrustedCompanyContextMiddleware(),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/protected",
		nil,
	)

	request.Header.Set(
		"Authorization",
		"Bearer "+testInternalServiceToken,
	)

	request.Header.Set(
		HeaderCompanyID,
		"550e8400-e29b-41d4-a716-446655440000",
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusNoContent {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusNoContent,
		)
	}
}

func TestInternalAuthenticationRejectsInvalidToken(
	t *testing.T,
) {
	authenticator, err :=
		NewInternalAuthenticator(
			testInternalServiceToken,
		)

	if err != nil {
		t.Fatalf(
			"create authenticator: %v",
			err,
		)
	}

	finalHandler := http.HandlerFunc(
		func(
			writer http.ResponseWriter,
			_ *http.Request,
		) {
			writer.WriteHeader(
				http.StatusNoContent,
			)
		},
	)

	handler := Chain(
		finalHandler,

		RequestIDMiddleware(),

		CorrelationIDMiddleware(),

		authenticator.Middleware,
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/protected",
		nil,
	)

	request.Header.Set(
		"Authorization",
		"Bearer wrong-token",
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusUnauthorized {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusUnauthorized,
		)
	}

	var body APIErrorResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {

		t.Fatalf(
			"decode error response: %v",
			err,
		)
	}

	if body.Code !=
		errorCodeUnauthorized {

		t.Fatalf(
			"error code = %q, want %q",
			body.Code,
			errorCodeUnauthorized,
		)
	}

	if actual :=
		response.Header().
			Get("WWW-Authenticate"); actual != "Bearer" {

		t.Fatalf(
			"WWW-Authenticate = %q, want %q",
			actual,
			"Bearer",
		)
	}
}

func TestTrustedCompanyContextRequiresAuthentication(
	t *testing.T,
) {
	handler := Chain(
		http.HandlerFunc(
			func(
				writer http.ResponseWriter,
				_ *http.Request,
			) {
				writer.WriteHeader(
					http.StatusNoContent,
				)
			},
		),

		RequestIDMiddleware(),

		CorrelationIDMiddleware(),

		TrustedCompanyContextMiddleware(),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/protected",
		nil,
	)

	request.Header.Set(
		HeaderCompanyID,
		"550e8400-e29b-41d4-a716-446655440000",
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusUnauthorized {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusUnauthorized,
		)
	}
}

func TestTrustedCompanyContextRequiresCompanyHeader(
	t *testing.T,
) {
	authenticator, err :=
		NewInternalAuthenticator(
			testInternalServiceToken,
		)

	if err != nil {
		t.Fatalf(
			"create authenticator: %v",
			err,
		)
	}

	handler := Chain(
		http.HandlerFunc(
			func(
				writer http.ResponseWriter,
				_ *http.Request,
			) {
				writer.WriteHeader(
					http.StatusNoContent,
				)
			},
		),

		RequestIDMiddleware(),

		CorrelationIDMiddleware(),

		authenticator.Middleware,

		TrustedCompanyContextMiddleware(),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/protected",
		nil,
	)

	request.Header.Set(
		"Authorization",
		"Bearer "+testInternalServiceToken,
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusBadRequest {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusBadRequest,
		)
	}

	var body APIErrorResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {

		t.Fatalf(
			"decode error response: %v",
			err,
		)
	}

	if body.Code !=
		errorCodeMissingCompanyContext {

		t.Fatalf(
			"error code = %q, want %q",
			body.Code,
			errorCodeMissingCompanyContext,
		)
	}
}

func TestRecoveryReturnsStructuredInternalError(
	t *testing.T,
) {
	logger := slog.New(
		slog.NewTextHandler(
			io.Discard,
			nil,
		),
	)

	handler := Chain(
		http.HandlerFunc(
			func(
				http.ResponseWriter,
				*http.Request,
			) {
				panic(
					"test panic",
				)
			},
		),

		RecoveryMiddleware(
			logger,
		),

		RequestIDMiddleware(),

		CorrelationIDMiddleware(),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/panic",
		nil,
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusInternalServerError {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusInternalServerError,
		)
	}

	var body APIErrorResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {

		t.Fatalf(
			"decode error response: %v",
			err,
		)
	}

	if body.Code !=
		errorCodeInternalServerError {

		t.Fatalf(
			"error code = %q, want %q",
			body.Code,
			errorCodeInternalServerError,
		)
	}

	if strings.Contains(
		body.Message,
		"test panic",
	) {
		t.Fatal(
			"panic detail must not leak into API response",
		)
	}
}

func TestTracingAndSecurityHeaders(
	t *testing.T,
) {
	handler := Chain(
		http.HandlerFunc(
			func(
				writer http.ResponseWriter,
				_ *http.Request,
			) {
				writer.WriteHeader(
					http.StatusNoContent,
				)
			},
		),

		RequestIDMiddleware(),

		SecurityHeadersMiddleware(),

		CorrelationIDMiddleware(),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/headers",
		nil,
	)

	request.Header.Set(
		HeaderRequestID,
		"request-123",
	)

	request.Header.Set(
		HeaderCorrelationID,
		"correlation-123",
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		request,
	)

	if actual :=
		response.Header().
			Get(HeaderRequestID); actual != "request-123" {

		t.Fatalf(
			"request ID = %q",
			actual,
		)
	}

	if actual :=
		response.Header().
			Get(HeaderCorrelationID); actual != "correlation-123" {

		t.Fatalf(
			"correlation ID = %q",
			actual,
		)
	}

	if actual :=
		response.Header().
			Get("X-Content-Type-Options"); actual != "nosniff" {

		t.Fatalf(
			"X-Content-Type-Options = %q",
			actual,
		)
	}

	if actual :=
		response.Header().
			Get("X-Frame-Options"); actual != "DENY" {

		t.Fatalf(
			"X-Frame-Options = %q",
			actual,
		)
	}
}

func TestOpenAPIDocumentMatchesCheckedInSpec(
	t *testing.T,
) {
	checkedIn, err :=
		os.ReadFile(
			"openapi.json",
		)

	if err != nil {
		t.Fatalf(
			"os.ReadFile() returned an error: %v",
			err,
		)
	}

	if !bytes.Equal(
		bytes.TrimSpace(
			checkedIn,
		),
		bytes.TrimSpace(
			openapi.DocumentBytes(),
		),
	) {
		t.Fatal(
			"runtime OpenAPI document drifted from checked-in openapi.json",
		)
	}
}

func TestOpenAPIDocumentContainsRequiredSurface(
	t *testing.T,
) {
	var document struct {
		OpenAPI string `json:"openapi"`

		Paths map[string]json.RawMessage `json:"paths"`

		Components struct {
			SecuritySchemes map[string]json.RawMessage `json:"securitySchemes"`
		} `json:"components"`
	}

	if err :=
		json.Unmarshal(
			openapi.DocumentBytes(),
			&document,
		); err != nil {

		t.Fatalf(
			"json.Unmarshal() returned an error: %v",
			err,
		)
	}

	if document.OpenAPI !=
		"3.0.3" {

		t.Fatalf(
			"OpenAPI version = %q, want 3.0.3",
			document.OpenAPI,
		)
	}

	requiredPaths :=
		[]string{
			"/health",
			"/ready",
			"/openapi.json",
			"/swagger/",
			"/api/v1/plugins",
			"/api/v1/executions/sync",
			"/api/v1/executions/async",
			"/api/v1/executions",
			"/api/v1/executions/{executionId}",
			"/api/v1/executions/{executionId}/definition",
			"/api/v1/executions/{executionId}/nodes",
			"/api/v1/executions/{executionId}/events",
			"/api/v1/executions/{executionId}/logs",
			"/api/v1/executions/{executionId}/errors",
		}

	for _, path := range requiredPaths {

		if _, exists :=
			document.Paths[path]; !exists {

			t.Fatalf(
				"OpenAPI path %q is missing",
				path,
			)
		}
	}

	if _, exists :=
		document.
			Components.
			SecuritySchemes["bearerAuth"]; !exists {

		t.Fatal(
			"bearerAuth security scheme is missing",
		)
	}

	body :=
		string(
			openapi.DocumentBytes(),
		)

	for _, forbidden := range []string{
		"technicalDetail",
		"technical_detail",
		"password=secret-value",
	} {

		if strings.Contains(
			body,
			forbidden,
		) {
			t.Fatalf(
				"OpenAPI document contains forbidden token %q",
				forbidden,
			)
		}
	}
}

func TestOpenAPIRouteIsAlwaysAvailable(
	t *testing.T,
) {
	router :=
		NewRouter(
			NewHandler(
				"miletos-go",
				"development",
			),
		)

	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/openapi.json",
			nil,
		)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusOK {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}

	if actual :=
		response.Header().
			Get(
				"Content-Type",
			); actual != "application/json" {

		t.Fatalf(
			"Content-Type = %q, want application/json",
			actual,
		)
	}

	if !bytes.Equal(
		bytes.TrimSpace(
			response.Body.Bytes(),
		),
		bytes.TrimSpace(
			openapi.DocumentBytes(),
		),
	) {
		t.Fatal(
			"/openapi.json did not serve the authoritative OpenAPI document",
		)
	}
}

func TestSwaggerIsDisabledByDefault(
	t *testing.T,
) {
	router :=
		NewRouter(
			NewHandler(
				"miletos-go",
				"development",
			),
		)

	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/swagger/",
			nil,
		)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusNotFound {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusNotFound,
		)
	}
}

func TestSwaggerUIIsAvailableWhenEnabled(
	t *testing.T,
) {
	router :=
		NewRouterWithOptions(
			NewHandler(
				"miletos-go",
				"development",
			),
			RouterOptions{
				SwaggerEnabled: true,

				HandlerTimeout: time.Second,
			},
		)

	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/swagger/",
			nil,
		)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusOK {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}

	body :=
		response.Body.String()

	for _, required := range []string{
		"SwaggerUIBundle",
		"/openapi.json",
		"swagger-ui",
	} {

		if !strings.Contains(
			body,
			required,
		) {
			t.Fatalf(
				"Swagger HTML does not contain %q",
				required,
			)
		}
	}
}

func TestSwaggerRootRedirectsToTrailingSlash(
	t *testing.T,
) {
	router :=
		NewRouterWithOptions(
			NewHandler(
				"miletos-go",
				"development",
			),
			RouterOptions{
				SwaggerEnabled: true,

				HandlerTimeout: time.Second,
			},
		)

	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/swagger",
			nil,
		)

	response :=
		httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusPermanentRedirect {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusPermanentRedirect,
		)
	}

	if actual :=
		response.Header().
			Get(
				"Location",
			); actual != "/swagger/" {

		t.Fatalf(
			"Location = %q, want /swagger/",
			actual,
		)
	}
}

type RegistryPluginCatalog = plugin.Registry

var NewRegistryPluginCatalog = func(registry plugin.Registry) plugin.Registry { return registry }

func TestPluginsReturnsRegisteredDescriptors(
	t *testing.T,
) {
	catalog := newTestPluginCatalog(
		t,
	)

	authenticator, err :=
		NewInternalAuthenticator(
			testInternalServiceToken,
		)

	if err != nil {
		t.Fatalf(
			"create authenticator: %v",
			err,
		)
	}

	handler := NewHandler(
		"miletos-go",
		"development",
	)

	router := NewRouterWithOptions(
		handler,
		RouterOptions{
			PluginHandler: NewPluginHandler(catalog),
			Logger: slog.New(
				slog.NewTextHandler(
					io.Discard,
					nil,
				),
			),
			HandlerTimeout: time.Second,
			Authenticator:  &authenticator,
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/plugins",
		nil,
	)

	request.Header.Set(
		"Authorization",
		"Bearer "+testInternalServiceToken,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}

	var body PluginListResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {
		t.Fatalf(
			"decode plugin response: %v",
			err,
		)
	}

	if body.Count != 4 {
		t.Fatalf(
			"plugin count = %d, want %d",
			body.Count,
			4,
		)
	}

	if len(body.Items) != 4 {
		t.Fatalf(
			"plugin items = %d, want %d",
			len(body.Items),
			4,
		)
	}

	expected := []string{
		"core.delay@v1",
		"core.pass-through@v1",
		"core.static-input@v1",
		"core.terminal@v1",
	}

	for index, item := range body.Items {
		actual :=
			item.Type +
				"@" +
				item.Version

		if actual != expected[index] {
			t.Fatalf(
				"plugin identity at index %d = %q, want %q",
				index,
				actual,
				expected[index],
			)
		}

		if item.DisplayName == "" {
			t.Fatalf(
				"plugin display name at index %d must not be empty",
				index,
			)
		}

		if item.Distribution == "" {
			t.Fatalf(
				"plugin distribution at index %d must not be empty",
				index,
			)
		}
	}
}

func TestPluginsRequiresInternalAuthentication(
	t *testing.T,
) {
	catalog := newTestPluginCatalog(
		t,
	)

	authenticator, err :=
		NewInternalAuthenticator(
			testInternalServiceToken,
		)

	if err != nil {
		t.Fatalf(
			"create authenticator: %v",
			err,
		)
	}

	handler := NewHandler(
		"miletos-go",
		"development",
	)

	router := NewRouterWithOptions(
		handler,
		RouterOptions{
			PluginHandler: NewPluginHandler(catalog),
			Authenticator: &authenticator,
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/plugins",
		nil,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusUnauthorized {
		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusUnauthorized,
		)
	}

	var body APIErrorResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {
		t.Fatalf(
			"decode error response: %v",
			err,
		)
	}

	if body.Code != errorCodeUnauthorized {
		t.Fatalf(
			"error code = %q, want %q",
			body.Code,
			errorCodeUnauthorized,
		)
	}

	if body.RequestID == "" {
		t.Fatal(
			"requestId must not be empty",
		)
	}
}

func TestPluginsRejectsUnsupportedMethod(
	t *testing.T,
) {
	catalog := newTestPluginCatalog(
		t,
	)

	authenticator, err :=
		NewInternalAuthenticator(
			testInternalServiceToken,
		)

	if err != nil {
		t.Fatalf(
			"create authenticator: %v",
			err,
		)
	}

	handler := NewHandler(
		"miletos-go",
		"development",
	)

	router := NewRouterWithOptions(
		handler,
		RouterOptions{
			PluginHandler: NewPluginHandler(catalog),
			Authenticator: &authenticator,
		},
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/plugins",
		nil,
	)

	request.Header.Set(
		"Authorization",
		"Bearer "+testInternalServiceToken,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusMethodNotAllowed {
		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusMethodNotAllowed,
		)
	}

	if actual :=
		response.Header().
			Get("Allow"); actual != http.MethodGet {
		t.Fatalf(
			"Allow header = %q, want %q",
			actual,
			http.MethodGet,
		)
	}
}

func TestPluginsReturnsServiceUnavailableWithoutCatalog(
	t *testing.T,
) {
	authenticator, err :=
		NewInternalAuthenticator(
			testInternalServiceToken,
		)

	if err != nil {
		t.Fatalf(
			"create authenticator: %v",
			err,
		)
	}

	handler := NewHandler(
		"miletos-go",
		"development",
	)

	router := NewRouterWithOptions(
		handler,
		RouterOptions{
			PluginHandler: NewPluginHandler(nil),
			Authenticator: &authenticator,
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/plugins",
		nil,
	)

	request.Header.Set(
		"Authorization",
		"Bearer "+testInternalServiceToken,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusServiceUnavailable {
		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusServiceUnavailable,
		)
	}
}

func newTestPluginCatalog(
	t *testing.T,
) plugin.Registry {
	t.Helper()

	descriptors, err :=
		core.CoreDescriptors()

	if err != nil {
		t.Fatalf(
			"create core descriptors: %v",
			err,
		)
	}

	registry, err :=
		plugin.NewRegistry(
			descriptors,
		)

	if err != nil {
		t.Fatalf(
			"create plugin registry: %v",
			err,
		)
	}

	return NewRegistryPluginCatalog(
		registry,
	)
}

type readinessProbeStub struct {
	err error
}

func (
	stub readinessProbeStub,
) Ping(
	context.Context,
) error {
	return stub.err
}

type readinessPluginCatalogStub struct {
	count int
}

func (
	stub readinessPluginCatalogStub,
) Len() int {
	return stub.count
}

func TestReadyReturnsOKWhenRequiredDependenciesAreUp(
	t *testing.T,
) {
	service := healthfeature.NewReadinessService(
		readinessProbeStub{},
		readinessPluginCatalogStub{
			count: 4,
		},
		readinessProbeStub{},
		true,
	)

	handler := NewHandler(
		"miletos-go",
		"development",
	)

	router := NewRouterWithOptions(
		handler,
		RouterOptions{
			HealthHandler: NewHealthHandler(service),
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/ready",
		nil,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}

	var body healthfeature.ReadinessReport

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {
		t.Fatalf(
			"decode readiness response: %v",
			err,
		)
	}

	if body.Status != healthfeature.StatusReady {
		t.Fatalf(
			"readiness status = %q, want %q",
			body.Status,
			healthfeature.StatusReady,
		)
	}

	if len(body.Checks) != 3 {
		t.Fatalf(
			"readiness checks = %d, want %d",
			len(body.Checks),
			3,
		)
	}

	assertReadinessCheck(
		t,
		body.Checks[0],
		healthfeature.CheckPostgreSQL,
		healthfeature.CheckStatusUp,
	)

	assertReadinessCheck(
		t,
		body.Checks[1],
		healthfeature.CheckPlugins,
		healthfeature.CheckStatusUp,
	)

	assertReadinessCheck(
		t,
		body.Checks[2],
		healthfeature.CheckKafka,
		healthfeature.CheckStatusUp,
	)
}

func TestReadyReturnsServiceUnavailableWhenPostgreSQLIsDown(
	t *testing.T,
) {
	service := healthfeature.NewReadinessService(
		readinessProbeStub{
			err: errors.New(
				"database unavailable",
			),
		},
		readinessPluginCatalogStub{
			count: 4,
		},
		readinessProbeStub{},
		true,
	)

	handler := NewHandler(
		"miletos-go",
		"development",
	)

	router := NewRouterWithOptions(
		handler,
		RouterOptions{
			HealthHandler: NewHealthHandler(service),
		},
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/ready",
		nil,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusServiceUnavailable {
		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusServiceUnavailable,
		)
	}

	var body healthfeature.ReadinessReport

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {
		t.Fatalf(
			"decode readiness response: %v",
			err,
		)
	}

	if body.Status !=
		healthfeature.StatusNotReady {
		t.Fatalf(
			"readiness status = %q, want %q",
			body.Status,
			healthfeature.StatusNotReady,
		)
	}

	assertReadinessCheck(
		t,
		body.Checks[0],
		healthfeature.CheckPostgreSQL,
		healthfeature.CheckStatusDown,
	)
}

func TestReadinessDoesNotRequireKafkaWhenAsyncIsDisabled(
	t *testing.T,
) {
	service := healthfeature.NewReadinessService(
		readinessProbeStub{},
		readinessPluginCatalogStub{
			count: 4,
		},
		nil,
		false,
	)

	report := service.Check(
		context.Background(),
	)

	if report.Status !=
		healthfeature.StatusReady {
		t.Fatalf(
			"readiness status = %q, want %q",
			report.Status,
			healthfeature.StatusReady,
		)
	}

	assertReadinessCheck(
		t,
		report.Checks[2],
		healthfeature.CheckKafka,
		healthfeature.CheckStatusDisabled,
	)
}

func TestReadinessRequiresAtLeastOnePlugin(
	t *testing.T,
) {
	service := healthfeature.NewReadinessService(
		readinessProbeStub{},
		readinessPluginCatalogStub{},
		nil,
		false,
	)

	report := service.Check(
		context.Background(),
	)

	if report.Status !=
		healthfeature.StatusNotReady {
		t.Fatalf(
			"readiness status = %q, want %q",
			report.Status,
			healthfeature.StatusNotReady,
		)
	}

	assertReadinessCheck(
		t,
		report.Checks[1],
		healthfeature.CheckPlugins,
		healthfeature.CheckStatusDown,
	)
}

func TestReadyRejectsUnsupportedMethod(
	t *testing.T,
) {
	service := healthfeature.NewReadinessService(
		readinessProbeStub{},
		readinessPluginCatalogStub{
			count: 4,
		},
		nil,
		false,
	)

	handler := NewHandler(
		"miletos-go",
		"development",
	)

	router := NewRouterWithOptions(
		handler,
		RouterOptions{
			HealthHandler: NewHealthHandler(service),
		},
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/ready",
		nil,
	)

	response := httptest.NewRecorder()

	router.ServeHTTP(
		response,
		request,
	)

	if response.Code !=
		http.StatusMethodNotAllowed {
		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusMethodNotAllowed,
		)
	}
}

func assertReadinessCheck(
	t *testing.T,
	actual healthfeature.ReadinessCheck,
	expectedName string,
	expectedStatus string,
) {
	t.Helper()

	if actual.Name != expectedName {
		t.Fatalf(
			"check name = %q, want %q",
			actual.Name,
			expectedName,
		)
	}

	if actual.Status != expectedStatus {
		t.Fatalf(
			"check status = %q, want %q",
			actual.Status,
			expectedStatus,
		)
	}
}

type strictJSONTestRequest struct {
	Name string `json:"name"`
}

func TestDecodeJSONBodyAcceptsValidDocument(
	t *testing.T,
) {
	response := executeStrictJSONRequest(
		t,
		`{"name":"miletos"}`,
		"application/json",
		1024,
	)

	if response.Code !=
		http.StatusNoContent {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			http.StatusNoContent,
		)
	}
}

func TestDecodeJSONBodyRejectsUnknownField(
	t *testing.T,
) {
	response := executeStrictJSONRequest(
		t,
		`{"name":"miletos","unknown":true}`,
		"application/json",
		1024,
	)

	assertAPIErrorCode(
		t,
		response,
		http.StatusBadRequest,
		errorCodeInvalidJSON,
	)
}

func TestDecodeJSONBodyRejectsMalformedJSON(
	t *testing.T,
) {
	response := executeStrictJSONRequest(
		t,
		`{"name":`,
		"application/json",
		1024,
	)

	assertAPIErrorCode(
		t,
		response,
		http.StatusBadRequest,
		errorCodeInvalidJSON,
	)
}

func TestDecodeJSONBodyRejectsMultipleDocuments(
	t *testing.T,
) {
	response := executeStrictJSONRequest(
		t,
		`{"name":"one"} {"name":"two"}`,
		"application/json",
		1024,
	)

	assertAPIErrorCode(
		t,
		response,
		http.StatusBadRequest,
		errorCodeInvalidJSON,
	)
}

func TestDecodeJSONBodyRejectsEmptyBody(
	t *testing.T,
) {
	response := executeStrictJSONRequest(
		t,
		"",
		"application/json",
		1024,
	)

	assertAPIErrorCode(
		t,
		response,
		http.StatusBadRequest,
		errorCodeEmptyRequestBody,
	)
}

func TestDecodeJSONBodyRejectsUnsupportedMediaType(
	t *testing.T,
) {
	response := executeStrictJSONRequest(
		t,
		`{"name":"miletos"}`,
		"text/plain",
		1024,
	)

	assertAPIErrorCode(
		t,
		response,
		http.StatusUnsupportedMediaType,
		errorCodeUnsupportedMediaType,
	)
}

func TestDecodeJSONBodyRejectsOversizedBody(
	t *testing.T,
) {
	body := `{"name":"` +
		strings.Repeat(
			"x",
			128,
		) +
		`"}`

	response := executeStrictJSONRequest(
		t,
		body,
		"application/json",
		32,
	)

	assertAPIErrorCode(
		t,
		response,
		http.StatusRequestEntityTooLarge,
		errorCodePayloadTooLarge,
	)
}

func executeStrictJSONRequest(
	t *testing.T,
	body string,
	contentType string,
	maximumBytes int64,
) *httptest.ResponseRecorder {
	t.Helper()

	finalHandler := http.HandlerFunc(
		func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			var payload strictJSONTestRequest

			if apiError := decodeJSONBody(
				writer,
				request,
				&payload,
			); apiError != nil {

				writeAPIError(
					writer,
					request,
					apiError,
				)

				return
			}

			writer.WriteHeader(
				http.StatusNoContent,
			)
		},
	)

	handler := Chain(
		finalHandler,

		RequestIDMiddleware(),

		CorrelationIDMiddleware(),

		BodyLimitMiddleware(
			maximumBytes,
		),
	)

	request := httptest.NewRequest(
		http.MethodPost,
		"/json",
		strings.NewReader(body),
	)

	if contentType != "" {
		request.Header.Set(
			"Content-Type",
			contentType,
		)
	}

	response := httptest.NewRecorder()

	handler.ServeHTTP(
		response,
		request,
	)

	return response
}

func assertAPIErrorCode(
	t *testing.T,
	response *httptest.ResponseRecorder,
	expectedStatus int,
	expectedCode string,
) {
	t.Helper()

	if response.Code !=
		expectedStatus {

		t.Fatalf(
			"status code = %d, want %d",
			response.Code,
			expectedStatus,
		)
	}

	var body APIErrorResponse

	if err := json.NewDecoder(
		response.Body,
	).Decode(
		&body,
	); err != nil {

		t.Fatalf(
			"decode error response: %v",
			err,
		)
	}

	if body.Code !=
		expectedCode {

		t.Fatalf(
			"error code = %q, want %q",
			body.Code,
			expectedCode,
		)
	}

	if body.RequestID == "" {
		t.Fatal(
			"requestId must not be empty",
		)
	}

	if body.CorrelationID == "" {
		t.Fatal(
			"correlationId must not be empty",
		)
	}
}
