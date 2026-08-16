import {
  deserializeFormConfiguration,
  serializeFormConfiguration,
  validateFormConfiguration,
} from "@/shared/plugins/form/form-configuration";
import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const messages = pluginMessages.restOutput;

export const REST_OUTPUT_METHODS = ["POST", "PUT", "PATCH"] as const;
type RestMethod = (typeof REST_OUTPUT_METHODS)[number];

export const restOutputFormSchema: FlatFormSchema = {
  fields: [
    {
      key: "url",
      label: messages.urlLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      placeholder: messages.urlPlaceholder,
    },
    {
      key: "method",
      label: messages.methodLabel,
      dataType: "STRING",
      renderType: "DROPDOWN",
      required: true,
      defaultValue: "POST",
      options: REST_OUTPUT_METHODS.map((method) => ({ label: method, value: method })),
    },
  ],
};

function isRestMethod(value: string): value is RestMethod {
  return REST_OUTPUT_METHODS.some((method) => method === value);
}

function isAbsoluteHttpUrl(value: string) {
  try {
    const parsed = new URL(value);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      Boolean(parsed.host) &&
      !parsed.username &&
      !parsed.password
    );
  } catch {
    return false;
  }
}

const restOutputBaseDefinition = createFormPluginDefinition({
  pluginType: "core.rest-output",
  schema: restOutputFormSchema,
  validate: (values) => {
    const generic = validateFormConfiguration(restOutputFormSchema, values);
    const errors = { ...generic.errors };
    const url = typeof values.url === "string" ? values.url.trim() : "";
    const method = typeof values.method === "string" ? values.method.trim().toUpperCase() : "";
    if (!url) errors.url = messages.urlRequired;
    else if (!isAbsoluteHttpUrl(url)) errors.url = messages.urlInvalid;
    if (!isRestMethod(method)) errors.method = messages.methodInvalid;
    return { valid: Object.keys(errors).length === 0, errors };
  },
});

export const restOutputConfigurationDefinition = {
  ...restOutputBaseDefinition,
  deserialize: (configuration: Parameters<typeof restOutputBaseDefinition.deserialize>[0]) => {
    const values = deserializeFormConfiguration(restOutputFormSchema, configuration);
    if (typeof values.method === "string") {
      values.method = values.method.trim().toUpperCase();
    }
    return values;
  },
  serialize: (values: Parameters<typeof restOutputBaseDefinition.serialize>[0]) => {
    const serialized = serializeFormConfiguration(restOutputFormSchema, values);
    if (typeof serialized.method === "string") {
      serialized.method = serialized.method.toUpperCase();
    }
    return serialized;
  },
};
