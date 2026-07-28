package executionhttp

import (
	context "context"
	json "encoding/json"
	errors "errors"
	math "math"
	engine "miletos-go/internal/engine"
	core "miletos-go/internal/engine/nodes/core"
	plugin "miletos-go/internal/engine/plugin"
	runtime "miletos-go/internal/engine/runtime"
	execution "miletos-go/internal/features/execution"
	executionfeature "miletos-go/internal/features/execution/application"
	workflow "miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
	sharedclock "miletos-go/internal/shared/clock"
	http "net/http"
	httptest "net/http/httptest"
	strings "strings"
	testing "testing"
)

func TestEngineExecutionApplicationRunsSyncExecution(
	t *testing.T,
) {
	application :=
		mustExecutionApplicationForTest(
			t,
			true,
		)

	if application.AsyncEnabled() {
		t.Fatal(
			"sync-only application reports async capability",
		)
	}

	request :=
		mustMappedSyncExecutionRequestForTest(
			t,
			validExecutionApplicationBody(),
		)

	outcome, err :=
		application.Execute(
			context.Background(),
			request,
		)

	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if outcome.Rejected {
		t.Fatal(
			"successful workflow was rejected",
		)
	}

	if outcome.ExecutionID !=
		request.
			WorkflowExecutionID().
			String() {

		t.Fatalf(
			"execution ID = %q, want %q",
			outcome.ExecutionID,
			request.
				WorkflowExecutionID().
				String(),
		)
	}

	if outcome.WorkflowID !=
		"workflow-application-success" {

		t.Fatalf(
			"workflow ID = %q",
			outcome.WorkflowID,
		)
	}

	if outcome.WorkflowRevision != 1 {
		t.Fatalf(
			"workflow revision = %d, want 1",
			outcome.WorkflowRevision,
		)
	}

	if outcome.Mode !=
		execution.
			ExecutionModeSync.
			String() {

		t.Fatalf(
			"mode = %q, want %q",
			outcome.Mode,
			execution.
				ExecutionModeSync.
				String(),
		)
	}

	if outcome.Status !=
		execution.
			WorkflowExecutionStatusSucceeded.
			String() {

		t.Fatalf(
			"status = %q, want %q",
			outcome.Status,
			execution.
				WorkflowExecutionStatusSucceeded.
				String(),
		)
	}

	if outcome.CorrelationID !=
		"corr-application-test" {

		t.Fatalf(
			"correlation ID = %q",
			outcome.CorrelationID,
		)
	}

	if outcome.CreatedAt.IsZero() {
		t.Fatal(
			"createdAt is zero",
		)
	}

	if outcome.StartedAt == nil {
		t.Fatal(
			"startedAt is nil",
		)
	}

	if outcome.FinishedAt == nil {
		t.Fatal(
			"finishedAt is nil",
		)
	}

	if len(
		outcome.ValidationDetails,
	) != 0 {

		t.Fatalf(
			"validation details length = %d, want 0",
			len(
				outcome.
					ValidationDetails,
			),
		)
	}
}

func TestEngineExecutionApplicationMapsPluginValidationRejection(
	t *testing.T,
) {
	application :=
		mustExecutionApplicationForTest(
			t,
			true,
		)

	body :=
		ExecutionRequestBody{
			Definition: WorkflowDefinitionRequest{
				ID: "workflow-missing-plugin",

				Name: "Missing Plugin",

				Revision: 1,

				Nodes: []WorkflowNodeRequest{
					{
						ID: "missing",

						PluginType: "missing.plugin",

						PluginVersion: "v1",

						Configuration: json.RawMessage(
							`{}`,
						),
					},
				},

				Edges: []WorkflowEdgeRequest{},

				Metadata: json.RawMessage(
					`{}`,
				),
			},
		}

	request :=
		mustMappedSyncExecutionRequestForTest(
			t,
			body,
		)

	outcome, err :=
		application.Execute(
			context.Background(),
			request,
		)

	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if !outcome.Rejected {
		t.Fatal(
			"workflow with missing plugin was not rejected",
		)
	}

	if outcome.Status !=
		execution.
			WorkflowExecutionStatusRejected.
			String() {

		t.Fatalf(
			"status = %q, want %q",
			outcome.Status,
			execution.
				WorkflowExecutionStatusRejected.
				String(),
		)
	}

	if len(
		outcome.ValidationDetails,
	) == 0 {

		t.Fatal(
			"missing plugin rejection contains no validation details",
		)
	}

	found :=
		false

	for _, detail := range outcome.ValidationDetails {

		if detail.Code ==
			"PLUGIN_NOT_FOUND" {

			found =
				true

			break
		}
	}

	if !found {
		t.Fatalf(
			"validation details = %#v, want PLUGIN_NOT_FOUND",
			outcome.ValidationDetails,
		)
	}
}

