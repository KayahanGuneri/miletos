"use client";

import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfigurationDefinition,
  PluginConfigurationEditorContext,
  PluginConfigurationEditorProps,
  PluginConfigurationValidationResult,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

const messages = pluginMessages.merge;
const DATABASE_FIELD = /^[A-Za-z_][A-Za-z0-9_]*$/;
type MergeEmissionMode = "ANY_SOURCE" | "PRIMARY_ONLY";

interface MergeMappingValues {
  key: string;
  sourceNodeId: string;
  correlationEnabled: boolean;
  correlationPrimaryField: string;
  correlationSourceField: string;
}

interface MergeValues {
  primaryInputNodeId: string;
  emissionMode: MergeEmissionMode;
  mappings: MergeMappingValues[];
}

function MergeConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
  editorContext,
}: PluginConfigurationEditorProps<MergeValues>) {
  const inputs = editorContext?.incomingInputs ?? [];
  const mappingOptions = inputs.filter(
    (input) => input.sourceNodeId !== initialValues.primaryInputNodeId,
  );
  const maximumMappings = Math.max(0, inputs.length - 1);

  return (
    <section className={styles.pluginConfiguration__formSection}>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.primaryInputLabel}</Typography>
        <select
          disabled={disabled || inputs.length === 0}
          value={initialValues.primaryInputNodeId}
          aria-invalid={Boolean(validationErrors.primaryInputNodeId)}
          onChange={(event) =>
            onChange({ ...initialValues, primaryInputNodeId: event.target.value })
          }
        >
          <option value="">{messages.selectInputPlaceholder}</option>
          {inputs.map((input) => (
            <option key={input.edgeId} value={input.sourceNodeId}>
              {input.sourceLabel}
            </option>
          ))}
        </select>
        {validationErrors.primaryInputNodeId ? (
          <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
            {validationErrors.primaryInputNodeId}
          </Typography>
        ) : null}
      </label>

      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.emissionModeLabel}</Typography>
        <select
          disabled={disabled}
          value={initialValues.emissionMode}
          onChange={(event) =>
            onChange({
              ...initialValues,
              emissionMode: event.target.value as MergeEmissionMode,
            })
          }
        >
          <option value="ANY_SOURCE">{messages.emissionModeAnySource}</option>
          <option value="PRIMARY_ONLY">{messages.emissionModePrimaryOnly}</option>
        </select>
        <Typography as="small">
          {initialValues.emissionMode === "PRIMARY_ONLY"
            ? messages.emissionModePrimaryOnlyHint
            : messages.emissionModeAnySourceHint}
        </Typography>
      </label>

      <Typography as="span">{messages.mappedInputsLabel}</Typography>
      {initialValues.mappings.map((mapping, index) => (
        <Box key={index} className={styles.pluginConfiguration__formSection}>
          <label className={styles.pluginConfiguration__field}>
            <Typography as="span">{messages.mappingKeyLabel}</Typography>
            <Input
              disabled={disabled}
              value={mapping.key}
              aria-invalid={Boolean(validationErrors[`mappings.${index}.key`])}
              onChange={(event) =>
                onChange(updateMapping(initialValues, index, { key: event.target.value }))
              }
            />
            {validationErrors[`mappings.${index}.key`] ? (
              <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
                {validationErrors[`mappings.${index}.key`]}
              </Typography>
            ) : null}
          </label>
          <label className={styles.pluginConfiguration__field}>
            <Typography as="span">{messages.mappingModeLabel}</Typography>
            <select
              disabled={disabled}
              value={mapping.correlationEnabled ? "CORRELATION" : "LATEST"}
              onChange={(event) =>
                onChange(
                  updateMapping(initialValues, index, {
                    correlationEnabled: event.target.value === "CORRELATION",
                  }),
                )
              }
            >
              <option value="LATEST">{messages.mappingModeLatest}</option>
              <option value="CORRELATION">{messages.mappingModeCorrelation}</option>
            </select>
          </label>
          {mapping.correlationEnabled ? (
            <>
              <label className={styles.pluginConfiguration__field}>
                <Typography as="span">{messages.correlationPrimaryFieldLabel}</Typography>
                <Input
                  disabled={disabled}
                  value={mapping.correlationPrimaryField}
                  aria-invalid={Boolean(
                    validationErrors[`mappings.${index}.correlation.primaryField`],
                  )}
                  onChange={(event) =>
                    onChange(
                      updateMapping(initialValues, index, {
                        correlationPrimaryField: event.target.value,
                      }),
                    )
                  }
                />
                {validationErrors[`mappings.${index}.correlation.primaryField`] ? (
                  <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
                    {validationErrors[`mappings.${index}.correlation.primaryField`]}
                  </Typography>
                ) : null}
              </label>
              <label className={styles.pluginConfiguration__field}>
                <Typography as="span">{messages.correlationSourceFieldLabel}</Typography>
                <Input
                  disabled={disabled}
                  value={mapping.correlationSourceField}
                  aria-invalid={Boolean(
                    validationErrors[`mappings.${index}.correlation.sourceField`],
                  )}
                  onChange={(event) =>
                    onChange(
                      updateMapping(initialValues, index, {
                        correlationSourceField: event.target.value,
                      }),
                    )
                  }
                />
                {validationErrors[`mappings.${index}.correlation.sourceField`] ? (
                  <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
                    {validationErrors[`mappings.${index}.correlation.sourceField`]}
                  </Typography>
                ) : null}
              </label>
            </>
          ) : null}
          <label className={styles.pluginConfiguration__field}>
            <Typography as="span">{messages.mappingSourceLabel}</Typography>
            <select
              disabled={disabled || mappingOptions.length === 0}
              value={mapping.sourceNodeId}
              aria-invalid={Boolean(validationErrors[`mappings.${index}.sourceNodeId`])}
              onChange={(event) =>
                onChange(updateMapping(initialValues, index, { sourceNodeId: event.target.value }))
              }
            >
              <option value="">{messages.selectInputPlaceholder}</option>
              {mappingOptions.map((input) => (
                <option key={input.edgeId} value={input.sourceNodeId}>
                  {input.sourceLabel}
                </option>
              ))}
            </select>
            {validationErrors[`mappings.${index}.sourceNodeId`] ? (
              <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
                {validationErrors[`mappings.${index}.sourceNodeId`]}
              </Typography>
            ) : null}
          </label>
          <Button
            type="button"
            variant={ButtonVariant.Ghost}
            disabled={disabled || initialValues.mappings.length <= 1}
            onClick={() =>
              onChange({
                ...initialValues,
                mappings: initialValues.mappings.filter((_, itemIndex) => itemIndex !== index),
              })
            }
          >
            {messages.removeMapping}
          </Button>
        </Box>
      ))}
      <Button
        type="button"
        variant={ButtonVariant.Secondary}
        disabled={
          disabled || maximumMappings === 0 || initialValues.mappings.length >= maximumMappings
        }
        onClick={() =>
          onChange({
            ...initialValues,
            mappings: [...initialValues.mappings, emptyMapping()],
          })
        }
      >
        {messages.addMapping}
      </Button>
      {validationErrors.mappings ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {validationErrors.mappings}
        </Typography>
      ) : null}
    </section>
  );
}

