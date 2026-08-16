package workflow

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"miletos-go/internal/features/workflow-runtime/plugin"
)

type ValidationIssue struct {
	Code          string
	Field         string
	Reason        string
	NodeID        string
	EdgeID        string
	PluginType    string
	PluginVersion string
	Expected      string
	Actual        string
	CyclePath     []string
}

type WorkflowDefinitionValidationError struct {
	Issues []ValidationIssue
}

func (validationError *WorkflowDefinitionValidationError) Error() string {
	if len(validationError.Issues) == 0 {
		return "workflow definition is invalid"
	}
	return validationError.Issues[0].Reason
}

type RegistrationLookup func(string) (plugin.NodeRegistration, bool)
type ConfigurationValidator func(string, map[string]any) error
type ConnectionRestrictionLookup func(string, string, string) bool

func ValidateWorkflow(workflow Workflow, nodeDefined func(string) bool) error {
	inputPorts := make([]plugin.Port, 0, len(workflow.Edges))
	outputPorts := make([]plugin.Port, 0, len(workflow.Edges))
	for _, edge := range workflow.Edges {
		inputPorts = append(inputPorts, plugin.Port{Name: edge.TargetInputPort})
		outputPorts = append(outputPorts, plugin.Port{Name: edge.SourceOutputPort})
	}
	lookup := func(nodeType string) (plugin.NodeRegistration, bool) {
		if nodeDefined == nil || nodeDefined(nodeType) {
			return plugin.NodeRegistration{
				Key:        nodeType,
				InputPorts: inputPorts, OutputPorts: outputPorts,
				InputEdgeConstraint:  plugin.EdgeConstraint{},
				OutputEdgeConstraint: plugin.EdgeConstraint{},
			}, true
		}
		return plugin.NodeRegistration{}, false
	}
	return ValidateWorkflowDefinition(workflow, lookup, nil)
}

