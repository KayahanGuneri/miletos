import type {
  JsonObject,
  WorkflowPlugin,
  WorkflowStatus,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";

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

export interface WorkflowAuditUser {
  id: number;
  email: string;
  firstName: string;
  lastName: string;
}

export interface WorkflowSummary {
  id: number;
  name: string;
  description: string | null;
  status: WorkflowStatus;
  revision: number;
  nodeCount: number;
  edgeCount: number;
  createdBy: WorkflowAuditUser;
  updatedBy: WorkflowAuditUser;
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

export interface UpdateWorkflowVariables {
  workflowId: number;
  request: SaveWorkflowRequest;
}

export interface PluginPageResponse {
  items: WorkflowPlugin[];
  count: number;
}

export interface WorkflowFilterState {
  searchDraft: string;
  search: string;
  status?: WorkflowStatus;
}

export interface CreateWorkflowTriggerRequest {
  triggerNodeId: string;
}

export interface HTTPTrigger {
  triggerId: string;
  workflowId: string;
  workflowRevision: number;
  snapshotId: string;
  triggerNodeId: string;
  httpMethod: string;
  status: string;
  resolvedMode: string;
  createdAt: string;
  updatedAt: string;
  disabledAt?: string;
}

export interface CreateHTTPTriggerResponse {
  trigger: HTTPTrigger;
  publicUrl: string;
}

export interface CronTrigger {
  triggerId: string;
  workflowId: string;
  workflowRevision: number;
  snapshotId: string;
  triggerNodeId: string;
  cronExpression: string;
  timezone: string;
  status: string;
  nextFireAt: string;
  lastScheduledAt?: string;
  lastFiredAt?: string;
  createdAt: string;
  updatedAt: string;
  disabledAt?: string;
}

export interface CreateCronTriggerResponse {
  trigger: CronTrigger;
}

export interface RunWorkflowRequest {
  initialVariables?: JsonObject;
}

export interface WorkflowExecutionResponse {
  executionId: string;
  workflowId: string;
  workflowRevision: number;
  snapshotId: string;
  mode: string;
  status: string;
  executionOrigin: string;
  scheduledRoots: number;
  replayed: boolean;
}

export interface CreateTriggerVariables {
  workflowId: number;
  triggerNodeId: string;
}

export interface DisableTriggerVariables {
  workflowId: number;
  triggerId: string;
}

export interface RunWorkflowVariables {
  workflowId: number;
  initialVariables: JsonObject;
  idempotencyKey: string;
}
