package workflow

import (
	bytes "bytes"
	errors "errors"
	math "math"
	testing "testing"
)

func TestNewWorkflowDefinitionStoresNormalizedValues(t *testing.T) {
	sourceNode := newTestNodeDefinition(
		t,
		"source-node",
		`{"value":"hello"}`,
	)
	targetNode := newTestNodeDefinition(
		t,
		"target-node",
		`{}`,
	)
	edge := newTestEdgeDefinition(
		t,
		"edge-1",
		"source-node",
		"target-node",
	)

	definition, err := NewWorkflowDefinition(
		WorkflowID(" workflow-1 "),
		CompanyID(" company-1 "),
		" Customer Import ",
		3,
		[]NodeDefinition{
			sourceNode,
			targetNode,
		},
		[]EdgeDefinition{
			edge,
		},
		[]byte(`  {"category":"import"}  `),
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowDefinition() returned an unexpected error: %v",
			err,
		)
	}

	if actual := definition.ID().String(); actual != "workflow-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"workflow-1",
		)
	}

	if actual := definition.CompanyID().String(); actual != "company-1" {
		t.Fatalf(
			"CompanyID() = %q, want %q",
			actual,
			"company-1",
		)
	}

	if actual := definition.Name(); actual != "Customer Import" {
		t.Fatalf(
			"Name() = %q, want %q",
			actual,
			"Customer Import",
		)
	}

	if actual := definition.Revision(); actual != 3 {
		t.Fatalf(
			"Revision() = %d, want %d",
			actual,
			3,
		)
	}

	if actual := len(definition.Nodes()); actual != 2 {
		t.Fatalf(
			"node count = %d, want %d",
			actual,
			2,
		)
	}

	if actual := len(definition.Edges()); actual != 1 {
		t.Fatalf(
			"edge count = %d, want %d",
			actual,
			1,
		)
	}

	if actual := definition.Metadata().String(); actual != `{"category":"import"}` {
		t.Fatalf(
			"Metadata() = %q, want %q",
			actual,
			`{"category":"import"}`,
		)
	}
}

func TestNewWorkflowDefinitionAllowsEmptyGraph(t *testing.T) {
	definition, err := NewWorkflowDefinition(
		WorkflowID("workflow-1"),
		CompanyID("company-1"),
		"Empty Workflow",
		1,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowDefinition() returned an unexpected error: %v",
			err,
		)
	}

	if actual := len(definition.Nodes()); actual != 0 {
		t.Fatalf(
			"node count = %d, want %d",
			actual,
			0,
		)
	}

	if actual := len(definition.Edges()); actual != 0 {
		t.Fatalf(
			"edge count = %d, want %d",
			actual,
			0,
		)
	}

	if actual := definition.Metadata().String(); actual != "{}" {
		t.Fatalf(
			"Metadata() = %q, want %q",
			actual,
			"{}",
		)
	}
}