function updateMapping(
  values: MergeValues,
  index: number,
  patch: Partial<MergeMappingValues>,
): MergeValues {
  return {
    ...values,
    mappings: values.mappings.map((mapping, itemIndex) =>
      itemIndex === index ? { ...mapping, ...patch } : mapping,
    ),
  };
}

function deserializeMappings(value: unknown): MergeMappingValues[] {
  if (value === undefined || value === null) {
    return [emptyMapping()];
  }
  if (!Array.isArray(value)) {
    throw new Error(messages.persistedMappingsInvalid);
  }
  if (value.length === 0) {
    return [emptyMapping()];
  }
  return value.map((item) => {
    if (typeof item !== "object" || item === null || Array.isArray(item)) {
      throw new Error(messages.persistedMappingsInvalid);
    }
    const record = item as Record<string, unknown>;
    if (record.key !== undefined && typeof record.key !== "string") {
      throw new Error(messages.persistedMappingKeyInvalid);
    }
    if (record.sourceNodeId !== undefined && typeof record.sourceNodeId !== "string") {
      throw new Error(messages.persistedMappingSourceInvalid);
    }
    const correlation = deserializeCorrelation(record.correlation);
    return {
      key: typeof record.key === "string" ? record.key : "",
      sourceNodeId: typeof record.sourceNodeId === "string" ? record.sourceNodeId : "",
      ...correlation,
    };
  });
}

