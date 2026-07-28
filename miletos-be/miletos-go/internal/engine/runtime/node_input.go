package runtime

import (
	"fmt"
	"sort"
	"strings"
)

type NodeInput struct {
	inputs PortPayloads
	ports  []string
}

func NewNodeInput(inputs PortPayloads,
) (NodeInput, error) {
	if len(inputs) == 0 {
		return NodeInput{
			inputs: PortPayloads{}}, nil
	}
	normalizedInputs := make(PortPayloads,
		len(inputs))
	ports := make([]string, 0,
		len(inputs))
	for port, payloads := range inputs {
		normalizedPort, err := normalizeRuntimePortName("nodeInput.port",
			port)
		if err != nil {
			return NodeInput{}, err
		}
		if _, exists := normalizedInputs[normalizedPort]; exists {
			return NodeInput{}, newValidationError("nodeInput",
				fmt.Sprintf("contains duplicate port %q after normalization", normalizedPort))
		}
		if len(payloads) == 0 {
			return NodeInput{}, newValidationError(
				"nodeInput.payloads", fmt.Sprintf("port %q must contain at least one payload",
					normalizedPort))
		}
		normalizedPayloads := make(
			[]Payload, len(payloads))
		for index, payload := range payloads {
			if err := payload.validate(); err != nil {
				return NodeInput{}, newValidationError(fmt.Sprintf("nodeInput.%s[%d]",
					normalizedPort, index),
					err.Error())
			}
			normalizedPayloads[index] = clonePayload(payload)
		}
		normalizedInputs[normalizedPort] = normalizedPayloads
		ports = append(ports, normalizedPort)
	}
	sort.Strings(ports)
	return NodeInput{
		inputs: normalizedInputs, ports: ports}, nil
}
func (input NodeInput) IsEmpty() bool {
	return len(input.inputs) == 0
}
func (input NodeInput) PortCount() int { return len(input.inputs) }
func (input NodeInput) Ports() []string {
	if len(input.ports) == 0 {
		return nil
	}
	return append([]string(nil), input.ports...,
	)
}
func (input NodeInput) Payloads(port string) ([]Payload, bool, error) {
	normalizedPort, err := normalizeRuntimePortName("nodeInput.port", port)
	if err != nil {
		return nil, false, err
	}
	payloads, exists := input.inputs[normalizedPort]
	if !exists {
		return nil, false, nil
	}
	return clonePayloadSlice(payloads), true, nil
}
func (input NodeInput) TotalPayloadCount() int {
	total := 0
	for _, payloads := range input.inputs {
		total += len(payloads)
	}
	return total
}
func (input NodeInput) IsValid() bool {
	return input.validate() == nil
}
func (input NodeInput) validate() error {
	if len(input.inputs) == 0 {
		if len(input.ports) != 0 {
			return newValidationError("nodeInput", "port order must be empty when inputs are empty")
		}
		return nil
	}
	if len(input.inputs) != len(input.ports) {
		return newValidationError("nodeInput",
			"input and port-order counts must match")
	}
	seenPorts := make(map[string]struct{},
		len(input.ports))
	previousPort := ""
	for index, port := range input.ports {
		normalizedPort, err := normalizeRuntimePortName(fmt.Sprintf("nodeInput.ports[%d]",
			index), port,
		)
		if err != nil {
			return err
		}
		if _, exists := seenPorts[normalizedPort]; exists {
			return newValidationError("nodeInput", fmt.Sprintf(
				"contains duplicate port %q", normalizedPort),
			)
		}
		if previousPort != "" && previousPort > normalizedPort {
			return newValidationError(
				"nodeInput", "ports must be stored in deterministic order")
		}
		payloads, exists := input.inputs[normalizedPort]
		if !exists {
			return newValidationError("nodeInput",
				fmt.Sprintf("port %q does not have a payload collection", normalizedPort))
		}
		if len(payloads) == 0 {
			return newValidationError(
				"nodeInput.payloads", fmt.Sprintf("port %q must contain at least one payload",
					normalizedPort))
		}
		for payloadIndex, payload := range payloads {
			if err := payload.validate(); err != nil {
				return newValidationError(fmt.Sprintf(
					"nodeInput.%s[%d]", normalizedPort, payloadIndex,
				), err.Error())
			}
		}
		seenPorts[normalizedPort] = struct{}{}
		previousPort = normalizedPort
	}
	return nil
}
func cloneNodeInput(input NodeInput,
) NodeInput {
	clonedInputs := make(PortPayloads,
		len(input.inputs))
	for port, payloads := range input.inputs {
		clonedInputs[port] = clonePayloadSlice(payloads)
	}
	return NodeInput{
		inputs: clonedInputs, ports: append([]string(nil),
			input.ports...)}
}
func clonePayloadSlice(
	payloads []Payload) []Payload {
	if len(payloads) == 0 {
		return nil
	}
	cloned := make([]Payload, len(payloads))
	for index, payload := range payloads {
		cloned[index] = clonePayload(payload)
	}
	return cloned
}
func normalizeRuntimePortName(field string, value string,
) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", newValidationError(field, "must not be empty")
	}
	return normalized, nil
}
