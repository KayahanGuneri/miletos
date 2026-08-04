import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
} from "@/shared/plugins/contracts/plugin-configuration";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

const HTTP_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"] as const;
type HttpMethod = (typeof HTTP_METHODS)[number];

interface HttpTriggerValues {
  method: string;
}

function HttpTriggerConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
}: PluginConfigurationEditorProps<HttpTriggerValues>) {
  return (
    <label className={styles.pluginConfiguration__field}>
      <Typography as="span">HTTP method</Typography>
      <select
        disabled={disabled}
        value={initialValues.method}
        onChange={(event) => onChange({ ...initialValues, method: event.target.value })}
      >
        <option value="">Select a method</option>
        {HTTP_METHODS.map((method) => (
          <option key={method} value={method}>
            {method}
          </option>
        ))}
      </select>
      {validationErrors.method ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {validationErrors.method}
        </Typography>
      ) : null}
    </label>
  );
}

function isHttpMethod(value: string): value is HttpMethod {
  return HTTP_METHODS.some((method) => method === value);
}

export const httpTriggerConfigurationDefinition: PluginConfigurationDefinition<HttpTriggerValues> =
  {
    pluginType: "core.http-trigger",
    createDefaultConfiguration: () => ({}),
    deserialize: (configuration) => {
      const method = configuration.method;
      if (method === undefined) {
        return { method: "" };
      }
      if (typeof method !== "string") {
        throw new Error("The persisted HTTP method is not a string.");
      }
      return { method: method.trim().toUpperCase() };
    },
    validate: (values) => {
      const valid = isHttpMethod(values.method);
      const errors: Record<string, string> = valid
        ? {}
        : { method: "Select GET, POST, PUT, PATCH, or DELETE." };
      return {
        valid,
        errors,
      };
    },
    serialize: (values) => ({ method: values.method }),
    Editor: HttpTriggerConfigurationEditor,
  };
