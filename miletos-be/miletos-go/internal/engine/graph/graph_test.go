package graph

import (
	workflowdomain "miletos-go/internal/features/workflow"
	reflect "reflect"
	testing "testing"
)

func TestCyclePathReportsNoCycleForAcyclicGraph(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
			mustNode(t, "node-c"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-c",
			),
		},
	)

	built := Build(definition)

	cyclePath, found := built.CyclePath()
	if found {
		t.Fatalf(
			"CyclePath() found unexpected cycle %v",
			cyclePath,
		)
	}

	if cyclePath != nil {
		t.Fatalf(
			"CyclePath() path = %v, want nil",
			cyclePath,
		)
	}
}

func TestCyclePathDetectsTwoNodeCycle(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-b"),
			mustNode(t, "node-a"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-a",
			),
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
		},
	)

	built := Build(definition)

	cyclePath, found := built.CyclePath()
	if !found {
		t.Fatal(
			"CyclePath() did not find the two-node cycle",
		)
	}

	assertNodeIDs(
		t,
		cyclePath,
		[]string{
			"node-a",
			"node-b",
			"node-a",
		},
	)
}

func TestCyclePathDetectsCycleInDisconnectedComponent(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "root-a"),
			mustNode(t, "root-b"),
			mustNode(t, "cycle-a"),
			mustNode(t, "cycle-b"),
			mustNode(t, "cycle-c"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"root-a",
				"root-b",
			),
			mustEdge(
				t,
				"edge-2",
				"cycle-a",
				"cycle-b",
			),
			mustEdge(
				t,
				"edge-3",
				"cycle-b",
				"cycle-c",
			),
			mustEdge(
				t,
				"edge-4",
				"cycle-c",
				"cycle-a",
			),
		},
	)

	built := Build(definition)

	cyclePath, found := built.CyclePath()
	if !found {
		t.Fatal(
			"CyclePath() did not find the disconnected cycle",
		)
	}

	assertNodeIDs(
		t,
		cyclePath,
		[]string{
			"cycle-a",
			"cycle-b",
			"cycle-c",
			"cycle-a",
		},
	)
}

func TestCanonicalizeCyclePathUsesSmallestNodeID(
	t *testing.T,
) {
	actual := canonicalizeCyclePath(
		[]workflowdomain.NodeID{
			"node-b",
			"node-c",
			"node-a",
			"node-b",
		},
	)

	assertNodeIDs(
		t,
		actual,
		[]string{
			"node-a",
			"node-b",
			"node-c",
			"node-a",
		},
	)
}

func TestCyclePathReturnsIndependentResults(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-a",
			),
		},
	)

	built := Build(definition)

	first, found := built.CyclePath()
	if !found {
		t.Fatal(
			"CyclePath() did not find the cycle",
		)
	}

	first[0] = workflowdomain.NodeID(
		"changed-node",
	)

	second, found := built.CyclePath()
	if !found {
		t.Fatal(
			"CyclePath() did not find the cycle on the second call",
		)
	}

	if actual := second[0].String(); actual != "node-a" {
		t.Fatalf(
			"second cycle start = %q, want %q",
			actual,
			"node-a",
		)
	}
}

func TestValidateIncludesCanonicalCycleIssue(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-c"),
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-c",
			),
			mustEdge(
				t,
				"edge-3",
				"node-c",
				"node-a",
			),
		},
	)

	_, report := BuildValidated(definition)

	if !report.HasCode(IssueCodeCycleDetected) {
		t.Fatal(
			"validation report does not contain CYCLE_DETECTED",
		)
	}

	cycleIssue, found := findIssueByCode(
		report.Issues(),
		IssueCodeCycleDetected,
	)
	if !found {
		t.Fatal(
			"CYCLE_DETECTED issue was not found",
		)
	}

	assertNodeIDs(
		t,
		cycleIssue.Path,
		[]string{
			"node-a",
			"node-b",
			"node-c",
			"node-a",
		},
	)

	if actual := cycleIssue.NodeID.String(); actual != "node-a" {
		t.Fatalf(
			"cycle issue node ID = %q, want %q",
			actual,
			"node-a",
		)
	}
}

