import {
  createDefaultFormConfiguration,
  deserializeFormConfiguration,
  serializeFormConfiguration,
  validateFormConfiguration,
} from "@/shared/plugins/form/form-configuration";
import type { FlatFormSchema, FormConfiguration } from "@/shared/plugins/form/form-schema-types";
import type {
  PluginConfiguration,
  PluginConfigurationDefinition,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";

export function createFormPluginDefinition(params: {
  pluginType: string;
  schema: FlatFormSchema;
  dialogDescription?: string;
  dialogSize?: "default" | "large";
}): PluginConfigurationDefinition<FormConfiguration> {
  const { pluginType, schema, dialogDescription, dialogSize } = params;

  return {
    pluginType,
    dialogDescription,
    dialogSize,
    formSchema: schema,
    createDefaultConfiguration: () => createDefaultFormConfiguration(schema),
    deserialize: (configuration: PluginConfiguration) =>
      deserializeFormConfiguration(schema, configuration),
    validate: (values) => validateFormConfiguration(schema, values),
    serialize: (values) => serializeFormConfiguration(schema, values),
  };
}
