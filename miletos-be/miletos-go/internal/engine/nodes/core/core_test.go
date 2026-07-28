package core

import (
	context "context"
	errors "errors"
	plugin "miletos-go/internal/engine/plugin"
	runtime "miletos-go/internal/engine/runtime"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	reflect "reflect"
	strings "strings"
	testing "testing"
	time "time"
)

func TestCoreDescriptorsAreDeterministicAndRegistryCompatible(
	t *testing.T,
) {
	first, err := CoreDescriptors()
	if err != nil {
		t.Fatalf(
			"CoreDescriptors() returned an unexpected error: %v",
			err,
		)
	}

	second, err := CoreDescriptors()
	if err != nil {
		t.Fatalf(
			"second CoreDescriptors() returned an unexpected error: %v",
			err,
		)
	}

	expected := []string{
		"core.delay@v1",
		"core.pass-through@v1",
		"core.static-input@v1",
		"core.terminal@v1",
	}

	assertDescriptorIdentities(
		t,
		first,
		expected,
	)

	assertDescriptorIdentities(
		t,
		second,
		expected,
	)

	first[0] = first[1]

	assertDescriptorIdentities(
		t,
		second,
		expected,
	)

	registry, err := plugin.NewRegistry(second)
	if err != nil {
		t.Fatalf(
			"NewRegistry() rejected core descriptors: %v",
			err,
		)
	}

	if actual := registry.Len(); actual != 4 {
		t.Fatalf(
			"registry length = %d, want %d",
			actual,
			4,
		)
	}

	for _, identityText := range expected {
		found := false

		for _, descriptor := range registry.List() {
			if descriptor.Identity().String() ==
				identityText {
				found = true
				break
			}
		}

		if !found {
			t.Fatalf(
				"registry does not contain %q",
				identityText,
			)
		}
	}
}

func TestStaticInputDescriptorContract(
	t *testing.T,
) {
	descriptor := mustStaticInputDescriptor(t)

	assertIdentity(
		t,
		descriptor,
		"core.static-input@v1",
	)

	assertPortNames(
		t,
		descriptor.InputPorts(),
		nil,
	)

	assertPortNames(
		t,
		descriptor.OutputPorts(),
		[]string{
			OutputPortName,
		},
	)

	assertExactConstraint(
		t,
		descriptor.InputEdgeConstraint(),
		0,
	)

	assertUnlimitedConstraint(
		t,
		descriptor.OutputEdgeConstraint(),
		1,
	)

	assertCommonPolicyAndDistribution(
		t,
		descriptor,
	)
}

func TestPassThroughDescriptorContract(
	t *testing.T,
) {
	descriptor := mustPassThroughDescriptor(t)

	assertIdentity(
		t,
		descriptor,
		"core.pass-through@v1",
	)

	assertPortNames(
		t,
		descriptor.InputPorts(),
		[]string{
			InputPortName,
		},
	)

	assertPortNames(
		t,
		descriptor.OutputPorts(),
		[]string{
			OutputPortName,
		},
	)

	assertExactConstraint(
		t,
		descriptor.InputEdgeConstraint(),
		1,
	)

	assertUnlimitedConstraint(
		t,
		descriptor.OutputEdgeConstraint(),
		0,
	)

	assertCommonPolicyAndDistribution(
		t,
		descriptor,
	)
}

func TestDelayDescriptorContract(
	t *testing.T,
) {
	descriptor := mustDelayDescriptor(t)

	assertIdentity(
		t,
		descriptor,
		"core.delay@v1",
	)

	assertPortNames(
		t,
		descriptor.InputPorts(),
		[]string{
			InputPortName,
		},
	)

	assertPortNames(
		t,
		descriptor.OutputPorts(),
		[]string{
			OutputPortName,
		},
	)

	assertExactConstraint(
		t,
		descriptor.InputEdgeConstraint(),
		1,
	)

	assertUnlimitedConstraint(
		t,
		descriptor.OutputEdgeConstraint(),
		0,
	)

	assertCommonPolicyAndDistribution(
		t,
		descriptor,
	)
}

func TestTerminalDescriptorContract(
	t *testing.T,
) {
	descriptor := mustTerminalDescriptor(t)

	assertIdentity(
		t,
		descriptor,
		"core.terminal@v1",
	)

	assertPortNames(
		t,
		descriptor.InputPorts(),
		[]string{
			InputPortName,
		},
	)

	assertPortNames(
		t,
		descriptor.OutputPorts(),
		nil,
	)

	assertUnlimitedConstraint(
		t,
		descriptor.InputEdgeConstraint(),
		1,
	)

	assertExactConstraint(
		t,
		descriptor.OutputEdgeConstraint(),
		0,
	)

	assertCommonPolicyAndDistribution(
		t,
		descriptor,
	)
}

func TestStaticInputConfigurationValidation(
	t *testing.T,
) {
	descriptor := mustStaticInputDescriptor(t)

	validConfigurations := []string{
		`{"value":"hello"}`,
		`{"value":{"message":"hello"}}`,
		`{"value":[1,2,3]}`,
		`{"value":null}`,
	}

	for _, configuration := range validConfigurations {
		report := descriptor.ValidateConfiguration(
			mustJSONObject(
				t,
				configuration,
			),
		)

		if !report.IsValid() {
			t.Fatalf(
				"configuration %s produced issues: %v",
				configuration,
				report.Issues(),
			)
		}
	}

	assertConfigurationIssue(
		t,
		descriptor,
		`{}`,
		"value",
		"is required",
	)

	assertConfigurationIssue(
		t,
		descriptor,
		`{"value":"hello","unknown":true}`,
		"configuration",
		"must match the expected object schema",
	)
}

func TestPassThroughConfigurationValidation(
	t *testing.T,
) {
	descriptor := mustPassThroughDescriptor(t)

	report := descriptor.ValidateConfiguration(
		mustJSONObject(
			t,
			`{}`,
		),
	)

	if !report.IsValid() {
		t.Fatalf(
			"empty configuration produced issues: %v",
			report.Issues(),
		)
	}

	assertConfigurationIssue(
		t,
		descriptor,
		`{"unexpected":true}`,
		"configuration",
		"must match the expected object schema",
	)
}

