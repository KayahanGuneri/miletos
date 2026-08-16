import type {
  PluginConfiguration,
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import type { PluginConfigurationValue } from "@/shared/plugins/contracts/plugin-configuration-types";
import { SchemaConfigurationField } from "@/shared/plugins/editors/schema/SchemaConfigurationField";
import {
  parseDataSchema,
  toPersistedSchema,
  validateDataSchema,
} from "@/shared/plugins/editors/schema/schema-types";
import type { FormConfiguration } from "@/shared/plugins/form/form-schema-types";
import { formBuilderMessages } from "@/shared/plugins/messages/form-builder-messages";

interface SchemaConfigurationFieldDefinition {
  key: string;
  label: string;
}

export function withSchemaConfiguration({
  definition,
  field,
}: {
  definition: PluginConfigurationDefinition<FormConfiguration>;
  field: SchemaConfigurationFieldDefinition;
}): PluginConfigurationDefinition<FormConfiguration> {
  function SchemaEditor({
    initialValues,
    disabled,
    validationErrors,
    onChange,
  }: PluginConfigurationEditorProps<FormConfiguration>) {
    return (
      <SchemaConfigurationField
        label={field.label}
        value={initialValues[field.key]}
        disabled={disabled}
        error={validationErrors[field.key]}
        onChange={(value) => onChange({ ...initialValues, [field.key]: value })}
      />
    );
  }

  return {
    ...definition,
    deserialize: (configuration: PluginConfiguration) => {
      const values = definition.deserialize(configuration);
      const schema = configuration[field.key];
      if (schema === undefined || schema === null) {
        return values;
      }
      if (typeof schema !== "object" || Array.isArray(schema)) {
        throw new Error(formBuilderMessages.persistedTypeMismatch(field.key, "schema object"));
      }
      return { ...values, [field.key]: schema };
    },
    validate: (values, editorContext) => {
      const validation = definition.validate(values, editorContext);
      const schemaError = validateDataSchema(values[field.key]);
      const errors = schemaError
        ? { ...validation.errors, [field.key]: schemaError }
        : validation.errors;
      return { valid: Object.keys(errors).length === 0, errors };
    },
    serialize: (values) => {
      const configuration = definition.serialize(values);
      const schema = parseDataSchema(values[field.key]);
      if (schema) {
        configuration[field.key] = toPersistedSchema(schema) as PluginConfigurationValue;
      }
      return configuration;
    },
    Editor: SchemaEditor,
  };
}
