import type {
  PluginConfiguration,
  PluginConfigurationValue,
  PluginDescriptor,
  PluginPort as DescriptorPort,
} from "@/shared/plugins/contracts/plugin-configuration";

export type WorkflowStatus = "DRAFT" | "ACTIVE" | "ARCHIVED";

export type JsonValue = PluginConfigurationValue;
export type JsonObject = PluginConfiguration;

export interface NodePosition {
  x: number;
  y: number;
}

export interface WorkflowNode {
  nodeId: string;
  pluginType: string;
  pluginVersion: string;
  configuration: JsonObject;
  position: NodePosition;
}

export interface WorkflowEdge {
  edgeId: string;
  sourceNodeId: string;
  sourceOutputPort: string;
  targetNodeId: string;
  targetInputPort: string;
}

export interface SaveWorkflowRequest {
  name: string;
  description: string | null;
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  metadata: JsonObject;
}

export interface WorkflowSummary {
  id: number;
  name: string;
  description: string | null;
  status: WorkflowStatus;
  revision: number;
  nodeCount: number;
  edgeCount: number;
  createdByEmail: string;
  updatedByEmail: string;
  createdAt: string;
  updatedAt: string;
}

export interface Workflow extends WorkflowSummary {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  metadata: JsonObject;
}

export interface WorkflowPageResponse {
  content: WorkflowSummary[];
  page: number;
  size: number;
  totalElements: number;
  totalPages: number;
  first: boolean;
  last: boolean;
}

export interface WorkflowListParams {
  page: number;
  size: number;
  search?: string;
  status?: WorkflowStatus;
}

export type PluginPort = DescriptorPort;

export type WorkflowPlugin = PluginDescriptor;

export interface PluginPageResponse {
  items: WorkflowPlugin[];
  count: number;
}