func TestTopologicalOrderIsDeterministic(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-d"),
			mustNode(t, "node-c"),
			mustNode(t, "node-b"),
			mustNode(t, "node-a"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-4",
				"node-c",
				"node-d",
			),
			mustEdge(
				t,
				"edge-2",
				"node-a",
				"node-c",
			),
			mustEdge(
				t,
				"edge-3",
				"node-b",
				"node-d",
			),
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
		},
	)

	built := Build(definition)

	order, valid := built.TopologicalOrder()
	if !valid {
		t.Fatal(
			"TopologicalOrder() rejected an acyclic graph",
		)
	}

	assertNodeIDs(
		t,
		order,
		[]string{
			"node-a",
			"node-b",
			"node-c",
			"node-d",
		},
	)
}

func TestTopologicalOrderSupportsMultipleRootsAndIsolatedNode(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-d"),
			mustNode(t, "node-c"),
			mustNode(t, "node-b"),
			mustNode(t, "node-a"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-d",
			),
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-d",
			),
		},
	)

	built := Build(definition)

	order, valid := built.TopologicalOrder()
	if !valid {
		t.Fatal(
			"TopologicalOrder() rejected a valid multi-root graph",
		)
	}

	assertNodeIDs(
		t,
		order,
		[]string{
			"node-a",
			"node-b",
			"node-c",
			"node-d",
		},
	)
}

func TestTopologicalOrderRejectsCycle(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-a",
			),
		},
	)

	built := Build(definition)

	order, valid := built.TopologicalOrder()
	if valid {
		t.Fatalf(
			"TopologicalOrder() accepted a cycle with order %v",
			order,
		)
	}

	if order != nil {
		t.Fatalf(
			"TopologicalOrder() order = %v, want nil",
			order,
		)
	}
}

func TestTopologicalOrderPlacesEverySourceBeforeTarget(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-e"),
			mustNode(t, "node-d"),
			mustNode(t, "node-c"),
			mustNode(t, "node-b"),
			mustNode(t, "node-a"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-c",
			),
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-c",
			),
			mustEdge(
				t,
				"edge-3",
				"node-c",
				"node-d",
			),
			mustEdge(
				t,
				"edge-4",
				"node-c",
				"node-e",
			),
		},
	)

	built := Build(definition)

	order, valid := built.TopologicalOrder()
	if !valid {
		t.Fatal(
			"TopologicalOrder() rejected an acyclic graph",
		)
	}

	positions := make(
		map[workflowdomain.NodeID]int,
		len(order),
	)

	for index, nodeID := range order {
		positions[nodeID] = index
	}

	for _, edge := range built.Edges() {
		sourcePosition := positions[edge.SourceNodeID()]
		targetPosition := positions[edge.TargetNodeID()]

		if sourcePosition >= targetPosition {
			t.Fatalf(
				"source %q position %d is not before target %q position %d",
				edge.SourceNodeID(),
				sourcePosition,
				edge.TargetNodeID(),
				targetPosition,
			)
		}
	}
}

func TestTopologicalOrderReturnsIndependentResults(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
		},
	)

	built := Build(definition)

	first, valid := built.TopologicalOrder()
	if !valid {
		t.Fatal(
			"TopologicalOrder() rejected an acyclic graph",
		)
	}

	first[0] = workflowdomain.NodeID(
		"changed-node",
	)

	second, valid := built.TopologicalOrder()
	if !valid {
		t.Fatal(
			"TopologicalOrder() rejected the graph on the second call",
		)
	}

	assertNodeIDs(
		t,
		second,
		[]string{
			"node-a",
			"node-b",
		},
	)
}

func TestCanonicalizeCyclePathReturnsDefensiveCopyForShortPath(
	t *testing.T,
) {
	source := []workflowdomain.NodeID{
		"node-a",
	}

	result := canonicalizeCyclePath(source)
	result[0] = workflowdomain.NodeID(
		"changed-node",
	)

	if actual := source[0].String(); actual != "node-a" {
		t.Fatalf(
			"source path start = %q, want %q",
			actual,
			"node-a",
		)
	}
}