func TestDelayConfigurationValidation(
	t *testing.T,
) {
	descriptor := mustDelayDescriptor(t)

	validConfigurations := []string{
		`{"delay":"1ms"}`,
		`{"delay":"2s"}`,
		`{"delay":"30m"}`,
		`{"delay":"24h"}`,
	}

	for _, configuration := range validConfigurations {
		report := descriptor.ValidateConfiguration(
			mustJSONObject(
				t,
				configuration,
			),
		)

		if !report.IsValid() {
			t.Fatalf(
				"configuration %s produced issues: %v",
				configuration,
				report.Issues(),
			)
		}
	}

	tests := []struct {
		name          string
		configuration string
		field         string
		reason        string
	}{
		{
			name:          "missing delay",
			configuration: `{}`,
			field:         "delay",
			reason:        "is required",
		},
		{
			name:          "blank delay",
			configuration: `{"delay":"   "}`,
			field:         "delay",
			reason:        "is required",
		},
		{
			name:          "invalid delay",
			configuration: `{"delay":"invalid"}`,
			field:         "delay",
			reason:        "must be a valid duration",
		},
		{
			name:          "zero delay",
			configuration: `{"delay":"0s"}`,
			field:         "delay",
			reason:        "must be greater than zero",
		},
		{
			name:          "negative delay",
			configuration: `{"delay":"-1s"}`,
			field:         "delay",
			reason:        "must be greater than zero",
		},
		{
			name:          "delay above maximum",
			configuration: `{"delay":"24h1s"}`,
			field:         "delay",
			reason:        "must not exceed 24h",
		},
		{
			name:          "unknown field",
			configuration: `{"delay":"2s","unknown":true}`,
			field:         "configuration",
			reason:        "must match the expected object schema",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertConfigurationIssue(
				t,
				descriptor,
				test.configuration,
				test.field,
				test.reason,
			)
		})
	}
}

func TestTerminalConfigurationValidation(
	t *testing.T,
) {
	descriptor := mustTerminalDescriptor(t)

	report := descriptor.ValidateConfiguration(
		mustJSONObject(
			t,
			`{}`,
		),
	)

	if !report.IsValid() {
		t.Fatalf(
			"empty configuration produced issues: %v",
			report.Issues(),
		)
	}

	assertConfigurationIssue(
		t,
		descriptor,
		`{"unexpected":true}`,
		"configuration",
		"must match the expected object schema",
	)
}

