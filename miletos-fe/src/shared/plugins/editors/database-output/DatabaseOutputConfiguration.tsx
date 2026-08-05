import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

const messages = pluginMessages.databaseOutput;

interface DatabaseOutputValues {
  schema: string;
  table: string;
}

const IDENTIFIER = /^[A-Za-z_][A-Za-z0-9_]*$/;

function DatabaseOutputConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
}: PluginConfigurationEditorProps<DatabaseOutputValues>) {
  return (
    <section className={styles.pluginConfiguration__formSection}>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.schemaLabel}</Typography>
        <Input
          disabled={disabled}
          value={initialValues.schema}
          aria-invalid={Boolean(validationErrors.schema)}
          onChange={(event) => onChange({ ...initialValues, schema: event.target.value })}
          placeholder={messages.schemaPlaceholder}
        />
        <Typography as="small">{messages.schemaHint}</Typography>
        {validationErrors.schema ? (
          <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
            {validationErrors.schema}
          </Typography>
        ) : null}
      </label>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.tableLabel}</Typography>
        <Input
          disabled={disabled}
          value={initialValues.table}
          aria-invalid={Boolean(validationErrors.table)}
          onChange={(event) => onChange({ ...initialValues, table: event.target.value })}
          placeholder={messages.tablePlaceholder}
        />
        {validationErrors.table ? (
          <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
            {validationErrors.table}
          </Typography>
        ) : null}
      </label>
    </section>
  );
}

export const databaseOutputConfigurationDefinition: PluginConfigurationDefinition<DatabaseOutputValues> =
  {
    pluginType: "core.database-output",
    createDefaultConfiguration: () => ({ schema: "public", table: "" }),
    deserialize: (configuration) => {
      const schema = configuration.schema;
      const table = configuration.table;
      if (schema !== undefined && typeof schema !== "string") {
        throw new Error(messages.persistedSchemaNotAString);
      }
      if (table !== undefined && typeof table !== "string") {
        throw new Error(messages.persistedTableNotAString);
      }
      return {
        schema: schema?.trim() || "public",
        table: table?.trim() ?? "",
      };
    },
    validate: (values) => {
      const errors: Record<string, string> = {};
      const schema = values.schema.trim();
      const table = values.table.trim();
      if (!schema) errors.schema = messages.schemaRequired;
      else if (!IDENTIFIER.test(schema)) errors.schema = messages.schemaInvalid;
      if (!table) errors.table = messages.tableRequired;
      else if (!IDENTIFIER.test(table)) errors.table = messages.tableInvalid;
      return { valid: Object.keys(errors).length === 0, errors };
    },
    serialize: (values) => ({
      schema: values.schema.trim(),
      table: values.table.trim(),
    }),
    Editor: DatabaseOutputConfigurationEditor,
  };
