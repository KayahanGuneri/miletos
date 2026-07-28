package engine

import (
	"fmt"
	"miletos-go/internal/engine/graph"
	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/engine/runtime"
	"miletos-go/internal/features/workflow"
	"sort"
)

type validatedExecutionPlan struct {
	graph             graph.Graph
	topologicalOrder  []workflow.NodeID
	descriptorsByNode map[workflow.NodeID]plugin.Descriptor
}

func ValidateExecutionRequest(request ExecutionRequest, dependencies EngineDependencies,
) (PreflightValidationReport, error,
) {
	_, report, err := validateExecutionRequest(request,
		dependencies)
	return report, err
}
func validateExecutionRequest(request ExecutionRequest, dependencies EngineDependencies,
) (validatedExecutionPlan, PreflightValidationReport,
	error) {
	if !request.IsValid() {
		return validatedExecutionPlan{}, PreflightValidationReport{}, newValidationError(
			"request", "must be valid")
	}
	if !dependencies.IsValid() {
		return validatedExecutionPlan{}, PreflightValidationReport{}, newValidationError(
			"dependencies", "must be valid")
	}
	definition := request.Definition()
	builtGraph, structuralReport := graph.BuildValidated(
		definition)
	if !structuralReport.IsValid() {
		return validatedExecutionPlan{}, newPreflightValidationReport(
			structuralReport, plugin.WorkflowValidationReport{}, nil,
		), nil
	}
	topologicalOrder, available := builtGraph.TopologicalOrder()
	if !available {
		report := newPreflightValidationReport(
			structuralReport, plugin.WorkflowValidationReport{}, []PreflightValidationIssue{
				{Code: IssueCodeTopologicalOrderUnavailable, Message: "validated workflow graph does not provide a topological order",
					Field: "topologicalOrder"}},
		)
		return validatedExecutionPlan{},
			report, nil
	}
	pluginReport, err := plugin.ValidateWorkflowPlugins(
		definition, dependencies.PluginRegistry(), request.Mode(),
	)
	if err != nil {
		return validatedExecutionPlan{},
			PreflightValidationReport{}, fmt.Errorf("validate workflow plugins: %w",
				err)
	}
	if !pluginReport.IsValid() {
		return validatedExecutionPlan{},
			newPreflightValidationReport(structuralReport, pluginReport,
				nil), nil
	}
	nodes := builtGraph.Nodes()
	descriptorsByNode := make(map[workflow.NodeID]plugin.Descriptor,
		len(nodes))
	for _, node := range nodes {
		identity, err := plugin.NewPluginIdentity(node.PluginType(),
			node.PluginVersion())
		if err != nil {
			return validatedExecutionPlan{}, PreflightValidationReport{}, fmt.Errorf(
				"build plugin identity for node %s: %w", node.ID(), err,
			)
		}
		descriptor, found := dependencies.PluginRegistry().
			Lookup(identity)
		if !found {
			return validatedExecutionPlan{}, PreflightValidationReport{}, fmt.Errorf(
				"validated descriptor %s for node %s is unavailable", identity, node.ID(),
			)
		}
		descriptorsByNode[node.ID()] = descriptor
	}
	engineIssues := runtimeEdgeValidationIssues(builtGraph,
		descriptorsByNode, dependencies.RuntimeLimits())
	engineIssues = append(engineIssues,
		executorAvailabilityIssues(nodes, dependencies.ExecutorRegistry())...)
	report := newPreflightValidationReport(structuralReport, pluginReport,
		engineIssues)
	if !report.IsValid() {
		return validatedExecutionPlan{}, report,
			nil
	}
	return validatedExecutionPlan{graph: builtGraph, topologicalOrder: topologicalOrder,
		descriptorsByNode: cloneDescriptorsByNode(descriptorsByNode),
	}, report, nil
}
func runtimeEdgeValidationIssues(
	builtGraph graph.Graph, descriptorsByNode map[workflow.NodeID]plugin.Descriptor, limits runtime.RuntimeLimits,
) []PreflightValidationIssue {
	edges := builtGraph.Edges()
	issues := make([]PreflightValidationIssue, 0)
	for _, edge := range edges {
		targetNode, found := builtGraph.Node(edge.TargetNodeID())
		if !found {
			issues = append(issues,
				PreflightValidationIssue{Code: IssueCodeRuntimeEdgeInvalid, Message: "runtime edge target node is unavailable",
					NodeID: edge.TargetNodeID(), EdgeID: edge.ID(), Field: "targetNode",
				})
			continue
		}
		targetDescriptor, found := descriptorsByNode[edge.TargetNodeID()]
		if !found {
			issues = append(issues,
				PreflightValidationIssue{Code: IssueCodeRuntimeEdgeInvalid, Message: "runtime edge target descriptor is unavailable",
					NodeID: edge.TargetNodeID(), EdgeID: edge.ID(), Field: "targetDescriptor",
				})
			continue
		}
		_, err := runtime.NewEdgeRuntime(edge, targetNode,
			targetDescriptor, limits)
		if err == nil {
			continue
		}
		issues = append(issues,
			PreflightValidationIssue{Code: IssueCodeRuntimeEdgeInvalid, Message: err.Error(),
				NodeID: edge.TargetNodeID(), EdgeID: edge.ID(), PluginIdentity: targetDescriptor.Identity(),
				Field: "edgeRuntime"})
	}
	return issues
}
func executorAvailabilityIssues(
	nodes []workflow.NodeDefinition, registry runtime.ExecutorRegistry) []PreflightValidationIssue {
	issues := make([]PreflightValidationIssue, 0)
	for _, node := range nodes {
		identity, err := plugin.NewPluginIdentity(node.PluginType(), node.PluginVersion())
		if err != nil {
			issues = append(
				issues, PreflightValidationIssue{Code: IssueCodeExecutorNotFound,
					Message: "node executor identity is invalid", NodeID: node.ID(), Field: "executorIdentity",
				})
			continue
		}
		_, found := registry.Lookup(identity)
		if found {
			continue
		}
		issues = append(
			issues, PreflightValidationIssue{Code: IssueCodeExecutorNotFound,
				Message: fmt.Sprintf("executor %s is not registered", identity), NodeID: node.ID(), PluginIdentity: identity,
				Field: "executor"})
	}
	return issues
}
func cloneDescriptorsByNode(
	source map[workflow.NodeID]plugin.Descriptor) map[workflow.NodeID]plugin.Descriptor {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[workflow.NodeID]plugin.Descriptor, len(source))
	for nodeID, descriptor := range source {
		cloned[nodeID] = descriptor
	}
	return cloned
}