func ValidateWorkflowDefinition(
	workflow Workflow,
	registration RegistrationLookup,
	validateConfiguration ConfigurationValidator,
	connectionRestrictions ...ConnectionRestrictionLookup,
) error {
	issues := make([]ValidationIssue, 0)
	addInvalid := func(field, reason string) {
		issues = append(issues, ValidationIssue{
			Code: "INVALID_WORKFLOW_DEFINITION", Field: field, Reason: reason,
		})
	}
	if strings.TrimSpace(workflow.ID) == "" {
		addInvalid("definition.id", "Workflow id is required.")
	}
	if strings.TrimSpace(workflow.CompanyID) == "" {
		addInvalid("definition.companyId", "Company id is required.")
	}
	if strings.TrimSpace(workflow.Name) == "" {
		addInvalid("definition.name", "Workflow name is required.")
	}
	if workflow.Revision == 0 {
		addInvalid("definition.revision", "Workflow revision must be positive.")
	}
	if len(workflow.Nodes) == 0 {
		addInvalid("definition.nodes", "Workflow must contain at least one node.")
	}
	if len(workflow.Nodes) > 1000 {
		addInvalid("definition.nodes", "Workflow cannot contain more than 1000 nodes.")
	}
	if len(workflow.Edges) > 5000 {
		addInvalid("definition.edges", "Workflow cannot contain more than 5000 edges.")
	}

	nodes := append([]WorkflowNode(nil), workflow.Nodes...)
	sort.SliceStable(nodes, func(left, right int) bool {
		if nodes[left].ID == nodes[right].ID {
			return nodes[left].Type < nodes[right].Type
		}
		return nodes[left].ID < nodes[right].ID
	})
	nodeByID := make(map[string]WorkflowNode, len(nodes))
	registrations := make(map[string]plugin.NodeRegistration, len(nodes))
	for _, node := range nodes {
		if strings.TrimSpace(node.ID) == "" {
			issues = append(issues, ValidationIssue{
				Code: "INVALID_WORKFLOW_DEFINITION", Field: "definition.nodes[].id",
				Reason: "Node id is required.",
			})
			continue
		}
		if _, exists := nodeByID[node.ID]; exists {
			issues = append(issues, ValidationIssue{
				Code: "INVALID_WORKFLOW_DEFINITION", Field: "definition.nodes[].id",
				Reason: fmt.Sprintf("node id %q is duplicated", node.ID), NodeID: node.ID,
			})
			continue
		}
		nodeByID[node.ID] = node
		if strings.TrimSpace(node.Type) == "" {
			issues = append(issues, ValidationIssue{
				Code: "INVALID_WORKFLOW_DEFINITION", Field: "pluginType",
				Reason: "Plugin type is required.", NodeID: node.ID,
			})
			continue
		}
		pluginVersion := normalizedVersion(node.Version)
		declaration, exists := registration(node.Type)
		if !exists {
			reason := fmt.Sprintf(
				"node %q uses undefined plugin type %q",
				node.ID,
				node.Type,
			)
			issues = append(issues, ValidationIssue{
				Code: "PLUGIN_NOT_FOUND", Field: "pluginType", Reason: reason,
				NodeID: node.ID, PluginType: node.Type, PluginVersion: pluginVersion,
			})
			continue
		}
		registrations[node.ID] = declaration
		if validateConfiguration != nil {
			if err := validateConfiguration(node.Type, node.Configuration); err != nil {
				issues = append(issues, ValidationIssue{
					Code: configurationIssueCode(err), Field: "configuration",
					Reason: err.Error(), NodeID: node.ID,
					PluginType: node.Type, PluginVersion: pluginVersion,
				})
			}
		}
	}

	edges := append([]Edge(nil), workflow.Edges...)
	sort.SliceStable(edges, func(left, right int) bool { return edges[left].ID < edges[right].ID })
	edgeIDs := make(map[string]struct{}, len(edges))
	edgeKeys := make(map[string]string, len(edges))
	validEdges := make([]Edge, 0, len(edges))
	incoming := make(map[string]uint, len(nodes))
	incomingByPort := make(map[string]map[string]uint, len(nodes))
	outgoing := make(map[string]uint, len(nodes))
	for _, edge := range edges {
		if strings.TrimSpace(edge.ID) == "" {
			issues = append(issues, ValidationIssue{
				Code: "INVALID_WORKFLOW_DEFINITION", Field: "definition.edges[].id",
				Reason: "Edge id is required.",
			})
			continue
		}
		if _, exists := edgeIDs[edge.ID]; exists {
			issues = append(issues, ValidationIssue{
				Code: "INVALID_WORKFLOW_DEFINITION", Field: "definition.edges[].id",
				Reason: fmt.Sprintf("edge id %q is duplicated", edge.ID), EdgeID: edge.ID,
			})
			continue
		}
		edgeIDs[edge.ID] = struct{}{}
		sourcePortBlank := strings.TrimSpace(edge.SourceOutputPort) == ""
		targetPortBlank := strings.TrimSpace(edge.TargetInputPort) == ""
		if sourcePortBlank {
			issues = append(issues, ValidationIssue{
				Code: "UNKNOWN_OUTPUT_PORT", Field: "sourceOutputPort",
				Reason: "Edge ports must include a nonblank source output port.",
				EdgeID: edge.ID,
			})
		}
		if targetPortBlank {
			issues = append(issues, ValidationIssue{
				Code: "UNKNOWN_INPUT_PORT", Field: "targetInputPort",
				Reason: "Edge ports must include a nonblank target input port.",
				EdgeID: edge.ID,
			})
		}
		source, sourceExists := nodeByID[edge.SourceNodeID]
		target, targetExists := nodeByID[edge.TargetNodeID]
		structurallyValid := !sourcePortBlank && !targetPortBlank &&
			sourceExists && targetExists &&
			edge.SourceNodeID != edge.TargetNodeID
		if !sourceExists {
			issues = append(issues, ValidationIssue{
				Code: "INVALID_WORKFLOW_DEFINITION", Field: "sourceNodeId",
				Reason: "Edge references an unknown source node.", EdgeID: edge.ID,
				Actual: edge.SourceNodeID,
			})
		}
		if !targetExists {
			issues = append(issues, ValidationIssue{
				Code: "INVALID_WORKFLOW_DEFINITION", Field: "targetNodeId",
				Reason: "Edge references an unknown target node.", EdgeID: edge.ID,
				Actual: edge.TargetNodeID,
			})
		}
		if edge.SourceNodeID == edge.TargetNodeID {
			issues = append(issues, ValidationIssue{
				Code: "INVALID_WORKFLOW_DEFINITION", Field: "targetNodeId",
				Reason: "An edge cannot connect a node to itself.", EdgeID: edge.ID,
				NodeID: edge.SourceNodeID,
			})
		}
		if sourceExists {
			if declaration, exists := registrations[source.ID]; exists &&
				!sourcePortBlank &&
				!hasPort(declaration.OutputPorts, edge.SourceOutputPort) {
				structurallyValid = false
				issues = append(issues, ValidationIssue{
					Code: "UNKNOWN_OUTPUT_PORT", Field: "sourceOutputPort",
					Reason: "Source output port is not declared by the plugin.",
					NodeID: source.ID, EdgeID: edge.ID, PluginType: source.Type,
					PluginVersion: normalizedVersion(source.Version),
					Expected:      portNames(declaration.OutputPorts), Actual: edge.SourceOutputPort,
				})
			}
		}
		if targetExists {
			if declaration, exists := registrations[target.ID]; exists &&
				!targetPortBlank &&
				!hasPort(declaration.InputPorts, edge.TargetInputPort) {
				structurallyValid = false
				issues = append(issues, ValidationIssue{
					Code: "UNKNOWN_INPUT_PORT", Field: "targetInputPort",
					Reason: "Target input port is not declared by the plugin.",
					NodeID: target.ID, EdgeID: edge.ID, PluginType: target.Type,
					PluginVersion: normalizedVersion(target.Version),
					Expected:      portNames(declaration.InputPorts), Actual: edge.TargetInputPort,
				})
			}
		}
		if sourceExists && targetExists && !targetPortBlank &&
			len(connectionRestrictions) > 0 && connectionRestrictions[0] != nil {
			_, sourceRegistered := registrations[source.ID]
			_, targetRegistered := registrations[target.ID]
			if sourceRegistered && targetRegistered && connectionRestrictions[0](
				source.Type,
				target.Type,
				edge.TargetInputPort,
			) {
				structurallyValid = false
				issues = append(issues, ValidationIssue{
					Code: "EDGE_CONNECTION_RESTRICTED", Field: "targetInputPort",
					Reason: "The plugin connection is restricted for this target input port.",
					NodeID: target.ID, EdgeID: edge.ID, PluginType: target.Type,
					PluginVersion: normalizedVersion(target.Version),
					Actual:        source.Type + " -> " + target.Type + "." + edge.TargetInputPort,
				})
			}
		}
		if !sourcePortBlank && !targetPortBlank {
			edgeKey := strings.Join([]string{
				edge.SourceNodeID, edge.SourceOutputPort, edge.TargetNodeID, edge.TargetInputPort,
			}, "\x00")
			if existingEdgeID, exists := edgeKeys[edgeKey]; exists {
				structurallyValid = false
				issues = append(issues, ValidationIssue{
					Code: "DUPLICATE_EDGE", Field: "definition.edges",
					Reason: fmt.Sprintf(
						"Edge duplicates the connection already declared by edge %q.",
						existingEdgeID,
					),
					EdgeID: edge.ID,
					Actual: existingEdgeID,
				})
			} else {
				edgeKeys[edgeKey] = edge.ID
			}
		}
		if structurallyValid {
			validEdges = append(validEdges, edge)
		}
	}
	for _, edge := range validEdges {
		outgoing[edge.SourceNodeID]++
		incoming[edge.TargetNodeID]++
		if incomingByPort[edge.TargetNodeID] == nil {
			incomingByPort[edge.TargetNodeID] = make(map[string]uint)
		}
		incomingByPort[edge.TargetNodeID][edge.TargetInputPort]++
	}

	for _, node := range nodes {
		declaration, exists := registrations[node.ID]
		if !exists {
			continue
		}
		issues = append(issues, constraintIssues(
			node, "INPUT", incoming[node.ID], declaration.InputEdgeConstraint,
		)...)
		issues = append(issues, constraintIssues(
			node, "OUTPUT", outgoing[node.ID], declaration.OutputEdgeConstraint,
		)...)
		issues = append(issues, fixedMultiInputPortIssues(
			node, declaration, incomingByPort[node.ID],
		)...)
		issues = append(issues, portConstraintIssues(
			node, declaration.InputPorts, incomingByPort[node.ID],
		)...)
	}
	canonicalWorkflow := workflow
	canonicalWorkflow.Edges = validEdges
	if cyclePath := FindCyclePath(canonicalWorkflow); len(cyclePath) > 0 {
		issues = append(issues, ValidationIssue{
			Code: "CYCLE_DETECTED", Field: "definition.edges",
			Reason: "Workflow graph contains a cycle.", CyclePath: cyclePath,
		})
	}
	if len(issues) > 0 {
		return &WorkflowDefinitionValidationError{Issues: issues}
	}
	return nil
}