func TestEngineExecutionApplicationMapsStructuralValidationRejection(
	t *testing.T,
) {
	application :=
		mustExecutionApplicationForTest(
			t,
			true,
		)

	body :=
		ExecutionRequestBody{
			Definition: WorkflowDefinitionRequest{
				ID: "workflow-self-loop",

				Name: "Self Loop",

				Revision: 1,

				Nodes: []WorkflowNodeRequest{
					{
						ID: "loop",

						PluginType: "core.pass-through",

						PluginVersion: "v1",

						Configuration: json.RawMessage(
							`{}`,
						),
					},
				},

				Edges: []WorkflowEdgeRequest{
					{
						ID: "self-loop",

						SourceNodeID: "loop",

						SourceOutputPort: "output",

						TargetNodeID: "loop",

						TargetInputPort: "input",
					},
				},

				Metadata: json.RawMessage(
					`{}`,
				),
			},
		}

	request :=
		mustMappedSyncExecutionRequestForTest(
			t,
			body,
		)

	outcome, err :=
		application.Execute(
			context.Background(),
			request,
		)

	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if !outcome.Rejected {
		t.Fatal(
			"self-loop workflow was not rejected",
		)
	}

	if len(
		outcome.ValidationDetails,
	) == 0 {

		t.Fatal(
			"self-loop rejection contains no validation details",
		)
	}

	for _, detail := range outcome.ValidationDetails {

		if detail.Code == "" {
			t.Fatal(
				"structural validation detail contains an empty code",
			)
		}
	}
}

func TestEngineExecutionApplicationMapsMissingExecutorRejection(
	t *testing.T,
) {
	application :=
		mustExecutionApplicationForTest(
			t,
			false,
		)

	request :=
		mustMappedSyncExecutionRequestForTest(
			t,
			validExecutionApplicationBody(),
		)

	outcome, err :=
		application.Execute(
			context.Background(),
			request,
		)

	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if !outcome.Rejected {
		t.Fatal(
			"workflow without registered executors was not rejected",
		)
	}

	found :=
		false

	for _, detail := range outcome.ValidationDetails {

		if detail.Code ==
			engine.
				IssueCodeExecutorNotFound.
				String() {

			found =
				true

			if detail.Reason !=
				"required node executor is unavailable" {

				t.Fatalf(
					"safe executor reason = %q",
					detail.Reason,
				)
			}
		}
	}

	if !found {
		t.Fatalf(
			"validation details = %#v, want %q",
			outcome.ValidationDetails,
			engine.
				IssueCodeExecutorNotFound.
				String(),
		)
	}
}

func TestEngineExecutionApplicationRejectsInvalidConstructionAndInputs(
	t *testing.T,
) {
	_, err :=
		executionfeature.NewEngineExecutionApplication(
			engine.ExecutionService{},
		)

	if err == nil {
		t.Fatal(
			"expected invalid execution service error",
		)
	}

	application :=
		mustExecutionApplicationForTest(
			t,
			true,
		)

	request :=
		mustMappedSyncExecutionRequestForTest(
			t,
			validExecutionApplicationBody(),
		)

	_, err =
		application.Execute(
			nil,
			request,
		)

	if err == nil {
		t.Fatal(
			"expected nil context error",
		)
	}

	_, err =
		application.Execute(
			context.Background(),
			engine.ExecutionRequest{},
		)

	if err == nil {
		t.Fatal(
			"expected invalid request error",
		)
	}

	asyncRequest, err :=
		engine.NewAsyncExecutionRequest(
			request.
				WorkflowExecutionID(),

			request.
				Definition(),

			request.
				CorrelationID(),

			request.
				InitialVariables(),
		)

	if err != nil {
		t.Fatalf(
			"NewAsyncExecutionRequest() returned an unexpected error: %v",
			err,
		)
	}

	_, err =
		application.Execute(
			context.Background(),
			asyncRequest,
		)

	if !errors.Is(
		err,
		engine.ErrAsyncExecutionUnavailable,
	) {
		t.Fatalf(
			"async Execute() error = %v, want %v",
			err,
			engine.ErrAsyncExecutionUnavailable,
		)
	}
}

