import type { ComponentType } from "react";
import type {
  PluginConfiguration,
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
  RegisteredPluginConfigurationDefinition,
} from "@/shared/plugins/contracts/plugin-configuration";
import { delayConfigurationDefinition } from "@/shared/plugins/editors/delay/DelayConfiguration";
import { httpTriggerConfigurationDefinition } from "@/shared/plugins/editors/http-trigger/HttpTriggerConfiguration";
import { joinConfigurationDefinition } from "@/shared/plugins/editors/join/JoinConfiguration";
import { passThroughConfigurationDefinition } from "@/shared/plugins/editors/pass-through/PassThroughConfiguration";
import { staticInputConfigurationDefinition } from "@/shared/plugins/editors/static-input/StaticInputConfiguration";
import { terminalConfigurationDefinition } from "@/shared/plugins/editors/terminal/TerminalConfiguration";

function registerDefinition<TValues>(
  definition: PluginConfigurationDefinition<TValues>,
): RegisteredPluginConfigurationDefinition {
  return {
    pluginType: definition.pluginType,
    dialogDescription: definition.dialogDescription,
    dialogSize: definition.dialogSize,
    createDefaultConfiguration: definition.createDefaultConfiguration,
    deserialize: definition.deserialize,
    validate: (values) => definition.validate(values as TValues),
    serialize: (values) => definition.serialize(values as TValues),
    // Runtime lookup erases TValues; deserialize and Editor always originate from this same definition.
    Editor: definition.Editor as ComponentType<PluginConfigurationEditorProps<unknown>>,
  };
}

const DEFINITIONS: RegisteredPluginConfigurationDefinition[] = [
  registerDefinition(delayConfigurationDefinition),
  registerDefinition(httpTriggerConfigurationDefinition),
  registerDefinition(joinConfigurationDefinition),
  registerDefinition(passThroughConfigurationDefinition),
  registerDefinition(staticInputConfigurationDefinition),
  registerDefinition(terminalConfigurationDefinition),
];

const DEFINITION_BY_PLUGIN_TYPE = new Map(
  DEFINITIONS.map((definition) => [definition.pluginType, definition]),
);

export function getPluginConfigurationDefinition(pluginType: string) {
  return DEFINITION_BY_PLUGIN_TYPE.get(pluginType);
}

export function createDefaultPluginConfiguration(pluginType: string): PluginConfiguration {
  return getPluginConfigurationDefinition(pluginType)?.createDefaultConfiguration() ?? {};
}

export function isPluginConfigurationValid(pluginType: string, configuration: PluginConfiguration) {
  const definition = getPluginConfigurationDefinition(pluginType);
  if (!definition) {
    return true;
  }

  try {
    return definition.validate(definition.deserialize(configuration)).valid;
  } catch {
    return false;
  }
}
