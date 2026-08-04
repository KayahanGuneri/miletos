import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfiguration,
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
  PluginConfigurationValue,
} from "@/shared/plugins/contracts/plugin-configuration";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

type StaticValueType = "unset" | "string" | "number" | "boolean" | "null" | "structured";

interface StaticInputValues {
  valueType: StaticValueType;
  stringValue: string;
  numberValue: string;
  booleanValue: boolean;
  structuredValue?: PluginConfigurationValue;
}

function StaticInputConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
}: PluginConfigurationEditorProps<StaticInputValues>) {
  return (
    <section className={styles.pluginConfiguration__formSection}>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">Value type</Typography>
        <select
          disabled={disabled}
          value={initialValues.valueType}
          onChange={(event) =>
            onChange({ ...initialValues, valueType: event.target.value as StaticValueType })
          }
        >
          <option value="unset">Select a value type</option>
          <option value="string">Text</option>
          <option value="number">Number</option>
          <option value="boolean">Boolean</option>
          <option value="null">Null</option>
          {initialValues.valueType === "structured" ? (
            <option value="structured">Existing structured value</option>
          ) : null}
        </select>
      </label>

      {initialValues.valueType === "string" ? (
        <label className={styles.pluginConfiguration__field}>
          <Typography as="span">Text value</Typography>
          <input
            disabled={disabled}
            value={initialValues.stringValue}
            onChange={(event) => onChange({ ...initialValues, stringValue: event.target.value })}
          />
        </label>
      ) : null}

      {initialValues.valueType === "number" ? (
        <label className={styles.pluginConfiguration__field}>
          <Typography as="span">Number value</Typography>
          <input
            disabled={disabled}
            inputMode="decimal"
            value={initialValues.numberValue}
            onChange={(event) => onChange({ ...initialValues, numberValue: event.target.value })}
          />
        </label>
      ) : null}

      {initialValues.valueType === "boolean" ? (
        <label className={styles.pluginConfiguration__field}>
          <Typography as="span">Boolean value</Typography>
          <select
            disabled={disabled}
            value={String(initialValues.booleanValue)}
            onChange={(event) =>
              onChange({ ...initialValues, booleanValue: event.target.value === "true" })
            }
          >
            <option value="true">True</option>
            <option value="false">False</option>
          </select>
        </label>
      ) : null}

      {initialValues.valueType === "null" ? (
        <Typography as="p" className={styles.pluginConfiguration__notice}>
          This node will emit a JSON null value.
        </Typography>
      ) : null}

      {initialValues.valueType === "structured" ? (
        <Typography as="p" className={styles.pluginConfiguration__notice}>
          The existing object or array value will be preserved. Select another value type to replace
          it.
        </Typography>
      ) : null}

      {validationErrors.value ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {validationErrors.value}
        </Typography>
      ) : null}
    </section>
  );
}

function deserializeValue(value: PluginConfigurationValue): StaticInputValues {
  if (value === null) {
    return { valueType: "null", stringValue: "", numberValue: "", booleanValue: false };
  }
  if (typeof value === "string") {
    return { valueType: "string", stringValue: value, numberValue: "", booleanValue: false };
  }
  if (typeof value === "number") {
    return {
      valueType: "number",
      stringValue: "",
      numberValue: String(value),
      booleanValue: false,
    };
  }
  if (typeof value === "boolean") {
    return { valueType: "boolean", stringValue: "", numberValue: "", booleanValue: value };
  }
  return {
    valueType: "structured",
    stringValue: "",
    numberValue: "",
    booleanValue: false,
    structuredValue: value,
  };
}

export const staticInputConfigurationDefinition: PluginConfigurationDefinition<StaticInputValues> =
  {
    pluginType: "core.static-input",
    createDefaultConfiguration: () => ({}),
    deserialize: (configuration) => {
      if (!("value" in configuration)) {
        return { valueType: "unset", stringValue: "", numberValue: "", booleanValue: false };
      }
      return deserializeValue(configuration.value);
    },
    validate: (values) => {
      const numberValid =
        values.valueType !== "number" ||
        (values.numberValue.trim() !== "" && Number.isFinite(Number(values.numberValue)));
      const valid = values.valueType !== "unset" && numberValid;
      const errors: Record<string, string> = valid
        ? {}
        : {
            value:
              values.valueType === "number" ? "Enter a finite number." : "Select a value type.",
          };
      return {
        valid,
        errors,
      };
    },
    serialize: (values): PluginConfiguration => {
      switch (values.valueType) {
        case "string":
          return { value: values.stringValue };
        case "number":
          return { value: Number(values.numberValue) };
        case "boolean":
          return { value: values.booleanValue };
        case "null":
          return { value: null };
        case "structured":
          return { value: values.structuredValue ?? null };
        case "unset":
          return {};
      }
    },
    Editor: StaticInputConfigurationEditor,
  };
