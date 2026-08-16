"use client";

import { useId, useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import type { PluginConfigurationValue } from "@/shared/plugins/contracts/plugin-configuration-types";
import {
  DATA_SCHEMA_FIELD_TYPES,
  emptyObjectSchema,
  parseDataSchema,
  schemaForType,
  type DataSchema,
  type DataSchemaType,
} from "@/shared/plugins/editors/schema/schema-types";
import { formBuilderMessages } from "@/shared/plugins/messages/form-builder-messages";
import styles from "./SchemaConfigurationField.module.css";

const TYPE_LABELS: Record<DataSchemaType, string> = {
  object: formBuilderMessages.schemaTypeObject,
  string: formBuilderMessages.schemaTypeString,
  number: formBuilderMessages.schemaTypeNumber,
  boolean: formBuilderMessages.schemaTypeBoolean,
};

export interface SchemaConfigurationFieldProps {
  label: string;
  value: PluginConfigurationValue | undefined;
  disabled: boolean;
  error?: string;
  onChange: (value: PluginConfigurationValue) => void;
}

export function SchemaConfigurationField({
  label,
  value,
  disabled,
  error,
  onChange,
}: SchemaConfigurationFieldProps) {
  const schema = parseDataSchema(value) ?? emptyObjectSchema();

  return (
    <Box className={styles.schemaField}>
      <Typography as="span">{label}</Typography>
      <Typography as="small" className={styles.schemaField__hint}>
        {formBuilderMessages.schemaHint}
      </Typography>
      <Box className={styles.schemaField__tree}>
        <SchemaObjectEditor
          properties={schema.properties ?? {}}
          propertyKeys={schema.propertyKeys}
          disabled={disabled}
          onChange={(properties, propertyKeys) =>
            onChange({
              type: "object",
              properties,
              ...(propertyKeys ? { propertyKeys } : {}),
            } as PluginConfigurationValue)
          }
        />
      </Box>
      {error ? (
        <Typography as="span" className={styles.schemaField__error} role="alert">
          {error}
        </Typography>
      ) : null}
    </Box>
  );
}

type PropertyRow = {
  id: string;
  name: string;
  schema: DataSchema;
};

function SchemaObjectEditor({
  properties,
  propertyKeys,
  disabled,
  onChange,
}: {
  properties: Record<string, DataSchema>;
  propertyKeys?: string[];
  disabled: boolean;
  onChange: (properties: Record<string, DataSchema>, propertyKeys?: string[]) => void;
}) {
  const idPrefix = useId();
  const [rows, setRows] = useState<PropertyRow[]>(() => {
    const keys = propertyKeys && propertyKeys.length > 0 ? propertyKeys : Object.keys(properties);
    return keys.map((name, index) => ({
      id: `${idPrefix}-${index}`,
      name,
      schema: properties[name] ?? { type: "string" },
    }));
  });

  function emit(next: PropertyRow[]) {
    setRows(next);
    const record: Record<string, DataSchema> = {};
    for (const row of next) {
      if (!(row.name in record)) {
        record[row.name] = row.schema;
      }
    }
    const nextKeys = next.map((row) => row.name);
    const invalid =
      nextKeys.some((key) => key.trim() === "") ||
      new Set(nextKeys.map((key) => key.trim())).size !== nextKeys.length;
    onChange(record, invalid ? nextKeys : undefined);
  }

  return (
    <Box className={styles.schemaObject}>
      {rows.length > 0 ? (
        <Box className={styles.schemaObject__header}>
          <Typography as="span">{formBuilderMessages.schemaKeyLabel}</Typography>
          <Typography as="span">{formBuilderMessages.schemaDataTypeLabel}</Typography>
        </Box>
      ) : null}
      {rows.map((row, index) => (
        <SchemaFieldRow
          key={row.id}
          name={row.name}
          schema={row.schema}
          disabled={disabled}
          error={siblingKeyError(
            rows.map((item) => item.name),
            index,
          )}
          onRename={(name) => {
            emit(rows.map((item, itemIndex) => (itemIndex === index ? { ...item, name } : item)));
          }}
          onChange={(schema) => {
            emit(rows.map((item, itemIndex) => (itemIndex === index ? { ...item, schema } : item)));
          }}
          onDelete={() => emit(rows.filter((_, itemIndex) => itemIndex !== index))}
        />
      ))}
      <Box className={styles.schemaNode__actions}>
        <Button
          type="button"
          variant={ButtonVariant.Secondary}
          disabled={disabled}
          onClick={() => {
            emit([
              ...rows,
              {
                id: `${idPrefix}-${rows.length}`,
                name: "",
                schema: { type: "string" },
              },
            ]);
          }}
        >
          {formBuilderMessages.schemaAddField}
        </Button>
      </Box>
    </Box>
  );
}

function SchemaFieldRow({
  name,
  schema,
  disabled,
  error,
  onRename,
  onChange,
  onDelete,
}: {
  name: string;
  schema: DataSchema;
  disabled: boolean;
  error: string | null;
  onRename: (name: string) => void;
  onChange: (schema: DataSchema) => void;
  onDelete: () => void;
}) {
  return (
    <Box className={styles.schemaNode}>
      <Box className={styles.schemaNode__row}>
        <label className={styles.schemaNode__control}>
          <Input
            disabled={disabled}
            value={name}
            placeholder={formBuilderMessages.schemaKeyPlaceholder}
            aria-invalid={Boolean(error)}
            onChange={(event) => onRename(event.target.value)}
          />
        </label>
        <label className={styles.schemaNode__control}>
          <select
            disabled={disabled}
            value={schema.type}
            onChange={(event) => onChange(schemaForType(event.target.value as DataSchemaType))}
          >
            {DATA_SCHEMA_FIELD_TYPES.map((type) => (
              <option key={type} value={type}>
                {TYPE_LABELS[type]}
              </option>
            ))}
          </select>
        </label>
        <Button
          type="button"
          variant={ButtonVariant.Ghost}
          className={styles.schemaNode__delete}
          disabled={disabled}
          onClick={onDelete}
        >
          {formBuilderMessages.schemaDeleteField}
        </Button>
      </Box>
      {error ? (
        <Typography as="span" className={styles.schemaField__error} role="alert">
          {error}
        </Typography>
      ) : null}
      {schema.type === "object" ? (
        <Box className={styles.schemaNode__children}>
          <SchemaObjectEditor
            properties={schema.properties ?? {}}
            propertyKeys={schema.propertyKeys}
            disabled={disabled}
            onChange={(properties, propertyKeys) =>
              onChange({ type: "object", properties, propertyKeys })
            }
          />
        </Box>
      ) : null}
    </Box>
  );
}

function siblingKeyError(keys: string[], index: number): string | null {
  const key = keys[index] ?? "";
  if (key.trim() === "") {
    return formBuilderMessages.schemaBlankPropertyKey;
  }
  const normalized = key.trim();
  if (keys.filter((candidate) => candidate.trim() === normalized).length > 1) {
    return formBuilderMessages.schemaDuplicatePropertyKey;
  }
  return null;
}