func mustExecutionApplicationForTest(
	t *testing.T,
	withExecutors bool,
) executionfeature.EngineExecutionApplication {
	t.Helper()

	limits, err :=
		runtime.NewRuntimeLimits(
			100,
			100,
			1<<20,
		)

	if err != nil {
		t.Fatalf(
			"NewRuntimeLimits() returned an unexpected error: %v",
			err,
		)
	}

	descriptors, err :=
		core.CoreDescriptors()

	if err != nil {
		t.Fatalf(
			"CoreDescriptors() returned an unexpected error: %v",
			err,
		)
	}

	plugins, err :=
		plugin.NewRegistry(
			descriptors,
		)

	if err != nil {
		t.Fatalf(
			"NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	var registrations []runtime.ExecutorRegistration

	if withExecutors {
		registrations, err =
			core.DefaultExecutorRegistrations(
				limits,
			)

		if err != nil {
			t.Fatalf(
				"DefaultExecutorRegistrations() returned an unexpected error: %v",
				err,
			)
		}
	}

	executors, err :=
		runtime.NewExecutorRegistry(
			registrations,
		)

	if err != nil {
		t.Fatalf(
			"NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	dependencies, err :=
		engine.NewEngineDependencies(
			plugins,
			executors,
			limits,
			sharedclock.System(),
		)

	if err != nil {
		t.Fatalf(
			"NewEngineDependencies() returned an unexpected error: %v",
			err,
		)
	}

	syncRunner, err :=
		engine.NewSyncRunner(
			dependencies,
		)

	if err != nil {
		t.Fatalf(
			"NewSyncRunner() returned an unexpected error: %v",
			err,
		)
	}

	service, err :=
		engine.NewSyncExecutionService(
			syncRunner,
		)

	if err != nil {
		t.Fatalf(
			"NewSyncExecutionService() returned an unexpected error: %v",
			err,
		)
	}

	application, err :=
		executionfeature.NewEngineExecutionApplication(
			service,
		)

	if err != nil {
		t.Fatalf(
			"NewEngineExecutionApplication() returned an unexpected error: %v",
			err,
		)
	}

	return application
}

func mustMappedSyncExecutionRequestForTest(
	t *testing.T,
	body ExecutionRequestBody,
) engine.ExecutionRequest {
	t.Helper()

	request, apiError :=
		mapSyncExecutionRequest(
			body,
			workflow.CompanyID(
				"company-api-test",
			),
			"corr-application-test",
		)

	if apiError != nil {
		t.Fatalf(
			"mapSyncExecutionRequest() returned API error: %v",
			apiError,
		)
	}

	return request
}

func validExecutionApplicationBody() ExecutionRequestBody {
	return ExecutionRequestBody{
		Definition: WorkflowDefinitionRequest{
			ID: "workflow-application-success",

			Name: "Application Success",

			Revision: 1,

			Nodes: []WorkflowNodeRequest{
				{
					ID: "source",

					PluginType: "core.static-input",

					PluginVersion: "v1",

					Configuration: json.RawMessage(
						`{"value":{"message":"hello"}}`,
					),
				},
				{
					ID: "terminal",

					PluginType: "core.terminal",

					PluginVersion: "v1",

					Configuration: json.RawMessage(
						`{}`,
					),
				},
			},

			Edges: []WorkflowEdgeRequest{
				{
					ID: "edge-1",

					SourceNodeID: "source",

					SourceOutputPort: "output",

					TargetNodeID: "terminal",

					TargetInputPort: "input",
				},
			},

			Metadata: json.RawMessage(
				`{"source":"api-test"}`,
			),
		},

		InitialVariables: map[string]json.RawMessage{
			"requestValue": json.RawMessage(
				`"hello"`,
			),
		},
	}
}

func TestAsyncExecutionFingerprintIsCanonical(
	t *testing.T,
) {
	first :=
		ExecutionRequestBody{
			Definition: WorkflowDefinitionRequest{
				ID:       "workflow-1",
				Name:     "Workflow",
				Revision: 1,

				Nodes: []WorkflowNodeRequest{
					{
						ID: "node-1",

						PluginType: "core.static-input",

						PluginVersion: "1.0.0",

						Configuration: json.RawMessage(
							`{"a":1,"b":2}`,
						),
					},
				},

				Edges: []WorkflowEdgeRequest{},

				Metadata: json.RawMessage(
					`{"x":1,"y":2}`,
				),
			},

			InitialVariables: map[string]json.RawMessage{
				"input": json.RawMessage(
					`{"first":1,"second":2}`,
				),
			},
		}

	second :=
		ExecutionRequestBody{
			Definition: WorkflowDefinitionRequest{
				ID:       "workflow-1",
				Name:     "Workflow",
				Revision: 1,

				Nodes: []WorkflowNodeRequest{
					{
						ID: "node-1",

						PluginType: "core.static-input",

						PluginVersion: "1.0.0",

						Configuration: json.RawMessage(
							`{"b":2,"a":1}`,
						),
					},
				},

				Edges: []WorkflowEdgeRequest{},

				Metadata: json.RawMessage(
					`{"y":2,"x":1}`,
				),
			},

			InitialVariables: map[string]json.RawMessage{
				"input": json.RawMessage(
					`{"second":2,"first":1}`,
				),
			},
		}

	firstFingerprint, apiError :=
		fingerprintAsyncExecutionRequest(
			first,
		)

	if apiError != nil {
		t.Fatalf(
			"first fingerprint error = %v",
			apiError,
		)
	}

	secondFingerprint, apiError :=
		fingerprintAsyncExecutionRequest(
			second,
		)

	if apiError != nil {
		t.Fatalf(
			"second fingerprint error = %v",
			apiError,
		)
	}

	if firstFingerprint !=
		secondFingerprint {

		t.Fatalf(
			"fingerprints differ: %q != %q",
			firstFingerprint,
			secondFingerprint,
		)
	}

	if len(firstFingerprint) != 64 {
		t.Fatalf(
			"fingerprint length = %d, want 64",
			len(firstFingerprint),
		)
	}
}

func TestAsyncExecutionFingerprintChangesForDifferentRequest(
	t *testing.T,
) {
	first :=
		ExecutionRequestBody{
			Definition: WorkflowDefinitionRequest{
				ID:       "workflow-1",
				Name:     "Workflow",
				Revision: 1,

				Nodes: []WorkflowNodeRequest{
					{
						ID: "node-1",

						PluginType: "core.static-input",

						PluginVersion: "1.0.0",

						Configuration: json.RawMessage(
							`{"value":"first"}`,
						),
					},
				},

				Edges: []WorkflowEdgeRequest{},
			},
		}

	second :=
		ExecutionRequestBody{
			Definition: WorkflowDefinitionRequest{
				ID:       "workflow-1",
				Name:     "Workflow",
				Revision: 1,

				Nodes: []WorkflowNodeRequest{
					{
						ID: "node-1",

						PluginType: "core.static-input",

						PluginVersion: "1.0.0",

						Configuration: json.RawMessage(
							`{"value":"second"}`,
						),
					},
				},

				Edges: []WorkflowEdgeRequest{},
			},
		}

	firstFingerprint, apiError :=
		fingerprintAsyncExecutionRequest(
			first,
		)

	if apiError != nil {
		t.Fatalf(
			"first fingerprint error = %v",
			apiError,
		)
	}

	secondFingerprint, apiError :=
		fingerprintAsyncExecutionRequest(
			second,
		)

	if apiError != nil {
		t.Fatalf(
			"second fingerprint error = %v",
			apiError,
		)
	}

	if firstFingerprint ==
		secondFingerprint {

		t.Fatalf(
			"different requests produced same fingerprint %q",
			firstFingerprint,
		)
	}
}

func TestMapAsyncExecutionRequestUsesAsyncMode(
	t *testing.T,
) {
	body :=
		ExecutionRequestBody{
			Definition: WorkflowDefinitionRequest{
				ID:       "workflow-1",
				Name:     "Workflow",
				Revision: 1,

				Nodes: []WorkflowNodeRequest{
					{
						ID: "node-1",

						PluginType: "core.static-input",

						PluginVersion: "1.0.0",

						Configuration: json.RawMessage(
							`{"value":"hello"}`,
						),
					},
				},

				Edges: []WorkflowEdgeRequest{},
			},
		}

	request, apiError :=
		mapAsyncExecutionRequest(
			body,
			workflow.CompanyID(
				"company-1",
			),
			"correlation-1",
		)

	if apiError != nil {
		t.Fatalf(
			"mapAsyncExecutionRequest() error = %v",
			apiError,
		)
	}

	if request.Mode() !=
		execution.ExecutionModeAsync {

		t.Fatalf(
			"mode = %q, want %q",
			request.Mode(),
			execution.ExecutionModeAsync,
		)
	}

	if request.Definition().
		CompanyID().
		String() !=
		"company-1" {

		t.Fatalf(
			"company ID = %q, want %q",
			request.
				Definition().
				CompanyID(),
			"company-1",
		)
	}

	if request.CorrelationID() !=
		"correlation-1" {

		t.Fatalf(
			"correlation ID = %q, want %q",
			request.CorrelationID(),
			"correlation-1",
		)
	}

	if request.WorkflowExecutionID().
		String() == "" {

		t.Fatal(
			"workflow execution ID is empty",
		)
	}
}

func TestReadIdempotencyKey(
	t *testing.T,
) {
	tests :=
		[]struct {
			name       string
			values     []string
			want       string
			wantStatus int
		}{
			{
				name: "valid",

				values: []string{
					" request-123 ",
				},

				want: "request-123",
			},
			{
				name: "missing",

				wantStatus: http.StatusBadRequest,
			},
			{
				name: "blank",

				values: []string{
					" ",
				},

				wantStatus: http.StatusBadRequest,
			},
			{
				name: "multiple",

				values: []string{
					"first",
					"second",
				},

				wantStatus: http.StatusBadRequest,
			},
			{
				name: "too long",

				values: []string{
					strings.Repeat(
						"a",
						repository.
							MaximumHTTPIdempotencyKeyCharacters+
							1,
					),
				},

				wantStatus: http.StatusBadRequest,
			},
		}

	for _, test := range tests {

		t.Run(
			test.name,
			func(
				t *testing.T,
			) {
				request, err :=
					http.NewRequest(
						http.MethodPost,
						"/api/v1/executions/async",
						nil,
					)

				if err != nil {
					t.Fatalf(
						"http.NewRequest() error = %v",
						err,
					)
				}

				for _, value := range test.values {

					request.Header.Add(
						HeaderIdempotencyKey,
						value,
					)
				}

				actual, apiError :=
					readIdempotencyKey(
						request,
					)

				if test.wantStatus != 0 {
					if apiError == nil {
						t.Fatal(
							"expected API error",
						)
					}

					if apiError.Status !=
						test.wantStatus {

						t.Fatalf(
							"status = %d, want %d",
							apiError.Status,
							test.wantStatus,
						)
					}

					return
				}

				if apiError != nil {
					t.Fatalf(
						"readIdempotencyKey() error = %v",
						apiError,
					)
				}

				if actual !=
					test.want {

					t.Fatalf(
						"key = %q, want %q",
						actual,
						test.want,
					)
				}
			},
		)
	}
}

func TestMapAsyncExecutionApplicationError(
	t *testing.T,
) {
	tests :=
		[]struct {
			name       string
			err        error
			wantStatus int
			wantCode   string
		}{
			{
				name: "deadline",

				err: context.DeadlineExceeded,

				wantStatus: http.StatusGatewayTimeout,

				wantCode: errorCodeRequestTimeout,
			},
			{
				name: "async unavailable",

				err: engine.
					ErrAsyncExecutionUnavailable,

				wantStatus: http.StatusServiceUnavailable,

				wantCode: errorCodeExecutionUnavailable,
			},
			{
				name: "idempotency reused",

				err: executionfeature.ErrIdempotencyKeyReused,

				wantStatus: http.StatusConflict,

				wantCode: errorCodeIdempotencyKeyReused,
			},
			{
				name: "idempotency in progress",

				err: executionfeature.ErrIdempotencyRequestInProgress,

				wantStatus: http.StatusConflict,

				wantCode: errorCodeIdempotencyRequestInProgress,
			},
			{
				name: "unexpected",

				err: errors.New(
					"unexpected",
				),

				wantStatus: http.StatusInternalServerError,

				wantCode: errorCodeInternalServerError,
			},
		}

	for _, test := range tests {

		t.Run(
			test.name,
			func(
				t *testing.T,
			) {
				apiError :=
					mapAsyncExecutionApplicationError(
						test.err,
					)

				if apiError.Status !=
					test.wantStatus {

					t.Fatalf(
						"status = %d, want %d",
						apiError.Status,
						test.wantStatus,
					)
				}

				if apiError.Code !=
					test.wantCode {

					t.Fatalf(
						"code = %q, want %q",
						apiError.Code,
						test.wantCode,
					)
				}
			},
		)
	}
}

func TestExecutionDTORejectsInvalidNodePosition(
	t *testing.T,
) {
	_, apiError :=
		mapWorkflowNodeRequest(
			WorkflowNodeRequest{
				ID: "node-1",

				PluginType: "core.pass-through",

				PluginVersion: "v1",

				Configuration: json.RawMessage(
					`{}`,
				),

				Position: &WorkflowNodePositionRequest{
					X: math.NaN(),

					Y: 10,
				},
			},
			0,
		)

	if apiError == nil {
		t.Fatal(
			"expected invalid node position API error",
		)
	}

	if apiError.Status !=
		http.StatusBadRequest {

		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}

	if len(apiError.Details) !=
		1 {

		t.Fatalf(
			"details length = %d, want 1",
			len(apiError.Details),
		)
	}
}

func TestExecutionDTORejectsInvalidEdge(
	t *testing.T,
) {
	_, apiError :=
		mapWorkflowEdgeRequest(
			WorkflowEdgeRequest{
				ID: " ",

				SourceNodeID: "source",

				SourceOutputPort: "output",

				TargetNodeID: "target",

				TargetInputPort: "input",
			},
			0,
		)

	if apiError == nil {
		t.Fatal(
			"expected invalid edge API error",
		)
	}

	if apiError.Code !=
		errorCodeInvalidExecutionRequest {

		t.Fatalf(
			"code = %q, want %q",
			apiError.Code,
			errorCodeInvalidExecutionRequest,
		)
	}
}

func TestExecutionDTORejectsInvalidInitialVariable(
	t *testing.T,
) {
	_, apiError :=
		mapInitialVariables(
			map[string]json.RawMessage{
				"broken": json.RawMessage(
					`{`,
				),
			},
		)

	if apiError == nil {
		t.Fatal(
			"expected invalid initial variable API error",
		)
	}

	if apiError.Code !=
		errorCodeInvalidExecutionRequest {

		t.Fatalf(
			"code = %q, want %q",
			apiError.Code,
			errorCodeInvalidExecutionRequest,
		)
	}

	if len(apiError.Details) !=
		1 {

		t.Fatalf(
			"details length = %d, want 1",
			len(apiError.Details),
		)
	}

	if apiError.Details[0].Field !=
		"initialVariables.broken" {

		t.Fatalf(
			"field = %q",
			apiError.Details[0].Field,
		)
	}
}

func TestJSONObjectOrDefaultHandlesEmptyAndProvidedValues(
	t *testing.T,
) {
	defaultValue :=
		jsonObjectOrDefault(
			nil,
		)

	if string(defaultValue) !=
		`{}` {

		t.Fatalf(
			"default JSON = %q, want {}",
			defaultValue,
		)
	}

	source :=
		json.RawMessage(
			`{"owner":"runtime"}`,
		)

	actual :=
		jsonObjectOrDefault(
			source,
		)

	if string(actual) !=
		string(source) {

		t.Fatalf(
			"JSON = %q, want %q",
			actual,
			source,
		)
	}

	actual[2] =
		'X'

	if string(source) !=
		`{"owner":"runtime"}` {

		t.Fatal(
			"jsonObjectOrDefault() did not clone the supplied JSON",
		)
	}
}

func TestMapExecutionApplicationErrorUsesStableStatusCodes(
	t *testing.T,
) {
	tests :=
		[]struct {
			name           string
			err            error
			expectedStatus int
			expectedCode   string
		}{
			{
				name: "deadline",

				err: context.DeadlineExceeded,

				expectedStatus: http.StatusGatewayTimeout,

				expectedCode: errorCodeRequestTimeout,
			},
			{
				name: "async unavailable",

				err: engine.
					ErrAsyncExecutionUnavailable,

				expectedStatus: http.StatusServiceUnavailable,

				expectedCode: errorCodeExecutionUnavailable,
			},
			{
				name: "unexpected",

				err: errors.New(
					"internal failure",
				),

				expectedStatus: http.StatusInternalServerError,

				expectedCode: errorCodeInternalServerError,
			},
		}

	for _, test := range tests {

		t.Run(
			test.name,
			func(t *testing.T) {
				apiError :=
					mapExecutionApplicationError(
						test.err,
					)

				if apiError.Status !=
					test.expectedStatus {

					t.Fatalf(
						"status = %d, want %d",
						apiError.Status,
						test.expectedStatus,
					)
				}

				if apiError.Code !=
					test.expectedCode {

					t.Fatalf(
						"code = %q, want %q",
						apiError.Code,
						test.expectedCode,
					)
				}
			},
		)
	}
}

func TestSanitizeExecutionErrorDetailsFlattensLegacyShape(
	t *testing.T,
) {
	actual :=
		sanitizeExecutionErrorDetails(
			[]byte(
				`{"details":{"issueCount":"1","structuralIssueCount":"0","pluginIssueCount":"1","engineIssueCount":"0"}}`,
			),
		)

	details :=
		decodeSanitizedExecutionErrorDetails(
			t,
			actual,
		)

	expected :=
		map[string]string{
			"issueCount":           "1",
			"structuralIssueCount": "0",
			"pluginIssueCount":     "1",
			"engineIssueCount":     "0",
		}

	requireExecutionErrorDetailsEqual(
		t,
		details,
		expected,
	)

	if _, exists :=
		details["details"]; exists {

		t.Fatal(
			"sanitized details retained legacy details wrapper",
		)
	}
}

func TestSanitizeExecutionErrorDetailsAcceptsFlatShape(
	t *testing.T,
) {
	actual :=
		sanitizeExecutionErrorDetails(
			[]byte(
				`{"failedNodeCount":"1","skippedNodeCount":"2","pendingNodeCount":"3","readyNodeCount":"4"}`,
			),
		)

	details :=
		decodeSanitizedExecutionErrorDetails(
			t,
			actual,
		)

	expected :=
		map[string]string{
			"failedNodeCount":  "1",
			"skippedNodeCount": "2",
			"pendingNodeCount": "3",
			"readyNodeCount":   "4",
		}

	requireExecutionErrorDetailsEqual(
		t,
		details,
		expected,
	)
}

func TestSanitizeExecutionErrorDetailsRedactsTechnicalAndUnknownFields(
	t *testing.T,
) {
	actual :=
		sanitizeExecutionErrorDetails(
			[]byte(
				`{"details":{"issueCount":"1","error":"password=secret-value","internalError":"connection refused password=secret-value","contextError":"context deadline exceeded","service":"billing","operation":"fetch-customer","kind":"unit","nodeID":"node-secret"}}`,
			),
		)

	details :=
		decodeSanitizedExecutionErrorDetails(
			t,
			actual,
		)

	expected :=
		map[string]string{
			"issueCount": "1",
		}

	requireExecutionErrorDetailsEqual(
		t,
		details,
		expected,
	)

	for _, forbidden := range []string{
		"error",
		"internalError",
		"contextError",
		"service",
		"operation",
		"kind",
		"nodeID",
		"details",
	} {

		if _, exists :=
			details[forbidden]; exists {

			t.Fatalf(
				"forbidden detail %q was exposed",
				forbidden,
			)
		}
	}

	if string(actual) ==
		"" {

		t.Fatal(
			"sanitized details must not be blank",
		)
	}
}

func TestSanitizeExecutionErrorDetailsReturnsEmptyObjectForUnknownShape(
	t *testing.T,
) {
	actual :=
		sanitizeExecutionErrorDetails(
			[]byte(
				`{"internalError":"password=secret-value","service":"billing"}`,
			),
		)

	details :=
		decodeSanitizedExecutionErrorDetails(
			t,
			actual,
		)

	if len(details) != 0 {
		t.Fatalf(
			"sanitized details = %v, want empty object",
			details,
		)
	}
}

func TestSanitizeExecutionErrorDetailsReturnsEmptyObjectForMalformedJSON(
	t *testing.T,
) {
	actual :=
		sanitizeExecutionErrorDetails(
			[]byte(
				`{"details":`,
			),
		)

	if string(actual) != "{}" {
		t.Fatalf(
			"sanitized malformed details = %s, want {}",
			actual,
		)
	}
}

func decodeSanitizedExecutionErrorDetails(
	t *testing.T,
	raw json.RawMessage,
) map[string]string {
	t.Helper()

	var details map[string]string

	if err :=
		json.Unmarshal(
			raw,
			&details,
		); err != nil {

		t.Fatalf(
			"json.Unmarshal() returned an error: %v",
			err,
		)
	}

	if details == nil {
		t.Fatal(
			"sanitized details must be a JSON object",
		)
	}

	return details
}

func requireExecutionErrorDetailsEqual(
	t *testing.T,
	actual map[string]string,
	expected map[string]string,
) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf(
			"detail count = %d, want %d: actual=%v",
			len(actual),
			len(expected),
			actual,
		)
	}

	for key, expectedValue := range expected {

		actualValue, exists :=
			actual[key]

		if !exists {
			t.Fatalf(
				"expected detail %q is missing",
				key,
			)
		}

		if actualValue !=
			expectedValue {

			t.Fatalf(
				"detail %q = %q, want %q",
				key,
				actualValue,
				expectedValue,
			)
		}
	}
}

func TestParseExecutionErrorsQueryAllowsOmittedLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/errors",
			nil,
		)

	pageRequest, apiError :=
		parseExecutionErrorsQuery(
			request,
		)

	if apiError != nil {
		t.Fatalf(
			"parseExecutionErrorsQuery() error = %v, want nil",
			apiError,
		)
	}

	if pageRequest.Limit() != 50 {
		t.Fatalf(
			"limit = %d, want 50",
			pageRequest.Limit(),
		)
	}
}

