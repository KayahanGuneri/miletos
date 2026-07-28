package runtime

import (
	"fmt"
	"sort"
	"strings"
)

type NodeResultStatus string

const (
	NodeResultStatusSucceeded NodeResultStatus = "SUCCEEDED"
	NodeResultStatusFailed    NodeResultStatus = "FAILED"
)

func (status NodeResultStatus) String() string {
	return string(status)
}
func (status NodeResultStatus) IsValid() bool {
	switch status {
	case NodeResultStatusSucceeded, NodeResultStatusFailed:
		return true
	default:
		return false
	}
}

type NodeResult struct {
	status         NodeResultStatus
	outputs        PortPayloads
	outputPorts    []string
	terminalOutput *Payload
	contextChanges ContextChanges
	failure        *RuntimeFailure
}

func NewNodeSuccessResult(outputs PortPayloads, contextChanges ContextChanges,
) (NodeResult, error) {
	normalizedOutputs, outputPorts, err := normalizeNodeResultOutputs(outputs)
	if err != nil {
		return NodeResult{}, err
	}
	if err := contextChanges.validate(); err != nil {
		return NodeResult{}, newValidationError(
			"contextChanges", err.Error())
	}
	return NodeResult{
		status: NodeResultStatusSucceeded, outputs: normalizedOutputs,
		outputPorts: outputPorts, contextChanges: cloneContextChanges(
			contextChanges)}, nil
}
func NewTerminalNodeSuccessResult(
	terminalOutput Payload, contextChanges ContextChanges) (NodeResult, error) {
	if err := terminalOutput.validate(); err != nil {
		return NodeResult{}, newValidationError("terminalOutput",
			err.Error())
	}
	if err := contextChanges.validate(); err != nil {
		return NodeResult{}, newValidationError(
			"contextChanges", err.Error())
	}
	clonedTerminalOutput := clonePayload(
		terminalOutput)
	return NodeResult{status: NodeResultStatusSucceeded,
		outputs: PortPayloads{}, terminalOutput: &clonedTerminalOutput,
		contextChanges: cloneContextChanges(contextChanges),
	}, nil
}
func NewNodeFailureResult(failure RuntimeFailure) (NodeResult, error) {
	if err := failure.validate(); err != nil {
		return NodeResult{}, newValidationError("failure",
			err.Error())
	}
	clonedFailure := cloneRuntimeFailure(failure)
	return NodeResult{
		status: NodeResultStatusFailed, outputs: PortPayloads{},
		failure: &clonedFailure}, nil
}
func (result NodeResult) Status() NodeResultStatus {
	return result.status
}
func (result NodeResult) IsSuccess() bool {
	return result.status == NodeResultStatusSucceeded
}
func (result NodeResult) IsFailure() bool {
	return result.status == NodeResultStatusFailed
}
func (result NodeResult) HasRoutedOutputs() bool {
	return len(result.outputs) > 0
}
func (result NodeResult) HasTerminalOutput() bool {
	return result.terminalOutput != nil
}
func (result NodeResult) OutputPorts() []string {
	if len(result.outputPorts) == 0 {
		return nil
	}
	return append(
		[]string(nil), result.outputPorts...)
}
func (result NodeResult) OutputPayloads(
	port string) ([]Payload, bool, error) {
	normalizedPort, err := normalizeRuntimePortName(
		"nodeResult.outputPort", port)
	if err != nil {
		return nil, false, err
	}
	payloads, exists := result.outputs[normalizedPort]
	if !exists {
		return nil, false, nil
	}
	return clonePayloadSlice(
		payloads), true, nil
}
func (result NodeResult) OutputsSnapshot() PortPayloads {
	if len(result.outputs) == 0 {
		return nil
	}
	snapshot := make(PortPayloads, len(result.outputs))
	for port, payloads := range result.outputs {
		snapshot[port] = clonePayloadSlice(payloads)
	}
	return snapshot
}
func (result NodeResult) TotalOutputPayloadCount() int {
	total := 0
	for _, payloads := range result.outputs {
		total += len(payloads)
	}
	return total
}
func (result NodeResult) TerminalOutput() (Payload, bool) {
	if result.terminalOutput == nil {
		return Payload{}, false
	}
	return clonePayload(
		*result.terminalOutput), true
}
func (result NodeResult) ContextChanges() ContextChanges {
	return cloneContextChanges(
		result.contextChanges)
}
func (result NodeResult) Failure() (RuntimeFailure,
	bool) {
	if result.failure == nil {
		return RuntimeFailure{}, false
	}
	return cloneRuntimeFailure(*result.failure), true
}
func (result NodeResult) IsValid() bool {
	return result.validate() == nil
}
func (result NodeResult) validate() error {
	if !result.status.IsValid() {
		return newValidationError(
			"nodeResult.status", "must contain a supported result status")
	}
	switch result.status {
	case NodeResultStatusSucceeded:
		return result.validateSuccessfulResult()
	case NodeResultStatusFailed:
		return result.validateFailedResult()
	default:
		return newValidationError("nodeResult.status",
			"must contain a supported result status")
	}
}
func (result NodeResult) validateSuccessfulResult() error {
	if result.failure != nil {
		return newValidationError("nodeResult.failure",
			"must not exist for a successful result")
	}
	if err := validateStoredNodeResultOutputs(result.outputs,
		result.outputPorts); err != nil {
		return err
	}
	if result.terminalOutput != nil {
		if err := result.terminalOutput.validate(); err != nil {
			return newValidationError("nodeResult.terminalOutput",
				err.Error())
		}
	}
	if len(result.outputs) > 0 &&
		result.terminalOutput != nil {
		return newValidationError("nodeResult",
			"cannot contain routed outputs and terminal output together")
	}
	if err := result.contextChanges.validate(); err != nil {
		return newValidationError(
			"nodeResult.contextChanges", err.Error())
	}
	return nil
}
func (result NodeResult) validateFailedResult() error {
	if result.failure == nil {
		return newValidationError("nodeResult.failure",
			"must exist for a failed result")
	}
	if err := result.failure.validate(); err != nil {
		return newValidationError(
			"nodeResult.failure", err.Error())
	}
	if len(result.outputs) != 0 ||
		len(result.outputPorts) != 0 {
		return newValidationError("nodeResult.outputs",
			"must not exist for a failed result")
	}
	if result.terminalOutput != nil {
		return newValidationError(
			"nodeResult.terminalOutput", "must not exist for a failed result")
	}
	if !result.contextChanges.IsEmpty() {
		return newValidationError("nodeResult.contextChanges", "must be empty for a failed result")
	}
	return nil
}
func cloneNodeResult(result NodeResult) NodeResult {
	cloned := NodeResult{status: result.status,
		outputs: make(PortPayloads, len(result.outputs)), outputPorts: append([]string(nil),
			result.outputPorts...),
		contextChanges: cloneContextChanges(result.contextChanges),
	}
	for port, payloads := range result.outputs {
		cloned.outputs[port] = clonePayloadSlice(payloads)
	}
	if result.terminalOutput != nil {
		terminalOutput := clonePayload(*result.terminalOutput)
		cloned.terminalOutput = &terminalOutput
	}
	if result.failure != nil {
		failure := cloneRuntimeFailure(*result.failure)
		cloned.failure = &failure
	}
	return cloned
}
func normalizeNodeResultOutputs(outputs PortPayloads,
) (PortPayloads, []string,
	error) {
	if len(outputs) == 0 {
		return map[string][]Payload{}, nil, nil
	}
	normalizedOutputs := make(
		PortPayloads, len(outputs))
	outputPorts := make([]string,
		0, len(outputs))
	for port, payloads := range outputs {
		normalizedPort, err :=
			normalizeRuntimePortName("nodeResult.outputPort", port)
		if err != nil {
			return nil, nil, err
		}
		if _, exists :=
			normalizedOutputs[normalizedPort]; exists {
			return nil, nil, newValidationError("nodeResult.outputs",
				fmt.Sprintf("contains duplicate port %q after normalization", normalizedPort))
		}
		if len(payloads) == 0 {
			return nil, nil, newValidationError(
				"nodeResult.outputs", fmt.Sprintf("port %q must contain at least one payload",
					normalizedPort))
		}
		normalizedPayloads := make(
			[]Payload, len(payloads))
		for index, payload := range payloads {
			if err := payload.validate(); err != nil {
				return nil, nil, newValidationError(fmt.Sprintf("nodeResult.%s[%d]",
					normalizedPort, index),
					err.Error())
			}
			normalizedPayloads[index] = clonePayload(payload)
		}
		normalizedOutputs[normalizedPort] =
			normalizedPayloads
		outputPorts = append(
			outputPorts, normalizedPort)
	}
	sort.Strings(outputPorts)
	return normalizedOutputs, outputPorts,
		nil
}
func validateStoredNodeResultOutputs(outputs PortPayloads, outputPorts []string,
) error {
	if len(outputs) == 0 {
		if len(outputPorts) != 0 {
			return newValidationError("nodeResult.outputs", "output-port order must be empty when outputs are empty")
		}
		return nil
	}
	if len(outputs) != len(outputPorts) {
		return newValidationError("nodeResult.outputs",
			"output and port-order counts must match")
	}
	seenPorts := make(map[string]struct{},
		len(outputPorts))
	previousPort := ""
	for index, port := range outputPorts {
		normalizedPort, err := normalizeRuntimePortName(fmt.Sprintf(
			"nodeResult.outputPorts[%d]", index),
			port)
		if err != nil {
			return err
		}
		if normalizedPort != port {
			return newValidationError("nodeResult.outputPorts",
				"ports must be stored in normalized form")
		}
		if _, exists := seenPorts[port]; exists {
			return newValidationError(
				"nodeResult.outputPorts", fmt.Sprintf("contains duplicate port %q",
					port))
		}
		if previousPort != "" &&
			previousPort > port {
			return newValidationError("nodeResult.outputPorts",
				"ports must be stored in deterministic order")
		}
		payloads, exists := outputs[port]
		if !exists {
			return newValidationError("nodeResult.outputs", fmt.Sprintf(
				"port %q does not have an output collection", port),
			)
		}
		if len(payloads) == 0 {
			return newValidationError("nodeResult.outputs",
				fmt.Sprintf("port %q must contain at least one payload", port))
		}
		for payloadIndex, payload := range payloads {
			if err := payload.validate(); err != nil {
				return newValidationError(fmt.Sprintf("nodeResult.%s[%d]",
					port, payloadIndex),
					err.Error())
			}
		}
		seenPorts[port] = struct{}{}
		previousPort = port
	}
	for port := range outputs {
		normalizedPort, err := normalizeRuntimePortName(
			"nodeResult.outputPort", port)
		if err != nil {
			return err
		}
		if normalizedPort != port {
			return newValidationError(
				"nodeResult.outputs", "ports must be stored in normalized form")
		}
		if _, exists := seenPorts[port]; !exists {
			return newValidationError("nodeResult.outputs", fmt.Sprintf(
				"port %q is missing from output-port order", port),
			)
		}
	}
	return nil
}

