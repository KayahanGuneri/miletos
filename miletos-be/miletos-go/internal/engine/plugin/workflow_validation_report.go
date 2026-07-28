package plugin

import (
	"fmt"
	"sort"

	"miletos-go/internal/features/workflow"
)

type WorkflowValidationIssueCode string

const (
	IssueCodePluginNotFound              WorkflowValidationIssueCode = "PLUGIN_NOT_FOUND"
	IssueCodePluginVersionNotFound       WorkflowValidationIssueCode = "PLUGIN_VERSION_NOT_FOUND"
	IssueCodeInvalidPluginConfiguration  WorkflowValidationIssueCode = "INVALID_PLUGIN_CONFIGURATION"
	IssueCodeUnknownOutputPort           WorkflowValidationIssueCode = "UNKNOWN_OUTPUT_PORT"
	IssueCodeUnknownInputPort            WorkflowValidationIssueCode = "UNKNOWN_INPUT_PORT"
	IssueCodeInputEdgeCountBelowMinimum  WorkflowValidationIssueCode = "INPUT_EDGE_COUNT_BELOW_MINIMUM"
	IssueCodeInputEdgeCountAboveMaximum  WorkflowValidationIssueCode = "INPUT_EDGE_COUNT_ABOVE_MAXIMUM"
	IssueCodeOutputEdgeCountBelowMinimum WorkflowValidationIssueCode = "OUTPUT_EDGE_COUNT_BELOW_MINIMUM"
	IssueCodeOutputEdgeCountAboveMaximum WorkflowValidationIssueCode = "OUTPUT_EDGE_COUNT_ABOVE_MAXIMUM"
	IssueCodeAsyncNodeNotDistributable   WorkflowValidationIssueCode = "ASYNC_NODE_NOT_DISTRIBUTABLE"
)

func (code WorkflowValidationIssueCode) String() string {
	return string(code)
}

type WorkflowValidationIssue struct {
	Code           WorkflowValidationIssueCode
	Message        string
	NodeID         workflow.NodeID
	EdgeID         workflow.EdgeID
	PluginIdentity PluginIdentity
	Field          string
	Minimum        uint
	Maximum        uint
	Actual         uint
	HasMaximum     bool
}
type WorkflowValidationReport struct {
	issues []WorkflowValidationIssue
}
type StructuralValidationError struct{ IssueCount int }

func (e *StructuralValidationError) Error() string {
	if e == nil {
		return "workflow plugin validation requires a structurally valid graph"
	}
	if e.IssueCount <= 0 {
		return "workflow plugin validation requires a structurally valid graph"
	}
	return fmt.Sprintf("workflow plugin validation requires a structurally valid graph: %d structural issue(s) found",
		e.IssueCount)
}
func newWorkflowValidationReport(issues []WorkflowValidationIssue,
) WorkflowValidationReport {
	normalized := cloneWorkflowValidationIssues(issues)
	sortWorkflowValidationIssues(
		normalized)
	return WorkflowValidationReport{issues: normalized}
}
func (report WorkflowValidationReport) IsValid() bool {
	return len(report.issues) == 0
}
func (report WorkflowValidationReport) Len() int {
	return len(report.issues)
}
func (report WorkflowValidationReport) Issues() []WorkflowValidationIssue {
	return cloneWorkflowValidationIssues(report.issues)
}
func (report WorkflowValidationReport) HasCode(code WorkflowValidationIssueCode) bool {
	for _, issue := range report.issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
func cloneWorkflowValidationIssues(issues []WorkflowValidationIssue) []WorkflowValidationIssue {
	if len(issues) == 0 {
		return nil
	}
	cloned := make([]WorkflowValidationIssue,
		len(issues))
	copy(cloned, issues)
	return cloned
}

func sortWorkflowValidationIssues(
	issues []WorkflowValidationIssue) {
	sort.SliceStable(
		issues, func(leftIndex int,
			rightIndex int) bool {
			left := issues[leftIndex]
			right := issues[rightIndex]
			if left.NodeID != right.NodeID {
				return left.NodeID.String() < right.NodeID.String()
			}
			leftRank := workflowValidationIssueRank(left.Code)
			rightRank := workflowValidationIssueRank(right.Code)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			if left.Code != right.Code {
				return left.Code.String() < right.Code.String()
			}
			if left.EdgeID != right.EdgeID {
				return left.EdgeID.String() < right.EdgeID.String()
			}
			if left.PluginIdentity != right.PluginIdentity {
				return left.PluginIdentity.String() <
					right.PluginIdentity.String()
			}
			if left.Field != right.Field {
				return left.Field < right.Field
			}
			if left.Minimum != right.Minimum {
				return left.Minimum < right.Minimum
			}
			if left.HasMaximum != right.HasMaximum {
				return !left.HasMaximum && right.HasMaximum
			}
			if left.Maximum != right.Maximum {
				return left.Maximum < right.Maximum
			}
			if left.Actual != right.Actual {
				return left.Actual < right.Actual
			}
			return left.Message < right.Message
		})
}
func workflowValidationIssueRank(
	code WorkflowValidationIssueCode) int {
	switch code {
	case IssueCodePluginNotFound:
		return 0
	case IssueCodePluginVersionNotFound:
		return 1
	case IssueCodeInvalidPluginConfiguration:
		return 2
	case IssueCodeUnknownOutputPort:
		return 3
	case IssueCodeUnknownInputPort:
		return 4
	case IssueCodeInputEdgeCountBelowMinimum:
		return 5
	case IssueCodeInputEdgeCountAboveMaximum:
		return 6
	case IssueCodeOutputEdgeCountBelowMinimum:
		return 7
	case IssueCodeOutputEdgeCountAboveMaximum:
		return 8
	case IssueCodeAsyncNodeNotDistributable:
		return 9
	default:
		return 100
	}
}