func mustStaticInputDescriptor(
	t *testing.T,
) plugin.Descriptor {
	t.Helper()

	descriptor, err := StaticInputDescriptor()
	if err != nil {
		t.Fatalf(
			"StaticInputDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	return descriptor
}

func mustPassThroughDescriptor(
	t *testing.T,
) plugin.Descriptor {
	t.Helper()

	descriptor, err := PassThroughDescriptor()
	if err != nil {
		t.Fatalf(
			"PassThroughDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	return descriptor
}

func mustDelayDescriptor(
	t *testing.T,
) plugin.Descriptor {
	t.Helper()

	descriptor, err := DelayDescriptor()
	if err != nil {
		t.Fatalf(
			"DelayDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	return descriptor
}

func mustTerminalDescriptor(
	t *testing.T,
) plugin.Descriptor {
	t.Helper()

	descriptor, err := TerminalDescriptor()
	if err != nil {
		t.Fatalf(
			"TerminalDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	return descriptor
}

func mustJSONObject(
	t *testing.T,
	value string,
) workflow.JSONObject {
	t.Helper()

	object, err := workflow.NewJSONObject(
		[]byte(value),
	)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	return object
}

func assertIdentity(
	t *testing.T,
	descriptor plugin.Descriptor,
	expected string,
) {
	t.Helper()

	if actual := descriptor.Identity().String(); actual != expected {
		t.Fatalf(
			"identity = %q, want %q",
			actual,
			expected,
		)
	}
}

func assertDescriptorIdentities(
	t *testing.T,
	descriptors []plugin.Descriptor,
	expected []string,
) {
	t.Helper()

	actual := make(
		[]string,
		len(descriptors),
	)

	for index, descriptor := range descriptors {
		actual[index] = descriptor.Identity().String()
	}

	if !reflect.DeepEqual(
		actual,
		expected,
	) {
		t.Fatalf(
			"descriptor identities = %v, want %v",
			actual,
			expected,
		)
	}
}

func assertPortNames(
	t *testing.T,
	ports []plugin.Port,
	expected []string,
) {
	t.Helper()

	actual := make(
		[]string,
		len(ports),
	)

	for index, port := range ports {
		actual[index] = port.Name()
	}

	if len(actual) == 0 {
		actual = nil
	}

	if !reflect.DeepEqual(
		actual,
		expected,
	) {
		t.Fatalf(
			"port names = %v, want %v",
			actual,
			expected,
		)
	}
}

func assertExactConstraint(
	t *testing.T,
	constraint plugin.EdgeConstraint,
	expected uint,
) {
	t.Helper()

	if constraint.Minimum() != expected {
		t.Fatalf(
			"minimum = %d, want %d",
			constraint.Minimum(),
			expected,
		)
	}

	maximum, limited := constraint.Maximum()
	if !limited {
		t.Fatal(
			"constraint maximum is unlimited",
		)
	}

	if maximum != expected {
		t.Fatalf(
			"maximum = %d, want %d",
			maximum,
			expected,
		)
	}

	if !constraint.Allows(expected) {
		t.Fatalf(
			"constraint rejected expected count %d",
			expected,
		)
	}
}

func assertUnlimitedConstraint(
	t *testing.T,
	constraint plugin.EdgeConstraint,
	expectedMinimum uint,
) {
	t.Helper()

	if constraint.Minimum() != expectedMinimum {
		t.Fatalf(
			"minimum = %d, want %d",
			constraint.Minimum(),
			expectedMinimum,
		)
	}

	if !constraint.IsUnlimited() {
		t.Fatal(
			"constraint is not unlimited",
		)
	}

	if _, limited := constraint.Maximum(); limited {
		t.Fatal(
			"Maximum() reported a finite maximum",
		)
	}

	if !constraint.Allows(expectedMinimum) {
		t.Fatalf(
			"constraint rejected minimum %d",
			expectedMinimum,
		)
	}

	if !constraint.Allows(
		expectedMinimum + 1_000,
	) {
		t.Fatal(
			"unlimited constraint rejected a large count",
		)
	}
}

func assertCommonPolicyAndDistribution(
	t *testing.T,
	descriptor plugin.Descriptor,
) {
	t.Helper()

	queuePolicy := descriptor.QueuePolicy()

	if queuePolicy.DefaultCapacity() !=
		DefaultQueueCapacity {
		t.Fatalf(
			"queue capacity = %d, want %d",
			queuePolicy.DefaultCapacity(),
			DefaultQueueCapacity,
		)
	}

	if queuePolicy.OverflowStrategy() !=
		plugin.OverflowStrategyDropOldest {
		t.Fatalf(
			"queue overflow strategy = %q, want %q",
			queuePolicy.OverflowStrategy(),
			plugin.OverflowStrategyDropOldest,
		)
	}

	cachePolicy := descriptor.CachePolicy()

	if cachePolicy.DefaultCapacity() !=
		DefaultCacheCapacity {
		t.Fatalf(
			"cache capacity = %d, want %d",
			cachePolicy.DefaultCapacity(),
			DefaultCacheCapacity,
		)
	}

	if cachePolicy.OverflowStrategy() !=
		plugin.OverflowStrategyDropOldest {
		t.Fatalf(
			"cache overflow strategy = %q, want %q",
			cachePolicy.OverflowStrategy(),
			plugin.OverflowStrategyDropOldest,
		)
	}

	if descriptor.Distribution() !=
		plugin.DistributionDistributable {
		t.Fatalf(
			"distribution = %q, want %q",
			descriptor.Distribution(),
			plugin.DistributionDistributable,
		)
	}

	if !descriptor.Distribution().SupportsAsync() {
		t.Fatal(
			"core descriptor does not support async execution",
		)
	}
}

func assertConfigurationIssue(
	t *testing.T,
	descriptor plugin.Descriptor,
	configuration string,
	expectedField string,
	expectedReason string,
) {
	t.Helper()

	report := descriptor.ValidateConfiguration(
		mustJSONObject(
			t,
			configuration,
		),
	)

	if report.IsValid() {
		t.Fatalf(
			"configuration %s was accepted",
			configuration,
		)
	}

	if actual := report.Len(); actual != 1 {
		t.Fatalf(
			"issue count = %d, want %d: %v",
			actual,
			1,
			report.Issues(),
		)
	}

	issue := report.Issues()[0]

	if issue.Field != expectedField {
		t.Fatalf(
			"issue field = %q, want %q",
			issue.Field,
			expectedField,
		)
	}

	if issue.Reason != expectedReason {
		t.Fatalf(
			"issue reason = %q, want %q",
			issue.Reason,
			expectedReason,
		)
	}
}

func TestStaticInputExecutorProducesConfiguredJSONPayload(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"static-input-node",
	)

	executor := staticInputExecutor{
		maximumInlinePayloadBytes: 1024,
	}

	result, err := executor.Execute(
		nodeContext,
		mustEmptyCoreNodeInput(t),
		mustCoreJSONObject(
			t,
			`{"value":{"message":"hello","count":2}}`,
		),
	)
	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"Execute() returned an invalid NodeResult",
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"result IsSuccess() = false",
		)
	}

	if result.IsFailure() {
		t.Fatal(
			"result IsFailure() = true",
		)
	}

	if !result.HasRoutedOutputs() {
		t.Fatal(
			"result HasRoutedOutputs() = false",
		)
	}

	if result.HasTerminalOutput() {
		t.Fatal(
			"result HasTerminalOutput() = true",
		)
	}

	payload := requireCoreOutputPayload(
		t,
		result,
	)

	if actual := payload.ContentType(); actual != runtime.ContentTypeApplicationJSON {
		t.Fatalf(
			"payload ContentType() = %q, want %q",
			actual,
			runtime.ContentTypeApplicationJSON,
		)
	}

	assertCoreInlinePayload(
		t,
		payload,
		`{"message":"hello","count":2}`,
	)

	if !result.ContextChanges().IsEmpty() {
		t.Fatal(
			"static input result contains unexpected context changes",
		)
	}
}

func TestStaticInputExecutorReturnsControlledFailures(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"static-input-failure-node",
	)

	t.Run("invalid configuration", func(t *testing.T) {
		executor := staticInputExecutor{
			maximumInlinePayloadBytes: 1024,
		}

		result, err := executor.Execute(
			nodeContext,
			mustEmptyCoreNodeInput(t),
			mustCoreJSONObject(
				t,
				`{}`,
			),
		)

		requireCoreControlledFailure(
			t,
			result,
			err,
			failureCodeInvalidConfiguration,
		)
	})

	t.Run("unexpected input", func(t *testing.T) {
		executor := staticInputExecutor{
			maximumInlinePayloadBytes: 1024,
		}

		result, err := executor.Execute(
			nodeContext,
			mustCoreNodeInput(
				t,
				mustCoreInlinePayload(
					t,
					runtime.ContentTypeTextPlain,
					"unexpected",
					nil,
				),
			),
			mustCoreJSONObject(
				t,
				`{"value":"hello"}`,
			),
		)

		requireCoreControlledFailure(
			t,
			result,
			err,
			failureCodeInvalidInput,
		)
	})

	t.Run("inline payload limit", func(t *testing.T) {
		executor := staticInputExecutor{
			maximumInlinePayloadBytes: 4,
		}

		result, err := executor.Execute(
			nodeContext,
			mustEmptyCoreNodeInput(t),
			mustCoreJSONObject(
				t,
				`{"value":"long"}`,
			),
		)

		requireCoreControlledFailure(
			t,
			result,
			err,
			failureCodeInvalidConfiguration,
		)
	})
}

func TestPassThroughExecutorPreservesInlinePayload(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"pass-through-inline-node",
	)

	inputPayload := mustCoreInlinePayload(
		t,
		runtime.ContentTypeTextPlain,
		"payload-content",
		map[string]string{
			"source": "test-source",
		},
	)

	result, err := (passThroughExecutor{}).Execute(
		nodeContext,
		mustCoreNodeInput(
			t,
			inputPayload,
		),
		mustCoreJSONObject(
			t,
			`{}`,
		),
	)
	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"result IsSuccess() = false",
		)
	}

	outputPayload := requireCoreOutputPayload(
		t,
		result,
	)

	if actual := outputPayload.ContentType(); actual != runtime.ContentTypeTextPlain {
		t.Fatalf(
			"output ContentType() = %q, want %q",
			actual,
			runtime.ContentTypeTextPlain,
		)
	}

	assertCoreInlinePayload(
		t,
		outputPayload,
		"payload-content",
	)

	if actual := outputPayload.Metadata()["source"]; actual != "test-source" {
		t.Fatalf(
			"output metadata source = %q, want %q",
			actual,
			"test-source",
		)
	}
}

func TestPassThroughExecutorPreservesArtifactPayload(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"pass-through-artifact-node",
	)

	artifact, err := runtime.NewArtifactReference(
		"artifact-1",
		"workflow/execution-1/output.bin",
		runtime.ContentTypeApplicationOctetStream,
		2048,
		"sha256:abc",
		map[string]string{
			"storage": "local",
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewArtifactReference() returned an unexpected error: %v",
			err,
		)
	}

	inputPayload, err := runtime.NewArtifactPayload(
		artifact,
		map[string]string{
			"execution": "execution-1",
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewArtifactPayload() returned an unexpected error: %v",
			err,
		)
	}

	result, err := (passThroughExecutor{}).Execute(
		nodeContext,
		mustCoreNodeInput(
			t,
			inputPayload,
		),
		mustCoreJSONObject(
			t,
			`{}`,
		),
	)
	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	outputPayload := requireCoreOutputPayload(
		t,
		result,
	)

	if !outputPayload.IsArtifact() {
		t.Fatal(
			"output payload IsArtifact() = false",
		)
	}

	if outputPayload.IsInline() {
		t.Fatal(
			"output payload IsInline() = true",
		)
	}

	actualArtifact, exists := outputPayload.Artifact()
	if !exists {
		t.Fatal(
			"output payload Artifact() exists = false",
		)
	}

	if actual := actualArtifact.ID(); actual != "artifact-1" {
		t.Fatalf(
			"artifact ID = %q, want %q",
			actual,
			"artifact-1",
		)
	}

	if actual := actualArtifact.Location(); actual != "workflow/execution-1/output.bin" {
		t.Fatalf(
			"artifact location = %q",
			actual,
		)
	}

	if actual := actualArtifact.SizeBytes(); actual != 2048 {
		t.Fatalf(
			"artifact size = %d, want 2048",
			actual,
		)
	}

	if actual := actualArtifact.Checksum(); actual != "sha256:abc" {
		t.Fatalf(
			"artifact checksum = %q, want %q",
			actual,
			"sha256:abc",
		)
	}

	if actual := actualArtifact.Metadata()["storage"]; actual != "local" {
		t.Fatalf(
			"artifact metadata storage = %q, want %q",
			actual,
			"local",
		)
	}

	if actual := outputPayload.Metadata()["execution"]; actual != "execution-1" {
		t.Fatalf(
			"payload metadata execution = %q, want %q",
			actual,
			"execution-1",
		)
	}
}

