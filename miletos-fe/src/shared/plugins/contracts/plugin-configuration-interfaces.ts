import type { ComponentType } from "react";

import type {
  PluginCategory,
  PluginConfigurationValue,
} from "@/shared/plugins/contracts/plugin-configuration-types";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";

export interface PluginConfiguration {
  [key: string]: PluginConfigurationValue;
}

export interface PluginPort {
  name: string;
  displayName: string;
  description: string;
  edgeConstraint?: PluginEdgeConstraint | null;
}

export interface PluginEdgeConstraint {
  minimum: number;
  maximum?: number | null;
  unlimited: boolean;
}

export type ConnectionRestrictionSelector = "PRIMARY" | "NOT_PRIMARY" | "POSITION";

export interface PluginConnectionRestriction {
  from: string;
  to: string;
  selector: ConnectionRestrictionSelector;
  position?: number;
}

export interface PluginDescriptor {
  type: string;
  version: string;
  displayName: string;
  description: string;
  category: PluginCategory;
  inputMode: string;
  acceptsInitialVariables: boolean;
  inputPorts: PluginPort[];
  outputPorts: PluginPort[];
  inputEdgeConstraint: PluginEdgeConstraint;
  outputEdgeConstraint: PluginEdgeConstraint;
  allowedRootOrigins: string[];
  contextProvider?: string;
  connectionRestrictions: PluginConnectionRestriction[];
}

export interface PluginConfigurationValidationResult {
  valid: boolean;
  errors: Record<string, string>;
}

export interface PluginIncomingInput {
  edgeId: string;
  edgeOrder: number;
  sourceNodeId: string;
  sourceLabel: string;
  sourcePluginType: string;
  sourceConfiguration: PluginConfiguration;
}

export interface PluginWorkflowOption {
  id: number;
  name: string;
  status: string;
}

export interface PluginConfigurationEditorContext {
  nodeId: string;
  incomingInputs: PluginIncomingInput[];
  workflowId?: number;
  workflows?: PluginWorkflowOption[];
}

export interface PluginConfigurationEditorProps<TValues> {
  initialValues: TValues;
  disabled: boolean;
  validationErrors: Record<string, string>;
  onChange: (values: TValues) => void;
  onUploadFile?: (file: File) => Promise<string>;
  uploadPending?: boolean;
  uploadError?: boolean;
  editorContext?: PluginConfigurationEditorContext;
}

export interface PluginConfigurationDefinition<TValues> {
  pluginType: string;
  dialogDescription?: string;
  dialogSize?: "default" | "large";
  formSchema?: FlatFormSchema;
  createDefaultConfiguration: () => PluginConfiguration;
  deserialize: (configuration: PluginConfiguration) => TValues;
  validate: (
    values: TValues,
    editorContext?: PluginConfigurationEditorContext,
  ) => PluginConfigurationValidationResult;
  serialize: (values: TValues) => PluginConfiguration;
  Editor?: ComponentType<PluginConfigurationEditorProps<TValues>>;
}

export interface RegisteredPluginConfigurationDefinition {
  pluginType: string;
  dialogDescription?: string;
  dialogSize?: "default" | "large";
  formSchema?: FlatFormSchema;
  createDefaultConfiguration: () => PluginConfiguration;
  deserialize: (configuration: PluginConfiguration) => unknown;
  validate: (
    values: unknown,
    editorContext?: PluginConfigurationEditorContext,
  ) => PluginConfigurationValidationResult;
  serialize: (values: unknown) => PluginConfiguration;
  Editor?: ComponentType<PluginConfigurationEditorProps<unknown>>;
}