func TestValidationIssueRankPlacesUnknownCodeLast(
	t *testing.T,
) {
	issues := []ValidationIssue{
		{
			Code: ValidationIssueCode(
				"UNKNOWN_ISSUE",
			),
			Message: "unknown",
		},
		{
			Code:    IssueCodeCycleDetected,
			Message: "cycle",
		},
	}

	report := newValidationReport(issues)
	actual := report.Issues()

	expectedCodes := []ValidationIssueCode{
		IssueCodeCycleDetected,
		ValidationIssueCode("UNKNOWN_ISSUE"),
	}

	actualCodes := make(
		[]ValidationIssueCode,
		len(actual),
	)

	for index, issue := range actual {
		actualCodes[index] = issue.Code
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
}

func findIssueByCode(
	issues []ValidationIssue,
	code ValidationIssueCode,
) (ValidationIssue, bool) {
	for _, issue := range issues {
		if issue.Code == code {
			return issue, true
		}
	}

	return ValidationIssue{}, false
}

func TestBuildCreatesDeterministicLookupAndAdjacency(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-c"),
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-c",
			),
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
		},
	)

	built := Build(definition)

	assertNodeDefinitionIDs(
		t,
		built.Nodes(),
		[]string{
			"node-a",
			"node-b",
			"node-c",
		},
	)

	assertEdgeIDs(
		t,
		built.Edges(),
		[]string{
			"edge-1",
			"edge-2",
		},
	)

	node, exists := built.Node(
		workflowdomain.NodeID("node-b"),
	)
	if !exists {
		t.Fatal(
			"Node() did not find node-b",
		)
	}

	if actual := node.ID().String(); actual != "node-b" {
		t.Fatalf(
			"Node() ID = %q, want %q",
			actual,
			"node-b",
		)
	}

	if _, exists := built.Node(
		workflowdomain.NodeID("missing-node"),
	); exists {
		t.Fatal(
			"Node() found an unknown node",
		)
	}

	edge, exists := built.Edge(
		workflowdomain.EdgeID("edge-1"),
	)
	if !exists {
		t.Fatal(
			"Edge() did not find edge-1",
		)
	}

	if actual := edge.ID().String(); actual != "edge-1" {
		t.Fatalf(
			"Edge() ID = %q, want %q",
			actual,
			"edge-1",
		)
	}

	if _, exists := built.Edge(
		workflowdomain.EdgeID("missing-edge"),
	); exists {
		t.Fatal(
			"Edge() found an unknown edge",
		)
	}

	assertEdgeIDs(
		t,
		built.OutgoingEdges(
			workflowdomain.NodeID("node-a"),
		),
		[]string{
			"edge-1",
		},
	)

	assertEdgeIDs(
		t,
		built.IncomingEdges(
			workflowdomain.NodeID("node-b"),
		),
		[]string{
			"edge-1",
		},
	)

	assertEdgeIDs(
		t,
		built.OutgoingEdges(
			workflowdomain.NodeID("node-b"),
		),
		[]string{
			"edge-2",
		},
	)

	assertEdgeIDs(
		t,
		built.IncomingEdges(
			workflowdomain.NodeID("node-c"),
		),
		[]string{
			"edge-2",
		},
	)

	assertNodeIDs(
		t,
		built.Roots(),
		[]string{
			"node-a",
		},
	)

	assertNodeIDs(
		t,
		built.Terminals(),
		[]string{
			"node-c",
		},
	)
}

func TestBuildDetectsMultipleRootsAndTerminals(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-e"),
			mustNode(t, "node-c"),
			mustNode(t, "node-a"),
			mustNode(t, "node-d"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-4",
				"node-c",
				"node-e",
			),
			mustEdge(
				t,
				"edge-2",
				"node-b",
				"node-c",
			),
			mustEdge(
				t,
				"edge-3",
				"node-c",
				"node-d",
			),
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-c",
			),
		},
	)

	built := Build(definition)

	assertNodeIDs(
		t,
		built.Roots(),
		[]string{
			"node-a",
			"node-b",
		},
	)

	assertNodeIDs(
		t,
		built.Terminals(),
		[]string{
			"node-d",
			"node-e",
		},
	)
}

