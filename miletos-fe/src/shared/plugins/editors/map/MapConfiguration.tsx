import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
  PluginConfigurationValidationResult,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

const messages = pluginMessages.map;

interface MapMappingValues {
  sourceField: string;
  targetField: string;
}

type MapLiteralType = "STRING" | "NUMBER" | "BOOLEAN" | "NULL";

interface MapAssignmentValues {
  targetField: string;
  valueType: MapLiteralType;
  value: string;
}

interface MapValues {
  mappings: MapMappingValues[];
  assignments: MapAssignmentValues[];
}

function MapConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
}: PluginConfigurationEditorProps<MapValues>) {
  return (
    <section className={styles.pluginConfiguration__formSection}>
      <Typography as="span">{messages.mappingsLabel}</Typography>
      {initialValues.mappings.map((mapping, index) => (
        <Box key={index} className={styles.pluginConfiguration__formSection}>
          <label className={styles.pluginConfiguration__field}>
            <Typography as="span">{messages.sourceFieldLabel}</Typography>
            <Input
              disabled={disabled}
              value={mapping.sourceField}
              aria-invalid={Boolean(validationErrors[`mappings.${index}.sourceField`])}
              onChange={(event) =>
                onChange(updateMapping(initialValues, index, { sourceField: event.target.value }))
              }
            />
            {validationErrors[`mappings.${index}.sourceField`] ? (
              <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
                {validationErrors[`mappings.${index}.sourceField`]}
              </Typography>
            ) : null}
          </label>
          <label className={styles.pluginConfiguration__field}>
            <Typography as="span">{messages.targetFieldLabel}</Typography>
            <Input
              disabled={disabled}
              value={mapping.targetField}
              aria-invalid={Boolean(validationErrors[`mappings.${index}.targetField`])}
              onChange={(event) =>
                onChange(updateMapping(initialValues, index, { targetField: event.target.value }))
              }
            />
            {validationErrors[`mappings.${index}.targetField`] ? (
              <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
                {validationErrors[`mappings.${index}.targetField`]}
              </Typography>
            ) : null}
          </label>
          <Button
            type="button"
            variant={ButtonVariant.Ghost}
            disabled={disabled}
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
        disabled={disabled}
        onClick={() =>
          onChange({
            ...initialValues,
            mappings: [...initialValues.mappings, { sourceField: "", targetField: "" }],
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

      <Typography as="span">{messages.assignmentsLabel}</Typography>
      {initialValues.assignments.map((assignment, index) => (
        <Box key={index} className={styles.pluginConfiguration__formSection}>
          <label className={styles.pluginConfiguration__field}>
            <Typography as="span">{messages.targetFieldLabel}</Typography>
            <Input
              disabled={disabled}
              value={assignment.targetField}
              aria-invalid={Boolean(validationErrors[`assignments.${index}.targetField`])}
              onChange={(event) =>
                onChange(
                  updateAssignment(initialValues, index, { targetField: event.target.value }),
                )
              }
            />
            {validationErrors[`assignments.${index}.targetField`] ? (
              <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
                {validationErrors[`assignments.${index}.targetField`]}
              </Typography>
            ) : null}
          </label>
          <label className={styles.pluginConfiguration__field}>
            <Typography as="span">{messages.literalTypeLabel}</Typography>
            <select
              disabled={disabled}
              value={assignment.valueType}
              onChange={(event) =>
                onChange(
                  updateAssignment(initialValues, index, {
                    valueType: event.target.value as MapLiteralType,
                    value:
                      event.target.value === "NULL"
                        ? ""
                        : event.target.value === "BOOLEAN" &&
                            assignment.value !== "true" &&
                            assignment.value !== "false"
                          ? "true"
                          : assignment.value,
                  }),
                )
              }
            >
              <option value="STRING">{messages.literalTypeString}</option>
              <option value="NUMBER">{messages.literalTypeNumber}</option>
              <option value="BOOLEAN">{messages.literalTypeBoolean}</option>
              <option value="NULL">{messages.literalTypeNull}</option>
            </select>
          </label>
          {assignment.valueType === "BOOLEAN" ? (
            <label className={styles.pluginConfiguration__field}>
              <Typography as="span">{messages.literalValueLabel}</Typography>
              <select
                disabled={disabled}
                value={assignment.value}
                aria-invalid={Boolean(validationErrors[`assignments.${index}.value`])}
                onChange={(event) =>
                  onChange(updateAssignment(initialValues, index, { value: event.target.value }))
                }
              >
                <option value="true">true</option>
                <option value="false">false</option>
              </select>
            </label>
          ) : assignment.valueType !== "NULL" ? (
            <label className={styles.pluginConfiguration__field}>
              <Typography as="span">{messages.literalValueLabel}</Typography>
              <Input
                type={assignment.valueType === "NUMBER" ? "number" : "text"}
                disabled={disabled}
                value={assignment.value}
                aria-invalid={Boolean(validationErrors[`assignments.${index}.value`])}
                onChange={(event) =>
                  onChange(updateAssignment(initialValues, index, { value: event.target.value }))
                }
              />
              {validationErrors[`assignments.${index}.value`] ? (
                <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
                  {validationErrors[`assignments.${index}.value`]}
                </Typography>
              ) : null}
            </label>
          ) : null}
          <Button
            type="button"
            variant={ButtonVariant.Ghost}
            disabled={disabled}
            onClick={() =>
              onChange({
                ...initialValues,
                assignments: initialValues.assignments.filter(
                  (_, itemIndex) => itemIndex !== index,
                ),
              })
            }
          >
            {messages.removeAssignment}
          </Button>
        </Box>
      ))}
      <Button
        type="button"
        variant={ButtonVariant.Secondary}
        disabled={disabled}
        onClick={() =>
          onChange({
            ...initialValues,
            assignments: [
              ...initialValues.assignments,
              { targetField: "", valueType: "STRING", value: "" },
            ],
          })
        }
      >
        {messages.addAssignment}
      </Button>
      {validationErrors.assignments ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {validationErrors.assignments}
        </Typography>
      ) : null}
    </section>
  );
}

function updateMapping(
  values: MapValues,
  index: number,
  patch: Partial<MapMappingValues>,
): MapValues {
  return {
    ...values,
    mappings: values.mappings.map((mapping, itemIndex) =>
      itemIndex === index ? { ...mapping, ...patch } : mapping,
    ),
  };
}

function updateAssignment(
  values: MapValues,
  index: number,
  patch: Partial<MapAssignmentValues>,
): MapValues {
  return {
    ...values,
    assignments: values.assignments.map((assignment, itemIndex) =>
      itemIndex === index ? { ...assignment, ...patch } : assignment,
    ),
  };
}

function deserializeMappings(value: unknown): MapMappingValues[] {
  if (value === undefined || value === null) {
    return [];
  }
  if (!Array.isArray(value)) {
    throw new Error(messages.persistedMappingsInvalid);
  }
  if (value.length === 0) {
    return [];
  }
  return value.map((item) => {
    if (typeof item !== "object" || item === null || Array.isArray(item)) {
      throw new Error(messages.persistedMappingsInvalid);
    }
    const record = item as Record<string, unknown>;
    if (record.sourceField !== undefined && typeof record.sourceField !== "string") {
      throw new Error(messages.persistedSourceInvalid);
    }
    if (record.targetField !== undefined && typeof record.targetField !== "string") {
      throw new Error(messages.persistedTargetInvalid);
    }
    return {
      sourceField: typeof record.sourceField === "string" ? record.sourceField : "",
      targetField: typeof record.targetField === "string" ? record.targetField : "",
    };
  });
}

function deserializeAssignments(value: unknown): MapAssignmentValues[] {
  if (value === undefined || value === null) {
    return [];
  }
  if (!Array.isArray(value)) {
    throw new Error(messages.persistedAssignmentsInvalid);
  }
  return value.map((item) => {
    if (typeof item !== "object" || item === null || Array.isArray(item)) {
      throw new Error(messages.persistedAssignmentsInvalid);
    }
    const record = item as Record<string, unknown>;
    if (record.targetField !== undefined && typeof record.targetField !== "string") {
      throw new Error(messages.persistedAssignmentTargetInvalid);
    }
    const literal = deserializeLiteral(record.value);
    return {
      targetField: typeof record.targetField === "string" ? record.targetField : "",
      ...literal,
    };
  });
}

function deserializeLiteral(value: unknown): Pick<MapAssignmentValues, "valueType" | "value"> {
  if (value === null) return { valueType: "NULL", value: "" };
  if (typeof value === "string") return { valueType: "STRING", value };
  if (typeof value === "boolean") {
    return { valueType: "BOOLEAN", value: value ? "true" : "false" };
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    return { valueType: "NUMBER", value: String(value) };
  }
  throw new Error(messages.persistedAssignmentValueInvalid);
}

function validateMapValues(values: MapValues): PluginConfigurationValidationResult {
  const errors: Record<string, string> = {};
  const sources = values.mappings.map((mapping) => mapping.sourceField.trim());
  const targets = [
    ...values.mappings.map((mapping) => mapping.targetField.trim()),
    ...values.assignments.map((assignment) => assignment.targetField.trim()),
  ];
  const filled = values.mappings.filter(
    (mapping) => mapping.sourceField.trim() !== "" || mapping.targetField.trim() !== "",
  );

  if (filled.length === 0 && values.assignments.length === 0) {
    errors.mappings = messages.operationsRequired;
  }

  values.mappings.forEach((mapping, index) => {
    const source = mapping.sourceField.trim();
    const target = mapping.targetField.trim();
    if (!source) {
      errors[`mappings.${index}.sourceField`] = messages.sourceRequired;
    } else if (sources.filter((candidate) => candidate === source).length > 1) {
      errors[`mappings.${index}.sourceField`] = messages.sourceDuplicate;
    }
    if (!target) {
      errors[`mappings.${index}.targetField`] = messages.targetRequired;
    } else if (targets.filter((candidate) => candidate === target).length > 1) {
      errors[`mappings.${index}.targetField`] = messages.targetDuplicate;
    }
  });

  values.assignments.forEach((assignment, index) => {
    const target = assignment.targetField.trim();
    if (!target) {
      errors[`assignments.${index}.targetField`] = messages.targetRequired;
    } else if (targets.filter((candidate) => candidate === target).length > 1) {
      errors[`assignments.${index}.targetField`] = messages.targetDuplicate;
    }
    if (
      assignment.valueType === "NUMBER" &&
      (assignment.value.trim() === "" || !Number.isFinite(Number(assignment.value)))
    ) {
      errors[`assignments.${index}.value`] = messages.literalNumberInvalid;
    }
  });

  return { valid: Object.keys(errors).length === 0, errors };
}

export const mapConfigurationDefinition: PluginConfigurationDefinition<MapValues> = {
  pluginType: "core.map",
  createDefaultConfiguration: () => ({
    mappings: [{ sourceField: "", targetField: "" }],
    assignments: [],
  }),
  deserialize: (configuration) => ({
    mappings: deserializeMappings(configuration.mappings),
    assignments: deserializeAssignments(configuration.assignments),
  }),
  validate: validateMapValues,
  serialize: (values) => {
    const configuration = {
      mappings: values.mappings.map((mapping) => ({
        sourceField: mapping.sourceField.trim(),
        targetField: mapping.targetField.trim(),
      })),
      assignments: values.assignments.map((assignment) => ({
        targetField: assignment.targetField.trim(),
        value: serializeLiteral(assignment),
      })),
    };
    return configuration;
  },
  Editor: MapConfigurationEditor,
};

function serializeLiteral(assignment: MapAssignmentValues): string | number | boolean | null {
  switch (assignment.valueType) {
    case "NUMBER":
      return Number(assignment.value);
    case "BOOLEAN":
      return assignment.value === "true";
    case "NULL":
      return null;
    default:
      return assignment.value;
  }
}
