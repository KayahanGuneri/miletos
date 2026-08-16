import { validateFormConfiguration } from "@/shared/plugins/form/form-configuration";
import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import {
  databaseConnectionFields,
  validateDatabaseConnection,
} from "@/shared/plugins/editors/database/database-connection";
import type { FlatFormSchema, FormConfiguration } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const messages = pluginMessages.databaseOutput;

const IDENTIFIER = /^[A-Za-z_][A-Za-z0-9_]*$/;
const DATABASE_OUTPUT_OPERATIONS = ["INSERT", "UPDATE", "UPSERT"] as const;

function normalizeOperation(value: unknown) {
  return typeof value === "string" ? value.trim().toUpperCase() : "";
}

function requiresKey(values: FormConfiguration) {
  const operation = normalizeOperation(values.operation);
  return operation === "UPDATE" || operation === "UPSERT";
}

export const databaseOutputFormSchema: FlatFormSchema = {
  fields: [
    ...databaseConnectionFields,
    {
      key: "operation",
      label: messages.operationLabel,
      dataType: "STRING",
      renderType: "DROPDOWN",
      required: true,
      defaultValue: "INSERT",
      options: DATABASE_OUTPUT_OPERATIONS.map((operation) => ({
        label: operation,
        value: operation,
      })),
    },
    {
      key: "schema",
      label: messages.schemaLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "public",
      placeholder: messages.schemaPlaceholder,
    },
    {
      key: "table",
      label: messages.tableLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      placeholder: messages.tablePlaceholder,
    },
    {
      key: "keyField",
      label: messages.keyFieldLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      placeholder: messages.keyFieldPlaceholder,
      when: requiresKey,
    },
    {
      key: "keyColumn",
      label: messages.keyColumnLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      placeholder: messages.keyColumnPlaceholder,
      when: requiresKey,
    },
    {
      key: "payloadColumn",
      label: messages.payloadColumnLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "payload",
      placeholder: messages.payloadColumnPlaceholder,
    },
  ],
};

const databaseOutputBaseDefinition = createFormPluginDefinition({
  pluginType: "core.database-output",
  schema: databaseOutputFormSchema,
  validate: (values) => {
    const generic = validateFormConfiguration(databaseOutputFormSchema, values);
    const errors = { ...generic.errors };
    Object.assign(errors, validateDatabaseConnection(values));
    const operation = normalizeOperation(values.operation);
    const schema = typeof values.schema === "string" ? values.schema.trim() : "";
    const table = typeof values.table === "string" ? values.table.trim() : "";
    const keyField = typeof values.keyField === "string" ? values.keyField.trim() : "";
    const keyColumn = typeof values.keyColumn === "string" ? values.keyColumn.trim() : "";
    const payloadColumn =
      typeof values.payloadColumn === "string" ? values.payloadColumn.trim() : "";
    if (!DATABASE_OUTPUT_OPERATIONS.some((candidate) => candidate === operation)) {
      errors.operation = messages.operationInvalid;
    }
    if (!schema) errors.schema = messages.schemaRequired;
    else if (!IDENTIFIER.test(schema)) errors.schema = messages.schemaInvalid;
    if (!table) errors.table = messages.tableRequired;
    else if (!IDENTIFIER.test(table)) errors.table = messages.tableInvalid;
    if (!payloadColumn) errors.payloadColumn = messages.payloadColumnRequired;
    else if (!IDENTIFIER.test(payloadColumn)) {
      errors.payloadColumn = messages.payloadColumnInvalid;
    }
    if (operation === "UPDATE" || operation === "UPSERT") {
      if (!keyField) errors.keyField = messages.keyFieldRequired;
      else if (keyField.includes(".")) errors.keyField = messages.keyFieldInvalid;
      if (!keyColumn) errors.keyColumn = messages.keyColumnRequired;
      else if (!IDENTIFIER.test(keyColumn)) errors.keyColumn = messages.keyColumnInvalid;
      else if (keyColumn === payloadColumn) errors.keyColumn = messages.columnsConflict;
    }
    return { valid: Object.keys(errors).length === 0, errors };
  },
});

export const databaseOutputConfigurationDefinition = {
  ...databaseOutputBaseDefinition,
  deserialize: (configuration: Parameters<typeof databaseOutputBaseDefinition.deserialize>[0]) => {
    const values = databaseOutputBaseDefinition.deserialize(configuration);
    values.operation = normalizeOperation(values.operation) || "INSERT";
    values.payloadColumn =
      typeof values.payloadColumn === "string" && values.payloadColumn.trim()
        ? values.payloadColumn.trim()
        : "payload";
    return values;
  },
};