func portConstraintIssues(
	node WorkflowNode,
	ports []plugin.Port,
	incoming map[string]uint,
) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, port := range ports {
		if port.EdgeConstraint == nil {
			continue
		}
		actual := incoming[port.Name]
		for _, issue := range constraintIssues(node, "INPUT", actual, *port.EdgeConstraint) {
			issue.Code = "INPUT_PORT_EDGE_COUNT_INVALID"
			issue.Field = "inputEdges"
			issue.Reason = "Input edge count for the declared port violates its plugin constraint."
			issue.Expected = port.Name + ": " + issue.Expected
			issue.Actual = port.Name + ": " + issue.Actual
			issues = append(issues, issue)
		}
	}
	return issues
}

func fixedMultiInputPortIssues(
	node WorkflowNode,
	declaration plugin.NodeRegistration,
	incoming map[string]uint,
) []ValidationIssue {
	constraint := declaration.InputEdgeConstraint
	if declaration.InputMode != plugin.NodeInputMulti ||
		constraint.Maximum == nil ||
		constraint.Minimum != *constraint.Maximum ||
		constraint.Minimum != uint(len(declaration.InputPorts)) {
		return nil
	}
	issues := make([]ValidationIssue, 0)
	for _, port := range declaration.InputPorts {
		actual := incoming[port.Name]
		if actual == 1 {
			continue
		}
		issues = append(issues, ValidationIssue{
			Code: "INPUT_PORT_EDGE_COUNT_INVALID", Field: "inputEdges",
			Reason: "Each declared input port must receive exactly one edge.",
			NodeID: node.ID, PluginType: node.Type,
			PluginVersion: normalizedVersion(node.Version),
			Expected:      port.Name + ": 1",
			Actual:        port.Name + ": " + strconv.FormatUint(uint64(actual), 10),
		})
	}
	return issues
}

