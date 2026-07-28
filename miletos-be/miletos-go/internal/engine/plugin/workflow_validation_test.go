package plugin

import (
	"errors"
	"reflect"
	"testing"

	executiondomain "miletos-go/internal/features/execution"
	workflowdomain "miletos-go/internal/features/workflow"
)

func TestValidateWorkflowPluginsAcceptsValidCoreStyleWorkflow(
	t *testing.T,
) {
	registry := mustPluginValidationRegistry(
		t,
		pluginValidationStaticDescriptor(t),
		pluginValidationPassDescriptor(t),
		pluginValidationDelayDescriptor(t),
		pluginValidationTerminalDescriptor(t),
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"static",
				"core.static-input",
				"v1",
				`{"value":"hello"}`,
			),
			mustPluginValidationNode(
				t,
				"pass",
				"core.pass-through",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"delay",
				"core.delay",
				"v1",
				`{"delay":"2s"}`,
			),
			mustPluginValidationNode(
				t,
				"terminal",
				"core.terminal",
				"v1",
				`{}`,
			),
		},
		[]workflowdomain.EdgeDefinition{
			mustPluginValidationEdge(
				t,
				"edge-1",
				"static",
				"output",
				"pass",
				"input",
			),
			mustPluginValidationEdge(
				t,
				"edge-2",
				"pass",
				"output",
				"delay",
				"input",
			),
			mustPluginValidationEdge(
				t,
				"edge-3",
				"delay",
				"output",
				"terminal",
				"input",
			),
		},
	)

	modes := []executiondomain.ExecutionMode{
		executiondomain.ExecutionModeSync,
		executiondomain.ExecutionModeAsync,
	}

	for _, mode := range modes {
		t.Run(mode.String(), func(t *testing.T) {
			report, err := ValidateWorkflowPlugins(
				definition,
				registry,
				mode,
			)
			if err != nil {
				t.Fatalf(
					"ValidateWorkflowPlugins() returned an unexpected error: %v",
					err,
				)
			}

			if !report.IsValid() {
				t.Fatalf(
					"valid workflow produced issues: %v",
					report.Issues(),
				)
			}
		})
	}
}

func TestValidateWorkflowPluginsDistinguishesUnknownTypeAndVersion(
	t *testing.T,
) {
	registry := mustPluginValidationRegistry(
		t,
		pluginValidationPassDescriptor(t),
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"node-a",
				"core.unknown",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"node-b",
				"core.pass-through",
				"v2",
				`{}`,
			),
		},
		nil,
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"ValidateWorkflowPlugins() returned an unexpected error: %v",
			err,
		)
	}

	assertPluginValidationIssueCodes(
		t,
		report.Issues(),
		[]WorkflowValidationIssueCode{
			IssueCodePluginNotFound,
			IssueCodePluginVersionNotFound,
		},
	)

	issues := report.Issues()

	if actual := issues[0].NodeID.String(); actual != "node-a" {
		t.Fatalf(
			"first issue node ID = %q, want %q",
			actual,
			"node-a",
		)
	}

	if actual := issues[0].Field; actual != "pluginType" {
		t.Fatalf(
			"first issue field = %q, want %q",
			actual,
			"pluginType",
		)
	}

	if actual := issues[1].NodeID.String(); actual != "node-b" {
		t.Fatalf(
			"second issue node ID = %q, want %q",
			actual,
			"node-b",
		)
	}

	if actual := issues[1].Field; actual != "pluginVersion" {
		t.Fatalf(
			"second issue field = %q, want %q",
			actual,
			"pluginVersion",
		)
	}
}