function deserializeEmissionMode(value: unknown): MergeEmissionMode {
  if (value === undefined || value === null) {
    return "ANY_SOURCE";
  }
  if (typeof value === "string") {
    const mode = value.trim();
    if (mode === "") {
      return "ANY_SOURCE";
    }
    if (mode === "ANY_SOURCE" || mode === "PRIMARY_ONLY") {
      return mode;
    }
  }
  throw new Error(messages.persistedEmissionModeInvalid);
}

function emptyMapping() {
  return {
    key: "",
    sourceNodeId: "",
    correlationEnabled: false,
    correlationPrimaryField: "",
    correlationSourceField: "",
  };
}

function deserializeCorrelation(
  value: unknown,
): Pick<
  MergeMappingValues,
  "correlationEnabled" | "correlationPrimaryField" | "correlationSourceField"
> {
  if (value === undefined || value === null) {
    return {
      correlationEnabled: false,
      correlationPrimaryField: "",
      correlationSourceField: "",
    };
  }
  if (typeof value !== "object" || Array.isArray(value)) {
    throw new Error(messages.persistedCorrelationInvalid);
  }
  const record = value as Record<string, unknown>;
  if (
    (record.primaryField !== undefined && typeof record.primaryField !== "string") ||
    (record.sourceField !== undefined && typeof record.sourceField !== "string")
  ) {
    throw new Error(messages.persistedCorrelationInvalid);
  }
  return {
    correlationEnabled: true,
    correlationPrimaryField: typeof record.primaryField === "string" ? record.primaryField : "",
    correlationSourceField: typeof record.sourceField === "string" ? record.sourceField : "",
  };
}

