import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import { withSchemaConfiguration } from "@/shared/plugins/editors/schema/with-schema-configuration";
import { validateFormConfiguration } from "@/shared/plugins/form/form-configuration";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const messages = pluginMessages.excelInput;

const excelInputSchema: FlatFormSchema = {
  fields: [
    {
      key: "sourceType",
      label: messages.sourceTypeLabel,
      dataType: "STRING",
      renderType: "DROPDOWN",
      required: true,
      defaultValue: "LOCAL",
      options: [
        { label: messages.sourceTypeLocal, value: "LOCAL" },
        { label: messages.sourceTypeSftp, value: "SFTP" },
      ],
    },
    {
      key: "fileName",
      label: messages.localUploadLabel,
      dataType: "STRING",
      renderType: "INPUT",
      inputType: "file",
      required: true,
      defaultValue: "",
      accept: ".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
      when: (data) => data.sourceType === "LOCAL",
    },
    {
      key: "fileName",
      label: messages.fileNameLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: messages.fileNamePlaceholder,
      when: (data) => data.sourceType === "SFTP",
    },
    {
      key: "sheetName",
      label: messages.sheetNameLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: false,
      defaultValue: "",
      placeholder: messages.sheetNamePlaceholder,
    },
    {
      key: "sftpHost",
      label: messages.sftpHostLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: "localhost",
      when: (data) => data.sourceType === "SFTP",
    },
    {
      key: "sftpPort",
      label: messages.sftpPortLabel,
      dataType: "NUMBER",
      renderType: "INPUT",
      required: true,
      defaultValue: 22,
      min: 1,
      max: 65535,
      when: (data) => data.sourceType === "SFTP",
    },
    {
      key: "sftpUsername",
      label: messages.sftpUsernameLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      when: (data) => data.sourceType === "SFTP",
    },
    {
      key: "sftpPassword",
      label: messages.sftpPasswordLabel,
      dataType: "STRING",
      renderType: "INPUT",
      inputType: "password",
      required: false,
      defaultValue: "",
      when: (data) => data.sourceType === "SFTP",
    },
    {
      key: "sftpBaseDirectory",
      label: messages.sftpBaseDirectoryLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: "/upload",
      when: (data) => data.sourceType === "SFTP",
    },
    {
      key: "sftpHostKeySha256",
      label: messages.sftpHostKeyLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      defaultValue: "",
      placeholder: "SHA256:...",
      when: (data) => data.sourceType === "SFTP",
    },
  ],
};

const excelInputBaseDefinition = createFormPluginDefinition({
  pluginType: "core.excel-input",
  schema: excelInputSchema,
  dialogSize: "large",
  validate: (values) => validateFormConfiguration(excelInputSchema, values),
});

export const excelInputConfigurationDefinition = withSchemaConfiguration({
  definition: excelInputBaseDefinition,
  field: { key: "schema", label: messages.schemaLabel },
});