func TestBuildTreatsSingleNodeAsRootAndTerminal(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "only-node"),
		},
		nil,
	)

	built := Build(definition)

	assertNodeIDs(
		t,
		built.Roots(),
		[]string{
			"only-node",
		},
	)

	assertNodeIDs(
		t,
		built.Terminals(),
		[]string{
			"only-node",
		},
	)
}

func TestBuildSupportsEmptyDefinition(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		nil,
		nil,
	)

	built := Build(definition)

	if actual := len(built.Nodes()); actual != 0 {
		t.Fatalf(
			"node count = %d, want %d",
			actual,
			0,
		)
	}

	if actual := len(built.Edges()); actual != 0 {
		t.Fatalf(
			"edge count = %d, want %d",
			actual,
			0,
		)
	}

	if actual := len(built.Roots()); actual != 0 {
		t.Fatalf(
			"root count = %d, want %d",
			actual,
			0,
		)
	}

	if actual := len(built.Terminals()); actual != 0 {
		t.Fatalf(
			"terminal count = %d, want %d",
			actual,
			0,
		)
	}
}

func TestBuildExcludesEdgesWithMissingEndpointsFromAdjacency(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-3",
				"node-a",
				"missing-target",
			),
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
			mustEdge(
				t,
				"edge-2",
				"missing-source",
				"node-b",
			),
		},
	)

	built := Build(definition)

	assertEdgeIDs(
		t,
		built.Edges(),
		[]string{
			"edge-1",
			"edge-2",
			"edge-3",
		},
	)

	assertEdgeIDs(
		t,
		built.OutgoingEdges(
			workflowdomain.NodeID("node-a"),
		),
		[]string{
			"edge-1",
		},
	)

	assertEdgeIDs(
		t,
		built.IncomingEdges(
			workflowdomain.NodeID("node-b"),
		),
		[]string{
			"edge-1",
		},
	)

	assertNodeIDs(
		t,
		built.Roots(),
		[]string{
			"node-a",
		},
	)

	assertNodeIDs(
		t,
		built.Terminals(),
		[]string{
			"node-b",
		},
	)
}

func TestBuildPreservesDuplicateDefinitionsForValidation(
	t *testing.T,
) {
	firstDuplicate := mustNodeWithPlugin(
		t,
		"duplicate-node",
		"core.static-input",
	)
	secondDuplicate := mustNodeWithPlugin(
		t,
		"duplicate-node",
		"core.delay",
	)

	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			firstDuplicate,
			secondDuplicate,
			mustNode(t, "target-node"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"duplicate-edge",
				"duplicate-node",
				"target-node",
			),
			mustEdge(
				t,
				"duplicate-edge",
				"duplicate-node",
				"target-node",
			),
		},
	)

	built := Build(definition)

	assertNodeDefinitionIDs(
		t,
		built.Nodes(),
		[]string{
			"duplicate-node",
			"duplicate-node",
			"target-node",
		},
	)

	assertEdgeIDs(
		t,
		built.Edges(),
		[]string{
			"duplicate-edge",
			"duplicate-edge",
		},
	)

	indexedNode, exists := built.Node(
		workflowdomain.NodeID("duplicate-node"),
	)
	if !exists {
		t.Fatal(
			"Node() did not find duplicate-node",
		)
	}

	if actual := indexedNode.PluginType().String(); actual != "core.static-input" {
		t.Fatalf(
			"indexed plugin type = %q, want %q",
			actual,
			"core.static-input",
		)
	}
}

func TestGraphAccessorsReturnDefensiveCopies(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
		},
	)

	built := Build(definition)

	returnedNodes := built.Nodes()
	returnedNodes[0] = mustNode(
		t,
		"replacement-node",
	)

	returnedEdges := built.Edges()
	returnedEdges[0] = mustEdge(
		t,
		"replacement-edge",
		"replacement-source",
		"replacement-target",
	)

	returnedOutgoing := built.OutgoingEdges(
		workflowdomain.NodeID("node-a"),
	)
	returnedOutgoing[0] = mustEdge(
		t,
		"replacement-outgoing-edge",
		"replacement-source",
		"replacement-target",
	)

	returnedRoots := built.Roots()
	returnedRoots[0] = workflowdomain.NodeID(
		"replacement-root",
	)

	returnedTerminals := built.Terminals()
	returnedTerminals[0] = workflowdomain.NodeID(
		"replacement-terminal",
	)

	assertNodeDefinitionIDs(
		t,
		built.Nodes(),
		[]string{
			"node-a",
			"node-b",
		},
	)

	assertEdgeIDs(
		t,
		built.Edges(),
		[]string{
			"edge-1",
		},
	)

	assertEdgeIDs(
		t,
		built.OutgoingEdges(
			workflowdomain.NodeID("node-a"),
		),
		[]string{
			"edge-1",
		},
	)

	assertNodeIDs(
		t,
		built.Roots(),
		[]string{
			"node-a",
		},
	)

	assertNodeIDs(
		t,
		built.Terminals(),
		[]string{
			"node-b",
		},
	)
}

