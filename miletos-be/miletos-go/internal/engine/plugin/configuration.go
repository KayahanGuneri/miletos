package plugin

import (
	"sort"
	"strings"

	"miletos-go/internal/features/workflow"
)

type ConfigurationIssue struct {
	Field  string
	Reason string
}
type ConfigurationReport struct {
	issues []ConfigurationIssue
}
type ConfigurationValidator func(
	configuration workflow.JSONObject) []ConfigurationIssue

func NewConfigurationIssue(field string, reason string,
) (ConfigurationIssue, error) {
	normalizedField, err := normalizeRequiredString("configurationIssue.field",
		field)
	if err != nil {
		return ConfigurationIssue{}, err
	}
	normalizedReason, err := normalizeRequiredString("configurationIssue.reason", reason)
	if err != nil {
		return ConfigurationIssue{}, err
	}
	return ConfigurationIssue{
		Field: normalizedField, Reason: normalizedReason}, nil
}
func ValidConfigurationReport() ConfigurationReport {
	return ConfigurationReport{}
}
func NewConfigurationReport(issues ...ConfigurationIssue) ConfigurationReport {
	return newConfigurationReport(issues)
}
func (report ConfigurationReport) IsValid() bool { return len(report.issues) == 0 }
func (report ConfigurationReport) Len() int {
	return len(report.issues)
}
func (report ConfigurationReport) Issues() []ConfigurationIssue {
	return cloneConfigurationIssues(report.issues)
}
func (report ConfigurationReport) HasField(
	field string) bool {
	normalizedField := strings.TrimSpace(field)
	for _, issue := range report.issues {
		if issue.Field == normalizedField {
			return true
		}
	}
	return false
}
func (report ConfigurationReport) clone() ConfigurationReport {
	return ConfigurationReport{
		issues: cloneConfigurationIssues(report.issues),
	}
}
func newConfigurationReport(issues []ConfigurationIssue) ConfigurationReport {
	normalized := make([]ConfigurationIssue, 0,
		len(issues))
	for _, issue := range issues {
		field := strings.TrimSpace(issue.Field)
		reason := strings.TrimSpace(issue.Reason)
		if field == "" {
			field = "configuration"
		}
		if reason == "" {
			reason = "validation failed"
		}
		normalized = append(normalized,
			ConfigurationIssue{Field: field, Reason: reason})
	}
	sort.SliceStable(normalized,
		func(left int, right int,
		) bool {
			if normalized[left].Field != normalized[right].Field {
				return normalized[left].Field < normalized[right].Field
			}
			return normalized[left].Reason < normalized[right].Reason
		})
	return ConfigurationReport{issues: cloneConfigurationIssues(normalized)}
}
func cloneConfigurationIssues(issues []ConfigurationIssue,
) []ConfigurationIssue {
	if len(issues) == 0 {
		return nil
	}
	cloned := make(
		[]ConfigurationIssue, len(issues))
	copy(cloned, issues)
	return cloned
}