func TestValidateWorkflowPluginsReportsConfigurationIssues(
	t *testing.T,
) {
	descriptor := mustPluginValidationDescriptor(
		t,
		"test.configuration",
		"v1",
		nil,
		nil,
		NewExactEdgeConstraint(0),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		func(
			workflowdomain.JSONObject,
		) []ConfigurationIssue {
			return []ConfigurationIssue{
				{
					Field:  "zeta",
					Reason: "is invalid",
				},
				{
					Field:  "alpha",
					Reason: "is required",
				},
			}
		},
	)

	registry := mustPluginValidationRegistry(
		t,
		descriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"config-node",
				"test.configuration",
				"v1",
				`{}`,
			),
		},
		nil,
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"ValidateWorkflowPlugins() returned an unexpected error: %v",
			err,
		)
	}

	assertPluginValidationIssueCodes(
		t,
		report.Issues(),
		[]WorkflowValidationIssueCode{
			IssueCodeInvalidPluginConfiguration,
			IssueCodeInvalidPluginConfiguration,
		},
	)

	issues := report.Issues()

	expectedFields := []string{
		"alpha",
		"zeta",
	}

	actualFields := []string{
		issues[0].Field,
		issues[1].Field,
	}

	if !reflect.DeepEqual(
		actualFields,
		expectedFields,
	) {
		t.Fatalf(
			"configuration fields = %v, want %v",
			actualFields,
			expectedFields,
		)
	}

	if issues[0].Message != "is required" {
		t.Fatalf(
			"first issue message = %q, want %q",
			issues[0].Message,
			"is required",
		)
	}
}

func TestValidateWorkflowPluginsReportsBothUnknownPorts(
	t *testing.T,
) {
	sourceDescriptor := mustPluginValidationDescriptor(
		t,
		"test.source",
		"v1",
		nil,
		[]string{
			"output",
		},
		NewExactEdgeConstraint(0),
		NewUnlimitedEdgeConstraint(1),
		DistributionDistributable,
		nil,
	)

	targetDescriptor := mustPluginValidationDescriptor(
		t,
		"test.target",
		"v1",
		[]string{
			"input",
		},
		nil,
		NewUnlimitedEdgeConstraint(1),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		sourceDescriptor,
		targetDescriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"source",
				"test.source",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"target",
				"test.target",
				"v1",
				`{}`,
			),
		},
		[]workflowdomain.EdgeDefinition{
			mustPluginValidationEdge(
				t,
				"edge-1",
				"source",
				"wrong-output",
				"target",
				"wrong-input",
			),
		},
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"ValidateWorkflowPlugins() returned an unexpected error: %v",
			err,
		)
	}

	assertPluginValidationIssueCodes(
		t,
		report.Issues(),
		[]WorkflowValidationIssueCode{
			IssueCodeUnknownOutputPort,
			IssueCodeUnknownInputPort,
		},
	)

	issues := report.Issues()

	if issues[0].EdgeID.String() != "edge-1" {
		t.Fatalf(
			"output-port issue edge ID = %q, want %q",
			issues[0].EdgeID,
			"edge-1",
		)
	}

	if issues[1].EdgeID.String() != "edge-1" {
		t.Fatalf(
			"input-port issue edge ID = %q, want %q",
			issues[1].EdgeID,
			"edge-1",
		)
	}
}

func TestValidateWorkflowPluginsSkipsPortValidationForUnresolvedSide(
	t *testing.T,
) {
	targetDescriptor := mustPluginValidationDescriptor(
		t,
		"test.target",
		"v1",
		[]string{
			"input",
		},
		nil,
		NewUnlimitedEdgeConstraint(1),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		targetDescriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"source",
				"test.missing-source",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"target",
				"test.target",
				"v1",
				`{}`,
			),
		},
		[]workflowdomain.EdgeDefinition{
			mustPluginValidationEdge(
				t,
				"edge-1",
				"source",
				"wrong-output",
				"target",
				"wrong-input",
			),
		},
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"ValidateWorkflowPlugins() returned an unexpected error: %v",
			err,
		)
	}

	if !report.HasCode(
		IssueCodePluginNotFound,
	) {
		t.Fatal(
			"report does not contain PLUGIN_NOT_FOUND",
		)
	}

	if !report.HasCode(
		IssueCodeUnknownInputPort,
	) {
		t.Fatal(
			"report does not contain UNKNOWN_INPUT_PORT",
		)
	}

	if report.HasCode(
		IssueCodeUnknownOutputPort,
	) {
		t.Fatal(
			"report contains UNKNOWN_OUTPUT_PORT for an unresolved source plugin",
		)
	}

	if report.Len() != 2 {
		t.Fatalf(
			"issue count = %d, want %d: %v",
			report.Len(),
			2,
			report.Issues(),
		)
	}
}

