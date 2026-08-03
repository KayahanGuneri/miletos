import {
  type CursorPageParams,
  type ExecutionDefinitionResponse,
  type ExecutionErrorPageResponse,
  type ExecutionEventPageResponse,
  type ExecutionListParams,
  type ExecutionLogPageResponse,
  type ExecutionPageResponse,
  type ExecutionSummaryResponse,
  type NodeExecutionPageResponse,
} from "@/app/(panel)/_modules/dashboard/types/execution-types";
import {
  type RunWorkflowRequest,
  type RunWorkflowResponse,
} from "@/app/(panel)/_modules/dashboard/types/execution-types";
import { httpClient } from "@/shared/api/http-client";

const EXECUTIONS_PATH = "/api/v1/executions";

function tenantHeaders(companyId?: number) {
  return companyId ? { "X-Miletos-Company-ID": String(companyId) } : undefined;
}

export async function listExecutions(params: ExecutionListParams = {}, companyId?: number) {
  const response = await httpClient.get<ExecutionPageResponse>(EXECUTIONS_PATH, {
    params,
    headers: tenantHeaders(companyId),
  });

  return response.data;
}

export async function runWorkflow(
  request: RunWorkflowRequest,
  idempotencyKey: string,
  companyId?: number,
) {
  const response = await httpClient.post<RunWorkflowResponse>(EXECUTIONS_PATH, request, {
    headers: {
      ...tenantHeaders(companyId),
      "Idempotency-Key": idempotencyKey,
    },
  });

  return response.data;
}

export async function getExecution(executionId: string, companyId?: number) {
  const response = await httpClient.get<ExecutionSummaryResponse>(
    `${EXECUTIONS_PATH}/${executionId}`,
    { headers: tenantHeaders(companyId) },
  );

  return response.data;
}

export async function getExecutionDefinition(executionId: string, companyId?: number) {
  const response = await httpClient.get<ExecutionDefinitionResponse>(
    `${EXECUTIONS_PATH}/${executionId}/definition`,
    { headers: tenantHeaders(companyId) },
  );

  return response.data;
}

export async function listExecutionNodes(
  executionId: string,
  params: CursorPageParams = {},
  companyId?: number,
) {
  const response = await httpClient.get<NodeExecutionPageResponse>(
    `${EXECUTIONS_PATH}/${executionId}/nodes`,
    {
      params,
      headers: tenantHeaders(companyId),
    },
  );

  return response.data;
}

export async function listExecutionEvents(
  executionId: string,
  params: CursorPageParams = {},
  companyId?: number,
) {
  const response = await httpClient.get<ExecutionEventPageResponse>(
    `${EXECUTIONS_PATH}/${executionId}/events`,
    {
      params,
      headers: tenantHeaders(companyId),
    },
  );

  return response.data;
}

export async function listExecutionLogs(
  executionId: string,
  params: CursorPageParams = {},
  companyId?: number,
) {
  const response = await httpClient.get<ExecutionLogPageResponse>(
    `${EXECUTIONS_PATH}/${executionId}/logs`,
    {
      params,
      headers: tenantHeaders(companyId),
    },
  );

  return response.data;
}

export async function listExecutionErrors(
  executionId: string,
  params: CursorPageParams = {},
  companyId?: number,
) {
  const response = await httpClient.get<ExecutionErrorPageResponse>(
    `${EXECUTIONS_PATH}/${executionId}/errors`,
    {
      params,
      headers: tenantHeaders(companyId),
    },
  );

  return response.data;
}
