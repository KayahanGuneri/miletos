package plugin

import (
	errors "errors"
	fmt "fmt"
	workflow "miletos-go/internal/features/workflow"
	reflect "reflect"
	sync "sync"
	testing "testing"
)

func TestPluginMetadataNormalizesValues(
	t *testing.T,
) {
	metadata, err := NewPluginMetadata(
		" Pass Through ",
		" Returns the incoming payload ",
	)
	if err != nil {
		t.Fatalf(
			"NewPluginMetadata() returned an unexpected error: %v",
			err,
		)
	}

	if actual := metadata.DisplayName(); actual != "Pass Through" {
		t.Fatalf(
			"DisplayName() = %q, want %q",
			actual,
			"Pass Through",
		)
	}

	if actual := metadata.Description(); actual != "Returns the incoming payload" {
		t.Fatalf(
			"Description() = %q, want %q",
			actual,
			"Returns the incoming payload",
		)
	}

	if !metadata.IsValid() {
		t.Fatal(
			"IsValid() = false for valid metadata",
		)
	}
}

func TestPluginMetadataAllowsEmptyDescription(
	t *testing.T,
) {
	metadata, err := NewPluginMetadata(
		"Terminal",
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPluginMetadata() returned an unexpected error: %v",
			err,
		)
	}

	if metadata.Description() != "" {
		t.Fatalf(
			"Description() = %q, want empty",
			metadata.Description(),
		)
	}
}

func TestPluginMetadataRejectsBlankDisplayName(
	t *testing.T,
) {
	_, err := NewPluginMetadata(
		" ",
		"description",
	)
	if err == nil {
		t.Fatal(
			"NewPluginMetadata() accepted a blank display name",
		)
	}

	if (PluginMetadata{}).IsValid() {
		t.Fatal(
			"zero-value PluginMetadata is valid",
		)
	}
}

func TestConfigurationIssueValidatesFields(
	t *testing.T,
) {
	issue, err := NewConfigurationIssue(
		" duration ",
		" must be positive ",
	)
	if err != nil {
		t.Fatalf(
			"NewConfigurationIssue() returned an unexpected error: %v",
			err,
		)
	}

	if issue.Field != "duration" {
		t.Fatalf(
			"Field = %q, want %q",
			issue.Field,
			"duration",
		)
	}

	if issue.Reason != "must be positive" {
		t.Fatalf(
			"Reason = %q, want %q",
			issue.Reason,
			"must be positive",
		)
	}

	_, err = NewConfigurationIssue(
		" ",
		"invalid",
	)
	if err == nil {
		t.Fatal(
			"NewConfigurationIssue() accepted a blank field",
		)
	}

	_, err = NewConfigurationIssue(
		"duration",
		" ",
	)
	if err == nil {
		t.Fatal(
			"NewConfigurationIssue() accepted a blank reason",
		)
	}
}

func TestConfigurationReportIsDeterministicAndCopySafe(
	t *testing.T,
) {
	report := NewConfigurationReport(
		ConfigurationIssue{
			Field:  "duration",
			Reason: "must be positive",
		},
		ConfigurationIssue{
			Field:  " ",
			Reason: " ",
		},
		ConfigurationIssue{
			Field:  "duration",
			Reason: "is required",
		},
	)

	if report.IsValid() {
		t.Fatal(
			"IsValid() = true for report containing issues",
		)
	}

	if actual := report.Len(); actual != 3 {
		t.Fatalf(
			"Len() = %d, want %d",
			actual,
			3,
		)
	}

	expected := []ConfigurationIssue{
		{
			Field:  "configuration",
			Reason: "validation failed",
		},
		{
			Field:  "duration",
			Reason: "is required",
		},
		{
			Field:  "duration",
			Reason: "must be positive",
		},
	}

	first := report.Issues()

	if !reflect.DeepEqual(
		first,
		expected,
	) {
		t.Fatalf(
			"Issues() = %v, want %v",
			first,
			expected,
		)
	}

	if !report.HasField(" duration ") {
		t.Fatal(
			"HasField() = false for duration",
		)
	}

	first[0].Field = "changed"
	first[0].Reason = "changed"

	second := report.Issues()

	if !reflect.DeepEqual(
		second,
		expected,
	) {
		t.Fatalf(
			"stored issues changed after returned slice mutation: %v",
			second,
		)
	}

	validReport := ValidConfigurationReport()

	if !validReport.IsValid() {
		t.Fatal(
			"ValidConfigurationReport() is invalid",
		)
	}

	if validReport.Len() != 0 {
		t.Fatalf(
			"valid report length = %d, want %d",
			validReport.Len(),
			0,
		)
	}
}

