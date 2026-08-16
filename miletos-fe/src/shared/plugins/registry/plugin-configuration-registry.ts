import type { ComponentType } from "react";
import type {
  PluginConfiguration,
  PluginConfigurationDefinition,
  PluginConfigurationEditorContext,
  PluginConfigurationEditorProps,
  RegisteredPluginConfigurationDefinition,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { csvOutputConfigurationDefinition } from "@/shared/plugins/editors/csv-output/CsvOutputConfiguration";
import {
  filterConfigurationDefinition,
  ifConfigurationDefinition,
} from "@/shared/plugins/editors/condition/ConditionConfiguration";
import { databaseInputConfigurationDefinition } from "@/shared/plugins/editors/database-input/DatabaseInputConfiguration";
import { databaseOutputConfigurationDefinition } from "@/shared/plugins/editors/database-output/DatabaseOutputConfiguration";
import { delayConfigurationDefinition } from "@/shared/plugins/editors/delay/DelayConfiguration";
import { excelInputConfigurationDefinition } from "@/shared/plugins/editors/excel-input/ExcelInputConfiguration";
import { fileInputConfigurationDefinition } from "@/shared/plugins/editors/file-input/FileInputConfiguration";
import { httpTriggerConfigurationDefinition } from "@/shared/plugins/editors/http-trigger/HttpTriggerConfiguration";
import { cronTriggerConfigurationDefinition } from "@/shared/plugins/editors/cron-trigger/CronTriggerConfiguration";
import { mapConfigurationDefinition } from "@/shared/plugins/editors/map/MapConfiguration";
import { mergeConfigurationDefinition } from "@/shared/plugins/editors/merge/MergeConfiguration";
import { restOutputConfigurationDefinition } from "@/shared/plugins/editors/rest-output/RestOutputConfiguration";
import { subflowConfigurationDefinition } from "@/shared/plugins/editors/subflow/SubflowConfiguration";
import { subflowReturnConfigurationDefinition } from "@/shared/plugins/editors/subflow-return/SubflowReturnConfiguration";
import { terminalConfigurationDefinition } from "@/shared/plugins/editors/terminal/TerminalConfiguration";

function registerDefinition<TValues>(
  definition: PluginConfigurationDefinition<TValues>,
): RegisteredPluginConfigurationDefinition {
  return {
    pluginType: definition.pluginType,
    dialogDescription: definition.dialogDescription,
    dialogSize: definition.dialogSize,
    formSchema: definition.formSchema,
    createDefaultConfiguration: definition.createDefaultConfiguration,
    deserialize: definition.deserialize,
    validate: (values, editorContext) => definition.validate(values as TValues, editorContext),
    serialize: (values) => definition.serialize(values as TValues),
    Editor: definition.Editor as ComponentType<PluginConfigurationEditorProps<unknown>> | undefined,
  };
}

const DEFINITIONS: RegisteredPluginConfigurationDefinition[] = [
  registerDefinition(delayConfigurationDefinition),
  registerDefinition(httpTriggerConfigurationDefinition),
  registerDefinition(cronTriggerConfigurationDefinition),
  registerDefinition(ifConfigurationDefinition),
  registerDefinition(filterConfigurationDefinition),
  registerDefinition(restOutputConfigurationDefinition),
  registerDefinition(databaseOutputConfigurationDefinition),
  registerDefinition(csvOutputConfigurationDefinition),
  registerDefinition(fileInputConfigurationDefinition),
  registerDefinition(excelInputConfigurationDefinition),
  registerDefinition(databaseInputConfigurationDefinition),
  registerDefinition(mergeConfigurationDefinition),
  registerDefinition(mapConfigurationDefinition),
  registerDefinition(subflowConfigurationDefinition),
  registerDefinition(subflowReturnConfigurationDefinition),
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

export function isPluginConfigurationValid(
  pluginType: string,
  configuration: PluginConfiguration,
  editorContext?: PluginConfigurationEditorContext,
) {
  const definition = getPluginConfigurationDefinition(pluginType);
  if (!definition) {
    return true;
  }

  try {
    return definition.validate(definition.deserialize(configuration), editorContext).valid;
  } catch {
    return false;
  }
}