func TestPassThroughExecutorRejectsMissingAndAmbiguousInput(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"pass-through-input-failure-node",
	)

	executor := passThroughExecutor{}

	t.Run("missing input", func(t *testing.T) {
		result, err := executor.Execute(
			nodeContext,
			mustEmptyCoreNodeInput(t),
			mustCoreJSONObject(
				t,
				`{}`,
			),
		)

		requireCoreControlledFailure(
			t,
			result,
			err,
			failureCodeInvalidInput,
		)
	})

	t.Run("wrong input port", func(t *testing.T) {
		payload := mustCoreInlinePayload(
			t,
			runtime.ContentTypeTextPlain,
			"payload",
			nil,
		)

		input, err := runtime.NewNodeInput(
			map[string][]runtime.Payload{
				"other": {
					payload,
				},
			},
		)
		if err != nil {
			t.Fatalf(
				"runtime.NewNodeInput() returned an unexpected error: %v",
				err,
			)
		}

		result, err := executor.Execute(
			nodeContext,
			input,
			mustCoreJSONObject(
				t,
				`{}`,
			),
		)

		requireCoreControlledFailure(
			t,
			result,
			err,
			failureCodeInvalidInput,
		)
	})

	t.Run("multiple payloads", func(t *testing.T) {
		result, err := executor.Execute(
			nodeContext,
			mustCoreNodeInput(
				t,
				mustCoreInlinePayload(
					t,
					runtime.ContentTypeTextPlain,
					"first",
					nil,
				),
				mustCoreInlinePayload(
					t,
					runtime.ContentTypeTextPlain,
					"second",
					nil,
				),
			),
			mustCoreJSONObject(
				t,
				`{}`,
			),
		)

		requireCoreControlledFailure(
			t,
			result,
			err,
			failureCodeInvalidInput,
		)
	})
}

func TestDelayExecutorWaitsAndForwardsPayload(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"delay-node",
	)

	inputPayload := mustCoreInlinePayload(
		t,
		runtime.ContentTypeTextPlain,
		"delayed-payload",
		map[string]string{
			"source": "delay-test",
		},
	)

	called := 0

	executor := delayExecutor{
		waiter: WaiterFunc(
			func(
				actualContext context.Context,
				actualDuration time.Duration,
			) error {
				called++

				if actualContext != nodeContext.Context() {
					t.Fatal(
						"waiter received a different context",
					)
				}

				if actualDuration != 2*time.Second {
					t.Fatalf(
						"wait duration = %v, want %v",
						actualDuration,
						2*time.Second,
					)
				}

				return nil
			},
		),
	}

	result, err := executor.Execute(
		nodeContext,
		mustCoreNodeInput(
			t,
			inputPayload,
		),
		mustCoreJSONObject(
			t,
			`{"delay":"2s"}`,
		),
	)
	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if called != 1 {
		t.Fatalf(
			"waiter call count = %d, want 1",
			called,
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"result IsSuccess() = false",
		)
	}

	outputPayload := requireCoreOutputPayload(
		t,
		result,
	)

	assertCoreInlinePayload(
		t,
		outputPayload,
		"delayed-payload",
	)

	if actual := outputPayload.Metadata()["source"]; actual != "delay-test" {
		t.Fatalf(
			"output metadata source = %q, want %q",
			actual,
			"delay-test",
		)
	}
}

func TestDelayExecutorPreservesWaiterErrors(
	t *testing.T,
) {
	errorsToReturn := map[string]error{
		"cancelled": context.Canceled,
		"timed out": context.DeadlineExceeded,
		"technical": errors.New(
			"waiter technical failure",
		),
	}

	for name, expectedError := range errorsToReturn {
		t.Run(name, func(t *testing.T) {
			nodeContext := mustCoreNodeExecutionContext(
				t,
				context.Background(),
				"delay-error-node-"+name,
			)

			executor := delayExecutor{
				waiter: WaiterFunc(
					func(
						context.Context,
						time.Duration,
					) error {
						return expectedError
					},
				),
			}

			result, err := executor.Execute(
				nodeContext,
				mustCoreNodeInput(
					t,
					mustCoreInlinePayload(
						t,
						runtime.ContentTypeTextPlain,
						"payload",
						nil,
					),
				),
				mustCoreJSONObject(
					t,
					`{"delay":"1s"}`,
				),
			)

			if !errors.Is(err, expectedError) {
				t.Fatalf(
					"Execute() error = %v, want %v",
					err,
					expectedError,
				)
			}

			if result.IsValid() {
				t.Fatal(
					"waiter error path returned a valid NodeResult",
				)
			}
		})
	}
}

func TestDelayExecutorRejectsNilWaiter(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"delay-nil-waiter-node",
	)

	result, err := (delayExecutor{}).Execute(
		nodeContext,
		mustCoreNodeInput(
			t,
			mustCoreInlinePayload(
				t,
				runtime.ContentTypeTextPlain,
				"payload",
				nil,
			),
		),
		mustCoreJSONObject(
			t,
			`{"delay":"1s"}`,
		),
	)

	if err == nil {
		t.Fatal(
			"Execute() returned nil error for a nil waiter",
		)
	}

	if result.IsValid() {
		t.Fatal(
			"nil waiter path returned a valid NodeResult",
		)
	}
}

