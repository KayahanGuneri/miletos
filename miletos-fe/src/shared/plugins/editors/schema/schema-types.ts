import type { PluginConfigurationValue } from "@/shared/plugins/contracts/plugin-configuration-types";
import { formBuilderMessages } from "@/shared/plugins/messages/form-builder-messages";

export const DATA_SCHEMA_FIELD_TYPES = ["string", "number", "boolean", "object"] as const;

export type DataSchemaType = (typeof DATA_SCHEMA_FIELD_TYPES)[number];

export type DataSchema = {
  type: DataSchemaType;
  properties?: Record<string, DataSchema>;
  propertyKeys?: string[];
};

export function isDataSchemaType(value: unknown): value is DataSchemaType {
  return DATA_SCHEMA_FIELD_TYPES.some((type) => type === value);
}

export function emptyObjectSchema(): DataSchema {
  return { type: "object", properties: {} };
}

export function schemaForType(type: DataSchemaType): DataSchema {
  if (type === "object") {
    return { type: "object", properties: {} };
  }
  return { type };
}

export function toPersistedSchema(schema: DataSchema): PluginConfigurationValue {
  if (schema.type !== "object") {
    return { type: schema.type };
  }
  const properties: { [key: string]: PluginConfigurationValue } = {};
  for (const key of schema.propertyKeys ?? Object.keys(schema.properties ?? {})) {
    const trimmed = key.trim();
    if (!trimmed || trimmed in properties) {
      continue;
    }
    const nested = schema.properties?.[key];
    if (nested) {
      properties[trimmed] = toPersistedSchema(nested);
    }
  }
  for (const [key, nested] of Object.entries(schema.properties ?? {})) {
    const trimmed = key.trim();
    if (!trimmed || trimmed in properties) {
      continue;
    }
    properties[trimmed] = toPersistedSchema(nested);
  }
  return { type: "object", properties };
}

export function parseDataSchema(value: unknown): DataSchema | null {
  const validated = readDataSchema(value);
  return validated.schema;
}

export function validateDataSchema(value: unknown): string | null {
  return readDataSchema(value).error;
}

function readDataSchema(value: unknown): { schema: DataSchema | null; error: string | null } {
  if (value === undefined || value === null) {
    return { schema: null, error: null };
  }
  const root = readDataSchemaNode(value);
  if (root.schema && root.schema.type !== "object") {
    return { schema: root.schema, error: formBuilderMessages.schemaInvalidStructure };
  }
  return root;
}

function readDataSchemaNode(value: unknown): { schema: DataSchema | null; error: string | null } {
  if (!isPlainObject(value) || !isDataSchemaType(value.type)) {
    return { schema: null, error: formBuilderMessages.schemaInvalidStructure };
  }
  if (value.type !== "object") {
    return { schema: { type: value.type }, error: null };
  }
  const propertiesValue = value.properties ?? {};
  if (!isPlainObject(propertiesValue)) {
    return { schema: null, error: formBuilderMessages.schemaInvalidStructure };
  }
  let error = validatePropertyKeys(value.propertyKeys);
  const properties: Record<string, DataSchema> = {};
  for (const [key, property] of Object.entries(propertiesValue)) {
    if (key.trim() === "") {
      error = error ?? formBuilderMessages.schemaBlankPropertyKey;
    }
    const nested = readDataSchemaNode(property);
    error = error ?? nested.error;
    if (!nested.schema) {
      error = error ?? formBuilderMessages.schemaInvalidStructure;
      continue;
    }
    properties[key] = nested.schema;
  }
  return {
    schema: {
      type: "object",
      properties,
      propertyKeys: Array.isArray(value.propertyKeys)
        ? value.propertyKeys.filter((key): key is string => typeof key === "string")
        : undefined,
    },
    error,
  };
}

function validatePropertyKeys(value: unknown): string | null {
  if (value === undefined) {
    return null;
  }
  if (!Array.isArray(value) || value.some((key) => typeof key !== "string")) {
    return formBuilderMessages.schemaInvalidStructure;
  }
  const trimmed = value.map((key) => key.trim());
  if (trimmed.some((key) => key === "")) {
    return formBuilderMessages.schemaBlankPropertyKey;
  }
  if (new Set(trimmed).size !== trimmed.length) {
    return formBuilderMessages.schemaDuplicatePropertyKey;
  }
  return null;
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
