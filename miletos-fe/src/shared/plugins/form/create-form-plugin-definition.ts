import {
  createDefaultFormConfiguration,
  deserializeFormConfiguration,
  serializeFormConfiguration,
} from "@/shared/plugins/form/form-configuration";
import type { FlatFormSchema, FormConfiguration } from "@/shared/plugins/form/form-schema-types";
import type {
  PluginConfiguration,
  PluginConfigurationDefinition,
  PluginConfigurationValidationResult,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";

export function createFormPluginDefinition(params: {
  pluginType: string;
  schema: FlatFormSchema;
  validate: (values: FormConfiguration) => PluginConfigurationValidationResult;
  dialogDescription?: string;
  dialogSize?: "default" | "large";
}): PluginConfigurationDefinition<FormConfiguration> {
  const { pluginType, schema, validate, dialogDescription, dialogSize } = params;

  return {
    pluginType,
    dialogDescription,
    dialogSize,
    formSchema: schema,
    createDefaultConfiguration: () => createDefaultFormConfiguration(schema),
    deserialize: (configuration: PluginConfiguration) =>
      deserializeFormConfiguration(schema, configuration),
    validate,
    serialize: (values) => serializeFormConfiguration(schema, values),
  };
}