func TestDelayExecutorChecksCancellationAfterWait(
	t *testing.T,
) {
	parent, cancel := context.WithCancel(
		context.Background(),
	)
	defer cancel()

	nodeContext := mustCoreNodeExecutionContext(
		t,
		parent,
		"delay-post-wait-cancel-node",
	)

	executor := delayExecutor{
		waiter: WaiterFunc(
			func(
				context.Context,
				time.Duration,
			) error {
				cancel()
				return nil
			},
		),
	}

	result, err := executor.Execute(
		nodeContext,
		mustCoreNodeInput(
			t,
			mustCoreInlinePayload(
				t,
				runtime.ContentTypeTextPlain,
				"payload",
				nil,
			),
		),
		mustCoreJSONObject(
			t,
			`{"delay":"1s"}`,
		),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"Execute() error = %v, want context.Canceled",
			err,
		)
	}

	if result.IsValid() {
		t.Fatal(
			"cancelled execution returned a valid NodeResult",
		)
	}
}

func TestTerminalExecutorProducesTerminalOutput(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"terminal-node",
	)

	inputPayload := mustCoreInlinePayload(
		t,
		runtime.ContentTypeApplicationJSON,
		`{"result":"completed"}`,
		map[string]string{
			"source": "terminal-test",
		},
	)

	result, err := (terminalExecutor{}).Execute(
		nodeContext,
		mustCoreNodeInput(
			t,
			inputPayload,
		),
		mustCoreJSONObject(
			t,
			`{}`,
		),
	)
	if err != nil {
		t.Fatalf(
			"Execute() returned an unexpected error: %v",
			err,
		)
	}

	if !result.IsSuccess() {
		t.Fatal(
			"result IsSuccess() = false",
		)
	}

	if result.HasRoutedOutputs() {
		t.Fatal(
			"terminal result HasRoutedOutputs() = true",
		)
	}

	if !result.HasTerminalOutput() {
		t.Fatal(
			"terminal result HasTerminalOutput() = false",
		)
	}

	if result.OutputPorts() != nil {
		t.Fatal(
			"terminal result OutputPorts() must return nil",
		)
	}

	terminalOutput, exists := result.TerminalOutput()
	if !exists {
		t.Fatal(
			"TerminalOutput() exists = false",
		)
	}

	if actual := terminalOutput.ContentType(); actual != runtime.ContentTypeApplicationJSON {
		t.Fatalf(
			"terminal content type = %q, want %q",
			actual,
			runtime.ContentTypeApplicationJSON,
		)
	}

	assertCoreInlinePayload(
		t,
		terminalOutput,
		`{"result":"completed"}`,
	)

	if actual := terminalOutput.Metadata()["source"]; actual != "terminal-test" {
		t.Fatalf(
			"terminal metadata source = %q, want %q",
			actual,
			"terminal-test",
		)
	}
}

func TestTerminalExecutorRejectsMissingAndAmbiguousInput(
	t *testing.T,
) {
	nodeContext := mustCoreNodeExecutionContext(
		t,
		context.Background(),
		"terminal-input-failure-node",
	)

	executor := terminalExecutor{}

	t.Run("missing input", func(t *testing.T) {
		result, err := executor.Execute(
			nodeContext,
			mustEmptyCoreNodeInput(t),
			mustCoreJSONObject(
				t,
				`{}`,
			),
		)

		requireCoreControlledFailure(
			t,
			result,
			err,
			failureCodeInvalidInput,
		)
	})

	t.Run("multiple payloads", func(t *testing.T) {
		result, err := executor.Execute(
			nodeContext,
			mustCoreNodeInput(
				t,
				mustCoreInlinePayload(
					t,
					runtime.ContentTypeTextPlain,
					"first",
					nil,
				),
				mustCoreInlinePayload(
					t,
					runtime.ContentTypeTextPlain,
					"second",
					nil,
				),
			),
			mustCoreJSONObject(
				t,
				`{}`,
			),
		)

		requireCoreControlledFailure(
			t,
			result,
			err,
			failureCodeInvalidInput,
		)
	})
}

func TestCoreExecutorsRejectInvalidConfiguration(
	t *testing.T,
) {
	inputPayload := mustCoreInlinePayload(
		t,
		runtime.ContentTypeTextPlain,
		"payload",
		nil,
	)

	tests := []struct {
		name          string
		nodeID        string
		executor      runtime.NodeExecutor
		input         runtime.NodeInput
		configuration workflow.JSONObject
	}{
		{
			name:     "static input",
			nodeID:   "static-invalid-config-node",
			executor: staticInputExecutor{maximumInlinePayloadBytes: 1024},
			input:    mustEmptyCoreNodeInput(t),
			configuration: mustCoreJSONObject(
				t,
				`{}`,
			),
		},
		{
			name:     "pass through",
			nodeID:   "pass-invalid-config-node",
			executor: passThroughExecutor{},
			input: mustCoreNodeInput(
				t,
				inputPayload,
			),
			configuration: mustCoreJSONObject(
				t,
				`{"unexpected":true}`,
			),
		},
		{
			name:   "delay",
			nodeID: "delay-invalid-config-node",
			executor: delayExecutor{
				waiter: TimerWaiter{},
			},
			input: mustCoreNodeInput(
				t,
				inputPayload,
			),
			configuration: mustCoreJSONObject(
				t,
				`{"delay":"invalid"}`,
			),
		},
		{
			name:     "terminal",
			nodeID:   "terminal-invalid-config-node",
			executor: terminalExecutor{},
			input: mustCoreNodeInput(
				t,
				inputPayload,
			),
			configuration: mustCoreJSONObject(
				t,
				`{"unexpected":true}`,
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodeContext := mustCoreNodeExecutionContext(
				t,
				context.Background(),
				test.nodeID,
			)

			result, err := test.executor.Execute(
				nodeContext,
				test.input,
				test.configuration,
			)

			requireCoreControlledFailure(
				t,
				result,
				err,
				failureCodeInvalidConfiguration,
			)
		})
	}
}

