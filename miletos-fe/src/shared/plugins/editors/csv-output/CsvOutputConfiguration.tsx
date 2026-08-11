import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

const messages = pluginMessages.csvOutput;

interface CsvOutputValues {
  fileName: string;
}

function isSafeCsvFileName(fileName: string) {
  if (!fileName || fileName !== fileName.trim()) {
    return false;
  }
  if (fileName.includes("..") || /[/\\:]/.test(fileName)) {
    return false;
  }
  return fileName.toLowerCase().endsWith(".csv");
}

function CsvOutputConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
}: PluginConfigurationEditorProps<CsvOutputValues>) {
  return (
    <label className={styles.pluginConfiguration__field}>
      <Typography as="span">{messages.fileNameLabel}</Typography>
      <Input
        disabled={disabled}
        value={initialValues.fileName}
        aria-invalid={Boolean(validationErrors.fileName)}
        onChange={(event) => onChange({ fileName: event.target.value })}
        placeholder={messages.fileNamePlaceholder}
      />
      <Typography as="small">{messages.fileNameHint}</Typography>
      {validationErrors.fileName ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {validationErrors.fileName}
        </Typography>
      ) : null}
    </label>
  );
}

export const csvOutputConfigurationDefinition: PluginConfigurationDefinition<CsvOutputValues> = {
  pluginType: "core.csv-output",
  createDefaultConfiguration: () => ({ fileName: "result.csv" }),
  deserialize: (configuration) => {
    const fileName = configuration.fileName;
    if (fileName !== undefined && typeof fileName !== "string") {
      throw new Error(messages.persistedFileNameNotAString);
    }
    return { fileName: fileName?.trim() || "result.csv" };
  },
  validate: (values) => {
    const fileName = values.fileName.trim();
    const errors: Record<string, string> = {};
    if (!isSafeCsvFileName(fileName)) {
      errors.fileName = messages.fileNameInvalid;
    }
    return { valid: Object.keys(errors).length === 0, errors };
  },
  serialize: (values) => ({ fileName: values.fileName.trim() }),
  Editor: CsvOutputConfigurationEditor,
};