func TestNewWorkflowDefinitionRejectsInvalidTopLevelFields(t *testing.T) {
	tests := []struct {
		name      string
		id        WorkflowID
		companyID CompanyID
		workflow  string
		revision  uint64
		metadata  []byte
		field     string
	}{
		{
			name:      "blank workflow ID",
			id:        WorkflowID(" "),
			companyID: CompanyID("company-1"),
			workflow:  "Workflow",
			revision:  1,
			metadata:  nil,
			field:     "workflowID",
		},
		{
			name:      "blank company ID",
			id:        WorkflowID("workflow-1"),
			companyID: CompanyID(" "),
			workflow:  "Workflow",
			revision:  1,
			metadata:  nil,
			field:     "companyID",
		},
		{
			name:      "blank workflow name",
			id:        WorkflowID("workflow-1"),
			companyID: CompanyID("company-1"),
			workflow:  " ",
			revision:  1,
			metadata:  nil,
			field:     "name",
		},
		{
			name:      "zero revision",
			id:        WorkflowID("workflow-1"),
			companyID: CompanyID("company-1"),
			workflow:  "Workflow",
			revision:  0,
			metadata:  nil,
			field:     "revision",
		},
		{
			name:      "invalid metadata JSON",
			id:        WorkflowID("workflow-1"),
			companyID: CompanyID("company-1"),
			workflow:  "Workflow",
			revision:  1,
			metadata:  []byte(`{"category":`),
			field:     "metadata",
		},
		{
			name:      "metadata is an array",
			id:        WorkflowID("workflow-1"),
			companyID: CompanyID("company-1"),
			workflow:  "Workflow",
			revision:  1,
			metadata:  []byte(`[]`),
			field:     "metadata",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWorkflowDefinition(
				test.id,
				test.companyID,
				test.workflow,
				test.revision,
				nil,
				nil,
				test.metadata,
			)
			if err == nil {
				t.Fatal(
					"NewWorkflowDefinition() returned nil error for an invalid field",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewWorkflowDefinitionRejectsInvalidChildDefinitions(t *testing.T) {
	tests := []struct {
		name  string
		nodes []NodeDefinition
		edges []EdgeDefinition
		field string
	}{
		{
			name: "invalid node",
			nodes: []NodeDefinition{
				{},
			},
			edges: nil,
			field: "nodes[0]",
		},
		{
			name:  "invalid edge",
			nodes: nil,
			edges: []EdgeDefinition{
				{},
			},
			field: "edges[0]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewWorkflowDefinition(
				WorkflowID("workflow-1"),
				CompanyID("company-1"),
				"Workflow",
				1,
				test.nodes,
				test.edges,
				nil,
			)
			if err == nil {
				t.Fatal(
					"NewWorkflowDefinition() returned nil error for an invalid child definition",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewWorkflowDefinitionDefensivelyCopiesInputCollections(t *testing.T) {
	sourceNode := newTestNodeDefinition(
		t,
		"source-node",
		`{"value":"original"}`,
	)
	targetNode := newTestNodeDefinition(
		t,
		"target-node",
		`{}`,
	)
	edge := newTestEdgeDefinition(
		t,
		"edge-1",
		"source-node",
		"target-node",
	)

	nodes := []NodeDefinition{
		sourceNode,
		targetNode,
	}
	edges := []EdgeDefinition{
		edge,
	}

	definition, err := NewWorkflowDefinition(
		WorkflowID("workflow-1"),
		CompanyID("company-1"),
		"Workflow",
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

	replacementNode := newTestNodeDefinition(
		t,
		"replacement-node",
		`{}`,
	)
	replacementEdge := newTestEdgeDefinition(
		t,
		"replacement-edge",
		"replacement-node",
		"target-node",
	)

	nodes[0].configuration.raw[2] = 'X'
	nodes[0] = replacementNode
	edges[0] = replacementEdge

	storedNodes := definition.Nodes()
	if actual := storedNodes[0].ID().String(); actual != "source-node" {
		t.Fatalf(
			"stored node ID = %q, want %q",
			actual,
			"source-node",
		)
	}

	if actual := storedNodes[0].Configuration().String(); actual != `{"value":"original"}` {
		t.Fatalf(
			"stored node configuration = %q, want %q",
			actual,
			`{"value":"original"}`,
		)
	}

	storedEdges := definition.Edges()
	if actual := storedEdges[0].ID().String(); actual != "edge-1" {
		t.Fatalf(
			"stored edge ID = %q, want %q",
			actual,
			"edge-1",
		)
	}
}

func TestWorkflowDefinitionCollectionsReturnDefensiveCopies(t *testing.T) {
	sourceNode := newTestNodeDefinition(
		t,
		"source-node",
		`{"value":"original"}`,
	)
	targetNode := newTestNodeDefinition(
		t,
		"target-node",
		`{}`,
	)
	edge := newTestEdgeDefinition(
		t,
		"edge-1",
		"source-node",
		"target-node",
	)

	definition, err := NewWorkflowDefinition(
		WorkflowID("workflow-1"),
		CompanyID("company-1"),
		"Workflow",
		1,
		[]NodeDefinition{
			sourceNode,
			targetNode,
		},
		[]EdgeDefinition{
			edge,
		},
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowDefinition() returned an unexpected error: %v",
			err,
		)
	}

	returnedNodes := definition.Nodes()
	returnedNodes[0].configuration.raw[2] = 'X'
	returnedNodes[0] = newTestNodeDefinition(
		t,
		"replacement-node",
		`{}`,
	)

	returnedEdges := definition.Edges()
	returnedEdges[0] = newTestEdgeDefinition(
		t,
		"replacement-edge",
		"replacement-node",
		"target-node",
	)

	storedNodes := definition.Nodes()
	if actual := storedNodes[0].ID().String(); actual != "source-node" {
		t.Fatalf(
			"stored node ID = %q, want %q",
			actual,
			"source-node",
		)
	}

	if actual := storedNodes[0].Configuration().String(); actual != `{"value":"original"}` {
		t.Fatalf(
			"stored node configuration = %q, want %q",
			actual,
			`{"value":"original"}`,
		)
	}

	storedEdges := definition.Edges()
	if actual := storedEdges[0].ID().String(); actual != "edge-1" {
		t.Fatalf(
			"stored edge ID = %q, want %q",
			actual,
			"edge-1",
		)
	}
}

func TestWorkflowDefinitionMetadataIsDefensivelyCopied(t *testing.T) {
	source := []byte(`{"owner":"operations"}`)
	expected := bytes.Clone(source)

	definition, err := NewWorkflowDefinition(
		WorkflowID("workflow-1"),
		CompanyID("company-1"),
		"Workflow",
		1,
		nil,
		nil,
		source,
	)
	if err != nil {
		t.Fatalf(
			"NewWorkflowDefinition() returned an unexpected error: %v",
			err,
		)
	}

	source[2] = 'X'

	if actual := definition.Metadata().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"stored metadata = %q, want %q",
			actual,
			expected,
		)
	}

	returned := definition.Metadata()
	returned.raw[2] = 'X'

	if actual := definition.Metadata().Bytes(); !bytes.Equal(
		actual,
		expected,
	) {
		t.Fatalf(
			"stored metadata after returned-value mutation = %q, want %q",
			actual,
			expected,
		)
	}
}

func newTestNodeDefinition(
	t *testing.T,
	id string,
	configuration string,
) NodeDefinition {
	t.Helper()

	definition, err := NewNodeDefinition(
		NodeID(id),
		PluginType("core.pass-through"),
		PluginVersion("v1"),
		[]byte(configuration),
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return definition
}

func newTestEdgeDefinition(
	t *testing.T,
	id string,
	sourceNodeID string,
	targetNodeID string,
) EdgeDefinition {
	t.Helper()

	definition, err := NewEdgeDefinition(
		EdgeID(id),
		NodeID(sourceNodeID),
		"output",
		NodeID(targetNodeID),
		"input",
	)
	if err != nil {
		t.Fatalf(
			"NewEdgeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	return definition
}

func TestNewEdgeDefinitionStoresNormalizedValues(t *testing.T) {
	definition, err := NewEdgeDefinition(
		EdgeID(" edge-1 "),
		NodeID(" source-node "),
		" output ",
		NodeID(" target-node "),
		" input ",
	)
	if err != nil {
		t.Fatalf(
			"NewEdgeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	if actual := definition.ID().String(); actual != "edge-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"edge-1",
		)
	}

	if actual := definition.SourceNodeID().String(); actual != "source-node" {
		t.Fatalf(
			"SourceNodeID() = %q, want %q",
			actual,
			"source-node",
		)
	}

	if actual := definition.SourceOutputPort(); actual != "output" {
		t.Fatalf(
			"SourceOutputPort() = %q, want %q",
			actual,
			"output",
		)
	}

	if actual := definition.TargetNodeID().String(); actual != "target-node" {
		t.Fatalf(
			"TargetNodeID() = %q, want %q",
			actual,
			"target-node",
		)
	}

	if actual := definition.TargetInputPort(); actual != "input" {
		t.Fatalf(
			"TargetInputPort() = %q, want %q",
			actual,
			"input",
		)
	}
}

func TestNewEdgeDefinitionRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name             string
		id               EdgeID
		sourceNodeID     NodeID
		sourceOutputPort string
		targetNodeID     NodeID
		targetInputPort  string
		field            string
	}{
		{
			name:             "blank edge ID",
			id:               EdgeID(" "),
			sourceNodeID:     NodeID("source-node"),
			sourceOutputPort: "output",
			targetNodeID:     NodeID("target-node"),
			targetInputPort:  "input",
			field:            "edgeID",
		},
		{
			name:             "blank source node ID",
			id:               EdgeID("edge-1"),
			sourceNodeID:     NodeID(" "),
			sourceOutputPort: "output",
			targetNodeID:     NodeID("target-node"),
			targetInputPort:  "input",
			field:            "sourceNodeID",
		},
		{
			name:             "blank source output port",
			id:               EdgeID("edge-1"),
			sourceNodeID:     NodeID("source-node"),
			sourceOutputPort: " ",
			targetNodeID:     NodeID("target-node"),
			targetInputPort:  "input",
			field:            "sourceOutputPort",
		},
		{
			name:             "blank target node ID",
			id:               EdgeID("edge-1"),
			sourceNodeID:     NodeID("source-node"),
			sourceOutputPort: "output",
			targetNodeID:     NodeID(" "),
			targetInputPort:  "input",
			field:            "targetNodeID",
		},
		{
			name:             "blank target input port",
			id:               EdgeID("edge-1"),
			sourceNodeID:     NodeID("source-node"),
			sourceOutputPort: "output",
			targetNodeID:     NodeID("target-node"),
			targetInputPort:  " ",
			field:            "targetInputPort",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewEdgeDefinition(
				test.id,
				test.sourceNodeID,
				test.sourceOutputPort,
				test.targetNodeID,
				test.targetInputPort,
			)
			if err == nil {
				t.Fatal(
					"NewEdgeDefinition() returned nil error for an invalid field",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewEdgeDefinitionDoesNotPerformGraphValidation(t *testing.T) {
	definition, err := NewEdgeDefinition(
		EdgeID("self-loop-edge"),
		NodeID("node-1"),
		"output",
		NodeID("node-1"),
		"input",
	)
	if err != nil {
		t.Fatalf(
			"NewEdgeDefinition() returned an unexpected error for a structurally representable edge: %v",
			err,
		)
	}

	if definition.SourceNodeID() != definition.TargetNodeID() {
		t.Fatalf(
			"source node = %q, target node = %q, want a self-referencing edge",
			definition.SourceNodeID(),
			definition.TargetNodeID(),
		)
	}
}

type identifierConstructor struct {
	name      string
	field     string
	construct func(string) (string, error)
}

func TestIdentifierConstructorsNormalizeValues(t *testing.T) {
	for _, constructor := range identifierConstructors() {
		t.Run(constructor.name, func(t *testing.T) {
			actual, err := constructor.construct(
				"  identifier-value  ",
			)
			if err != nil {
				t.Fatalf(
					"constructor returned an unexpected error: %v",
					err,
				)
			}

			if actual != "identifier-value" {
				t.Fatalf(
					"identifier = %q, want %q",
					actual,
					"identifier-value",
				)
			}
		})
	}
}

func TestIdentifierConstructorsRejectBlankValues(t *testing.T) {
	for _, constructor := range identifierConstructors() {
		t.Run(constructor.name, func(t *testing.T) {
			_, err := constructor.construct("   ")
			if err == nil {
				t.Fatal(
					"constructor returned nil error for a blank identifier",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != constructor.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					constructor.field,
				)
			}

			if validationError.Reason != "must not be empty" {
				t.Fatalf(
					"validation reason = %q, want %q",
					validationError.Reason,
					"must not be empty",
				)
			}
		})
	}
}

func TestValidationErrorFormatsFieldAndReason(t *testing.T) {
	err := &ValidationError{
		Field:  "workflowID",
		Reason: "must not be empty",
	}

	expected := "workflowID: must not be empty"

	if actual := err.Error(); actual != expected {
		t.Fatalf(
			"Error() = %q, want %q",
			actual,
			expected,
		)
	}
}

func identifierConstructors() []identifierConstructor {
	return []identifierConstructor{
		{
			name:  "company ID",
			field: "companyID",
			construct: func(value string) (string, error) {
				identifier, err := NewCompanyID(value)
				return identifier.String(), err
			},
		},
		{
			name:  "workflow ID",
			field: "workflowID",
			construct: func(value string) (string, error) {
				identifier, err := NewWorkflowID(value)
				return identifier.String(), err
			},
		},
		{
			name:  "node ID",
			field: "nodeID",
			construct: func(value string) (string, error) {
				identifier, err := NewNodeID(value)
				return identifier.String(), err
			},
		},
		{
			name:  "edge ID",
			field: "edgeID",
			construct: func(value string) (string, error) {
				identifier, err := NewEdgeID(value)
				return identifier.String(), err
			},
		},
		{
			name:  "plugin type",
			field: "pluginType",
			construct: func(value string) (string, error) {
				pluginType, err := NewPluginType(value)
				return pluginType.String(), err
			},
		},
		{
			name:  "plugin version",
			field: "pluginVersion",
			construct: func(value string) (string, error) {
				version, err := NewPluginVersion(value)
				return version.String(), err
			},
		},
	}
}

func TestNewJSONObjectAcceptsObject(t *testing.T) {
	object, err := NewJSONObject(
		[]byte(`  {"delay":"2s","enabled":true}  `),
	)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	expected := `{"delay":"2s","enabled":true}`

	if actual := object.String(); actual != expected {
		t.Fatalf(
			"String() = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestNewJSONObjectUsesEmptyObjectForBlankInput(t *testing.T) {
	tests := map[string][]byte{
		"nil":        nil,
		"empty":      {},
		"whitespace": []byte(" \n\t "),
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			object, err := NewJSONObject(value)
			if err != nil {
				t.Fatalf(
					"NewJSONObject() returned an unexpected error: %v",
					err,
				)
			}

			if actual := object.String(); actual != "{}" {
				t.Fatalf(
					"String() = %q, want %q",
					actual,
					"{}",
				)
			}
		})
	}
}

func TestNewJSONObjectRejectsInvalidOrNonObjectValues(t *testing.T) {
	tests := map[string][]byte{
		"invalid JSON": []byte(`{"enabled":`),
		"array":        []byte(`[]`),
		"string":       []byte(`"value"`),
		"number":       []byte(`42`),
		"boolean":      []byte(`true`),
		"null":         []byte(`null`),
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NewJSONObject(value)
			if err == nil {
				t.Fatal(
					"NewJSONObject() returned nil error for an invalid value",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != "jsonObject" {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					"jsonObject",
				)
			}
		})
	}
}

func TestNewJSONObjectDefensivelyCopiesInput(t *testing.T) {
	source := []byte(`{"enabled":true}`)
	expected := bytes.Clone(source)

	object, err := NewJSONObject(source)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	source[2] = 'X'

	if actual := object.Bytes(); !bytes.Equal(actual, expected) {
		t.Fatalf(
			"stored bytes = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestJSONObjectBytesReturnsDefensiveCopy(t *testing.T) {
	object, err := NewJSONObject(
		[]byte(`{"enabled":true}`),
	)
	if err != nil {
		t.Fatalf(
			"NewJSONObject() returned an unexpected error: %v",
			err,
		)
	}

	first := object.Bytes()
	first[2] = 'X'

	second := object.Bytes()
	expected := []byte(`{"enabled":true}`)

	if !bytes.Equal(second, expected) {
		t.Fatalf(
			"stored bytes = %q, want %q",
			second,
			expected,
		)
	}
}

func TestJSONObjectZeroValueBehavesAsEmptyObject(t *testing.T) {
	var object JSONObject

	if actual := object.String(); actual != "{}" {
		t.Fatalf(
			"String() = %q, want %q",
			actual,
			"{}",
		)
	}

	if actual := object.Bytes(); !bytes.Equal(
		actual,
		[]byte(`{}`),
	) {
		t.Fatalf(
			"Bytes() = %q, want %q",
			actual,
			[]byte(`{}`),
		)
	}
}

func TestNewNodePositionStoresCoordinates(t *testing.T) {
	position, err := NewNodePosition(
		-12.5,
		42.25,
	)
	if err != nil {
		t.Fatalf(
			"NewNodePosition() returned an unexpected error: %v",
			err,
		)
	}

	if actual := position.X(); actual != -12.5 {
		t.Fatalf(
			"X() = %f, want %f",
			actual,
			-12.5,
		)
	}

	if actual := position.Y(); actual != 42.25 {
		t.Fatalf(
			"Y() = %f, want %f",
			actual,
			42.25,
		)
	}
}

func TestNewNodePositionRejectsNonFiniteCoordinates(t *testing.T) {
	tests := []struct {
		name  string
		x     float64
		y     float64
		field string
	}{
		{
			name:  "x is NaN",
			x:     math.NaN(),
			y:     1,
			field: "position.x",
		},
		{
			name:  "x is positive infinity",
			x:     math.Inf(1),
			y:     1,
			field: "position.x",
		},
		{
			name:  "x is negative infinity",
			x:     math.Inf(-1),
			y:     1,
			field: "position.x",
		},
		{
			name:  "y is NaN",
			x:     1,
			y:     math.NaN(),
			field: "position.y",
		},
		{
			name:  "y is positive infinity",
			x:     1,
			y:     math.Inf(1),
			field: "position.y",
		},
		{
			name:  "y is negative infinity",
			x:     1,
			y:     math.Inf(-1),
			field: "position.y",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewNodePosition(
				test.x,
				test.y,
			)
			if err == nil {
				t.Fatal(
					"NewNodePosition() returned nil error for a non-finite coordinate",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewNodeDefinitionStoresNormalizedValues(t *testing.T) {
	position, err := NewNodePosition(
		10.5,
		20.25,
	)
	if err != nil {
		t.Fatalf(
			"NewNodePosition() returned an unexpected error: %v",
			err,
		)
	}

	definition, err := NewNodeDefinition(
		NodeID(" node-1 "),
		PluginType(" core.delay "),
		PluginVersion(" v1 "),
		[]byte(`  {"delay":"2s"}  `),
		&position,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	if actual := definition.ID().String(); actual != "node-1" {
		t.Fatalf(
			"ID() = %q, want %q",
			actual,
			"node-1",
		)
	}

	if actual := definition.PluginType().String(); actual != "core.delay" {
		t.Fatalf(
			"PluginType() = %q, want %q",
			actual,
			"core.delay",
		)
	}

	if actual := definition.PluginVersion().String(); actual != "v1" {
		t.Fatalf(
			"PluginVersion() = %q, want %q",
			actual,
			"v1",
		)
	}

	if actual := definition.Configuration().String(); actual != `{"delay":"2s"}` {
		t.Fatalf(
			"Configuration() = %q, want %q",
			actual,
			`{"delay":"2s"}`,
		)
	}

	actualPosition, exists := definition.Position()
	if !exists {
		t.Fatal(
			"Position() reported that the configured position does not exist",
		)
	}

	if actualPosition.X() != 10.5 {
		t.Fatalf(
			"position X = %f, want %f",
			actualPosition.X(),
			10.5,
		)
	}

	if actualPosition.Y() != 20.25 {
		t.Fatalf(
			"position Y = %f, want %f",
			actualPosition.Y(),
			20.25,
		)
	}
}

func TestNewNodeDefinitionAllowsMissingPosition(t *testing.T) {
	definition, err := NewNodeDefinition(
		NodeID("node-1"),
		PluginType("core.static-input"),
		PluginVersion("v1"),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	_, exists := definition.Position()
	if exists {
		t.Fatal(
			"Position() reported a position for a node without visual coordinates",
		)
	}

	if actual := definition.Configuration().String(); actual != "{}" {
		t.Fatalf(
			"Configuration() = %q, want %q",
			actual,
			"{}",
		)
	}
}

func TestNewNodeDefinitionRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name          string
		id            NodeID
		pluginType    PluginType
		pluginVersion PluginVersion
		configuration []byte
		field         string
	}{
		{
			name:          "blank node ID",
			id:            NodeID(" "),
			pluginType:    PluginType("core.delay"),
			pluginVersion: PluginVersion("v1"),
			configuration: nil,
			field:         "nodeID",
		},
		{
			name:          "blank plugin type",
			id:            NodeID("node-1"),
			pluginType:    PluginType(" "),
			pluginVersion: PluginVersion("v1"),
			configuration: nil,
			field:         "pluginType",
		},
		{
			name:          "blank plugin version",
			id:            NodeID("node-1"),
			pluginType:    PluginType("core.delay"),
			pluginVersion: PluginVersion(" "),
			configuration: nil,
			field:         "pluginVersion",
		},
		{
			name:          "invalid configuration JSON",
			id:            NodeID("node-1"),
			pluginType:    PluginType("core.delay"),
			pluginVersion: PluginVersion("v1"),
			configuration: []byte(`{"delay":`),
			field:         "configuration",
		},
		{
			name:          "configuration is an array",
			id:            NodeID("node-1"),
			pluginType:    PluginType("core.delay"),
			pluginVersion: PluginVersion("v1"),
			configuration: []byte(`[]`),
			field:         "configuration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewNodeDefinition(
				test.id,
				test.pluginType,
				test.pluginVersion,
				test.configuration,
				nil,
			)
			if err == nil {
				t.Fatal(
					"NewNodeDefinition() returned nil error for an invalid field",
				)
			}

			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf(
					"error type = %T, want *ValidationError",
					err,
				)
			}

			if validationError.Field != test.field {
				t.Fatalf(
					"validation field = %q, want %q",
					validationError.Field,
					test.field,
				)
			}
		})
	}
}

func TestNewNodeDefinitionDefensivelyCopiesConfigurationInput(t *testing.T) {
	source := []byte(`{"enabled":true}`)
	expected := bytes.Clone(source)

	definition, err := NewNodeDefinition(
		NodeID("node-1"),
		PluginType("core.pass-through"),
		PluginVersion("v1"),
		source,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	source[2] = 'X'

	actual := definition.Configuration().Bytes()
	if !bytes.Equal(actual, expected) {
		t.Fatalf(
			"stored configuration = %q, want %q",
			actual,
			expected,
		)
	}
}

func TestNodeDefinitionConfigurationReturnsDefensiveCopy(t *testing.T) {
	definition, err := NewNodeDefinition(
		NodeID("node-1"),
		PluginType("core.pass-through"),
		PluginVersion("v1"),
		[]byte(`{"enabled":true}`),
		nil,
	)
	if err != nil {
		t.Fatalf(
			"NewNodeDefinition() returned an unexpected error: %v",
			err,
		)
	}

	returned := definition.Configuration()
	returned.raw[2] = 'X'

	actual := definition.Configuration().String()
	expected := `{"enabled":true}`

	if actual != expected {
		t.Fatalf(
			"stored configuration = %q, want %q",
			actual,
			expected,
		)
	}
}