func TestCoreExecutorsRespectCancelledContext(
	t *testing.T,
) {
	inputPayload := mustCoreInlinePayload(
		t,
		runtime.ContentTypeTextPlain,
		"payload",
		nil,
	)

	delayWaitCalls := 0

	tests := []struct {
		name          string
		nodeID        string
		executor      runtime.NodeExecutor
		input         runtime.NodeInput
		configuration workflow.JSONObject
	}{
		{
			name:     "static input",
			nodeID:   "cancelled-static-node",
			executor: staticInputExecutor{maximumInlinePayloadBytes: 1024},
			input:    mustEmptyCoreNodeInput(t),
			configuration: mustCoreJSONObject(
				t,
				`{"value":"hello"}`,
			),
		},
		{
			name:     "pass through",
			nodeID:   "cancelled-pass-node",
			executor: passThroughExecutor{},
			input: mustCoreNodeInput(
				t,
				inputPayload,
			),
			configuration: mustCoreJSONObject(
				t,
				`{}`,
			),
		},
		{
			name:   "delay",
			nodeID: "cancelled-delay-node",
			executor: delayExecutor{
				waiter: WaiterFunc(
					func(
						context.Context,
						time.Duration,
					) error {
						delayWaitCalls++
						return nil
					},
				),
			},
			input: mustCoreNodeInput(
				t,
				inputPayload,
			),
			configuration: mustCoreJSONObject(
				t,
				`{"delay":"1s"}`,
			),
		},
		{
			name:     "terminal",
			nodeID:   "cancelled-terminal-node",
			executor: terminalExecutor{},
			input: mustCoreNodeInput(
				t,
				inputPayload,
			),
			configuration: mustCoreJSONObject(
				t,
				`{}`,
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parent, cancel := context.WithCancel(
				context.Background(),
			)

			nodeContext := mustCoreNodeExecutionContext(
				t,
				parent,
				test.nodeID,
			)

			cancel()

			result, err := test.executor.Execute(
				nodeContext,
				test.input,
				test.configuration,
			)

			if !errors.Is(err, context.Canceled) {
				t.Fatalf(
					"Execute() error = %v, want context.Canceled",
					err,
				)
			}

			if result.IsValid() {
				t.Fatal(
					"cancelled execution returned a valid NodeResult",
				)
			}
		})
	}

	if delayWaitCalls != 0 {
		t.Fatalf(
			"delay waiter call count = %d, want 0",
			delayWaitCalls,
		)
	}
}

func TestCoreExecutorRejectsMissingExecutionContext(
	t *testing.T,
) {
	executor := staticInputExecutor{
		maximumInlinePayloadBytes: 1024,
	}

	t.Run("nil context", func(t *testing.T) {
		result, err := executor.Execute(
			nil,
			mustEmptyCoreNodeInput(t),
			mustCoreJSONObject(
				t,
				`{"value":"hello"}`,
			),
		)

		if err == nil {
			t.Fatal(
				"Execute() returned nil error for a nil node context",
			)
		}

		if result.IsValid() {
			t.Fatal(
				"nil-context path returned a valid NodeResult",
			)
		}
	})

	t.Run("zero context", func(t *testing.T) {
		result, err := executor.Execute(
			&runtime.NodeExecutionContext{},
			mustEmptyCoreNodeInput(t),
			mustCoreJSONObject(
				t,
				`{"value":"hello"}`,
			),
		)

		if err == nil {
			t.Fatal(
				"Execute() returned nil error for a zero node context",
			)
		}

		if !strings.Contains(
			err.Error(),
			"parent context",
		) {
			t.Fatalf(
				"Execute() error = %q, want parent-context failure",
				err,
			)
		}

		if result.IsValid() {
			t.Fatal(
				"zero-context path returned a valid NodeResult",
			)
		}
	})
}

func mustCoreNodeExecutionContext(
	t *testing.T,
	parent context.Context,
	nodeID string,
) *runtime.NodeExecutionContext {
	t.Helper()

	baseTime := time.Date(
		2026,
		time.July,
		17,
		8,
		0,
		0,
		0,
		time.UTC,
	)

	workflowExecution, err :=
		execution.NewWorkflowExecution(
			execution.WorkflowExecutionID(
				"workflow-execution-"+nodeID,
			),
			workflow.CompanyID("company-1"),
			workflow.WorkflowID("workflow-1"),
			1,
			execution.ExecutionModeSync,
			baseTime,
		)
	if err != nil {
		t.Fatalf(
			"execution.NewWorkflowExecution() returned an unexpected error: %v",
			err,
		)
	}

	if err := workflowExecution.StartValidation(
		baseTime.Add(time.Minute),
	); err != nil {
		t.Fatalf(
			"workflow StartValidation() returned an unexpected error: %v",
			err,
		)
	}

	if err := workflowExecution.Start(
		baseTime.Add(2 * time.Minute),
	); err != nil {
		t.Fatalf(
			"workflow Start() returned an unexpected error: %v",
			err,
		)
	}

	executionContext, err := runtime.NewExecutionContext(
		parent,
		workflowExecution,
		"correlation-"+nodeID,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	nodeExecution, err := execution.NewNodeExecution(
		execution.NodeExecutionID(
			"node-execution-"+nodeID,
		),
		workflowExecution.ID(),
		workflow.NodeID(nodeID),
		baseTime,
	)
	if err != nil {
		t.Fatalf(
			"execution.NewNodeExecution() returned an unexpected error: %v",
			err,
		)
	}

	if err := nodeExecution.MarkReady(
		baseTime.Add(3 * time.Minute),
	); err != nil {
		t.Fatalf(
			"node MarkReady() returned an unexpected error: %v",
			err,
		)
	}

	if err := nodeExecution.Start(
		baseTime.Add(4 * time.Minute),
	); err != nil {
		t.Fatalf(
			"node Start() returned an unexpected error: %v",
			err,
		)
	}

	nodeContext, err := runtime.NewNodeExecutionContext(
		executionContext,
		nodeExecution,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeExecutionContext() returned an unexpected error: %v",
			err,
		)
	}

	return nodeContext
}

func mustCoreJSONObject(
	t *testing.T,
	value string,
) workflow.JSONObject {
	t.Helper()

	configuration, err := workflow.NewJSONObject(
		[]byte(value),
	)
	if err != nil {
		t.Fatalf(
			"workflow.NewJSONObject(%q) returned an unexpected error: %v",
			value,
			err,
		)
	}

	return configuration
}

func mustEmptyCoreNodeInput(
	t *testing.T,
) runtime.NodeInput {
	t.Helper()

	input, err := runtime.NewNodeInput(nil)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeInput(nil) returned an unexpected error: %v",
			err,
		)
	}

	return input
}

func mustCoreNodeInput(
	t *testing.T,
	payloads ...runtime.Payload,
) runtime.NodeInput {
	t.Helper()

	copiedPayloads := append(
		[]runtime.Payload(nil),
		payloads...,
	)

	input, err := runtime.NewNodeInput(
		map[string][]runtime.Payload{
			InputPortName: copiedPayloads,
		},
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewNodeInput() returned an unexpected error: %v",
			err,
		)
	}

	return input
}

