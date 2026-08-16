import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import {
  databaseConnectionFields,
  validateDatabaseConnection,
} from "@/shared/plugins/editors/database/database-connection";
import { withSchemaConfiguration } from "@/shared/plugins/editors/schema/with-schema-configuration";
import { validateFormConfiguration } from "@/shared/plugins/form/form-configuration";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const messages = pluginMessages.databaseInput;

const databaseInputSchema: FlatFormSchema = {
  fields: [
    ...databaseConnectionFields,
    {
      key: "query",
      label: messages.queryLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: messages.queryPlaceholder,
    },
    {
      key: "cursorColumn",
      label: messages.cursorColumnLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: false,
      defaultValue: "",
      placeholder: messages.cursorColumnPlaceholder,
    },
  ],
};

const databaseInputBaseDefinition = createFormPluginDefinition({
  pluginType: "core.database-input",
  schema: databaseInputSchema,
  dialogSize: "large",
  validate: (values) => {
    const generic = validateFormConfiguration(databaseInputSchema, values);
    const errors = { ...generic.errors, ...validateDatabaseConnection(values) };
    return { valid: Object.keys(errors).length === 0, errors };
  },
});

export const databaseInputConfigurationDefinition = withSchemaConfiguration({
  definition: databaseInputBaseDefinition,
  field: { key: "schema", label: messages.schemaLabel },
});
