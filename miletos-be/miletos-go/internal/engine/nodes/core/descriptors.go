package core

import (
	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/features/workflow"
)

const (
	StaticInputPluginType workflow.PluginType    = "core.static-input"
	PassThroughPluginType workflow.PluginType    = "core.pass-through"
	DelayPluginType       workflow.PluginType    = "core.delay"
	TerminalPluginType    workflow.PluginType    = "core.terminal"
	CorePluginVersion     workflow.PluginVersion = "v1"
	InputPortName                                = "input"
	OutputPortName                               = "output"
	DefaultQueueCapacity  uint                   = 64
	DefaultCacheCapacity  uint                   = 1
)

func CoreDescriptors() ([]plugin.Descriptor, error) {
	builders := []func() (plugin.Descriptor, error){DelayDescriptor,
		PassThroughDescriptor, StaticInputDescriptor, TerminalDescriptor,
	}
	descriptors := make(
		[]plugin.Descriptor, 0, len(builders),
	)
	for _, build := range builders {
		descriptor, err := build()
		if err != nil {
			return nil, err
		}
		descriptors = append(
			descriptors, descriptor)
	}
	return descriptors, nil
}
func StaticInputDescriptor() (
	plugin.Descriptor, error) {
	outputPort, err := newOutputPort()
	if err != nil {
		return plugin.Descriptor{}, err
	}
	return newCoreDescriptor(
		StaticInputPluginType, "Static Input", "Provides configured static content to a workflow",
		nil, []plugin.Port{outputPort}, plugin.NewExactEdgeConstraint(0), plugin.NewUnlimitedEdgeConstraint(1),
		validateStaticInputConfiguration)
}
func PassThroughDescriptor() (plugin.Descriptor,
	error) {
	inputPort, err := newInputPort()
	if err != nil {
		return plugin.Descriptor{}, err
	}
	outputPort, err := newOutputPort()
	if err != nil {
		return plugin.Descriptor{}, err
	}
	return newCoreDescriptor(PassThroughPluginType, "Pass Through",
		"Forwards one incoming payload without changing it", []plugin.Port{inputPort}, []plugin.Port{outputPort}, plugin.NewExactEdgeConstraint(1), plugin.NewUnlimitedEdgeConstraint(0),
		validatePassThroughConfiguration)
}
func DelayDescriptor() (plugin.Descriptor,
	error) {
	inputPort, err := newInputPort()
	if err != nil {
		return plugin.Descriptor{}, err
	}
	outputPort, err := newOutputPort()
	if err != nil {
		return plugin.Descriptor{}, err
	}
	return newCoreDescriptor(DelayPluginType, "Delay",
		"Delays forwarding one incoming payload", []plugin.Port{inputPort}, []plugin.Port{outputPort}, plugin.NewExactEdgeConstraint(1), plugin.NewUnlimitedEdgeConstraint(0),
		validateDelayConfiguration)
}
func TerminalDescriptor() (plugin.Descriptor,
	error) {
	inputPort, err := newInputPort()
	if err != nil {
		return plugin.Descriptor{}, err
	}
	return newCoreDescriptor(TerminalPluginType,
		"Terminal", "Receives final workflow output without producing downstream output", []plugin.Port{
			inputPort}, nil,
		plugin.NewUnlimitedEdgeConstraint(1), plugin.NewExactEdgeConstraint(0), validateTerminalConfiguration,
	)
}
func newCoreDescriptor(pluginType workflow.PluginType, displayName string,
	description string, inputPorts []plugin.Port, outputPorts []plugin.Port,
	inputEdgeConstraint plugin.EdgeConstraint, outputEdgeConstraint plugin.EdgeConstraint, configurationValidator plugin.ConfigurationValidator,
) (plugin.Descriptor, error,
) {
	identity, err := plugin.NewPluginIdentity(pluginType,
		CorePluginVersion)
	if err != nil {
		return plugin.Descriptor{}, err
	}
	metadata, err := plugin.NewPluginMetadata(displayName, description)
	if err != nil {
		return plugin.Descriptor{}, err
	}
	queuePolicy, err := plugin.NewQueuePolicy(
		DefaultQueueCapacity, plugin.OverflowStrategyDropOldest)
	if err != nil {
		return plugin.Descriptor{}, err
	}
	cachePolicy, err := plugin.NewCachePolicy(DefaultCacheCapacity,
		plugin.OverflowStrategyDropOldest)
	if err != nil {
		return plugin.Descriptor{}, err
	}
	return plugin.NewDescriptor(plugin.DescriptorConfig{Identity: identity,
		Metadata: metadata, InputPorts: inputPorts,
		OutputPorts: outputPorts, InputEdgeConstraint: inputEdgeConstraint,
		OutputEdgeConstraint: outputEdgeConstraint, QueuePolicy: queuePolicy,
		CachePolicy: cachePolicy, Distribution: plugin.DistributionDistributable,
		ConfigurationValidator: configurationValidator},
	)
}
func newInputPort() (plugin.Port, error) {
	return plugin.NewPort(InputPortName,
		"Input", "Receives an incoming payload")
}
func newOutputPort() (plugin.Port, error) {
	return plugin.NewPort(OutputPortName, "Output",
		"Provides an outgoing payload")
}