func mustCoreInlinePayload(
	t *testing.T,
	contentType runtime.ContentType,
	value string,
	metadata map[string]string,
) runtime.Payload {
	t.Helper()

	payload, err := runtime.NewInlinePayload(
		contentType,
		[]byte(value),
		metadata,
		4096,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewInlinePayload() returned an unexpected error: %v",
			err,
		)
	}

	return payload
}

func requireCoreOutputPayload(
	t *testing.T,
	result runtime.NodeResult,
) runtime.Payload {
	t.Helper()

	payloads, exists, err := result.OutputPayloads(
		OutputPortName,
	)
	if err != nil {
		t.Fatalf(
			"OutputPayloads(%q) returned an unexpected error: %v",
			OutputPortName,
			err,
		)
	}

	if !exists {
		t.Fatalf(
			"OutputPayloads(%q) exists = false",
			OutputPortName,
		)
	}

	if len(payloads) != 1 {
		t.Fatalf(
			"output payload count = %d, want 1",
			len(payloads),
		)
	}

	if result.TotalOutputPayloadCount() != 1 {
		t.Fatalf(
			"TotalOutputPayloadCount() = %d, want 1",
			result.TotalOutputPayloadCount(),
		)
	}

	return payloads[0]
}

func assertCoreInlinePayload(
	t *testing.T,
	payload runtime.Payload,
	expected string,
) {
	t.Helper()

	if !payload.IsInline() {
		t.Fatal(
			"payload IsInline() = false",
		)
	}

	if payload.IsArtifact() {
		t.Fatal(
			"payload IsArtifact() = true",
		)
	}

	data, exists := payload.InlineData()
	if !exists {
		t.Fatal(
			"payload InlineData() exists = false",
		)
	}

	if actual := string(data); actual != expected {
		t.Fatalf(
			"payload inline data = %q, want %q",
			actual,
			expected,
		)
	}
}

func requireCoreControlledFailure(
	t *testing.T,
	result runtime.NodeResult,
	err error,
	expectedCode string,
) runtime.RuntimeFailure {
	t.Helper()

	if err != nil {
		t.Fatalf(
			"controlled failure returned an unexpected Go error: %v",
			err,
		)
	}

	if !result.IsValid() {
		t.Fatal(
			"controlled failure returned an invalid NodeResult",
		)
	}

	if !result.IsFailure() {
		t.Fatal(
			"result IsFailure() = false",
		)
	}

	if result.IsSuccess() {
		t.Fatal(
			"result IsSuccess() = true",
		)
	}

	failure, exists := result.Failure()
	if !exists {
		t.Fatal(
			"result Failure() exists = false",
		)
	}

	if actual := failure.Category(); actual != runtime.FailureCategoryValidation {
		t.Fatalf(
			"failure category = %q, want %q",
			actual,
			runtime.FailureCategoryValidation,
		)
	}

	if actual := failure.Code(); actual != expectedCode {
		t.Fatalf(
			"failure code = %q, want %q",
			actual,
			expectedCode,
		)
	}

	if failure.Retryable() {
		t.Fatal(
			"validation failure Retryable() = true",
		)
	}

	return failure
}

func TestExecutorRegistrationsBuildsDeterministicCoreSet(
	t *testing.T,
) {
	limits := mustCoreRuntimeLimits(
		t,
		4096,
	)

	waiter := WaiterFunc(
		func(
			context.Context,
			time.Duration,
		) error {
			return nil
		},
	)

	registrations, err := ExecutorRegistrations(
		limits,
		waiter,
	)
	if err != nil {
		t.Fatalf(
			"ExecutorRegistrations() returned an unexpected error: %v",
			err,
		)
	}

	if actual := len(registrations); actual != 4 {
		t.Fatalf(
			"registration count = %d, want 4",
			actual,
		)
	}

	actualIdentities := make(
		[]string,
		len(registrations),
	)

	for index, registration := range registrations {
		actualIdentities[index] =
			registration.Identity().String()

		if registration.Executor() == nil {
			t.Fatalf(
				"registration %d contains a nil executor",
				index,
			)
		}
	}

	expectedIdentities := []string{
		"core.delay@v1",
		"core.pass-through@v1",
		"core.static-input@v1",
		"core.terminal@v1",
	}

	if !reflect.DeepEqual(
		actualIdentities,
		expectedIdentities,
	) {
		t.Fatalf(
			"registration identities = %#v, want %#v",
			actualIdentities,
			expectedIdentities,
		)
	}

	registry, err := runtime.NewExecutorRegistry(
		registrations,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	if actual := registry.Len(); actual != 4 {
		t.Fatalf(
			"registry length = %d, want 4",
			actual,
		)
	}

	assertCoreExecutorType(
		t,
		registry,
		DelayPluginType,
		func(executor runtime.NodeExecutor) bool {
			_, ok := executor.(delayExecutor)
			return ok
		},
	)

	assertCoreExecutorType(
		t,
		registry,
		PassThroughPluginType,
		func(executor runtime.NodeExecutor) bool {
			_, ok := executor.(passThroughExecutor)
			return ok
		},
	)

	assertCoreExecutorType(
		t,
		registry,
		StaticInputPluginType,
		func(executor runtime.NodeExecutor) bool {
			staticExecutor, ok :=
				executor.(staticInputExecutor)

			return ok &&
				staticExecutor.maximumInlinePayloadBytes ==
					limits.MaximumInlinePayloadBytes()
		},
	)

	assertCoreExecutorType(
		t,
		registry,
		TerminalPluginType,
		func(executor runtime.NodeExecutor) bool {
			_, ok := executor.(terminalExecutor)
			return ok
		},
	)
}

func TestDefaultExecutorRegistrationsUsesTimerWaiter(
	t *testing.T,
) {
	registrations, err :=
		DefaultExecutorRegistrations(
			mustCoreRuntimeLimits(
				t,
				1024,
			),
		)
	if err != nil {
		t.Fatalf(
			"DefaultExecutorRegistrations() returned an unexpected error: %v",
			err,
		)
	}

	registry, err := runtime.NewExecutorRegistry(
		registrations,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewExecutorRegistry() returned an unexpected error: %v",
			err,
		)
	}

	executor, exists :=
		registry.LookupByTypeAndVersion(
			DelayPluginType,
			CorePluginVersion,
		)

	if !exists {
		t.Fatal(
			"delay executor was not registered",
		)
	}

	delay, ok := executor.(delayExecutor)
	if !ok {
		t.Fatalf(
			"delay executor type = %T, want delayExecutor",
			executor,
		)
	}

	if _, ok := delay.waiter.(TimerWaiter); !ok {
		t.Fatalf(
			"default waiter type = %T, want TimerWaiter",
			delay.waiter,
		)
	}
}