func TestParseExecutionErrorsQueryRejectsExplicitZeroLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/errors?limit=0",
			nil,
		)

	_, apiError :=
		parseExecutionErrorsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionErrorsQuery() accepted explicit zero limit",
		)
	}

	if apiError.Status != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}

	if apiError.Code != errorCodeInvalidExecutionQuery {
		t.Fatalf(
			"code = %q, want %q",
			apiError.Code,
			errorCodeInvalidExecutionQuery,
		)
	}
}

func TestParseExecutionErrorsQueryRejectsUnsupportedParameter(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/errors?category=VALIDATION",
			nil,
		)

	_, apiError :=
		parseExecutionErrorsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionErrorsQuery() accepted unsupported parameter",
		)
	}

	if apiError.Status != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}
}

func TestParseExecutionErrorsQueryRejectsRepeatedParameter(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/errors?limit=1&limit=2",
			nil,
		)

	_, apiError :=
		parseExecutionErrorsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionErrorsQuery() accepted repeated parameter",
		)
	}

	if apiError.Status != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}
}

func TestParseExecutionEventsQueryAllowsOmittedLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/events",
			nil,
		)

	pageRequest, apiError :=
		parseExecutionEventsQuery(
			request,
		)

	if apiError != nil {
		t.Fatalf(
			"parseExecutionEventsQuery() error = %v, want nil",
			apiError,
		)
	}

	if pageRequest.Limit() != 50 {
		t.Fatalf(
			"limit = %d, want 50",
			pageRequest.Limit(),
		)
	}
}

