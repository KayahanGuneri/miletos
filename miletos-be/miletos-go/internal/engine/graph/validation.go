package graph

import (
	"sort"

	"miletos-go/internal/features/workflow"
)

type ValidationIssueCode string

const (
	IssueCodeEmptyWorkflow     ValidationIssueCode = "EMPTY_WORKFLOW"
	IssueCodeDuplicateNodeID   ValidationIssueCode = "DUPLICATE_NODE_ID"
	IssueCodeDuplicateEdgeID   ValidationIssueCode = "DUPLICATE_EDGE_ID"
	IssueCodeMissingSourceNode ValidationIssueCode = "MISSING_SOURCE_NODE"
	IssueCodeMissingTargetNode ValidationIssueCode = "MISSING_TARGET_NODE"
	IssueCodeSelfLoopDetected  ValidationIssueCode = "SELF_LOOP_DETECTED"
	IssueCodeUnreachableNode   ValidationIssueCode = "UNREACHABLE_NODE"
	IssueCodeCycleDetected     ValidationIssueCode = "CYCLE_DETECTED"
)

func (code ValidationIssueCode) String() string {
	return string(code)
}

type ValidationIssue struct {
	Code    ValidationIssueCode
	Message string
	NodeID  workflow.NodeID
	EdgeID  workflow.EdgeID
	Path    []workflow.NodeID
}
type ValidationReport struct {
	issues []ValidationIssue
}

func BuildValidated(definition workflow.WorkflowDefinition) (Graph, ValidationReport) {
	built := Build(definition)
	return built, Validate(built)
}
func Validate(
	built Graph) ValidationReport {
	issues := make(
		[]ValidationIssue, 0)
	if len(built.nodeDefinitions) == 0 {
		issues = append(
			issues, ValidationIssue{Code: IssueCodeEmptyWorkflow,
				Message: "workflow must contain at least one node"})
	}
	issues = append(
		issues, duplicateNodeIssues(built.nodeDefinitions)...)
	issues = append(issues, duplicateEdgeIssues(
		built.edgeDefinitions)...)
	issues = append(issues,
		edgeStructureIssues(built)...)
	for _, nodeID := range unreachableNodeIDs(built) {
		issues = append(issues,
			ValidationIssue{Code: IssueCodeUnreachableNode, Message: "node is not reachable from any root node",
				NodeID: nodeID})
	}
	if cyclePath, found := built.CyclePath(); found {
		issues = append(issues, ValidationIssue{
			Code: IssueCodeCycleDetected, Message: "workflow contains a directed cycle", NodeID: cyclePath[0],
			Path: cyclePath})
	}
	return newValidationReport(issues)
}

func duplicateNodeIssues(
	nodes []workflow.NodeDefinition) []ValidationIssue {
	counts := make(
		map[workflow.NodeID]int, len(nodes))
	for _, node := range nodes {
		counts[node.ID()]++
	}
	duplicateIDs := make(
		[]workflow.NodeID, 0)
	for nodeID, count := range counts {
		if count > 1 {
			duplicateIDs = append(duplicateIDs, nodeID)
		}
	}
	sort.Slice(duplicateIDs,
		func(left int, right int,
		) bool {
			return duplicateIDs[left].String() < duplicateIDs[right].String()
		})
	issues := make([]ValidationIssue, 0,
		len(duplicateIDs))
	for _, nodeID := range duplicateIDs {
		issues = append(issues,
			ValidationIssue{Code: IssueCodeDuplicateNodeID, Message: "node ID is used more than once",
				NodeID: nodeID})
	}
	return issues
}
func duplicateEdgeIssues(
	edges []workflow.EdgeDefinition) []ValidationIssue {
	counts := make(
		map[workflow.EdgeID]int, len(edges))
	for _, edge := range edges {
		counts[edge.ID()]++
	}
	duplicateIDs := make(
		[]workflow.EdgeID, 0)
	for edgeID, count := range counts {
		if count > 1 {
			duplicateIDs = append(duplicateIDs, edgeID)
		}
	}
	sort.Slice(duplicateIDs,
		func(left int, right int,
		) bool {
			return duplicateIDs[left].String() < duplicateIDs[right].String()
		})
	issues := make([]ValidationIssue, 0,
		len(duplicateIDs))
	for _, edgeID := range duplicateIDs {
		issues = append(issues,
			ValidationIssue{Code: IssueCodeDuplicateEdgeID, Message: "edge ID is used more than once",
				EdgeID: edgeID})
	}
	return issues
}
func edgeStructureIssues(
	built Graph) []ValidationIssue {
	issues := make(
		[]ValidationIssue, 0)
	for _, edge := range built.edgeDefinitions {
		sourceNodeID := edge.SourceNodeID()
		targetNodeID := edge.TargetNodeID()
		_, sourceExists := built.nodesByID[sourceNodeID]
		_, targetExists := built.nodesByID[targetNodeID]
		if !sourceExists {
			issues = append(issues, ValidationIssue{
				Code: IssueCodeMissingSourceNode, Message: "edge source node does not exist", NodeID: sourceNodeID,
				EdgeID: edge.ID()})
		}
		if !targetExists {
			issues = append(issues, ValidationIssue{
				Code: IssueCodeMissingTargetNode, Message: "edge target node does not exist", NodeID: targetNodeID,
				EdgeID: edge.ID()})
		}
		if sourceExists &&
			targetExists && sourceNodeID == targetNodeID {
			issues = append(
				issues, ValidationIssue{Code: IssueCodeSelfLoopDetected,
					Message: "edge source and target must be different nodes", NodeID: sourceNodeID, EdgeID: edge.ID(),
				})
		}
	}
	return issues
}