func TestExecutorRegistrationsRejectsInvalidLimits(
	t *testing.T,
) {
	_, err := ExecutorRegistrations(
		runtime.RuntimeLimits{},
		TimerWaiter{},
	)

	if err == nil {
		t.Fatal(
			"ExecutorRegistrations() returned nil error for invalid limits",
		)
	}
}

func TestExecutorRegistrationsRejectsNilWaiter(
	t *testing.T,
) {
	_, err := ExecutorRegistrations(
		mustCoreRuntimeLimits(
			t,
			1024,
		),
		nil,
	)

	if err == nil {
		t.Fatal(
			"ExecutorRegistrations() returned nil error for a nil waiter",
		)
	}
}

func TestExecutorRegistrationsReturnsIndependentCollections(
	t *testing.T,
) {
	limits := mustCoreRuntimeLimits(
		t,
		1024,
	)

	first, err := ExecutorRegistrations(
		limits,
		TimerWaiter{},
	)
	if err != nil {
		t.Fatalf(
			"first ExecutorRegistrations() returned an unexpected error: %v",
			err,
		)
	}

	second, err := ExecutorRegistrations(
		limits,
		TimerWaiter{},
	)
	if err != nil {
		t.Fatalf(
			"second ExecutorRegistrations() returned an unexpected error: %v",
			err,
		)
	}

	first[0] = first[1]

	actual := make(
		[]string,
		len(second),
	)

	for index, registration := range second {
		actual[index] =
			registration.Identity().String()
	}

	expected := []string{
		"core.delay@v1",
		"core.pass-through@v1",
		"core.static-input@v1",
		"core.terminal@v1",
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"second registration collection changed through first collection: got %#v, want %#v",
			actual,
			expected,
		)
	}
}

func assertCoreExecutorType(
	t *testing.T,
	registry runtime.ExecutorRegistry,
	pluginType workflow.PluginType,
	assertion func(runtime.NodeExecutor) bool,
) {
	t.Helper()

	executor, exists :=
		registry.LookupByTypeAndVersion(
			pluginType,
			CorePluginVersion,
		)

	if !exists {
		t.Fatalf(
			"executor %s@%s was not registered",
			pluginType,
			CorePluginVersion,
		)
	}

	if !assertion(executor) {
		t.Fatalf(
			"executor %s@%s has unexpected type or configuration: %T",
			pluginType,
			CorePluginVersion,
			executor,
		)
	}
}

func mustCoreRuntimeLimits(
	t *testing.T,
	maximumInlinePayloadBytes int,
) runtime.RuntimeLimits {
	t.Helper()

	limits, err := runtime.NewRuntimeLimits(
		DefaultQueueCapacity,
		DefaultCacheCapacity,
		maximumInlinePayloadBytes,
	)
	if err != nil {
		t.Fatalf(
			"runtime.NewRuntimeLimits() returned an unexpected error: %v",
			err,
		)
	}

	return limits
}

func TestWaiterFuncForwardsContextAndDuration(
	t *testing.T,
) {
	type contextKey string

	parent := context.WithValue(
		context.Background(),
		contextKey("request"),
		"request-value",
	)

	expectedDuration := 2 * time.Second
	called := false

	waiter := WaiterFunc(
		func(
			actualContext context.Context,
			actualDuration time.Duration,
		) error {
			called = true

			if actualContext != parent {
				t.Fatal(
					"Wait() received a different context",
				)
			}

			if actualDuration != expectedDuration {
				t.Fatalf(
					"duration = %v, want %v",
					actualDuration,
					expectedDuration,
				)
			}

			if actual := actualContext.Value(
				contextKey("request"),
			); actual != "request-value" {
				t.Fatalf(
					"context value = %#v, want %q",
					actual,
					"request-value",
				)
			}

			return nil
		},
	)

	if err := waiter.Wait(
		parent,
		expectedDuration,
	); err != nil {
		t.Fatalf(
			"Wait() returned an unexpected error: %v",
			err,
		)
	}

	if !called {
		t.Fatal(
			"waiter function was not called",
		)
	}
}

func TestWaiterFuncPreservesReturnedError(
	t *testing.T,
) {
	expectedError := errors.New(
		"controlled waiter failure",
	)

	waiter := WaiterFunc(
		func(
			context.Context,
			time.Duration,
		) error {
			return expectedError
		},
	)

	err := waiter.Wait(
		context.Background(),
		time.Second,
	)

	if !errors.Is(err, expectedError) {
		t.Fatalf(
			"Wait() error = %v, want %v",
			err,
			expectedError,
		)
	}
}

func TestNilWaiterFuncIsRejected(
	t *testing.T,
) {
	var waiter WaiterFunc

	err := waiter.Wait(
		context.Background(),
		time.Second,
	)

	if err == nil {
		t.Fatal(
			"nil WaiterFunc returned nil error",
		)
	}
}

func TestTimerWaiterCompletesAfterDuration(
	t *testing.T,
) {
	err := (TimerWaiter{}).Wait(
		context.Background(),
		time.Nanosecond,
	)
	if err != nil {
		t.Fatalf(
			"TimerWaiter.Wait() returned an unexpected error: %v",
			err,
		)
	}
}

func TestTimerWaiterReturnsCancellation(
	t *testing.T,
) {
	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	cancel()

	err := (TimerWaiter{}).Wait(
		ctx,
		time.Hour,
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"Wait() error = %v, want context.Canceled",
			err,
		)
	}
}

func TestTimerWaiterReturnsDeadlineExceeded(
	t *testing.T,
) {
	ctx, cancel := context.WithDeadline(
		context.Background(),
		time.Unix(0, 0),
	)
	defer cancel()

	err := (TimerWaiter{}).Wait(
		ctx,
		time.Hour,
	)

	if !errors.Is(
		err,
		context.DeadlineExceeded,
	) {
		t.Fatalf(
			"Wait() error = %v, want context.DeadlineExceeded",
			err,
		)
	}
}

func TestTimerWaiterRejectsNilContext(
	t *testing.T,
) {
	err := (TimerWaiter{}).Wait(
		nil,
		time.Second,
	)

	if err == nil {
		t.Fatal(
			"Wait() returned nil error for a nil context",
		)
	}
}

func TestTimerWaiterRejectsNonPositiveDurations(
	t *testing.T,
) {
	durations := map[string]time.Duration{
		"zero":     0,
		"negative": -time.Second,
	}

	for name, duration := range durations {
		t.Run(name, func(t *testing.T) {
			err := (TimerWaiter{}).Wait(
				context.Background(),
				duration,
			)

			if err == nil {
				t.Fatalf(
					"Wait() returned nil error for duration %v",
					duration,
				)
			}
		})
	}
}