type PreflightValidationIssueCode string

const (
	IssueCodeTopologicalOrderUnavailable PreflightValidationIssueCode = "TOPOLOGICAL_ORDER_UNAVAILABLE"
	IssueCodeRuntimeEdgeInvalid          PreflightValidationIssueCode = "RUNTIME_EDGE_INVALID"
	IssueCodeExecutorNotFound            PreflightValidationIssueCode = "EXECUTOR_NOT_FOUND"
)

func (code PreflightValidationIssueCode) String() string {
	return string(code)
}

type PreflightValidationIssue struct {
	Code           PreflightValidationIssueCode
	Message        string
	NodeID         workflow.NodeID
	EdgeID         workflow.EdgeID
	PluginIdentity plugin.PluginIdentity
	Field          string
}
type PreflightValidationReport struct {
	structuralReport graph.ValidationReport
	pluginReport     plugin.WorkflowValidationReport
	engineIssues     []PreflightValidationIssue
}

func newPreflightValidationReport(
	structuralReport graph.ValidationReport, pluginReport plugin.WorkflowValidationReport, engineIssues []PreflightValidationIssue,
) PreflightValidationReport {
	normalizedEngineIssues := clonePreflightValidationIssues(
		engineIssues)
	sortPreflightValidationIssues(normalizedEngineIssues)
	return PreflightValidationReport{structuralReport: structuralReport,
		pluginReport: pluginReport, engineIssues: normalizedEngineIssues}
}
func (report PreflightValidationReport) IsValid() bool {
	return report.structuralReport.IsValid() &&
		report.pluginReport.IsValid() && len(report.engineIssues) == 0
}
func (report PreflightValidationReport) Len() int {
	return report.structuralReport.Len() +
		report.pluginReport.Len() + len(report.engineIssues)
}
func (report PreflightValidationReport) StructuralIssues() []graph.ValidationIssue {
	return report.structuralReport.Issues()
}
func (report PreflightValidationReport) PluginIssues() []plugin.WorkflowValidationIssue {
	return report.pluginReport.Issues()
}
func (report PreflightValidationReport) EngineIssues() []PreflightValidationIssue {
	return clonePreflightValidationIssues(report.engineIssues)
}
func (report PreflightValidationReport) HasEngineCode(code PreflightValidationIssueCode) bool {
	for _, issue := range report.engineIssues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
func clonePreflightValidationIssues(issues []PreflightValidationIssue) []PreflightValidationIssue {
	if len(issues) == 0 {
		return nil
	}
	cloned := make([]PreflightValidationIssue,
		len(issues))
	copy(cloned, issues)
	return cloned
}
func sortPreflightValidationIssues(
	issues []PreflightValidationIssue) {
	sort.SliceStable(
		issues, func(left int, right int) bool {
			leftIssue := issues[left]
			rightIssue := issues[right]
			if leftIssue.NodeID.String() !=
				rightIssue.NodeID.String() {
				return leftIssue.NodeID.String() < rightIssue.NodeID.String()
			}
			leftRank :=
				preflightValidationIssueRank(leftIssue.Code)
			rightRank := preflightValidationIssueRank(
				rightIssue.Code)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			if leftIssue.EdgeID.String() != rightIssue.EdgeID.String() {
				return leftIssue.EdgeID.String() < rightIssue.EdgeID.String()
			}
			if leftIssue.PluginIdentity.String() != rightIssue.PluginIdentity.String() {
				return leftIssue.PluginIdentity.String() < rightIssue.PluginIdentity.String()
			}
			if leftIssue.Field != rightIssue.Field {
				return leftIssue.Field <
					rightIssue.Field
			}
			return leftIssue.Message < rightIssue.Message
		},
	)
}
func preflightValidationIssueRank(code PreflightValidationIssueCode) int {
	switch code {
	case IssueCodeTopologicalOrderUnavailable:
		return 0
	case IssueCodeRuntimeEdgeInvalid:
		return 1
	case IssueCodeExecutorNotFound:
		return 2
	default:
		return 100
	}
}