func newValidationReport(
	issues []ValidationIssue) ValidationReport {
	sortValidationIssues(issues)
	return ValidationReport{issues: cloneValidationIssues(issues)}
}
func (report ValidationReport) IsValid() bool { return len(report.issues) == 0 }
func (report ValidationReport) Len() int {
	return len(report.issues)
}
func (report ValidationReport) Issues() []ValidationIssue {
	return cloneValidationIssues(report.issues)
}
func (report ValidationReport) HasCode(code ValidationIssueCode) bool {
	for _, issue := range report.issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
func cloneValidationIssues(issues []ValidationIssue) []ValidationIssue {
	if len(issues) == 0 {
		return nil
	}
	cloned := make([]ValidationIssue,
		len(issues))
	for index, issue := range issues {
		cloned[index] = issue
		cloned[index].Path = cloneNodeIDs(
			issue.Path)
	}
	return cloned
}

func sortValidationIssues(issues []ValidationIssue,
) {
	sort.SliceStable(issues,
		func(leftIndex int, rightIndex int,
		) bool {
			left := issues[leftIndex]
			right := issues[rightIndex]
			leftRank := validationIssueRank(left.Code)
			rightRank := validationIssueRank(right.Code)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			if left.Code != right.Code {
				return left.Code.String() < right.Code.String()
			}
			if left.NodeID != right.NodeID {
				return left.NodeID.String() <
					right.NodeID.String()
			}
			if left.EdgeID != right.EdgeID {
				return left.EdgeID.String() < right.EdgeID.String()
			}
			pathComparison := compareNodeIDPaths(
				left.Path, right.Path)
			if pathComparison != 0 {
				return pathComparison < 0
			}
			return left.Message < right.Message
		},
	)
}
func validationIssueRank(code ValidationIssueCode) int {
	switch code {
	case IssueCodeEmptyWorkflow:
		return 0
	case IssueCodeDuplicateNodeID:
		return 1
	case IssueCodeDuplicateEdgeID:
		return 2
	case IssueCodeMissingSourceNode:
		return 3
	case IssueCodeMissingTargetNode:
		return 4
	case IssueCodeSelfLoopDetected:
		return 5
	case IssueCodeUnreachableNode:
		return 6
	case IssueCodeCycleDetected:
		return 7
	default:
		return 100
	}
}
func compareNodeIDPaths(left []workflow.NodeID, right []workflow.NodeID,
) int {
	shorterLength := len(left)
	if len(right) < shorterLength {
		shorterLength = len(right)
	}
	for index := 0; index < shorterLength; index++ {
		leftValue := left[index].String()
		rightValue := right[index].String()
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	switch {
	case len(left) < len(right):
		return -1
	case len(left) > len(right):
		return 1
	default:
		return 0
	}
}