func TestParseExecutionEventsQueryRejectsExplicitZeroLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/events?limit=0",
			nil,
		)

	_, apiError :=
		parseExecutionEventsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionEventsQuery() accepted explicit zero limit",
		)
	}

	if apiError.Status !=
		http.StatusBadRequest {

		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}

	if apiError.Code !=
		errorCodeInvalidExecutionQuery {

		t.Fatalf(
			"code = %q, want %q",
			apiError.Code,
			errorCodeInvalidExecutionQuery,
		)
	}
}

func TestParseExecutionEventsQueryRejectsUnsupportedParameter(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/events?status=RUNNING",
			nil,
		)

	_, apiError :=
		parseExecutionEventsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionEventsQuery() accepted unsupported parameter",
		)
	}

	if apiError.Status !=
		http.StatusBadRequest {

		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}
}

func TestParseExecutionEventsQueryRejectsRepeatedParameter(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/events?limit=1&limit=2",
			nil,
		)

	_, apiError :=
		parseExecutionEventsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionEventsQuery() accepted repeated parameter",
		)
	}

	if apiError.Status !=
		http.StatusBadRequest {

		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}
}

func TestParseExecutionLogsQueryAllowsOmittedLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/logs",
			nil,
		)

	pageRequest, apiError :=
		parseExecutionLogsQuery(
			request,
		)

	if apiError != nil {
		t.Fatalf(
			"parseExecutionLogsQuery() error = %v, want nil",
			apiError,
		)
	}

	if pageRequest.Limit() != 50 {
		t.Fatalf(
			"limit = %d, want 50",
			pageRequest.Limit(),
		)
	}
}

