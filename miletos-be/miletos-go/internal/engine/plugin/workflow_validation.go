package plugin

import (
	"fmt"

	"miletos-go/internal/engine/graph"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

func ValidateWorkflowPlugins(definition workflow.WorkflowDefinition, registry Registry,
	mode execution.ExecutionMode) (WorkflowValidationReport,
	error) {
	normalizedMode, err := execution.ParseExecutionMode(
		mode.String())
	if err != nil {
		return newWorkflowValidationReport(nil), err
	}
	builtGraph, structuralReport := graph.BuildValidated(definition)
	if !structuralReport.IsValid() {
		return newWorkflowValidationReport(nil), &StructuralValidationError{
			IssueCount: structuralReport.Len()}
	}
	nodes := builtGraph.Nodes()
	resolvedDescriptors := make(map[workflow.NodeID]Descriptor, len(nodes))
	issues := make(
		[]WorkflowValidationIssue, 0)
	for _, node := range nodes {
		identity, err := NewPluginIdentity(
			node.PluginType(), node.PluginVersion())
		if err != nil {
			return newWorkflowValidationReport(nil), err
		}
		descriptor, found := registry.Lookup(identity)
		if !found {
			issues = append(
				issues, unresolvedPluginIssue(registry,
					node, identity),
			)
			continue
		}
		resolvedDescriptors[node.ID()] = descriptor
		issues = append(issues,
			configurationValidationIssues(node, descriptor)...)
		incomingCount := uint(len(builtGraph.IncomingEdges(
			node.ID())),
		)
		outgoingCount := uint(
			len(builtGraph.OutgoingEdges(node.ID())))
		issues = append(issues,
			edgeConstraintIssues(node.ID(), descriptor.Identity(),
				"incoming", "incomingEdges", incomingCount,
				descriptor.InputEdgeConstraint(), IssueCodeInputEdgeCountBelowMinimum, IssueCodeInputEdgeCountAboveMaximum,
			)...)
		issues = append(issues, edgeConstraintIssues(
			node.ID(), descriptor.Identity(), "outgoing",
			"outgoingEdges", outgoingCount, descriptor.OutputEdgeConstraint(),
			IssueCodeOutputEdgeCountBelowMinimum, IssueCodeOutputEdgeCountAboveMaximum)...,
		)
		if normalizedMode == execution.ExecutionModeAsync &&
			!descriptor.Distribution().SupportsAsync() {
			issues = append(issues,
				WorkflowValidationIssue{Code: IssueCodeAsyncNodeNotDistributable, Message: "plugin does not support asynchronous execution",
					NodeID: node.ID(), PluginIdentity: descriptor.Identity(), Field: "distribution",
				})
		}
	}
	for _, edge := range builtGraph.Edges() {
		sourceDescriptor, sourceResolved := resolvedDescriptors[edge.SourceNodeID()]
		if sourceResolved && !sourceDescriptor.HasOutputPort(edge.SourceOutputPort()) {
			issues = append(issues,
				WorkflowValidationIssue{Code: IssueCodeUnknownOutputPort, Message: fmt.Sprintf(
					"output port %q is not declared by the plugin", edge.SourceOutputPort()),
					NodeID: edge.SourceNodeID(), EdgeID: edge.ID(), PluginIdentity: sourceDescriptor.Identity(),
					Field: "sourceOutputPort"})
		}
		targetDescriptor, targetResolved :=
			resolvedDescriptors[edge.TargetNodeID()]
		if targetResolved &&
			!targetDescriptor.HasInputPort(edge.TargetInputPort()) {
			issues = append(issues, WorkflowValidationIssue{
				Code: IssueCodeUnknownInputPort, Message: fmt.Sprintf("input port %q is not declared by the plugin",
					edge.TargetInputPort()), NodeID: edge.TargetNodeID(),
				EdgeID: edge.ID(), PluginIdentity: targetDescriptor.Identity(), Field: "targetInputPort",
			})
		}
	}
	return newWorkflowValidationReport(
		issues), nil
}
func unresolvedPluginIssue(registry Registry,
	node workflow.NodeDefinition, identity PluginIdentity) WorkflowValidationIssue {
	if registry.HasType(identity.Type()) {
		return WorkflowValidationIssue{Code: IssueCodePluginVersionNotFound,
			Message: fmt.Sprintf("plugin version %q is not registered for plugin type %q", identity.Version().String(),
				identity.Type().String()), NodeID: node.ID(),
			PluginIdentity: identity, Field: "pluginVersion"}
	}
	return WorkflowValidationIssue{
		Code: IssueCodePluginNotFound, Message: fmt.Sprintf("plugin type %q is not registered",
			identity.Type().String()), NodeID: node.ID(),
		PluginIdentity: identity, Field: "pluginType"}
}
func configurationValidationIssues(
	node workflow.NodeDefinition, descriptor Descriptor) []WorkflowValidationIssue {
	configurationReport := descriptor.ValidateConfiguration(node.Configuration())
	configurationIssues :=
		configurationReport.Issues()
	if len(configurationIssues) == 0 {
		return nil
	}
	issues := make([]WorkflowValidationIssue, 0,
		len(configurationIssues))
	for _, configurationIssue := range configurationIssues {
		issues = append(issues,
			WorkflowValidationIssue{Code: IssueCodeInvalidPluginConfiguration, Message: configurationIssue.Reason,
				NodeID: node.ID(), PluginIdentity: descriptor.Identity(), Field: configurationIssue.Field,
			})
	}
	return issues
}
func edgeConstraintIssues(nodeID workflow.NodeID,
	identity PluginIdentity, direction string, field string,
	actual uint, constraint EdgeConstraint, belowCode WorkflowValidationIssueCode,
	aboveCode WorkflowValidationIssueCode) []WorkflowValidationIssue {
	minimum := constraint.Minimum()
	maximum, hasMaximum := constraint.Maximum()
	if actual < minimum {
		return []WorkflowValidationIssue{{Code: belowCode,
			Message: fmt.Sprintf("%s edge count is below the plugin minimum", direction), NodeID: nodeID, PluginIdentity: identity,
			Field: field, Minimum: minimum, Maximum: maximum,
			Actual: actual, HasMaximum: hasMaximum},
		}
	}
	if hasMaximum && actual > maximum {
		return []WorkflowValidationIssue{{
			Code: aboveCode, Message: fmt.Sprintf("%s edge count is above the plugin maximum",
				direction), NodeID: nodeID,
			PluginIdentity: identity, Field: field, Minimum: minimum,
			Maximum: maximum, Actual: actual, HasMaximum: true,
		}}
	}
	return nil
}
