import type { ComponentType } from "react";
import type {
  PluginCategory,
  PluginConfigurationValue,
} from "@/shared/plugins/contracts/plugin-configuration-types";

export interface PluginConfiguration {
  [key: string]: PluginConfigurationValue;
}

export interface PluginPort {
  name: string;
  displayName: string;
  description: string;
}

export interface PluginDescriptor {
  type: string;
  version: string;
  displayName: string;
  description: string;
  category: PluginCategory;
  inputPorts: PluginPort[];
  outputPorts: PluginPort[];
}

export interface PluginConfigurationValidationResult {
  valid: boolean;
  errors: Record<string, string>;
}

export interface PluginConfigurationEditorProps<TValues> {
  initialValues: TValues;
  disabled: boolean;
  validationErrors: Record<string, string>;
  onChange: (values: TValues) => void;
}

export interface PluginConfigurationDefinition<TValues> {
  pluginType: string;
  dialogDescription?: string;
  dialogSize?: "default" | "large";
  createDefaultConfiguration: () => PluginConfiguration;
  deserialize: (configuration: PluginConfiguration) => TValues;
  validate: (values: TValues) => PluginConfigurationValidationResult;
  serialize: (values: TValues) => PluginConfiguration;
  Editor: ComponentType<PluginConfigurationEditorProps<TValues>>;
}

export interface RegisteredPluginConfigurationDefinition {
  pluginType: string;
  dialogDescription?: string;
  dialogSize?: "default" | "large";
  createDefaultConfiguration: () => PluginConfiguration;
  deserialize: (configuration: PluginConfiguration) => unknown;
  validate: (values: unknown) => PluginConfigurationValidationResult;
  serialize: (values: unknown) => PluginConfiguration;
  Editor: ComponentType<PluginConfigurationEditorProps<unknown>>;
}