func TestParseExecutionLogsQueryRejectsExplicitZeroLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/logs?limit=0",
			nil,
		)

	_, apiError :=
		parseExecutionLogsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionLogsQuery() accepted explicit zero limit",
		)
	}

	if apiError.Status != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}

	if apiError.Code != errorCodeInvalidExecutionQuery {
		t.Fatalf(
			"code = %q, want %q",
			apiError.Code,
			errorCodeInvalidExecutionQuery,
		)
	}
}

func TestParseExecutionLogsQueryRejectsUnsupportedParameter(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/logs?level=INFO",
			nil,
		)

	_, apiError :=
		parseExecutionLogsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionLogsQuery() accepted unsupported parameter",
		)
	}

	if apiError.Status != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}
}

func TestParseExecutionLogsQueryRejectsRepeatedParameter(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/logs?limit=1&limit=2",
			nil,
		)

	_, apiError :=
		parseExecutionLogsQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionLogsQuery() accepted repeated parameter",
		)
	}

	if apiError.Status != http.StatusBadRequest {
		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}
}

func TestParseExecutionNodesQueryAllowsOmittedLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/nodes",
			nil,
		)

	pageRequest, apiError :=
		parseExecutionNodesQuery(
			request,
		)

	if apiError != nil {
		t.Fatalf(
			"parseExecutionNodesQuery() error = %v, want nil",
			apiError,
		)
	}

	if pageRequest.Limit() != 50 {
		t.Fatalf(
			"limit = %d, want 50",
			pageRequest.Limit(),
		)
	}
}

