package plugin

import (
	"fmt"
	"sort"
	"strings"

	"miletos-go/internal/features/workflow"
)

type DescriptorConfig struct {
	Identity               PluginIdentity
	Metadata               PluginMetadata
	InputPorts             []Port
	OutputPorts            []Port
	InputEdgeConstraint    EdgeConstraint
	OutputEdgeConstraint   EdgeConstraint
	QueuePolicy            QueuePolicy
	CachePolicy            CachePolicy
	Distribution           DistributionCapability
	ConfigurationValidator ConfigurationValidator
}
type Descriptor struct {
	identity               PluginIdentity
	metadata               PluginMetadata
	inputPorts             []Port
	outputPorts            []Port
	inputPortNames         map[string]struct{}
	outputPortNames        map[string]struct{}
	inputEdgeConstraint    EdgeConstraint
	outputEdgeConstraint   EdgeConstraint
	queuePolicy            QueuePolicy
	cachePolicy            CachePolicy
	distribution           DistributionCapability
	configurationValidator ConfigurationValidator
}

func NewDescriptor(config DescriptorConfig) (Descriptor, error) {
	identity, err := NewPluginIdentity(
		config.Identity.Type(), config.Identity.Version())
	if err != nil {
		return Descriptor{}, newValidationError("identity",
			err.Error())
	}
	metadata, err := config.Metadata.normalized()
	if err != nil {
		return Descriptor{}, err
	}
	inputPorts, inputPortNames, err := normalizePorts("inputPorts", config.InputPorts)
	if err != nil {
		return Descriptor{}, err
	}
	outputPorts, outputPortNames, err := normalizePorts(
		"outputPorts", config.OutputPorts)
	if err != nil {
		return Descriptor{}, err
	}
	inputEdgeConstraint, err := config.InputEdgeConstraint.normalized("inputEdgeConstraint")
	if err != nil {
		return Descriptor{}, err
	}
	outputEdgeConstraint, err := config.OutputEdgeConstraint.normalized(
		"outputEdgeConstraint")
	if err != nil {
		return Descriptor{}, err
	}
	queuePolicy, err := config.QueuePolicy.normalized()
	if err != nil {
		return Descriptor{}, err
	}
	cachePolicy, err := config.CachePolicy.normalized()
	if err != nil {
		return Descriptor{}, err
	}
	distribution, err := ParseDistributionCapability(config.Distribution.String())
	if err != nil {
		return Descriptor{}, newValidationError(
			"distribution", "must be LOCAL_ONLY or DISTRIBUTABLE")
	}
	if config.ConfigurationValidator == nil {
		return Descriptor{}, newValidationError("configurationValidator", "must not be nil")
	}
	return Descriptor{identity: identity, metadata: metadata,
		inputPorts: inputPorts, outputPorts: outputPorts,
		inputPortNames: inputPortNames, outputPortNames: outputPortNames,
		inputEdgeConstraint: inputEdgeConstraint, outputEdgeConstraint: outputEdgeConstraint,
		queuePolicy: queuePolicy, cachePolicy: cachePolicy,
		distribution:           distribution,
		configurationValidator: config.ConfigurationValidator}, nil
}
func (descriptor Descriptor) Identity() PluginIdentity {
	return descriptor.identity
}
func (descriptor Descriptor) Metadata() PluginMetadata {
	return descriptor.metadata
}
func (descriptor Descriptor) InputPorts() []Port {
	return clonePorts(descriptor.inputPorts)
}
func (descriptor Descriptor) OutputPorts() []Port {
	return clonePorts(descriptor.outputPorts)
}
func (descriptor Descriptor) HasInputPort(name string) bool {
	normalizedName := strings.TrimSpace(name)
	_, exists := descriptor.inputPortNames[normalizedName]
	return exists
}
func (descriptor Descriptor) HasOutputPort(name string) bool {
	normalizedName := strings.TrimSpace(name)
	_, exists := descriptor.outputPortNames[normalizedName]
	return exists
}
func (descriptor Descriptor) InputEdgeConstraint() EdgeConstraint {
	return descriptor.inputEdgeConstraint
}
func (descriptor Descriptor) OutputEdgeConstraint() EdgeConstraint {
	return descriptor.outputEdgeConstraint
}
func (descriptor Descriptor) QueuePolicy() QueuePolicy { return descriptor.queuePolicy }
func (descriptor Descriptor) CachePolicy() CachePolicy {
	return descriptor.cachePolicy
}
func (descriptor Descriptor) Distribution() DistributionCapability {
	return descriptor.distribution
}
func (descriptor Descriptor) ValidateConfiguration(configuration workflow.JSONObject) ConfigurationReport {
	if descriptor.configurationValidator == nil {
		return newConfigurationReport([]ConfigurationIssue{
			{Field: "configuration", Reason: "configuration validator is not available"}})
	}
	issues := descriptor.configurationValidator(
		configuration)
	return newConfigurationReport(issues)
}
func (descriptor Descriptor) IsValid() bool {
	_, err := descriptor.normalized()
	return err == nil
}
func (descriptor Descriptor) clone() Descriptor {
	return Descriptor{identity: descriptor.identity, metadata: descriptor.metadata,
		inputPorts: clonePorts(descriptor.inputPorts), outputPorts: clonePorts(descriptor.outputPorts), inputPortNames: clonePortNameSet(
			descriptor.inputPortNames), outputPortNames: clonePortNameSet(
			descriptor.outputPortNames),
		inputEdgeConstraint: descriptor.inputEdgeConstraint, outputEdgeConstraint: descriptor.outputEdgeConstraint,
		queuePolicy: descriptor.queuePolicy, cachePolicy: descriptor.cachePolicy,
		distribution: descriptor.distribution, configurationValidator: descriptor.configurationValidator,
	}
}

