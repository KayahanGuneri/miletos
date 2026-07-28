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
} from "@/app/(panel)/_modules/dashboard/model/execution-types";
import { httpClient } from "@/shared/api/http-client";

const EXECUTIONS_PATH = "/api/v1/executions";

export async function listExecutions(params: ExecutionListParams = {}) {
  const response = await httpClient.get<ExecutionPageResponse>(EXECUTIONS_PATH, {
    params,
  });

  return response.data;
}

export async function getExecution(executionId: string) {
  const response = await httpClient.get<ExecutionSummaryResponse>(
    `${EXECUTIONS_PATH}/${executionId}`,
  );

  return response.data;
}

export async function getExecutionDefinition(executionId: string) {
  const response = await httpClient.get<ExecutionDefinitionResponse>(
    `${EXECUTIONS_PATH}/${executionId}/definition`,
  );

  return response.data;
}

export async function listExecutionNodes(executionId: string, params: CursorPageParams = {}) {
  const response = await httpClient.get<NodeExecutionPageResponse>(
    `${EXECUTIONS_PATH}/${executionId}/nodes`,
    {
      params,
    },
  );

  return response.data;
}

export async function listExecutionEvents(executionId: string, params: CursorPageParams = {}) {
  const response = await httpClient.get<ExecutionEventPageResponse>(
    `${EXECUTIONS_PATH}/${executionId}/events`,
    {
      params,
    },
  );

  return response.data;
}

export async function listExecutionLogs(executionId: string, params: CursorPageParams = {}) {
  const response = await httpClient.get<ExecutionLogPageResponse>(
    `${EXECUTIONS_PATH}/${executionId}/logs`,
    {
      params,
    },
  );

  return response.data;
}

export async function listExecutionErrors(executionId: string, params: CursorPageParams = {}) {
  const response = await httpClient.get<ExecutionErrorPageResponse>(
    `${EXECUTIONS_PATH}/${executionId}/errors`,
    {
      params,
    },
  );

  return response.data;
}
