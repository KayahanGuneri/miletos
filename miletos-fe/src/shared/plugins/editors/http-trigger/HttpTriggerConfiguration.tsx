import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const httpTriggerSchema: FlatFormSchema = {
  fields: [
    {
      key: "method",
      label: pluginMessages.httpTrigger.methodLabel,
      dataType: "STRING",
      renderType: "DROPDOWN",
      required: true,
      defaultValue: "POST",
      options: [
        { label: "GET", value: "GET" },
        { label: "POST", value: "POST" },
        { label: "PUT", value: "PUT" },
        { label: "PATCH", value: "PATCH" },
        { label: "DELETE", value: "DELETE" },
      ],
    },
  ],
};

export const httpTriggerConfigurationDefinition = createFormPluginDefinition({
  pluginType: "core.http-trigger",
  schema: httpTriggerSchema,
});
