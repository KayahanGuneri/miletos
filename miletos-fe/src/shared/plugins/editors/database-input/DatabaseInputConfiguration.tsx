import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const messages = pluginMessages.databaseInput;

const databaseInputSchema: FlatFormSchema = {
  fields: [
    {
      key: "query",
      label: messages.queryLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: messages.queryPlaceholder,
    },
  ],
};

export const databaseInputConfigurationDefinition = createFormPluginDefinition({
  pluginType: "core.database-input",
  schema: databaseInputSchema,
});
