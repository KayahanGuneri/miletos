import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

const messages = pluginMessages.restOutput;

const REST_METHODS = ["POST", "PUT", "PATCH"] as const;
type RestMethod = (typeof REST_METHODS)[number];

interface RestOutputValues {
  url: string;
  method: string;
}

function RestOutputConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
}: PluginConfigurationEditorProps<RestOutputValues>) {
  return (
    <section className={styles.pluginConfiguration__formSection}>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.urlLabel}</Typography>
        <Input
          disabled={disabled}
          value={initialValues.url}
          aria-invalid={Boolean(validationErrors.url)}
          onChange={(event) => onChange({ ...initialValues, url: event.target.value })}
          placeholder={messages.urlPlaceholder}
        />
        {validationErrors.url ? (
          <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
            {validationErrors.url}
          </Typography>
        ) : null}
      </label>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.methodLabel}</Typography>
        <select
          disabled={disabled}
          value={initialValues.method}
          aria-invalid={Boolean(validationErrors.method)}
          onChange={(event) => onChange({ ...initialValues, method: event.target.value })}
        >
          {REST_METHODS.map((method) => (
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
    </section>
  );
}

function isRestMethod(value: string): value is RestMethod {
  return REST_METHODS.some((method) => method === value);
}

function isAbsoluteHttpUrl(value: string) {
  try {
    const parsed = new URL(value);
    return (parsed.protocol === "http:" || parsed.protocol === "https:") && Boolean(parsed.host);
  } catch {
    return false;
  }
}

export const restOutputConfigurationDefinition: PluginConfigurationDefinition<RestOutputValues> = {
  pluginType: "core.rest-output",
  createDefaultConfiguration: () => ({ url: "", method: "POST" }),
  deserialize: (configuration) => {
    const url = configuration.url;
    const method = configuration.method;
    if (url !== undefined && typeof url !== "string") {
      throw new Error(messages.persistedUrlNotAString);
    }
    if (method !== undefined && typeof method !== "string") {
      throw new Error(messages.persistedMethodNotAString);
    }
    const normalizedMethod = method?.trim().toUpperCase() || "POST";
    return {
      url: url?.trim() ?? "",
      method: isRestMethod(normalizedMethod) ? normalizedMethod : "POST",
    };
  },
  validate: (values) => {
    const errors: Record<string, string> = {};
    const url = values.url.trim();
    if (!url) errors.url = messages.urlRequired;
    else if (!isAbsoluteHttpUrl(url)) errors.url = messages.urlInvalid;
    if (!isRestMethod(values.method)) errors.method = messages.methodInvalid;
    return { valid: Object.keys(errors).length === 0, errors };
  },
  serialize: (values) => ({
    url: values.url.trim(),
    method: values.method.trim().toUpperCase() || "POST",
  }),
  Editor: RestOutputConfigurationEditor,
};