func mustWorkflowDefinition(
	t *testing.T,
	nodes []workflowdomain.NodeDefinition,
	edges []workflowdomain.EdgeDefinition,
) workflowdomain.WorkflowDefinition {
	t.Helper()

	definition, err := workflowdomain.NewWorkflowDefinition(
		workflowdomain.WorkflowID("workflow-1"),
		workflowdomain.CompanyID("company-1"),
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

	return definition
}

func mustNode(
	t *testing.T,
	id string,
) workflowdomain.NodeDefinition {
	t.Helper()

	return mustNodeWithPlugin(
		t,
		id,
		"core.pass-through",
	)
}

func mustNodeWithPlugin(
	t *testing.T,
	id string,
	pluginType string,
) workflowdomain.NodeDefinition {
	t.Helper()

	definition, err := workflowdomain.NewNodeDefinition(
		workflowdomain.NodeID(id),
		workflowdomain.PluginType(pluginType),
		workflowdomain.PluginVersion("v1"),
		nil,
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

func mustEdge(
	t *testing.T,
	id string,
	sourceNodeID string,
	targetNodeID string,
) workflowdomain.EdgeDefinition {
	t.Helper()

	definition, err := workflowdomain.NewEdgeDefinition(
		workflowdomain.EdgeID(id),
		workflowdomain.NodeID(sourceNodeID),
		"output",
		workflowdomain.NodeID(targetNodeID),
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

func assertNodeDefinitionIDs(
	t *testing.T,
	nodes []workflowdomain.NodeDefinition,
	expected []string,
) {
	t.Helper()

	actual := make(
		[]string,
		len(nodes),
	)

	for index, node := range nodes {
		actual[index] = node.ID().String()
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"node IDs = %v, want %v",
			actual,
			expected,
		)
	}
}

func assertEdgeIDs(
	t *testing.T,
	edges []workflowdomain.EdgeDefinition,
	expected []string,
) {
	t.Helper()

	actual := make(
		[]string,
		len(edges),
	)

	for index, edge := range edges {
		actual[index] = edge.ID().String()
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"edge IDs = %v, want %v",
			actual,
			expected,
		)
	}
}

func assertNodeIDs(
	t *testing.T,
	nodeIDs []workflowdomain.NodeID,
	expected []string,
) {
	t.Helper()

	actual := make(
		[]string,
		len(nodeIDs),
	)

	for index, nodeID := range nodeIDs {
		actual[index] = nodeID.String()
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"node IDs = %v, want %v",
			actual,
			expected,
		)
	}
}

func TestBuildValidatedAcceptsStructurallyValidGraph(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-b"),
			mustNode(t, "node-a"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
		},
	)

	built, report := BuildValidated(definition)

	if !report.IsValid() {
		t.Fatalf(
			"IsValid() = false, issues = %v",
			report.Issues(),
		)
	}

	if actual := report.Len(); actual != 0 {
		t.Fatalf(
			"Len() = %d, want %d",
			actual,
			0,
		)
	}

	assertNodeIDs(
		t,
		built.Roots(),
		[]string{
			"node-a",
		},
	)

	assertNodeIDs(
		t,
		built.Terminals(),
		[]string{
			"node-b",
		},
	)
}

func TestValidateReportsEmptyWorkflow(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		nil,
		nil,
	)

	_, report := BuildValidated(definition)

	assertIssueCodes(
		t,
		report.Issues(),
		[]ValidationIssueCode{
			IssueCodeEmptyWorkflow,
		},
	)

	if !report.HasCode(IssueCodeEmptyWorkflow) {
		t.Fatal(
			"HasCode() = false for EMPTY_WORKFLOW",
		)
	}
}

