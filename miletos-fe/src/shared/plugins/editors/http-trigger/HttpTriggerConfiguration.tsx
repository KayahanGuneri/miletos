import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import { validateFormConfiguration } from "@/shared/plugins/form/form-configuration";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";
import { withSchemaConfiguration } from "@/shared/plugins/editors/schema/with-schema-configuration";
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

const httpTriggerBaseDefinition = createFormPluginDefinition({
  pluginType: "core.http-trigger",
  schema: httpTriggerSchema,
  dialogSize: "large",
  validate: (values) => validateFormConfiguration(httpTriggerSchema, values),
});

export const httpTriggerConfigurationDefinition = withSchemaConfiguration({
  definition: httpTriggerBaseDefinition,
  field: { key: "schema", label: pluginMessages.httpTrigger.schemaLabel },
});