func TestDescriptorStoresNormalizedContract(
	t *testing.T,
) {
	config := validDescriptorConfig(t)

	secondaryInput := mustPort(
		t,
		" secondary ",
		"Secondary",
	)
	primaryInput := mustPort(
		t,
		" input ",
		"Input",
	)

	config.InputPorts = []Port{
		secondaryInput,
		primaryInput,
	}

	descriptor, err := NewDescriptor(config)
	if err != nil {
		t.Fatalf(
			"NewDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	if !descriptor.IsValid() {
		t.Fatal(
			"IsValid() = false for a valid descriptor",
		)
	}

	if actual := descriptor.Identity().String(); actual != "core.pass-through@v1" {
		t.Fatalf(
			"identity = %q, want %q",
			actual,
			"core.pass-through@v1",
		)
	}

	if actual := descriptor.Metadata().DisplayName(); actual != "Pass Through" {
		t.Fatalf(
			"display name = %q, want %q",
			actual,
			"Pass Through",
		)
	}

	assertPortNames(
		t,
		descriptor.InputPorts(),
		[]string{
			"input",
			"secondary",
		},
	)

	assertPortNames(
		t,
		descriptor.OutputPorts(),
		[]string{
			"output",
		},
	)

	if !descriptor.HasInputPort(" input ") {
		t.Fatal(
			"HasInputPort() = false for input",
		)
	}

	if descriptor.HasInputPort("missing") {
		t.Fatal(
			"HasInputPort() = true for missing port",
		)
	}

	if !descriptor.HasOutputPort("output") {
		t.Fatal(
			"HasOutputPort() = false for output",
		)
	}

	if descriptor.HasOutputPort("missing") {
		t.Fatal(
			"HasOutputPort() = true for missing port",
		)
	}

	if !descriptor.InputEdgeConstraint().Allows(1) {
		t.Fatal(
			"input edge constraint rejected one edge",
		)
	}

	if !descriptor.OutputEdgeConstraint().Allows(100) {
		t.Fatal(
			"unlimited output constraint rejected a valid count",
		)
	}

	if descriptor.QueuePolicy().DefaultCapacity() != 64 {
		t.Fatalf(
			"queue capacity = %d, want %d",
			descriptor.QueuePolicy().DefaultCapacity(),
			64,
		)
	}

	if descriptor.CachePolicy().DefaultCapacity() != 1 {
		t.Fatalf(
			"cache capacity = %d, want %d",
			descriptor.CachePolicy().DefaultCapacity(),
			1,
		)
	}

	if descriptor.Distribution() != DistributionDistributable {
		t.Fatalf(
			"distribution = %q, want %q",
			descriptor.Distribution(),
			DistributionDistributable,
		)
	}
}

func TestDescriptorAllowsSamePortNameAcrossDirections(
	t *testing.T,
) {
	config := validDescriptorConfig(t)

	sharedInput := mustPort(
		t,
		"data",
		"Input data",
	)
	sharedOutput := mustPort(
		t,
		"data",
		"Output data",
	)

	config.InputPorts = []Port{
		sharedInput,
	}
	config.OutputPorts = []Port{
		sharedOutput,
	}

	descriptor, err := NewDescriptor(config)
	if err != nil {
		t.Fatalf(
			"NewDescriptor() rejected separate input/output namespaces: %v",
			err,
		)
	}

	if !descriptor.HasInputPort("data") {
		t.Fatal(
			"input namespace does not contain data",
		)
	}

	if !descriptor.HasOutputPort("data") {
		t.Fatal(
			"output namespace does not contain data",
		)
	}
}

func TestDescriptorRejectsDuplicatePortsWithinDirection(
	t *testing.T,
) {
	tests := []struct {
		name   string
		mutate func(*DescriptorConfig)
	}{
		{
			name: "duplicate input ports",
			mutate: func(config *DescriptorConfig) {
				config.InputPorts = []Port{
					mustPort(t, "input", "Input"),
					mustPort(t, " input ", "Second input"),
				}
			},
		},
		{
			name: "duplicate output ports",
			mutate: func(config *DescriptorConfig) {
				config.OutputPorts = []Port{
					mustPort(t, "output", "Output"),
					mustPort(t, " output ", "Second output"),
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validDescriptorConfig(t)
			test.mutate(&config)

			_, err := NewDescriptor(config)
			if err == nil {
				t.Fatal(
					"NewDescriptor() accepted duplicate ports",
				)
			}
		})
	}
}

func TestDescriptorRejectsInvalidRequiredContracts(
	t *testing.T,
) {
	tests := []struct {
		name   string
		mutate func(*DescriptorConfig)
	}{
		{
			name: "invalid identity",
			mutate: func(config *DescriptorConfig) {
				config.Identity = PluginIdentity{}
			},
		},
		{
			name: "invalid metadata",
			mutate: func(config *DescriptorConfig) {
				config.Metadata = PluginMetadata{}
			},
		},
		{
			name: "invalid input port",
			mutate: func(config *DescriptorConfig) {
				config.InputPorts = []Port{
					{},
				}
			},
		},
		{
			name: "invalid input edge constraint",
			mutate: func(config *DescriptorConfig) {
				config.InputEdgeConstraint = EdgeConstraint{}
			},
		},
		{
			name: "invalid output edge constraint",
			mutate: func(config *DescriptorConfig) {
				config.OutputEdgeConstraint = EdgeConstraint{}
			},
		},
		{
			name: "invalid queue policy",
			mutate: func(config *DescriptorConfig) {
				config.QueuePolicy = QueuePolicy{}
			},
		},
		{
			name: "invalid cache policy",
			mutate: func(config *DescriptorConfig) {
				config.CachePolicy = CachePolicy{}
			},
		},
		{
			name: "invalid distribution",
			mutate: func(config *DescriptorConfig) {
				config.Distribution = DistributionCapability(
					"UNKNOWN",
				)
			},
		},
		{
			name: "nil configuration validator",
			mutate: func(config *DescriptorConfig) {
				config.ConfigurationValidator = nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validDescriptorConfig(t)
			test.mutate(&config)

			_, err := NewDescriptor(config)
			if err == nil {
				t.Fatal(
					"NewDescriptor() accepted an invalid required contract",
				)
			}
		})
	}

	if (Descriptor{}).IsValid() {
		t.Fatal(
			"zero-value Descriptor is valid",
		)
	}
}

func TestDescriptorDefensivelyCopiesPortCollections(
	t *testing.T,
) {
	config := validDescriptorConfig(t)

	inputPorts := []Port{
		mustPort(t, "input", "Input"),
	}
	outputPorts := []Port{
		mustPort(t, "output", "Output"),
	}

	config.InputPorts = inputPorts
	config.OutputPorts = outputPorts

	descriptor, err := NewDescriptor(config)
	if err != nil {
		t.Fatalf(
			"NewDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	inputPorts[0] = mustPort(
		t,
		"changed-input",
		"Changed",
	)
	outputPorts[0] = mustPort(
		t,
		"changed-output",
		"Changed",
	)

	assertPortNames(
		t,
		descriptor.InputPorts(),
		[]string{
			"input",
		},
	)

	assertPortNames(
		t,
		descriptor.OutputPorts(),
		[]string{
			"output",
		},
	)

	returnedInputs := descriptor.InputPorts()
	returnedInputs[0] = mustPort(
		t,
		"returned-change",
		"Changed",
	)

	returnedOutputs := descriptor.OutputPorts()
	returnedOutputs[0] = mustPort(
		t,
		"returned-change",
		"Changed",
	)

	assertPortNames(
		t,
		descriptor.InputPorts(),
		[]string{
			"input",
		},
	)

	assertPortNames(
		t,
		descriptor.OutputPorts(),
		[]string{
			"output",
		},
	)

	cloned := descriptor.clone()

	cloned.inputPorts[0] = mustPort(
		t,
		"clone-change",
		"Changed",
	)
	cloned.inputPortNames["clone-change"] = struct{}{}

	assertPortNames(
		t,
		descriptor.InputPorts(),
		[]string{
			"input",
		},
	)

	if descriptor.HasInputPort("clone-change") {
		t.Fatal(
			"clone map mutation changed the source descriptor",
		)
	}
}

func TestDescriptorConfigurationValidationIsDeterministic(
	t *testing.T,
) {
	config := validDescriptorConfig(t)

	config.ConfigurationValidator = func(
		configuration workflow.JSONObject,
	) []ConfigurationIssue {
		if configuration.String() == "{}" {
			return []ConfigurationIssue{
				{
					Field:  "value",
					Reason: "is required",
				},
				{
					Field:  "format",
					Reason: "is invalid",
				},
			}
		}

		return nil
	}

	descriptor, err := NewDescriptor(config)
	if err != nil {
		t.Fatalf(
			"NewDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	emptyConfiguration, err := workflow.NewJSONObject(nil)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	first := descriptor.ValidateConfiguration(
		emptyConfiguration,
	)

	expected := []ConfigurationIssue{
		{
			Field:  "format",
			Reason: "is invalid",
		},
		{
			Field:  "value",
			Reason: "is required",
		},
	}

	if !reflect.DeepEqual(
		first.Issues(),
		expected,
	) {
		t.Fatalf(
			"configuration issues = %v, want %v",
			first.Issues(),
			expected,
		)
	}

	returned := first.Issues()
	returned[0].Field = "changed"

	second := descriptor.ValidateConfiguration(
		emptyConfiguration,
	)

	if !reflect.DeepEqual(
		second.Issues(),
		expected,
	) {
		t.Fatalf(
			"second validation issues = %v, want %v",
			second.Issues(),
			expected,
		)
	}

	validConfiguration, err := workflow.NewJSONObject(
		[]byte(`{"value":"hello"}`),
	)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	validReport := descriptor.ValidateConfiguration(
		validConfiguration,
	)

	if !validReport.IsValid() {
		t.Fatalf(
			"valid configuration produced issues: %v",
			validReport.Issues(),
		)
	}
}

func validDescriptorConfig(
	t *testing.T,
) DescriptorConfig {
	t.Helper()

	identity, err := NewPluginIdentity(
		workflow.PluginType("core.pass-through"),
		workflow.PluginVersion("v1"),
	)
	if err != nil {
		t.Fatalf(
			"NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	metadata, err := NewPluginMetadata(
		"Pass Through",
		"Returns the incoming payload",
	)
	if err != nil {
		t.Fatalf(
			"NewPluginMetadata() returned an unexpected error: %v",
			err,
		)
	}

	queuePolicy, err := NewQueuePolicy(
		64,
		OverflowStrategyDropOldest,
	)
	if err != nil {
		t.Fatalf(
			"NewQueuePolicy() returned an unexpected error: %v",
			err,
		)
	}

	cachePolicy, err := NewCachePolicy(
		1,
		OverflowStrategyDropOldest,
	)
	if err != nil {
		t.Fatalf(
			"NewCachePolicy() returned an unexpected error: %v",
			err,
		)
	}

	return DescriptorConfig{
		Identity: identity,
		Metadata: metadata,

		InputPorts: []Port{
			mustPort(
				t,
				"input",
				"Input",
			),
		},
		OutputPorts: []Port{
			mustPort(
				t,
				"output",
				"Output",
			),
		},

		InputEdgeConstraint: NewExactEdgeConstraint(
			1,
		),
		OutputEdgeConstraint: NewUnlimitedEdgeConstraint(
			0,
		),

		QueuePolicy: queuePolicy,
		CachePolicy: cachePolicy,

		Distribution: DistributionDistributable,

		ConfigurationValidator: func(
			workflow.JSONObject,
		) []ConfigurationIssue {
			return nil
		},
	}
}

func mustPort(
	t *testing.T,
	name string,
	displayName string,
) Port {
	t.Helper()

	port, err := NewPort(
		name,
		displayName,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPort() returned an unexpected error: %v",
			err,
		)
	}

	return port
}

func assertPortNames(
	t *testing.T,
	ports []Port,
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

func TestRegistrySupportsDifferentVersionsAndDeterministicList(
	t *testing.T,
) {
	passThroughVersionTwo := mustDescriptorWithIdentity(
		t,
		"core.pass-through",
		"v2",
		"Pass Through V2",
	)

	delayVersionOne := mustDescriptorWithIdentity(
		t,
		"core.delay",
		"v1",
		"Delay",
	)

	passThroughVersionOne := mustDescriptorWithIdentity(
		t,
		"core.pass-through",
		"v1",
		"Pass Through",
	)

	registry, err := NewRegistry(
		[]Descriptor{
			passThroughVersionTwo,
			delayVersionOne,
			passThroughVersionOne,
		},
	)
	if err != nil {
		t.Fatalf(
			"NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	if actual := registry.Len(); actual != 3 {
		t.Fatalf(
			"Len() = %d, want %d",
			actual,
			3,
		)
	}

	if registry.IsEmpty() {
		t.Fatal(
			"IsEmpty() = true for a populated registry",
		)
	}

	assertDescriptorIdentities(
		t,
		registry.List(),
		[]string{
			"core.delay@v1",
			"core.pass-through@v1",
			"core.pass-through@v2",
		},
	)

	for iteration := 0; iteration < 20; iteration++ {
		assertDescriptorIdentities(
			t,
			registry.List(),
			[]string{
				"core.delay@v1",
				"core.pass-through@v1",
				"core.pass-through@v2",
			},
		)
	}

	if !registry.HasType(
		workflow.PluginType(" core.pass-through "),
	) {
		t.Fatal(
			"HasType() = false for a registered plugin type",
		)
	}

	if registry.HasType(
		workflow.PluginType("core.unknown"),
	) {
		t.Fatal(
			"HasType() = true for an unknown plugin type",
		)
	}
}

func TestRegistryRejectsDuplicateIdentity(
	t *testing.T,
) {
	descriptor := mustDescriptorWithIdentity(
		t,
		"core.delay",
		"v1",
		"Delay",
	)

	_, err := NewRegistry(
		[]Descriptor{
			descriptor,
			descriptor,
		},
	)
	if err == nil {
		t.Fatal(
			"NewRegistry() accepted a duplicate plugin identity",
		)
	}

	var duplicateError *DuplicatePluginError
	if !errors.As(err, &duplicateError) {
		t.Fatalf(
			"error type = %T, want *DuplicatePluginError",
			err,
		)
	}

	if actual := duplicateError.Identity.String(); actual != "core.delay@v1" {
		t.Fatalf(
			"duplicate identity = %q, want %q",
			actual,
			"core.delay@v1",
		)
	}

	expectedMessage :=
		"plugin registry contains duplicate descriptor core.delay@v1"

	if actual := duplicateError.Error(); actual != expectedMessage {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expectedMessage,
		)
	}
}

func TestRegistryRejectsInvalidDescriptor(
	t *testing.T,
) {
	_, err := NewRegistry(
		[]Descriptor{
			{},
		},
	)
	if err == nil {
		t.Fatal(
			"NewRegistry() accepted an invalid descriptor",
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if actual := validationError.Field; actual != "descriptors[0]" {
		t.Fatalf(
			"validation field = %q, want %q",
			actual,
			"descriptors[0]",
		)
	}
}

func TestRegistrySupportsEmptyConstructionAndZeroValueReads(
	t *testing.T,
) {
	registry, err := NewRegistry(nil)
	if err != nil {
		t.Fatalf(
			"NewRegistry(nil) returned an unexpected error: %v",
			err,
		)
	}

	if !registry.IsEmpty() {
		t.Fatal(
			"empty registry IsEmpty() = false",
		)
	}

	if registry.Len() != 0 {
		t.Fatalf(
			"empty registry length = %d, want %d",
			registry.Len(),
			0,
		)
	}

	if registry.List() != nil {
		t.Fatalf(
			"empty registry List() = %v, want nil",
			registry.List(),
		)
	}

	var zeroValue Registry

	if !zeroValue.IsEmpty() {
		t.Fatal(
			"zero-value registry IsEmpty() = false",
		)
	}

	if _, found := zeroValue.Lookup(PluginIdentity{}); found {
		t.Fatal(
			"zero-value registry resolved an invalid identity",
		)
	}

	if zeroValue.HasType(
		workflow.PluginType("core.delay"),
	) {
		t.Fatal(
			"zero-value registry contains a plugin type",
		)
	}

	var nilDuplicateError *DuplicatePluginError

	if actual := nilDuplicateError.Error(); actual !=
		"plugin registry contains a duplicate descriptor" {
		t.Fatalf(
			"nil duplicate error = %q",
			actual,
		)
	}
}

func TestRegistryLookupByIdentityAndVersion(
	t *testing.T,
) {
	descriptor := mustDescriptorWithIdentity(
		t,
		"core.delay",
		"v1",
		"Delay",
	)

	registry, err := NewRegistry(
		[]Descriptor{
			descriptor,
		},
	)
	if err != nil {
		t.Fatalf(
			"NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	identity, err := NewPluginIdentity(
		workflow.PluginType(" core.delay "),
		workflow.PluginVersion(" v1 "),
	)
	if err != nil {
		t.Fatalf(
			"NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	resolved, found := registry.Lookup(identity)
	if !found {
		t.Fatal(
			"Lookup() did not resolve a registered identity",
		)
	}

	if actual := resolved.Metadata().DisplayName(); actual != "Delay" {
		t.Fatalf(
			"resolved display name = %q, want %q",
			actual,
			"Delay",
		)
	}

	resolvedByValues, found := registry.LookupByTypeAndVersion(
		workflow.PluginType(" core.delay "),
		workflow.PluginVersion(" v1 "),
	)
	if !found {
		t.Fatal(
			"LookupByTypeAndVersion() did not resolve a registered plugin",
		)
	}

	if actual := resolvedByValues.Identity().String(); actual != "core.delay@v1" {
		t.Fatalf(
			"resolved identity = %q, want %q",
			actual,
			"core.delay@v1",
		)
	}

	_, found = registry.LookupByTypeAndVersion(
		workflow.PluginType("core.delay"),
		workflow.PluginVersion("v2"),
	)
	if found {
		t.Fatal(
			"LookupByTypeAndVersion() resolved an unknown version",
		)
	}

	_, found = registry.LookupByTypeAndVersion(
		workflow.PluginType(" "),
		workflow.PluginVersion("v1"),
	)
	if found {
		t.Fatal(
			"LookupByTypeAndVersion() resolved an invalid type",
		)
	}
}

func TestRegistryDefensivelyCopiesConstructorAndReadResults(
	t *testing.T,
) {
	descriptor := mustDescriptorWithIdentity(
		t,
		"core.pass-through",
		"v1",
		"Pass Through",
	)

	input := []Descriptor{
		descriptor,
	}

	registry, err := NewRegistry(input)
	if err != nil {
		t.Fatalf(
			"NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	input[0].inputPorts[0] = mustPort(
		t,
		"changed-input",
		"Changed",
	)
	input[0].inputPortNames["changed-input"] = struct{}{}

	resolved, found := registry.Lookup(
		descriptor.Identity(),
	)
	if !found {
		t.Fatal(
			"Lookup() did not resolve the stored descriptor",
		)
	}

	assertPortNames(
		t,
		resolved.InputPorts(),
		[]string{
			"input",
		},
	)

	if resolved.HasInputPort("changed-input") {
		t.Fatal(
			"constructor input mutation changed the registry",
		)
	}

	resolved.inputPorts[0] = mustPort(
		t,
		"lookup-change",
		"Changed",
	)
	resolved.inputPortNames["lookup-change"] = struct{}{}

	secondLookup, found := registry.Lookup(
		descriptor.Identity(),
	)
	if !found {
		t.Fatal(
			"second Lookup() did not resolve the descriptor",
		)
	}

	assertPortNames(
		t,
		secondLookup.InputPorts(),
		[]string{
			"input",
		},
	)

	if secondLookup.HasInputPort("lookup-change") {
		t.Fatal(
			"lookup result mutation changed the registry",
		)
	}

	firstList := registry.List()
	firstList[0].outputPorts[0] = mustPort(
		t,
		"list-change",
		"Changed",
	)
	firstList[0].outputPortNames["list-change"] = struct{}{}

	secondList := registry.List()

	assertPortNames(
		t,
		secondList[0].OutputPorts(),
		[]string{
			"output",
		},
	)

	if secondList[0].HasOutputPort("list-change") {
		t.Fatal(
			"list result mutation changed the registry",
		)
	}
}

func TestRegistrySupportsConcurrentReads(
	t *testing.T,
) {
	descriptors := []Descriptor{
		mustDescriptorWithIdentity(
			t,
			"core.static-input",
			"v1",
			"Static Input",
		),
		mustDescriptorWithIdentity(
			t,
			"core.pass-through",
			"v1",
			"Pass Through",
		),
		mustDescriptorWithIdentity(
			t,
			"core.delay",
			"v1",
			"Delay",
		),
		mustDescriptorWithIdentity(
			t,
			"core.terminal",
			"v1",
			"Terminal",
		),
	}

	registry, err := NewRegistry(descriptors)
	if err != nil {
		t.Fatalf(
			"NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	delayIdentity, err := NewPluginIdentity(
		workflow.PluginType("core.delay"),
		workflow.PluginVersion("v1"),
	)
	if err != nil {
		t.Fatalf(
			"NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	const workerCount = 32
	const iterationCount = 200

	var waitGroup sync.WaitGroup

	errorChannel := make(
		chan error,
		workerCount,
	)

	for worker := 0; worker < workerCount; worker++ {
		waitGroup.Add(1)

		go func() {
			defer waitGroup.Done()

			for iteration := 0; iteration < iterationCount; iteration++ {
				resolved, found := registry.Lookup(
					delayIdentity,
				)
				if !found {
					errorChannel <- fmt.Errorf(
						"delay plugin was not found",
					)

					return
				}

				if resolved.Identity() != delayIdentity {
					errorChannel <- fmt.Errorf(
						"resolved identity = %s, want %s",
						resolved.Identity(),
						delayIdentity,
					)

					return
				}

				listed := registry.List()
				if len(listed) != 4 {
					errorChannel <- fmt.Errorf(
						"registry list length = %d, want %d",
						len(listed),
						4,
					)

					return
				}

				if !registry.HasType(
					workflow.PluginType("core.terminal"),
				) {
					errorChannel <- fmt.Errorf(
						"terminal type was not found",
					)

					return
				}
			}
		}()
	}

	waitGroup.Wait()
	close(errorChannel)

	for concurrentError := range errorChannel {
		t.Fatal(concurrentError)
	}
}

func TestRegistryListOrderDoesNotDependOnInputOrder(
	t *testing.T,
) {
	firstInput := []Descriptor{
		mustDescriptorWithIdentity(
			t,
			"core.terminal",
			"v1",
			"Terminal",
		),
		mustDescriptorWithIdentity(
			t,
			"core.delay",
			"v2",
			"Delay V2",
		),
		mustDescriptorWithIdentity(
			t,
			"core.delay",
			"v1",
			"Delay V1",
		),
	}

	secondInput := []Descriptor{
		firstInput[2],
		firstInput[0],
		firstInput[1],
	}

	firstRegistry, err := NewRegistry(firstInput)
	if err != nil {
		t.Fatalf(
			"first NewRegistry() error: %v",
			err,
		)
	}

	secondRegistry, err := NewRegistry(secondInput)
	if err != nil {
		t.Fatalf(
			"second NewRegistry() error: %v",
			err,
		)
	}

	firstIdentities := descriptorIdentityStrings(
		firstRegistry.List(),
	)

	secondIdentities := descriptorIdentityStrings(
		secondRegistry.List(),
	)

	if !reflect.DeepEqual(
		firstIdentities,
		secondIdentities,
	) {
		t.Fatalf(
			"registry order differs:\nfirst:  %v\nsecond: %v",
			firstIdentities,
			secondIdentities,
		)
	}
}

func mustDescriptorWithIdentity(
	t *testing.T,
	pluginType string,
	version string,
	displayName string,
) Descriptor {
	t.Helper()

	config := validDescriptorConfig(t)

	identity, err := NewPluginIdentity(
		workflow.PluginType(pluginType),
		workflow.PluginVersion(version),
	)
	if err != nil {
		t.Fatalf(
			"NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	metadata, err := NewPluginMetadata(
		displayName,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPluginMetadata() returned an unexpected error: %v",
			err,
		)
	}

	config.Identity = identity
	config.Metadata = metadata

	descriptor, err := NewDescriptor(config)
	if err != nil {
		t.Fatalf(
			"NewDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	return descriptor
}

func assertDescriptorIdentities(
	t *testing.T,
	descriptors []Descriptor,
	expected []string,
) {
	t.Helper()

	actual := descriptorIdentityStrings(
		descriptors,
	)

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

func descriptorIdentityStrings(
	descriptors []Descriptor,
) []string {
	identities := make(
		[]string,
		len(descriptors),
	)

	for index, descriptor := range descriptors {
		identities[index] = descriptor.Identity().String()
	}

	return identities
}

func TestPluginIdentityNormalizesAndFormatsValues(
	t *testing.T,
) {
	identity, err := NewPluginIdentity(
		workflow.PluginType(" core.pass-through "),
		workflow.PluginVersion(" v1 "),
	)
	if err != nil {
		t.Fatalf(
			"NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	if actual := identity.Type().String(); actual != "core.pass-through" {
		t.Fatalf(
			"Type() = %q, want %q",
			actual,
			"core.pass-through",
		)
	}

	if actual := identity.Version().String(); actual != "v1" {
		t.Fatalf(
			"Version() = %q, want %q",
			actual,
			"v1",
		)
	}

	if actual := identity.String(); actual != "core.pass-through@v1" {
		t.Fatalf(
			"String() = %q, want %q",
			actual,
			"core.pass-through@v1",
		)
	}

	if !identity.IsValid() {
		t.Fatal(
			"IsValid() = false for a valid plugin identity",
		)
	}
}

func TestPluginIdentityRejectsBlankValues(
	t *testing.T,
) {
	tests := []struct {
		name       string
		pluginType workflow.PluginType
		version    workflow.PluginVersion
	}{
		{
			name:       "blank plugin type",
			pluginType: workflow.PluginType(" "),
			version:    workflow.PluginVersion("v1"),
		},
		{
			name:       "blank plugin version",
			pluginType: workflow.PluginType("core.delay"),
			version:    workflow.PluginVersion(" "),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewPluginIdentity(
				test.pluginType,
				test.version,
			)
			if err == nil {
				t.Fatal(
					"NewPluginIdentity() returned nil error for an invalid identity",
				)
			}
		})
	}

	var zeroValue PluginIdentity
	if zeroValue.IsValid() {
		t.Fatal(
			"zero-value PluginIdentity is valid",
		)
	}
}

func TestPluginIdentitySupportsSameTypeWithDifferentVersions(
	t *testing.T,
) {
	versionOne, err := NewPluginIdentity(
		workflow.PluginType("core.delay"),
		workflow.PluginVersion("v1"),
	)
	if err != nil {
		t.Fatalf(
			"first NewPluginIdentity() error: %v",
			err,
		)
	}

	versionTwo, err := NewPluginIdentity(
		workflow.PluginType("core.delay"),
		workflow.PluginVersion("v2"),
	)
	if err != nil {
		t.Fatalf(
			"second NewPluginIdentity() error: %v",
			err,
		)
	}

	if versionOne == versionTwo {
		t.Fatal(
			"different plugin versions produced equal identities",
		)
	}

	if !versionOne.Less(versionTwo) {
		t.Fatal(
			"v1 identity does not sort before v2 identity",
		)
	}
}

func TestPluginIdentityOrderingUsesTypeThenVersion(
	t *testing.T,
) {
	delay, err := NewPluginIdentity(
		workflow.PluginType("core.delay"),
		workflow.PluginVersion("v2"),
	)
	if err != nil {
		t.Fatalf(
			"delay identity error: %v",
			err,
		)
	}

	passThrough, err := NewPluginIdentity(
		workflow.PluginType("core.pass-through"),
		workflow.PluginVersion("v1"),
	)
	if err != nil {
		t.Fatalf(
			"pass-through identity error: %v",
			err,
		)
	}

	if !delay.Less(passThrough) {
		t.Fatal(
			"core.delay does not sort before core.pass-through",
		)
	}
}

func TestPortNormalizesValues(
	t *testing.T,
) {
	port, err := NewPort(
		" input ",
		" Input payload ",
		" Receives the node payload ",
	)
	if err != nil {
		t.Fatalf(
			"NewPort() returned an unexpected error: %v",
			err,
		)
	}

	if actual := port.Name(); actual != "input" {
		t.Fatalf(
			"Name() = %q, want %q",
			actual,
			"input",
		)
	}

	if actual := port.DisplayName(); actual != "Input payload" {
		t.Fatalf(
			"DisplayName() = %q, want %q",
			actual,
			"Input payload",
		)
	}

	if actual := port.Description(); actual != "Receives the node payload" {
		t.Fatalf(
			"Description() = %q, want %q",
			actual,
			"Receives the node payload",
		)
	}

	if !port.IsValid() {
		t.Fatal(
			"IsValid() = false for a valid port",
		)
	}
}

func TestPortAllowsOptionalMetadata(
	t *testing.T,
) {
	port, err := NewPort(
		"output",
		"",
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPort() returned an unexpected error: %v",
			err,
		)
	}

	if port.DisplayName() != "" {
		t.Fatalf(
			"DisplayName() = %q, want empty",
			port.DisplayName(),
		)
	}

	if port.Description() != "" {
		t.Fatalf(
			"Description() = %q, want empty",
			port.Description(),
		)
	}
}

func TestPortRejectsBlankName(
	t *testing.T,
) {
	_, err := NewPort(
		" ",
		"Input",
		"",
	)
	if err == nil {
		t.Fatal(
			"NewPort() returned nil error for a blank name",
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	if validationError.Field != "port.name" {
		t.Fatalf(
			"validation field = %q, want %q",
			validationError.Field,
			"port.name",
		)
	}

	var zeroValue Port
	if zeroValue.IsValid() {
		t.Fatal(
			"zero-value Port is valid",
		)
	}
}

func TestEdgeConstraintSupportsBoundedRange(
	t *testing.T,
) {
	constraint, err := NewBoundedEdgeConstraint(
		2,
		5,
	)
	if err != nil {
		t.Fatalf(
			"NewBoundedEdgeConstraint() returned an unexpected error: %v",
			err,
		)
	}

	if actual := constraint.Minimum(); actual != 2 {
		t.Fatalf(
			"Minimum() = %d, want %d",
			actual,
			2,
		)
	}

	maximum, limited := constraint.Maximum()
	if !limited {
		t.Fatal(
			"Maximum() reported an unlimited constraint",
		)
	}

	if maximum != 5 {
		t.Fatalf(
			"Maximum() = %d, want %d",
			maximum,
			5,
		)
	}

	if constraint.Allows(1) {
		t.Fatal(
			"Allows() accepted a value below minimum",
		)
	}

	if !constraint.Allows(2) {
		t.Fatal(
			"Allows() rejected the minimum boundary",
		)
	}

	if !constraint.Allows(5) {
		t.Fatal(
			"Allows() rejected the maximum boundary",
		)
	}

	if constraint.Allows(6) {
		t.Fatal(
			"Allows() accepted a value above maximum",
		)
	}
}

func TestEdgeConstraintSupportsUnlimitedMaximum(
	t *testing.T,
) {
	constraint := NewUnlimitedEdgeConstraint(1)

	if !constraint.IsValid() {
		t.Fatal(
			"unlimited constraint is invalid",
		)
	}

	if !constraint.IsUnlimited() {
		t.Fatal(
			"IsUnlimited() = false for an unlimited constraint",
		)
	}

	if maximum, limited := constraint.Maximum(); limited || maximum != 0 {
		t.Fatalf(
			"Maximum() = (%d, %t), want (0, false)",
			maximum,
			limited,
		)
	}

	if !constraint.Allows(1) {
		t.Fatal(
			"Allows() rejected the minimum",
		)
	}

	if !constraint.Allows(1_000_000) {
		t.Fatal(
			"Allows() rejected a value for an unlimited maximum",
		)
	}
}

func TestEdgeConstraintSupportsExactCount(
	t *testing.T,
) {
	constraint := NewExactEdgeConstraint(1)

	if !constraint.Allows(1) {
		t.Fatal(
			"exact constraint rejected its expected count",
		)
	}

	if constraint.Allows(0) {
		t.Fatal(
			"exact constraint accepted a lower count",
		)
	}

	if constraint.Allows(2) {
		t.Fatal(
			"exact constraint accepted a higher count",
		)
	}
}

func TestEdgeConstraintRejectsMaximumBelowMinimum(
	t *testing.T,
) {
	_, err := NewBoundedEdgeConstraint(
		3,
		2,
	)
	if err == nil {
		t.Fatal(
			"NewBoundedEdgeConstraint() returned nil error for maximum below minimum",
		)
	}

	var validationError *ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf(
			"error type = %T, want *ValidationError",
			err,
		)
	}

	var zeroValue EdgeConstraint
	if zeroValue.IsValid() {
		t.Fatal(
			"zero-value EdgeConstraint is valid",
		)
	}

	if zeroValue.Allows(0) {
		t.Fatal(
			"zero-value EdgeConstraint accepted a count",
		)
	}
}

func TestParseOverflowStrategyNormalizesSupportedValue(
	t *testing.T,
) {
	strategy, err := ParseOverflowStrategy(
		"  drop_oldest  ",
	)
	if err != nil {
		t.Fatalf(
			"ParseOverflowStrategy() returned an unexpected error: %v",
			err,
		)
	}

	if strategy != OverflowStrategyDropOldest {
		t.Fatalf(
			"strategy = %q, want %q",
			strategy,
			OverflowStrategyDropOldest,
		)
	}

	if strategy.String() != "DROP_OLDEST" {
		t.Fatalf(
			"String() = %q, want %q",
			strategy.String(),
			"DROP_OLDEST",
		)
	}
}

func TestQueueAndCachePoliciesAcceptDropOldest(
	t *testing.T,
) {
	queuePolicy, err := NewQueuePolicy(
		64,
		OverflowStrategyDropOldest,
	)
	if err != nil {
		t.Fatalf(
			"NewQueuePolicy() returned an unexpected error: %v",
			err,
		)
	}

	cachePolicy, err := NewCachePolicy(
		1,
		OverflowStrategyDropOldest,
	)
	if err != nil {
		t.Fatalf(
			"NewCachePolicy() returned an unexpected error: %v",
			err,
		)
	}

	if !queuePolicy.IsValid() {
		t.Fatal(
			"queue policy is invalid",
		)
	}

	if queuePolicy.DefaultCapacity() != 64 {
		t.Fatalf(
			"queue capacity = %d, want %d",
			queuePolicy.DefaultCapacity(),
			64,
		)
	}

	if queuePolicy.OverflowStrategy() != OverflowStrategyDropOldest {
		t.Fatalf(
			"queue strategy = %q, want %q",
			queuePolicy.OverflowStrategy(),
			OverflowStrategyDropOldest,
		)
	}

	if !cachePolicy.IsValid() {
		t.Fatal(
			"cache policy is invalid",
		)
	}

	if cachePolicy.DefaultCapacity() != 1 {
		t.Fatalf(
			"cache capacity = %d, want %d",
			cachePolicy.DefaultCapacity(),
			1,
		)
	}
}

func TestQueueAndCachePoliciesRejectInvalidValues(
	t *testing.T,
) {
	_, err := NewQueuePolicy(
		0,
		OverflowStrategyDropOldest,
	)
	if err == nil {
		t.Fatal(
			"NewQueuePolicy() accepted zero capacity",
		)
	}

	_, err = NewQueuePolicy(
		64,
		OverflowStrategy("UNKNOWN"),
	)
	if err == nil {
		t.Fatal(
			"NewQueuePolicy() accepted an unknown strategy",
		)
	}

	_, err = NewCachePolicy(
		0,
		OverflowStrategyDropOldest,
	)
	if err == nil {
		t.Fatal(
			"NewCachePolicy() accepted zero capacity",
		)
	}

	_, err = NewCachePolicy(
		1,
		OverflowStrategy("UNKNOWN"),
	)
	if err == nil {
		t.Fatal(
			"NewCachePolicy() accepted an unknown strategy",
		)
	}

	if (QueuePolicy{}).IsValid() {
		t.Fatal(
			"zero-value QueuePolicy is valid",
		)
	}

	if (CachePolicy{}).IsValid() {
		t.Fatal(
			"zero-value CachePolicy is valid",
		)
	}

	if OverflowStrategy("UNKNOWN").IsValid() {
		t.Fatal(
			"unknown overflow strategy is valid",
		)
	}
}

func TestDistributionCapabilityNormalizesSupportedValues(
	t *testing.T,
) {
	localOnly, err := ParseDistributionCapability(
		"  local_only  ",
	)
	if err != nil {
		t.Fatalf(
			"local ParseDistributionCapability() error: %v",
			err,
		)
	}

	distributable, err := ParseDistributionCapability(
		" distributable ",
	)
	if err != nil {
		t.Fatalf(
			"distributed ParseDistributionCapability() error: %v",
			err,
		)
	}

	if localOnly != DistributionLocalOnly {
		t.Fatalf(
			"local capability = %q, want %q",
			localOnly,
			DistributionLocalOnly,
		)
	}

	if distributable != DistributionDistributable {
		t.Fatalf(
			"distributed capability = %q, want %q",
			distributable,
			DistributionDistributable,
		)
	}

	if localOnly.SupportsAsync() {
		t.Fatal(
			"LOCAL_ONLY supports async execution",
		)
	}

	if !distributable.SupportsAsync() {
		t.Fatal(
			"DISTRIBUTABLE does not support async execution",
		)
	}

	if !localOnly.IsValid() ||
		!distributable.IsValid() {
		t.Fatal(
			"a supported distribution capability is invalid",
		)
	}

	if distributable.String() != "DISTRIBUTABLE" {
		t.Fatalf(
			"String() = %q, want %q",
			distributable.String(),
			"DISTRIBUTABLE",
		)
	}
}

func TestDistributionCapabilityRejectsUnsupportedValue(
	t *testing.T,
) {
	_, err := ParseDistributionCapability(
		"REMOTE_ONLY",
	)
	if err == nil {
		t.Fatal(
			"ParseDistributionCapability() accepted an unsupported value",
		)
	}

	if DistributionCapability("REMOTE_ONLY").IsValid() {
		t.Fatal(
			"unsupported distribution capability is valid",
		)
	}

	if DistributionCapability("").SupportsAsync() {
		t.Fatal(
			"zero-value distribution capability supports async execution",
		)
	}
}

func TestWorkflowValidationIssueCodesHaveStableStrings(
	t *testing.T,
) {
	tests := []struct {
		code     WorkflowValidationIssueCode
		expected string
	}{
		{
			code:     IssueCodePluginNotFound,
			expected: "PLUGIN_NOT_FOUND",
		},
		{
			code:     IssueCodePluginVersionNotFound,
			expected: "PLUGIN_VERSION_NOT_FOUND",
		},
		{
			code:     IssueCodeInvalidPluginConfiguration,
			expected: "INVALID_PLUGIN_CONFIGURATION",
		},
		{
			code:     IssueCodeUnknownOutputPort,
			expected: "UNKNOWN_OUTPUT_PORT",
		},
		{
			code:     IssueCodeUnknownInputPort,
			expected: "UNKNOWN_INPUT_PORT",
		},
		{
			code:     IssueCodeInputEdgeCountBelowMinimum,
			expected: "INPUT_EDGE_COUNT_BELOW_MINIMUM",
		},
		{
			code:     IssueCodeInputEdgeCountAboveMaximum,
			expected: "INPUT_EDGE_COUNT_ABOVE_MAXIMUM",
		},
		{
			code:     IssueCodeOutputEdgeCountBelowMinimum,
			expected: "OUTPUT_EDGE_COUNT_BELOW_MINIMUM",
		},
		{
			code:     IssueCodeOutputEdgeCountAboveMaximum,
			expected: "OUTPUT_EDGE_COUNT_ABOVE_MAXIMUM",
		},
		{
			code:     IssueCodeAsyncNodeNotDistributable,
			expected: "ASYNC_NODE_NOT_DISTRIBUTABLE",
		},
	}

	for _, test := range tests {
		t.Run(test.expected, func(t *testing.T) {
			if actual := test.code.String(); actual != test.expected {
				t.Fatalf(
					"String() = %q, want %q",
					actual,
					test.expected,
				)
			}
		})
	}
}

func TestWorkflowValidationReportIsDeterministic(
	t *testing.T,
) {
	delayIdentity := mustWorkflowValidationIdentity(
		t,
		"core.delay",
		"v1",
	)

	passIdentity := mustWorkflowValidationIdentity(
		t,
		"core.pass-through",
		"v1",
	)

	report := newWorkflowValidationReport(
		[]WorkflowValidationIssue{
			{
				Code:           IssueCodeAsyncNodeNotDistributable,
				Message:        "plugin does not support asynchronous execution",
				NodeID:         workflow.NodeID("node-b"),
				PluginIdentity: delayIdentity,
				Field:          "distribution",
			},
			{
				Code:           IssueCodeUnknownOutputPort,
				Message:        "edge source port is not declared by the plugin",
				NodeID:         workflow.NodeID("node-a"),
				EdgeID:         workflow.EdgeID("edge-b"),
				PluginIdentity: passIdentity,
				Field:          "sourceOutputPort",
			},
			{
				Code:           IssueCodeInvalidPluginConfiguration,
				Message:        "plugin configuration is invalid",
				NodeID:         workflow.NodeID("node-a"),
				PluginIdentity: passIdentity,
				Field:          "value",
			},
			{
				Code:           IssueCodeUnknownOutputPort,
				Message:        "edge source port is not declared by the plugin",
				NodeID:         workflow.NodeID("node-a"),
				EdgeID:         workflow.EdgeID("edge-a"),
				PluginIdentity: passIdentity,
				Field:          "sourceOutputPort",
			},
		},
	)

	expectedCodes := []WorkflowValidationIssueCode{
		IssueCodeInvalidPluginConfiguration,
		IssueCodeUnknownOutputPort,
		IssueCodeUnknownOutputPort,
		IssueCodeAsyncNodeNotDistributable,
	}

	expectedEdgeIDs := []string{
		"",
		"edge-a",
		"edge-b",
		"",
	}

	issues := report.Issues()

	actualCodes := make(
		[]WorkflowValidationIssueCode,
		len(issues),
	)

	actualEdgeIDs := make(
		[]string,
		len(issues),
	)

	for index, issue := range issues {
		actualCodes[index] = issue.Code
		actualEdgeIDs[index] = issue.EdgeID.String()
	}

	if !reflect.DeepEqual(
		actualCodes,
		expectedCodes,
	) {
		t.Fatalf(
			"issue codes = %v, want %v",
			actualCodes,
			expectedCodes,
		)
	}

	if !reflect.DeepEqual(
		actualEdgeIDs,
		expectedEdgeIDs,
	) {
		t.Fatalf(
			"edge IDs = %v, want %v",
			actualEdgeIDs,
			expectedEdgeIDs,
		)
	}
}

func TestWorkflowValidationReportReturnsDefensiveCopies(
	t *testing.T,
) {
	identity := mustWorkflowValidationIdentity(
		t,
		"core.delay",
		"v1",
	)

	report := newWorkflowValidationReport(
		[]WorkflowValidationIssue{
			{
				Code:           IssueCodeInvalidPluginConfiguration,
				Message:        "plugin configuration is invalid",
				NodeID:         workflow.NodeID("delay-node"),
				PluginIdentity: identity,
				Field:          "delay",
			},
		},
	)

	if report.IsValid() {
		t.Fatal(
			"IsValid() = true for report containing an issue",
		)
	}

	if actual := report.Len(); actual != 1 {
		t.Fatalf(
			"Len() = %d, want %d",
			actual,
			1,
		)
	}

	if !report.HasCode(
		IssueCodeInvalidPluginConfiguration,
	) {
		t.Fatal(
			"HasCode() = false for the stored issue code",
		)
	}

	if report.HasCode(
		IssueCodePluginNotFound,
	) {
		t.Fatal(
			"HasCode() = true for an absent issue code",
		)
	}

	first := report.Issues()

	first[0].Code = IssueCodePluginNotFound
	first[0].Message = "changed"
	first[0].Field = "changed"
	first[0].NodeID = workflow.NodeID(
		"changed-node",
	)

	second := report.Issues()

	if second[0].Code !=
		IssueCodeInvalidPluginConfiguration {
		t.Fatalf(
			"stored code = %q, want %q",
			second[0].Code,
			IssueCodeInvalidPluginConfiguration,
		)
	}

	if second[0].Message !=
		"plugin configuration is invalid" {
		t.Fatalf(
			"stored message = %q",
			second[0].Message,
		)
	}

	if second[0].Field != "delay" {
		t.Fatalf(
			"stored field = %q, want %q",
			second[0].Field,
			"delay",
		)
	}

	if second[0].NodeID.String() !=
		"delay-node" {
		t.Fatalf(
			"stored node ID = %q, want %q",
			second[0].NodeID,
			"delay-node",
		)
	}
}

func TestEmptyWorkflowValidationReportIsValid(
	t *testing.T,
) {
	report := newWorkflowValidationReport(nil)

	if !report.IsValid() {
		t.Fatal(
			"empty workflow validation report is invalid",
		)
	}

	if report.Len() != 0 {
		t.Fatalf(
			"Len() = %d, want %d",
			report.Len(),
			0,
		)
	}

	if report.Issues() != nil {
		t.Fatalf(
			"Issues() = %v, want nil",
			report.Issues(),
		)
	}
}

func TestStructuralValidationErrorFormatsIssueCount(
	t *testing.T,
) {
	err := &StructuralValidationError{
		IssueCount: 3,
	}

	expected :=
		"workflow plugin validation requires a structurally valid graph: 3 structural issue(s) found"

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}

	var nilError *StructuralValidationError

	expectedFallback :=
		"workflow plugin validation requires a structurally valid graph"

	if actual := nilError.Error(); actual != expectedFallback {
		t.Fatalf(
			"nil Error() = %q, want %q",
			actual,
			expectedFallback,
		)
	}

	zeroCountError := &StructuralValidationError{}

	if actual := zeroCountError.Error(); actual != expectedFallback {
		t.Fatalf(
			"zero-count Error() = %q, want %q",
			actual,
			expectedFallback,
		)
	}
}

func TestWorkflowValidationIssueRankPlacesUnknownCodeLast(
	t *testing.T,
) {
	issues := []WorkflowValidationIssue{
		{
			Code: WorkflowValidationIssueCode(
				"UNKNOWN_ISSUE",
			),
			NodeID: workflow.NodeID(
				"node-a",
			),
		},
		{
			Code:   IssueCodePluginNotFound,
			NodeID: workflow.NodeID("node-a"),
		},
	}

	report := newWorkflowValidationReport(
		issues,
	)

	actual := report.Issues()

	if actual[0].Code !=
		IssueCodePluginNotFound {
		t.Fatalf(
			"first issue code = %q, want %q",
			actual[0].Code,
			IssueCodePluginNotFound,
		)
	}

	if actual[1].Code !=
		WorkflowValidationIssueCode(
			"UNKNOWN_ISSUE",
		) {
		t.Fatalf(
			"second issue code = %q, want %q",
			actual[1].Code,
			WorkflowValidationIssueCode(
				"UNKNOWN_ISSUE",
			),
		)
	}
}

func mustWorkflowValidationIdentity(
	t *testing.T,
	pluginType string,
	version string,
) PluginIdentity {
	t.Helper()

	identity, err := NewPluginIdentity(
		workflow.PluginType(pluginType),
		workflow.PluginVersion(version),
	)
	if err != nil {
		t.Fatalf(
			"NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	return identity
}