func TestValidateReportsDuplicateIdentifiers(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "duplicate-node"),
			mustNodeWithPlugin(
				t,
				"duplicate-node",
				"core.delay",
			),
			mustNode(t, "target-node"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"duplicate-edge",
				"duplicate-node",
				"target-node",
			),
			mustEdge(
				t,
				"duplicate-edge",
				"duplicate-node",
				"target-node",
			),
		},
	)

	_, report := BuildValidated(definition)
	issues := report.Issues()

	assertIssueCodes(
		t,
		issues,
		[]ValidationIssueCode{
			IssueCodeDuplicateNodeID,
			IssueCodeDuplicateEdgeID,
		},
	)

	if actual := issues[0].NodeID.String(); actual != "duplicate-node" {
		t.Fatalf(
			"duplicate node ID = %q, want %q",
			actual,
			"duplicate-node",
		)
	}

	if actual := issues[1].EdgeID.String(); actual != "duplicate-edge" {
		t.Fatalf(
			"duplicate edge ID = %q, want %q",
			actual,
			"duplicate-edge",
		)
	}
}

func TestValidateAllowsParallelConnectionsWithDistinctEdgeIDs(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
			mustEdge(
				t,
				"edge-2",
				"node-a",
				"node-b",
			),
		},
	)

	_, report := BuildValidated(definition)

	if !report.IsValid() {
		t.Fatalf(
			"parallel connections produced issues: %v",
			report.Issues(),
		)
	}
}

func TestValidateReportsMissingEndpointsSeparately(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
			mustNode(t, "node-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-a",
				"missing-source",
				"node-b",
			),
			mustEdge(
				t,
				"edge-b",
				"node-a",
				"missing-target",
			),
			mustEdge(
				t,
				"edge-c",
				"missing-source-2",
				"missing-target-2",
			),
		},
	)

	_, report := BuildValidated(definition)
	issues := report.Issues()

	assertIssueCodes(
		t,
		issues,
		[]ValidationIssueCode{
			IssueCodeMissingSourceNode,
			IssueCodeMissingSourceNode,
			IssueCodeMissingTargetNode,
			IssueCodeMissingTargetNode,
		},
	)

	expectedNodeIDs := []string{
		"missing-source",
		"missing-source-2",
		"missing-target",
		"missing-target-2",
	}

	expectedEdgeIDs := []string{
		"edge-a",
		"edge-c",
		"edge-b",
		"edge-c",
	}

	for index, issue := range issues {
		if actual := issue.NodeID.String(); actual != expectedNodeIDs[index] {
			t.Fatalf(
				"issue %d node ID = %q, want %q",
				index,
				actual,
				expectedNodeIDs[index],
			)
		}

		if actual := issue.EdgeID.String(); actual != expectedEdgeIDs[index] {
			t.Fatalf(
				"issue %d edge ID = %q, want %q",
				index,
				actual,
				expectedEdgeIDs[index],
			)
		}
	}
}

func TestValidateReportsSelfLoopWithoutReachabilityNoise(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-a"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"self-loop-edge",
				"node-a",
				"node-a",
			),
		},
	)

	built, report := BuildValidated(definition)

	assertIssueCodes(
		t,
		report.Issues(),
		[]ValidationIssueCode{
			IssueCodeSelfLoopDetected,
		},
	)

	assertNodeIDs(
		t,
		built.Roots(),
		[]string{
			"node-a",
		},
	)

	assertNodeIDs(
		t,
		built.Terminals(),
		[]string{
			"node-a",
		},
	)
}

func TestValidateAcceptsMultipleValidRootComponents(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-d"),
			mustNode(t, "node-c"),
			mustNode(t, "node-b"),
			mustNode(t, "node-a"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-2",
				"node-c",
				"node-d",
			),
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
		},
	)

	_, report := BuildValidated(definition)

	if !report.IsValid() {
		t.Fatalf(
			"multiple valid root components produced issues: %v",
			report.Issues(),
		)
	}
}