func ValidateExecutionRequest(workflow Workflow) error {
	if len(workflow.Nodes) > 1000 {
		return fmt.Errorf("workflow cannot contain more than 1000 nodes")
	}
	if len(workflow.Edges) > 5000 {
		return fmt.Errorf("workflow cannot contain more than 5000 edges")
	}
	return nil
}

func configurationIssueCode(err error) string {
	var nodeError *plugin.NodeError
	if errors.As(err, &nodeError) && strings.TrimSpace(nodeError.Code) != "" {
		return nodeError.Code
	}
	return "INVALID_PLUGIN_CONFIGURATION"
}

func normalizedVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		return "v1"
	}
	return strings.TrimSpace(version)
}

func hasPort(ports []plugin.Port, requested string) bool {
	for _, port := range ports {
		if port.Name == requested {
			return true
		}
	}
	return false
}

func portNames(ports []plugin.Port) string {
	names := make([]string, 0, len(ports))
	for _, port := range ports {
		names = append(names, port.Name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "no ports"
	}
	return strings.Join(names, ", ")
}

func constraintIssues(
	node WorkflowNode,
	direction string,
	actual uint,
	constraint plugin.EdgeConstraint,
) []ValidationIssue {
	issues := make([]ValidationIssue, 0, 1)
	field := strings.ToLower(direction) + "Edges"
	if actual < constraint.Minimum {
		issues = append(issues, ValidationIssue{
			Code: direction + "_EDGE_COUNT_BELOW_MINIMUM", Field: field,
			Reason: directionLabel(direction) + " edge count is below the plugin minimum.",
			NodeID: node.ID, PluginType: node.Type,
			PluginVersion: normalizedVersion(node.Version),
			Expected:      strconv.FormatUint(uint64(constraint.Minimum), 10),
			Actual:        strconv.FormatUint(uint64(actual), 10),
		})
	}
	if constraint.Maximum != nil && actual > *constraint.Maximum {
		issues = append(issues, ValidationIssue{
			Code: direction + "_EDGE_COUNT_ABOVE_MAXIMUM", Field: field,
			Reason: directionLabel(direction) + " edge count exceeds the plugin maximum.",
			NodeID: node.ID, PluginType: node.Type,
			PluginVersion: normalizedVersion(node.Version),
			Expected:      strconv.FormatUint(uint64(*constraint.Maximum), 10),
			Actual:        strconv.FormatUint(uint64(actual), 10),
		})
	}
	return issues
}

func directionLabel(direction string) string {
	return strings.ToUpper(direction[:1]) + strings.ToLower(direction[1:])
}