func (descriptor Descriptor) normalized() (Descriptor, error,
) {
	return NewDescriptor(DescriptorConfig{
		Identity: descriptor.identity, Metadata: descriptor.metadata,
		InputPorts: descriptor.inputPorts, OutputPorts: descriptor.outputPorts,
		InputEdgeConstraint: descriptor.inputEdgeConstraint, OutputEdgeConstraint: descriptor.outputEdgeConstraint,
		QueuePolicy: descriptor.queuePolicy, CachePolicy: descriptor.cachePolicy,
		Distribution: descriptor.distribution, ConfigurationValidator: descriptor.configurationValidator,
	})
}
func normalizePorts(field string,
	ports []Port) ([]Port,
	map[string]struct{}, error) {
	normalized := make([]Port, 0,
		len(ports))
	names := make(map[string]struct{}, len(ports))
	for index, port := range ports {
		normalizedPort, err := NewPort(port.Name(), port.DisplayName(),
			port.Description())
		if err != nil {
			return nil, nil, newValidationError(fmt.Sprintf("%s[%d]",
				field, index),
				err.Error())
		}
		if _, exists := names[normalizedPort.Name()]; exists {
			return nil, nil, newValidationError(
				fmt.Sprintf("%s[%d].name", field,
					index), "must be unique within its port direction",
			)
		}
		names[normalizedPort.Name()] = struct{}{}
		normalized = append(
			normalized, normalizedPort)
	}
	sort.SliceStable(
		normalized, func(left int,
			right int) bool {
			if normalized[left].Name() !=
				normalized[right].Name() {
				return normalized[left].Name() < normalized[right].Name()
			}
			if normalized[left].DisplayName() !=
				normalized[right].DisplayName() {
				return normalized[left].DisplayName() < normalized[right].DisplayName()
			}
			return normalized[left].Description() <
				normalized[right].Description()
		})
	return normalized, names, nil
}
func clonePortNameSet(source map[string]struct{},
) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	cloned := make(
		map[string]struct{}, len(source))
	for name := range source {
		cloned[name] = struct{}{}
	}
	return cloned
}
