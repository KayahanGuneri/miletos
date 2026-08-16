import type { FormConfiguration, FormField } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const messages = pluginMessages.databaseConnection;

export const DATABASE_SSL_MODES = [
  "disable",
  "allow",
  "prefer",
  "require",
  "verify-ca",
  "verify-full",
] as const;

export const databaseConnectionFields: FormField[] = [
  {
    key: "databaseHost",
    label: messages.hostLabel,
    dataType: "STRING",
    renderType: "INPUT",
    required: true,
    defaultValue: "",
    placeholder: messages.hostPlaceholder,
  },
  {
    key: "databasePort",
    label: messages.portLabel,
    dataType: "NUMBER",
    renderType: "INPUT",
    required: true,
    defaultValue: 5432,
    min: 1,
    max: 65535,
  },
  {
    key: "databaseName",
    label: messages.nameLabel,
    dataType: "STRING",
    renderType: "INPUT",
    required: true,
    defaultValue: "",
    placeholder: messages.namePlaceholder,
  },
  {
    key: "databaseUsername",
    label: messages.usernameLabel,
    dataType: "STRING",
    renderType: "INPUT",
    required: true,
    defaultValue: "",
  },
  {
    key: "databasePassword",
    label: messages.passwordLabel,
    dataType: "STRING",
    renderType: "INPUT",
    inputType: "password",
    required: false,
    defaultValue: "",
  },
  {
    key: "databaseSslMode",
    label: messages.sslModeLabel,
    dataType: "STRING",
    renderType: "DROPDOWN",
    required: true,
    defaultValue: "disable",
    options: DATABASE_SSL_MODES.map((sslMode) => ({ label: sslMode, value: sslMode })),
  },
];

export function validateDatabaseConnection(values: FormConfiguration) {
  const errors: Record<string, string> = {};
  const host = typeof values.databaseHost === "string" ? values.databaseHost.trim() : "";
  const databaseName = typeof values.databaseName === "string" ? values.databaseName.trim() : "";
  const username =
    typeof values.databaseUsername === "string" ? values.databaseUsername.trim() : "";
  const port = values.databasePort;
  const sslMode = values.databaseSslMode;

  if (!host) errors.databaseHost = messages.hostRequired;
  if (typeof port !== "number" || !Number.isInteger(port) || port < 1 || port > 65535) {
    errors.databasePort = messages.portInvalid;
  }
  if (!databaseName) errors.databaseName = messages.nameRequired;
  if (!username) errors.databaseUsername = messages.usernameRequired;
  if (!DATABASE_SSL_MODES.some((candidate) => candidate === sslMode)) {
    errors.databaseSslMode = messages.sslModeInvalid;
  }
  return errors;
}