func TestValidateWorkflowPluginsReportsInputEdgeCountBelowMinimum(
	t *testing.T,
) {
	descriptor := mustPluginValidationDescriptor(
		t,
		"test.input-required",
		"v1",
		[]string{
			"input",
		},
		nil,
		NewExactEdgeConstraint(1),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		descriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"node",
				"test.input-required",
				"v1",
				`{}`,
			),
		},
		nil,
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"ValidateWorkflowPlugins() returned an unexpected error: %v",
			err,
		)
	}

	issue := requirePluginValidationIssue(
		t,
		report,
		IssueCodeInputEdgeCountBelowMinimum,
	)

	if issue.Minimum != 1 ||
		issue.Maximum != 1 ||
		issue.Actual != 0 ||
		!issue.HasMaximum {
		t.Fatalf(
			"unexpected edge-count metadata: %+v",
			issue,
		)
	}
}

func TestValidateWorkflowPluginsReportsInputEdgeCountAboveMaximum(
	t *testing.T,
) {
	sourceDescriptor := mustPluginValidationDescriptor(
		t,
		"test.source",
		"v1",
		nil,
		[]string{
			"output",
		},
		NewExactEdgeConstraint(0),
		NewUnlimitedEdgeConstraint(1),
		DistributionDistributable,
		nil,
	)

	targetDescriptor := mustPluginValidationDescriptor(
		t,
		"test.single-input",
		"v1",
		[]string{
			"input",
		},
		nil,
		NewExactEdgeConstraint(1),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		sourceDescriptor,
		targetDescriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"source-a",
				"test.source",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"source-b",
				"test.source",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"target",
				"test.single-input",
				"v1",
				`{}`,
			),
		},
		[]workflowdomain.EdgeDefinition{
			mustPluginValidationEdge(
				t,
				"edge-a",
				"source-a",
				"output",
				"target",
				"input",
			),
			mustPluginValidationEdge(
				t,
				"edge-b",
				"source-b",
				"output",
				"target",
				"input",
			),
		},
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"ValidateWorkflowPlugins() returned an unexpected error: %v",
			err,
		)
	}

	issue := requirePluginValidationIssue(
		t,
		report,
		IssueCodeInputEdgeCountAboveMaximum,
	)

	if issue.Minimum != 1 ||
		issue.Maximum != 1 ||
		issue.Actual != 2 ||
		!issue.HasMaximum {
		t.Fatalf(
			"unexpected edge-count metadata: %+v",
			issue,
		)
	}
}

func TestValidateWorkflowPluginsReportsOutputEdgeCountBelowMinimum(
	t *testing.T,
) {
	descriptor := mustPluginValidationDescriptor(
		t,
		"test.output-required",
		"v1",
		nil,
		[]string{
			"output",
		},
		NewExactEdgeConstraint(0),
		NewUnlimitedEdgeConstraint(1),
		DistributionDistributable,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		descriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"source",
				"test.output-required",
				"v1",
				`{}`,
			),
		},
		nil,
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"ValidateWorkflowPlugins() returned an unexpected error: %v",
			err,
		)
	}

	issue := requirePluginValidationIssue(
		t,
		report,
		IssueCodeOutputEdgeCountBelowMinimum,
	)

	if issue.Minimum != 1 ||
		issue.Actual != 0 ||
		issue.HasMaximum {
		t.Fatalf(
			"unexpected edge-count metadata: %+v",
			issue,
		)
	}
}

