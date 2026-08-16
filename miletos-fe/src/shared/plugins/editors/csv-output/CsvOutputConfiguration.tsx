import { validateFormConfiguration } from "@/shared/plugins/form/form-configuration";
import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const messages = pluginMessages.csvOutput;
const sftpMessages = pluginMessages.fileInput;

function isSafeCsvFileName(fileName: string) {
  if (!fileName || fileName !== fileName.trim()) {
    return false;
  }
  if (fileName.includes("..") || /[/\\:]/.test(fileName)) {
    return false;
  }
  return fileName.toLowerCase().endsWith(".csv");
}

export const csvOutputFormSchema: FlatFormSchema = {
  fields: [
    {
      key: "fileName",
      label: messages.fileNameLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "result.csv",
      placeholder: messages.fileNamePlaceholder,
    },
    {
      key: "sftpHost",
      label: sftpMessages.sftpHostLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: "localhost",
    },
    {
      key: "sftpPort",
      label: sftpMessages.sftpPortLabel,
      dataType: "NUMBER",
      renderType: "INPUT",
      required: true,
      defaultValue: 22,
      min: 1,
      max: 65535,
    },
    {
      key: "sftpUsername",
      label: sftpMessages.sftpUsernameLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
    },
    {
      key: "sftpPassword",
      label: sftpMessages.sftpPasswordLabel,
      dataType: "STRING",
      renderType: "INPUT",
      inputType: "password",
      required: false,
      defaultValue: "",
    },
    {
      key: "sftpBaseDirectory",
      label: sftpMessages.sftpBaseDirectoryLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: "/upload",
    },
    {
      key: "sftpHostKeySha256",
      label: sftpMessages.sftpHostKeyLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: "SHA256:...",
    },
  ],
};

const csvOutputBaseDefinition = createFormPluginDefinition({
  pluginType: "core.csv-output",
  schema: csvOutputFormSchema,
  dialogSize: "large",
  validate: (values) => {
    const generic = validateFormConfiguration(csvOutputFormSchema, values);
    const fileName = typeof values.fileName === "string" ? values.fileName.trim() : "";
    const errors = { ...generic.errors };
    if (!isSafeCsvFileName(fileName)) {
      errors.fileName = messages.fileNameInvalid;
    }
    if (
      typeof values.sftpPort !== "number" ||
      !Number.isInteger(values.sftpPort) ||
      values.sftpPort < 1 ||
      values.sftpPort > 65535
    ) {
      errors.sftpPort = messages.sftpPortInvalid;
    }
    return { valid: Object.keys(errors).length === 0, errors };
  },
});

export const csvOutputConfigurationDefinition = {
  ...csvOutputBaseDefinition,
  createDefaultConfiguration: () => ({
    ...csvOutputBaseDefinition.createDefaultConfiguration(),
    destinationType: "SFTP",
  }),
  deserialize: (configuration: Parameters<typeof csvOutputBaseDefinition.deserialize>[0]) => {
    const values = csvOutputBaseDefinition.deserialize(configuration);
    values.destinationType = "SFTP";
    return values;
  },
  serialize: (values: Parameters<typeof csvOutputBaseDefinition.serialize>[0]) => ({
    ...csvOutputBaseDefinition.serialize(values),
    destinationType: "SFTP",
  }),
};