type FailureCategory string

const (
	FailureCategoryValidation FailureCategory = "VALIDATION"
	FailureCategoryExecution  FailureCategory = "EXECUTION"
	FailureCategoryDependency FailureCategory = "DEPENDENCY"
	FailureCategoryTimeout    FailureCategory = "TIMEOUT"
	FailureCategoryCanceled   FailureCategory = "CANCELED"
	FailureCategoryInternal   FailureCategory = "INTERNAL"
)

func (category FailureCategory) String() string {
	return string(category)
}
func (category FailureCategory) IsValid() bool {
	switch category {
	case FailureCategoryValidation, FailureCategoryExecution,
		FailureCategoryDependency, FailureCategoryTimeout, FailureCategoryCanceled,
		FailureCategoryInternal:
		return true
	default:
		return false
	}
}

type RuntimeFailure struct {
	category  FailureCategory
	code      string
	message   string
	retryable bool
	details   map[string]string
}

func NewRuntimeFailure(category FailureCategory,
	code string, message string, retryable bool,
	details map[string]string) (RuntimeFailure, error) {
	normalizedCategory, err := normalizeFailureCategory(
		"failure.category", category)
	if err != nil {
		return RuntimeFailure{}, err
	}
	normalizedCode, err := normalizeRequiredString("failure.code",
		code)
	if err != nil {
		return RuntimeFailure{}, err
	}
	normalizedMessage, err := normalizeRequiredString("failure.message", message)
	if err != nil {
		return RuntimeFailure{}, err
	}
	normalizedDetails, err := normalizeMetadata(
		"failure.details", details)
	if err != nil {
		return RuntimeFailure{}, err
	}
	return RuntimeFailure{category: normalizedCategory,
		code: normalizedCode, message: normalizedMessage, retryable: retryable,
		details: normalizedDetails}, nil
}
func (failure RuntimeFailure) Category() FailureCategory {
	return failure.category
}
func (failure RuntimeFailure) Code() string {
	return failure.code
}
func (failure RuntimeFailure) Message() string { return failure.message }
func (failure RuntimeFailure) Retryable() bool {
	return failure.retryable
}
func (failure RuntimeFailure) Details() map[string]string {
	return cloneStringMap(failure.details)
}
func (failure RuntimeFailure) IsValid() bool {
	return failure.validate() == nil
}
func (failure RuntimeFailure) validate() error {
	_, err := NewRuntimeFailure(failure.category,
		failure.code, failure.message, failure.retryable,
		failure.details)
	return err
}
func cloneRuntimeFailure(failure RuntimeFailure) RuntimeFailure {
	return RuntimeFailure{category: failure.category, code: failure.code,
		message: failure.message, retryable: failure.retryable, details: cloneStringMap(
			failure.details)}
}
func normalizeFailureCategory(
	field string, category FailureCategory) (FailureCategory, error) {
	normalized := FailureCategory(strings.ToUpper(strings.TrimSpace(
		category.String())),
	)
	if !normalized.IsValid() {
		return "", newValidationError(field, "must contain a supported failure category")
	}
	return normalized, nil
}