func TestValidateWorkflowPluginsReportsOutputEdgeCountAboveMaximum(
	t *testing.T,
) {
	outputConstraint, err := NewBoundedEdgeConstraint(
		0,
		1,
	)
	if err != nil {
		t.Fatalf(
			"NewBoundedEdgeConstraint() returned an unexpected error: %v",
			err,
		)
	}

	sourceDescriptor := mustPluginValidationDescriptor(
		t,
		"test.single-output",
		"v1",
		nil,
		[]string{
			"output",
		},
		NewExactEdgeConstraint(0),
		outputConstraint,
		DistributionDistributable,
		nil,
	)

	targetDescriptor := mustPluginValidationDescriptor(
		t,
		"test.target",
		"v1",
		[]string{
			"input",
		},
		nil,
		NewUnlimitedEdgeConstraint(1),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		sourceDescriptor,
		targetDescriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"source",
				"test.single-output",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"target-a",
				"test.target",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"target-b",
				"test.target",
				"v1",
				`{}`,
			),
		},
		[]workflowdomain.EdgeDefinition{
			mustPluginValidationEdge(
				t,
				"edge-a",
				"source",
				"output",
				"target-a",
				"input",
			),
			mustPluginValidationEdge(
				t,
				"edge-b",
				"source",
				"output",
				"target-b",
				"input",
			),
		},
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"ValidateWorkflowPlugins() returned an unexpected error: %v",
			err,
		)
	}

	issue := requirePluginValidationIssue(
		t,
		report,
		IssueCodeOutputEdgeCountAboveMaximum,
	)

	if issue.Minimum != 0 ||
		issue.Maximum != 1 ||
		issue.Actual != 2 ||
		!issue.HasMaximum {
		t.Fatalf(
			"unexpected edge-count metadata: %+v",
			issue,
		)
	}
}

func TestValidateWorkflowPluginsValidatesAsyncEligibility(
	t *testing.T,
) {
	firstDescriptor := mustPluginValidationDescriptor(
		t,
		"test.local-a",
		"v1",
		nil,
		nil,
		NewExactEdgeConstraint(0),
		NewExactEdgeConstraint(0),
		DistributionLocalOnly,
		nil,
	)

	secondDescriptor := mustPluginValidationDescriptor(
		t,
		"test.local-b",
		"v1",
		nil,
		nil,
		NewExactEdgeConstraint(0),
		NewExactEdgeConstraint(0),
		DistributionLocalOnly,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		firstDescriptor,
		secondDescriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"node-a",
				"test.local-a",
				"v1",
				`{}`,
			),
			mustPluginValidationNode(
				t,
				"node-b",
				"test.local-b",
				"v1",
				`{}`,
			),
		},
		nil,
	)

	syncReport, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"SYNC validation returned an unexpected error: %v",
			err,
		)
	}

	if !syncReport.IsValid() {
		t.Fatalf(
			"SYNC validation rejected local-only nodes: %v",
			syncReport.Issues(),
		)
	}

	asyncReport, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeAsync,
	)
	if err != nil {
		t.Fatalf(
			"ASYNC validation returned an unexpected error: %v",
			err,
		)
	}

	assertPluginValidationIssueCodes(
		t,
		asyncReport.Issues(),
		[]WorkflowValidationIssueCode{
			IssueCodeAsyncNodeNotDistributable,
			IssueCodeAsyncNodeNotDistributable,
		},
	)
}