func TestParseExecutionNodesQueryRejectsExplicitZeroLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/nodes?limit=0",
			nil,
		)

	_, apiError :=
		parseExecutionNodesQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionNodesQuery() accepted explicit zero limit",
		)
	}

	if apiError.Status !=
		http.StatusBadRequest {

		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}

	if apiError.Code !=
		errorCodeInvalidExecutionQuery {

		t.Fatalf(
			"code = %q, want %q",
			apiError.Code,
			errorCodeInvalidExecutionQuery,
		)
	}
}

func TestParseExecutionNodesQueryRejectsUnsupportedParameter(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions/execution/nodes?status=RUNNING",
			nil,
		)

	_, apiError :=
		parseExecutionNodesQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionNodesQuery() accepted unsupported parameter",
		)
	}

	if apiError.Status !=
		http.StatusBadRequest {

		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}
}

func TestParseExecutionListQueryAllowsOmittedLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions",
			nil,
		)

	_, _, apiError :=
		parseExecutionListQuery(
			request,
		)

	if apiError != nil {
		t.Fatalf(
			"parseExecutionListQuery() error = %v, want nil",
			apiError,
		)
	}
}

func TestParseExecutionListQueryRejectsExplicitZeroLimit(
	t *testing.T,
) {
	request :=
		httptest.NewRequest(
			http.MethodGet,
			"/api/v1/executions?limit=0",
			nil,
		)

	_, _, apiError :=
		parseExecutionListQuery(
			request,
		)

	if apiError == nil {
		t.Fatal(
			"parseExecutionListQuery() accepted explicit zero limit",
		)
	}

	if apiError.Status !=
		http.StatusBadRequest {

		t.Fatalf(
			"status = %d, want %d",
			apiError.Status,
			http.StatusBadRequest,
		)
	}

	if apiError.Code !=
		errorCodeInvalidExecutionQuery {

		t.Fatalf(
			"code = %q, want %q",
			apiError.Code,
			errorCodeInvalidExecutionQuery,
		)
	}
}
