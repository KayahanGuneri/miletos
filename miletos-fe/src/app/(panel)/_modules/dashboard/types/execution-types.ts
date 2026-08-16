export type ExecutionMode = "SYNC" | "ASYNC";

export type ExecutionStatus =
  | "CREATED"
  | "VALIDATING"
  | "REJECTED"
  | "QUEUED"
  | "RUNNING"
  | "SUCCEEDED"
  | "FAILED"
  | "CANCELLED"
  | "TIMED_OUT";

export interface WorkflowDefinition {
  id: string;
  name: string;
  revision: number;
  nodes: Array<{
    id: string;
    displayName?: string;
    pluginType: string;
    pluginVersion: string;
    configuration?: Record<string, unknown>;
    position?: { x: number; y: number };
  }>;
  edges: Array<{
    id: string;
    sourceNodeId: string;
    sourceOutputPort: string;
    targetNodeId: string;
    targetInputPort: string;
  }>;
  metadata?: Record<string, unknown>;
}

export interface ExecutionSummaryResponse {
  executionId: string;
  workflowId: string;
  workflowRevision: number;
  mode: string;
  status: ExecutionStatus;
  correlationId: string;
  createdAt: string;
  validatingAt?: string;
  queuedAt?: string;
  startedAt?: string;
  finishedAt?: string;
  updatedAt: string;
  isStalled: boolean;
}

export interface ExecutionPageResponse {
  items: ExecutionSummaryResponse[];
  next: string;
  hasNext: boolean;
}

export interface ExecutionDefinitionResponse {
  executionId: string;
  snapshotId: string;
  workflowId: string;
  workflowRevision: number;
  workflowName: string;
  definition: WorkflowDefinition;
  createdAt: string;
}

export interface FailureSummary {
  category?: string;
  code?: string;
  message?: string;
  retryable?: boolean;
  details?: unknown;
  [key: string]: unknown;
}

export interface NodeExecutionResponse {
  nodeExecutionId: string;
  workflowExecutionId: string;
  nodeId: string;
  pluginType: string;
  pluginVersion: string;
  configuration?: Record<string, unknown>;
  status: string;
  attempt: number;
  createdAt: string;
  readyAt?: string;
  queuedAt?: string;
  startedAt?: string;
  finishedAt?: string;
  nextAttemptAt?: string;
  updatedAt: string;
  inputSummary?: Record<string, unknown>;
  outputSummary?: Record<string, unknown>;
  failureSummary?: FailureSummary;
}

export interface NodeExecutionPageResponse {
  items: NodeExecutionResponse[];
  next: string;
  hasNext: boolean;
}

export interface ExecutionMetadata {
  mode?: string;
  workflowRevision?: number;
  stalled?: boolean;
  failureCategory?: string;
  failureCode?: string;
  nodeId?: string;
  pluginType?: string;
  pluginVersion?: string;
  [key: string]: unknown;
}

export interface ExecutionEventResponse {
  eventId: string;
  workflowExecutionId: string;
  nodeExecutionId?: string;
  sequenceNumber: string;
  type: string;
  previousStatus?: string;
  newStatus?: string;
  correlationId?: string;
  causationId?: string;
  safeMessage?: string;
  metadata: ExecutionMetadata;
  createdAt: string;
}

export interface ExecutionEventPageResponse {
  items: ExecutionEventResponse[];
  next: string;
  hasNext: boolean;
}

export interface ExecutionLogResponse {
  logId: string;
  workflowExecutionId: string;
  nodeExecutionId?: string;
  sequenceNumber: string;
  level: string;
  message: string;
  metadata: ExecutionMetadata;
  createdAt: string;
}

export interface ExecutionLogPageResponse {
  items: ExecutionLogResponse[];
  next: string;
  hasNext: boolean;
}

export interface ExecutionErrorSafeDetails {
  issueCount?: string;
  structuralIssueCount?: string;
  pluginIssueCount?: string;
  engineIssueCount?: string;
  failedNodeCount?: string;
  skippedNodeCount?: string;
  pendingNodeCount?: string;
  readyNodeCount?: string;
}

export interface ExecutionErrorResponse {
  errorId: string;
  workflowExecutionId: string;
  nodeExecutionId?: string;
  relatedEventId?: string;
  category: string;
  code: string;
  safeMessage: string;
  retryable: boolean;
  details: ExecutionErrorSafeDetails;
  createdAt: string;
}

export interface ExecutionErrorPageResponse {
  items: ExecutionErrorResponse[];
  next: string;
  hasNext: boolean;
}

export interface ExecutionListParams {
  limit?: number;
  after?: string;
  workflowId?: string;
  status?: ExecutionStatus;
}

export interface CursorPageParams {
  limit?: number;
  after?: string;
}

export interface RunWorkflowRequest {
  definition: WorkflowDefinition;
  initialVariables?: Record<string, unknown>;
}

export interface RunWorkflowCommand {
  request: RunWorkflowRequest;
  companyId?: number;
  idempotencyKey: string;
}

export interface RunWorkflowResponse {
  executionId?: string;
  executionIds: string[];
  executionCount: number;
  workflowId: string;
  workflowRevision: number;
  mode: ExecutionMode;
  status: ExecutionStatus | "DISPATCHED" | "NO_RECORDS";
  correlationId: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  scheduledRoots?: number;
  replayed: boolean;
}
