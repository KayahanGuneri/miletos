import type {
  PluginConfiguration,
  PluginConfigurationValidationResult,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import {
  isFormFieldVisible,
  type FlatFormSchema,
  type FormConfiguration,
  type FormDataType,
  type FormField,
  type FormFieldValue,
} from "@/shared/plugins/form/form-schema-types";
import { formBuilderMessages } from "@/shared/plugins/messages/form-builder-messages";

type DefaultValueResolver = (field: FormField) => FormFieldValue | undefined;

const DEFAULT_VALUE_BY_DATA_TYPE: Record<FormDataType, DefaultValueResolver> = {
  STRING: (field) => {
    if (!field.required && field.renderType === "DROPDOWN") {
      return undefined;
    }
    return "";
  },
  NUMBER: (field) => (field.required ? 0 : undefined),
  BOOLEAN: (field) => {
    if (!field.required && field.renderType === "DROPDOWN") {
      return undefined;
    }
    return false;
  },
};

function defaultForField(field: FormField): FormFieldValue | undefined {
  if (field.defaultValue !== undefined) {
    return field.defaultValue;
  }
  return DEFAULT_VALUE_BY_DATA_TYPE[field.dataType](field);
}

export function createDefaultFormConfiguration(schema: FlatFormSchema): PluginConfiguration {
  const values: PluginConfiguration = {};

  for (const field of schema.fields) {
    const value = defaultForField(field);

    if (value !== undefined) {
      values[field.key] = value;
    }
  }

  return values;
}

export function deserializeFormConfiguration(
  schema: FlatFormSchema,
  configuration: Record<string, unknown>,
): FormConfiguration {
  const values = createDefaultFormConfiguration(schema) as FormConfiguration;
  for (const field of schema.fields) {
    if (!(field.key in configuration)) {
      continue;
    }
    const raw = configuration[field.key];
    const coerced = coerceFieldValue(field, raw);
    if (coerced === undefined) {
      delete values[field.key];
    } else {
      values[field.key] = coerced;
    }
  }
  return values;
}

type PersistedValueCoercer = (field: FormField, raw: unknown) => FormFieldValue | undefined;

const COERCE_VALUE_BY_DATA_TYPE: Record<FormDataType, PersistedValueCoercer> = {
  STRING: (field, raw) => {
    if (raw === undefined || raw === null) {
      return defaultForField(field);
    }
    if (typeof raw !== "string") {
      throw new Error(formBuilderMessages.persistedTypeMismatch(field.key, "string"));
    }
    return raw;
  },
  NUMBER: (field, raw) => {
    if (raw === undefined || raw === null || raw === "") {
      return field.required ? defaultForField(field) : undefined;
    }
    if (typeof raw !== "number" || !Number.isFinite(raw)) {
      throw new Error(formBuilderMessages.persistedTypeMismatch(field.key, "number"));
    }
    return raw;
  },
  BOOLEAN: (field, raw) => {
    if (raw === undefined || raw === null) {
      return defaultForField(field);
    }
    if (typeof raw !== "boolean") {
      throw new Error(formBuilderMessages.persistedTypeMismatch(field.key, "boolean"));
    }
    return raw;
  },
};

function coerceFieldValue(field: FormField, raw: unknown): FormFieldValue | undefined {
  return COERCE_VALUE_BY_DATA_TYPE[field.dataType](field, raw);
}

export function validateFormConfiguration(
  schema: FlatFormSchema,
  values: FormConfiguration,
): PluginConfigurationValidationResult {
  const errors: Record<string, string> = {};
  for (const field of schema.fields) {
    if (!isFormFieldVisible(field, values)) {
      continue;
    }
    const value = values[field.key];
    const fieldError = validateFieldValue(field, value);
    if (fieldError) {
      errors[field.key] = fieldError;
    }
  }
  return { valid: Object.keys(errors).length === 0, errors };
}

type FieldValueValidator = (field: FormField, value: FormFieldValue) => string | null;

const VALIDATE_VALUE_BY_DATA_TYPE: Record<FormDataType, FieldValueValidator> = {
  STRING: (field, value) => {
    if (typeof value !== "string") {
      return formBuilderMessages.mustBeString(field.label);
    }
    if (field.renderType === "DROPDOWN") {
      const allowed = field.options?.some((option) => option.value === value);
      if (!allowed) {
        return formBuilderMessages.invalidOption(field.label);
      }
    }
    return null;
  },
  NUMBER: (field, value) => {
    if (typeof value !== "number" || !Number.isFinite(value)) {
      return formBuilderMessages.mustBeNumber(field.label);
    }
    if (field.renderType === "INPUT") {
      if ("min" in field && typeof field.min === "number" && value < field.min) {
        return formBuilderMessages.numberTooSmall(field.label, field.min);
      }
      if ("max" in field && typeof field.max === "number" && value > field.max) {
        return formBuilderMessages.numberTooLarge(field.label, field.max);
      }
    }
    if (field.renderType === "DROPDOWN") {
      const allowed = field.options?.some((option) => option.value === value);
      if (!allowed) {
        return formBuilderMessages.invalidOption(field.label);
      }
    }
    return null;
  },
  BOOLEAN: (field, value) => {
    if (typeof value !== "boolean") {
      return formBuilderMessages.mustBeBoolean(field.label);
    }
    if (field.renderType === "DROPDOWN" && field.options) {
      const allowed = field.options.some((option) => option.value === value);
      if (!allowed) {
        return formBuilderMessages.invalidOption(field.label);
      }
    }
    return null;
  },
};

function validateFieldValue(field: FormField, value: FormFieldValue | undefined): string | null {
  if (field.required) {
    if (value === undefined || value === null || value === "") {
      return formBuilderMessages.required(field.label);
    }
  }

  if (value === undefined || value === null || value === "") {
    return null;
  }

  return VALIDATE_VALUE_BY_DATA_TYPE[field.dataType](field, value);
}

type ValueSerializer = (field: FormField, value: FormFieldValue) => FormFieldValue | undefined;

const SERIALIZE_VALUE_BY_DATA_TYPE: Record<FormDataType, ValueSerializer> = {
  STRING: (field, value) => {
    if (typeof value !== "string") {
      return value;
    }
    return field.dataType === "STRING" &&
      field.renderType === "INPUT" &&
      field.inputType === "password"
      ? value
      : value.trim();
  },
  NUMBER: (_field, value) => (typeof value === "number" ? value : undefined),
  BOOLEAN: (_field, value) => value,
};

export function serializeFormConfiguration(
  schema: FlatFormSchema,
  values: FormConfiguration,
): PluginConfiguration {
  const serialized: PluginConfiguration = {};
  for (const field of schema.fields) {
    if (!isFormFieldVisible(field, values)) {
      continue;
    }
    const value = values[field.key];
    if (value === undefined || value === null) {
      continue;
    }
    const serializedValue = SERIALIZE_VALUE_BY_DATA_TYPE[field.dataType](field, value);
    if (serializedValue === undefined) {
      continue;
    }
    serialized[field.key] = serializedValue;
  }
  return serialized;
}