func TestValidateWorkflowPluginsRejectsStructurallyInvalidWorkflow(
	t *testing.T,
) {
	registry := mustPluginValidationRegistry(t)

	definition := mustPluginValidationDefinition(
		t,
		nil,
		nil,
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err == nil {
		t.Fatal(
			"ValidateWorkflowPlugins() returned nil error for an empty graph",
		)
	}

	var structuralError *StructuralValidationError
	if !errors.As(
		err,
		&structuralError,
	) {
		t.Fatalf(
			"error type = %T, want *StructuralValidationError",
			err,
		)
	}

	if structuralError.IssueCount != 1 {
		t.Fatalf(
			"structural issue count = %d, want %d",
			structuralError.IssueCount,
			1,
		)
	}

	if !report.IsValid() || report.Len() != 0 {
		t.Fatalf(
			"plugin report contains issues after structural failure: %v",
			report.Issues(),
		)
	}
}

func TestValidateWorkflowPluginsRejectsInvalidExecutionMode(
	t *testing.T,
) {
	descriptor := mustPluginValidationDescriptor(
		t,
		"test.node",
		"v1",
		nil,
		nil,
		NewExactEdgeConstraint(0),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		descriptor,
	)

	definition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustPluginValidationNode(
				t,
				"node",
				"test.node",
				"v1",
				`{}`,
			),
		},
		nil,
	)

	report, err := ValidateWorkflowPlugins(
		definition,
		registry,
		executiondomain.ExecutionMode(
			"BACKGROUND",
		),
	)
	if err == nil {
		t.Fatal(
			"ValidateWorkflowPlugins() accepted an invalid mode",
		)
	}

	var invalidModeError *executiondomain.InvalidModeError
	if !errors.As(
		err,
		&invalidModeError,
	) {
		t.Fatalf(
			"error type = %T, want *execution.InvalidModeError",
			err,
		)
	}

	if !report.IsValid() || report.Len() != 0 {
		t.Fatalf(
			"plugin report contains issues after mode failure: %v",
			report.Issues(),
		)
	}
}

func TestValidateWorkflowPluginsIsDeterministicAcrossDefinitionOrder(
	t *testing.T,
) {
	outputConstraint, err := NewBoundedEdgeConstraint(
		0,
		1,
	)
	if err != nil {
		t.Fatalf(
			"NewBoundedEdgeConstraint() returned an unexpected error: %v",
			err,
		)
	}

	sourceDescriptor := mustPluginValidationDescriptor(
		t,
		"test.source",
		"v1",
		nil,
		[]string{
			"output",
		},
		NewExactEdgeConstraint(0),
		outputConstraint,
		DistributionDistributable,
		nil,
	)

	targetDescriptor := mustPluginValidationDescriptor(
		t,
		"test.target",
		"v1",
		[]string{
			"input",
		},
		nil,
		NewUnlimitedEdgeConstraint(1),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)

	registry := mustPluginValidationRegistry(
		t,
		sourceDescriptor,
		targetDescriptor,
	)

	sourceNode := mustPluginValidationNode(
		t,
		"node-source",
		"test.source",
		"v1",
		`{}`,
	)

	targetA := mustPluginValidationNode(
		t,
		"node-a",
		"test.target",
		"v1",
		`{}`,
	)

	targetB := mustPluginValidationNode(
		t,
		"node-b",
		"test.target",
		"v1",
		`{}`,
	)

	edgeA := mustPluginValidationEdge(
		t,
		"edge-a",
		"node-source",
		"wrong-output-a",
		"node-a",
		"wrong-input-a",
	)

	edgeB := mustPluginValidationEdge(
		t,
		"edge-b",
		"node-source",
		"wrong-output-b",
		"node-b",
		"wrong-input-b",
	)

	firstDefinition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			sourceNode,
			targetB,
			targetA,
		},
		[]workflowdomain.EdgeDefinition{
			edgeB,
			edgeA,
		},
	)

	secondDefinition := mustPluginValidationDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			targetA,
			sourceNode,
			targetB,
		},
		[]workflowdomain.EdgeDefinition{
			edgeA,
			edgeB,
		},
	)

	firstReport, err := ValidateWorkflowPlugins(
		firstDefinition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"first validation returned an unexpected error: %v",
			err,
		)
	}

	secondReport, err := ValidateWorkflowPlugins(
		secondDefinition,
		registry,
		executiondomain.ExecutionModeSync,
	)
	if err != nil {
		t.Fatalf(
			"second validation returned an unexpected error: %v",
			err,
		)
	}

	if !reflect.DeepEqual(
		firstReport.Issues(),
		secondReport.Issues(),
	) {
		t.Fatalf(
			"validation order differs:\nfirst:  %v\nsecond: %v",
			firstReport.Issues(),
			secondReport.Issues(),
		)
	}
}

