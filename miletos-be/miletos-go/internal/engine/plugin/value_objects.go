package plugin

import workflow "miletos-go/internal/features/workflow"

type PluginIdentity struct {
	pluginType workflow.PluginType
	version    workflow.PluginVersion
}

func NewPluginIdentity(
	pluginType workflow.PluginType, version workflow.PluginVersion) (PluginIdentity, error) {
	normalizedType, err := workflow.NewPluginType(pluginType.String())
	if err != nil {
		return PluginIdentity{}, err
	}
	normalizedVersion, err := workflow.NewPluginVersion(version.String())
	if err != nil {
		return PluginIdentity{}, err
	}
	return PluginIdentity{
		pluginType: normalizedType, version: normalizedVersion}, nil
}
func (identity PluginIdentity) Type() workflow.PluginType {
	return identity.pluginType
}
func (identity PluginIdentity) Version() workflow.PluginVersion { return identity.version }
func (identity PluginIdentity) String() string {
	return identity.pluginType.String() +
		"@" + identity.version.String()
}
func (identity PluginIdentity) Less(other PluginIdentity,
) bool {
	if identity.pluginType != other.pluginType {
		return identity.pluginType.String() <
			other.pluginType.String()
	}
	return identity.version.String() < other.version.String()
}
func (identity PluginIdentity) IsValid() bool {
	_, err := NewPluginIdentity(
		identity.pluginType, identity.version)
	return err == nil
}

type PluginMetadata struct {
	displayName string
	description string
}

func NewPluginMetadata(displayName string, description string,
) (PluginMetadata, error) {
	normalizedDisplayName, err := normalizeRequiredString("metadata.displayName",
		displayName)
	if err != nil {
		return PluginMetadata{}, err
	}
	return PluginMetadata{displayName: normalizedDisplayName, description: normalizeOptionalString(description)}, nil
}
func (metadata PluginMetadata) DisplayName() string { return metadata.displayName }
func (metadata PluginMetadata) Description() string {
	return metadata.description
}
func (metadata PluginMetadata) IsValid() bool {
	_, err := metadata.normalized()
	return err == nil
}
func (metadata PluginMetadata) normalized() (
	PluginMetadata, error) {
	return NewPluginMetadata(metadata.displayName, metadata.description)
}

type Port struct {
	name        string
	displayName string
	description string
}

func NewPort(name string,
	displayName string, description string) (Port, error) {
	normalizedName, err := normalizeRequiredString("port.name", name)
	if err != nil {
		return Port{}, err
	}
	return Port{
		name: normalizedName, displayName: normalizeOptionalString(displayName), description: normalizeOptionalString(description),
	}, nil
}
func (port Port) Name() string { return port.name }
func (port Port) DisplayName() string {
	return port.displayName
}
func (port Port) Description() string {
	return port.description
}
func (port Port) IsValid() bool {
	_, err := NewPort(port.name,
		port.displayName, port.description)
	return err == nil
}
func clonePorts(ports []Port,
) []Port {
	if len(ports) == 0 {
		return nil
	}
	cloned := make(
		[]Port, len(ports))
	copy(cloned, ports)
	return cloned
}