func TestValidateReportsUnreachableCycleComponent(
	t *testing.T,
) {
	definition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "node-d"),
			mustNode(t, "node-c"),
			mustNode(t, "node-b"),
			mustNode(t, "node-a"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-3",
				"node-c",
				"node-d",
			),
			mustEdge(
				t,
				"edge-2",
				"node-d",
				"node-c",
			),
			mustEdge(
				t,
				"edge-1",
				"node-a",
				"node-b",
			),
		},
	)

	_, report := BuildValidated(definition)
	issues := report.Issues()

	assertIssueCodes(
		t,
		issues,
		[]ValidationIssueCode{
			IssueCodeUnreachableNode,
			IssueCodeUnreachableNode,
			IssueCodeCycleDetected,
		},
	)

	expectedUnreachableNodeIDs := []string{
		"node-c",
		"node-d",
	}

	for index, expectedNodeID := range expectedUnreachableNodeIDs {
		if actual := issues[index].NodeID.String(); actual != expectedNodeID {
			t.Fatalf(
				"unreachable issue %d node ID = %q, want %q",
				index,
				actual,
				expectedNodeID,
			)
		}
	}

	cycleIssue := issues[2]

	if actual := cycleIssue.NodeID.String(); actual != "node-c" {
		t.Fatalf(
			"cycle issue node ID = %q, want %q",
			actual,
			"node-c",
		)
	}

	assertNodeIDs(
		t,
		cycleIssue.Path,
		[]string{
			"node-c",
			"node-d",
			"node-c",
		},
	)
}

func TestValidationIssueOrderingIsDeterministic(
	t *testing.T,
) {
	firstDefinition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "duplicate-b"),
			mustNode(t, "duplicate-a"),
			mustNode(t, "duplicate-b"),
			mustNode(t, "duplicate-a"),
			mustNode(t, "root-node"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-z",
				"missing-z",
				"root-node",
			),
			mustEdge(
				t,
				"edge-a",
				"missing-a",
				"root-node",
			),
		},
	)

	secondDefinition := mustWorkflowDefinition(
		t,
		[]workflowdomain.NodeDefinition{
			mustNode(t, "root-node"),
			mustNode(t, "duplicate-a"),
			mustNode(t, "duplicate-b"),
			mustNode(t, "duplicate-a"),
			mustNode(t, "duplicate-b"),
		},
		[]workflowdomain.EdgeDefinition{
			mustEdge(
				t,
				"edge-a",
				"missing-a",
				"root-node",
			),
			mustEdge(
				t,
				"edge-z",
				"missing-z",
				"root-node",
			),
		},
	)

	_, firstReport := BuildValidated(firstDefinition)
	_, secondReport := BuildValidated(secondDefinition)

	if !reflect.DeepEqual(
		firstReport.Issues(),
		secondReport.Issues(),
	) {
		t.Fatalf(
			"validation issue order is not deterministic:\nfirst:  %v\nsecond: %v",
			firstReport.Issues(),
			secondReport.Issues(),
		)
	}
}

func TestValidationReportReturnsDefensiveCopies(
	t *testing.T,
) {
	report := newValidationReport(
		[]ValidationIssue{
			{
				Code:    IssueCodeCycleDetected,
				Message: "cycle detected",
				Path: []workflowdomain.NodeID{
					"node-a",
					"node-b",
					"node-a",
				},
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

	first := report.Issues()
	first[0].Message = "changed"
	first[0].Path[0] = workflowdomain.NodeID(
		"changed-node",
	)

	second := report.Issues()

	if actual := second[0].Message; actual != "cycle detected" {
		t.Fatalf(
			"stored message = %q, want %q",
			actual,
			"cycle detected",
		)
	}

	if actual := second[0].Path[0].String(); actual != "node-a" {
		t.Fatalf(
			"stored path start = %q, want %q",
			actual,
			"node-a",
		)
	}
}

func assertIssueCodes(
	t *testing.T,
	issues []ValidationIssue,
	expected []ValidationIssueCode,
) {
	t.Helper()

	actual := make(
		[]ValidationIssueCode,
		len(issues),
	)

	for index, issue := range issues {
		actual[index] = issue.Code
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"issue codes = %v, want %v",
			actual,
			expected,
		)
	}
}