func pluginValidationStaticDescriptor(
	t *testing.T,
) Descriptor {
	t.Helper()

	return mustPluginValidationDescriptor(
		t,
		"core.static-input",
		"v1",
		nil,
		[]string{
			"output",
		},
		NewExactEdgeConstraint(0),
		NewUnlimitedEdgeConstraint(1),
		DistributionDistributable,
		func(
			configuration workflowdomain.JSONObject,
		) []ConfigurationIssue {
			if configuration.String() ==
				`{"value":"hello"}` {
				return nil
			}

			return []ConfigurationIssue{
				{
					Field:  "value",
					Reason: "is required",
				},
			}
		},
	)
}

func pluginValidationPassDescriptor(
	t *testing.T,
) Descriptor {
	t.Helper()

	return mustPluginValidationDescriptor(
		t,
		"core.pass-through",
		"v1",
		[]string{
			"input",
		},
		[]string{
			"output",
		},
		NewExactEdgeConstraint(1),
		NewUnlimitedEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)
}

func pluginValidationDelayDescriptor(
	t *testing.T,
) Descriptor {
	t.Helper()

	return mustPluginValidationDescriptor(
		t,
		"core.delay",
		"v1",
		[]string{
			"input",
		},
		[]string{
			"output",
		},
		NewExactEdgeConstraint(1),
		NewUnlimitedEdgeConstraint(0),
		DistributionDistributable,
		func(
			configuration workflowdomain.JSONObject,
		) []ConfigurationIssue {
			if configuration.String() ==
				`{"delay":"2s"}` {
				return nil
			}

			return []ConfigurationIssue{
				{
					Field:  "delay",
					Reason: "is invalid",
				},
			}
		},
	)
}

func pluginValidationTerminalDescriptor(
	t *testing.T,
) Descriptor {
	t.Helper()

	return mustPluginValidationDescriptor(
		t,
		"core.terminal",
		"v1",
		[]string{
			"input",
		},
		nil,
		NewUnlimitedEdgeConstraint(1),
		NewExactEdgeConstraint(0),
		DistributionDistributable,
		nil,
	)
}

func mustPluginValidationDescriptor(
	t *testing.T,
	pluginType string,
	version string,
	inputPortNames []string,
	outputPortNames []string,
	inputConstraint EdgeConstraint,
	outputConstraint EdgeConstraint,
	distribution DistributionCapability,
	validator ConfigurationValidator,
) Descriptor {
	t.Helper()

	identity, err := NewPluginIdentity(
		workflowdomain.PluginType(pluginType),
		workflowdomain.PluginVersion(version),
	)
	if err != nil {
		t.Fatalf(
			"NewPluginIdentity() returned an unexpected error: %v",
			err,
		)
	}

	metadata, err := NewPluginMetadata(
		pluginType,
		"",
	)
	if err != nil {
		t.Fatalf(
			"NewPluginMetadata() returned an unexpected error: %v",
			err,
		)
	}

	inputPorts := mustPluginValidationPorts(
		t,
		inputPortNames,
	)

	outputPorts := mustPluginValidationPorts(
		t,
		outputPortNames,
	)

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

	if validator == nil {
		validator = func(
			workflowdomain.JSONObject,
		) []ConfigurationIssue {
			return nil
		}
	}

	descriptor, err := NewDescriptor(
		DescriptorConfig{
			Identity: identity,
			Metadata: metadata,

			InputPorts:  inputPorts,
			OutputPorts: outputPorts,

			InputEdgeConstraint:  inputConstraint,
			OutputEdgeConstraint: outputConstraint,

			QueuePolicy: queuePolicy,
			CachePolicy: cachePolicy,

			Distribution: distribution,

			ConfigurationValidator: validator,
		},
	)
	if err != nil {
		t.Fatalf(
			"NewDescriptor() returned an unexpected error: %v",
			err,
		)
	}

	return descriptor
}

