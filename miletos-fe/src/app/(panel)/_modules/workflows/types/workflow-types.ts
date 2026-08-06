import type {
  PluginConfiguration,
  PluginDescriptor,
  PluginPort as DescriptorPort,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import type { PluginConfigurationValue } from "@/shared/plugins/contracts/plugin-configuration-types";

export type WorkflowStatus = "DRAFT" | "ACTIVE" | "ARCHIVED";

export type JsonValue = PluginConfigurationValue;
export type JsonObject = PluginConfiguration;

export type PluginPort = DescriptorPort;

export type WorkflowPlugin = PluginDescriptor;