function validateMergeValues(
  values: MergeValues,
  editorContext?: PluginConfigurationEditorContext,
): PluginConfigurationValidationResult {
  const errors: Record<string, string> = {};
  const primaryInputNodeId = values.primaryInputNodeId.trim();
  const connectedNodeIds = new Set(
    editorContext?.incomingInputs.map((input) => input.sourceNodeId) ?? [],
  );
  if (!primaryInputNodeId) {
    errors.primaryInputNodeId = messages.primaryRequired;
  } else if (editorContext && !connectedNodeIds.has(primaryInputNodeId)) {
    errors.primaryInputNodeId = messages.primaryDisconnected;
  }

  const keys = values.mappings.map((mapping) => mapping.key.trim());
  const sources = values.mappings.map((mapping) => mapping.sourceNodeId.trim());
  const primaryKeys = primarySchemaKeys(editorContext, primaryInputNodeId);
  if (editorContext && values.mappings.length > Math.max(0, connectedNodeIds.size - 1)) {
    errors.mappings = messages.tooManyMappings;
  }

  values.mappings.forEach((mapping, index) => {
    const key = mapping.key.trim();
    const sourceNodeId = mapping.sourceNodeId.trim();
    if (!key) {
      errors[`mappings.${index}.key`] = messages.mappingKeyRequired;
    } else if (keys.filter((candidate) => candidate === key).length > 1) {
      errors[`mappings.${index}.key`] = messages.mappingKeyDuplicate;
    } else if (primaryKeys.has(key)) {
      errors[`mappings.${index}.key`] = messages.mappingKeyCollision;
    }
    if (!sourceNodeId) {
      errors[`mappings.${index}.sourceNodeId`] = messages.mappingSourceRequired;
    } else if (sourceNodeId === primaryInputNodeId) {
      errors[`mappings.${index}.sourceNodeId`] = messages.mappingSourceIsPrimary;
    } else if (editorContext && !connectedNodeIds.has(sourceNodeId)) {
      errors[`mappings.${index}.sourceNodeId`] = messages.mappingSourceDisconnected;
    } else if (sources.filter((candidate) => candidate === sourceNodeId).length > 1) {
      errors[`mappings.${index}.sourceNodeId`] = messages.mappingSourceDuplicate;
    }
    if (mapping.correlationEnabled) {
      const primaryField = mapping.correlationPrimaryField.trim();
      const sourceField = mapping.correlationSourceField.trim();
      const sourceInput = editorContext?.incomingInputs.find(
        (input) => input.sourceNodeId === sourceNodeId,
      );
      if (!primaryField) {
        errors[`mappings.${index}.correlation.primaryField`] =
          messages.correlationPrimaryFieldRequired;
      } else if (primaryField.includes(".")) {
        errors[`mappings.${index}.correlation.primaryField`] =
          messages.correlationPrimaryFieldInvalid;
      }
      if (!sourceField) {
        errors[`mappings.${index}.correlation.sourceField`] =
          messages.correlationSourceFieldRequired;
      } else if (!DATABASE_FIELD.test(sourceField)) {
        errors[`mappings.${index}.correlation.sourceField`] =
          messages.correlationSourceFieldInvalid;
      } else if (sourceInput && sourceInput.sourcePluginType !== "core.database-input") {
        errors[`mappings.${index}.correlation.sourceField`] = messages.correlationSourceInvalid;
      }
    }
  });
  return { valid: Object.keys(errors).length === 0, errors };
}

function primarySchemaKeys(
  editorContext: PluginConfigurationEditorContext | undefined,
  primaryInputNodeId: string,
) {
  const input = editorContext?.incomingInputs.find(
    (candidate) => candidate.sourceNodeId === primaryInputNodeId,
  );
  const schema = input?.sourceConfiguration.schema;
  if (typeof schema !== "object" || schema === null || Array.isArray(schema)) {
    return new Set<string>();
  }
  const properties = (schema as Record<string, unknown>).properties;
  if (typeof properties !== "object" || properties === null || Array.isArray(properties)) {
    return new Set<string>();
  }
  return new Set(Object.keys(properties));
}

export const mergeConfigurationDefinition: PluginConfigurationDefinition<MergeValues> = {
  pluginType: "core.merge",
  dialogSize: "large",
  createDefaultConfiguration: () => ({
    primaryInputNodeId: "",
    emissionMode: "ANY_SOURCE",
    mappings: [emptyMapping()],
  }),
  deserialize: (configuration) => {
    const primaryInputNodeId = configuration.primaryInputNodeId;
    if (primaryInputNodeId !== undefined && typeof primaryInputNodeId !== "string") {
      throw new Error(messages.persistedPrimaryInvalid);
    }
    return {
      primaryInputNodeId: typeof primaryInputNodeId === "string" ? primaryInputNodeId : "",
      emissionMode: deserializeEmissionMode(configuration.emissionMode),
      mappings: deserializeMappings(configuration.mappings),
    };
  },
  validate: validateMergeValues,
  serialize: (values) => ({
    primaryInputNodeId: values.primaryInputNodeId.trim(),
    emissionMode: values.emissionMode,
    mappings: values.mappings.map((mapping) => ({
      key: mapping.key.trim(),
      sourceNodeId: mapping.sourceNodeId.trim(),
      ...(mapping.correlationEnabled
        ? {
            correlation: {
              primaryField: mapping.correlationPrimaryField.trim(),
              sourceField: mapping.correlationSourceField.trim(),
            },
          }
        : {}),
    })),
  }),
  Editor: MergeConfigurationEditor,
};