func mustPluginValidationPorts(
	t *testing.T,
	names []string,
) []Port {
	t.Helper()

	if len(names) == 0 {
		return nil
	}

	ports := make(
		[]Port,
		0,
		len(names),
	)

	for _, name := range names {
		port, err := NewPort(
			name,
			name,
			"",
		)
		if err != nil {
			t.Fatalf(
				"NewPort() returned an unexpected error: %v",
				err,
			)
		}

		ports = append(
			ports,
			port,
		)
	}

	return ports
}

func mustPluginValidationRegistry(
	t *testing.T,
	descriptors ...Descriptor,
) Registry {
	t.Helper()

	registry, err := NewRegistry(
		descriptors,
	)
	if err != nil {
		t.Fatalf(
			"NewRegistry() returned an unexpected error: %v",
			err,
		)
	}

	return registry
}

func mustPluginValidationNode(
	t *testing.T,
	id string,
	pluginType string,
	version string,
	configuration string,
) workflowdomain.NodeDefinition {
	t.Helper()

	node, err := workflowdomain.NewNodeDefinition(
		workflowdomain.NodeID(id),
		workflowdomain.PluginType(pluginType),
		workflowdomain.PluginVersion(version),
		[]byte(configuration),
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return node
}

func mustPluginValidationEdge(
	t *testing.T,
	id string,
	sourceNodeID string,
	sourceOutputPort string,
	targetNodeID string,
	targetInputPort string,
) workflowdomain.EdgeDefinition {
	t.Helper()

	edge, err := workflowdomain.NewEdgeDefinition(
		workflowdomain.EdgeID(id),
		workflowdomain.NodeID(sourceNodeID),
		sourceOutputPort,
		workflowdomain.NodeID(targetNodeID),
		targetInputPort,
	)
	if err != nil {
		t.Fatalf(
			"NewEdgeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return edge
}

func mustPluginValidationDefinition(
	t *testing.T,
	nodes []workflowdomain.NodeDefinition,
	edges []workflowdomain.EdgeDefinition,
) workflowdomain.WorkflowDefinition {
	t.Helper()

	definition, err :=
		workflowdomain.NewWorkflowDefinition(
			workflowdomain.WorkflowID(
				"workflow-1",
			),
			workflowdomain.CompanyID(
				"company-1",
			),
			"Plugin Validation Workflow",
			1,
			nodes,
			edges,
			nil,
		)
	if err != nil {
		t.Fatalf(
			"NewWorkflowDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return definition
}

func requirePluginValidationIssue(
	t *testing.T,
	report WorkflowValidationReport,
	code WorkflowValidationIssueCode,
) WorkflowValidationIssue {
	t.Helper()

	for _, issue := range report.Issues() {
		if issue.Code == code {
			return issue
		}
	}

	t.Fatalf(
		"report does not contain issue code %q: %v",
		code,
		report.Issues(),
	)

	return WorkflowValidationIssue{}
}

func assertPluginValidationIssueCodes(
	t *testing.T,
	issues []WorkflowValidationIssue,
	expected []WorkflowValidationIssueCode,
) {
	t.Helper()

	actual := make(
		[]WorkflowValidationIssueCode,
		len(issues),
	)

	for index, issue := range issues {
		actual[index] = issue.Code
	}

	if !reflect.DeepEqual(
		actual,
		expected,
	) {
		t.Fatalf(
			"issue codes = %v, want %v",
			actual,
			expected,
		)
	}
}
