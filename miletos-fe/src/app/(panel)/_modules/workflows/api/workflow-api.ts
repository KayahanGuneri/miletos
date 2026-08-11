import { httpClient } from "@/shared/api/http-client";
import { withPluginCategory } from "@/shared/plugins/registry/plugin-palette-registry";

import {
  type CreateCronTriggerResponse,
  type CreateHTTPTriggerResponse,
  type CreateWorkflowTriggerRequest,
  type CronTrigger,
  type HTTPTrigger,
  type PluginPageResponse,
  type RunWorkflowRequest,
  type SaveWorkflowRequest,
  type Workflow,
  type WorkflowExecutionResponse,
  type WorkflowListParams,
  type WorkflowPageResponse,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";

const WORKFLOWS_PATH = "/workflows";

type PluginApiResponse = Omit<PluginPageResponse, "items"> & {
  items: Array<
    Omit<PluginPageResponse["items"][number], "category"> & {
      category?: unknown;
    }
  >;
};

export async function listWorkflows(params: WorkflowListParams) {
  const response = await httpClient.get<WorkflowPageResponse>(WORKFLOWS_PATH, {
    params,
  });

  return response.data;
}

export async function getWorkflow(workflowId: number) {
  const response = await httpClient.get<Workflow>(`${WORKFLOWS_PATH}/${workflowId}`);

  return response.data;
}

export async function createWorkflow(request: SaveWorkflowRequest) {
  const response = await httpClient.post<Workflow>(WORKFLOWS_PATH, request);

  return response.data;
}

export async function updateWorkflow(workflowId: number, request: SaveWorkflowRequest) {
  const response = await httpClient.put<Workflow>(`${WORKFLOWS_PATH}/${workflowId}`, request);

  return response.data;
}

export async function deleteWorkflow(workflowId: number) {
  await httpClient.delete(`${WORKFLOWS_PATH}/${workflowId}`);
}

export async function activateWorkflow(workflowId: number) {
  const response = await httpClient.post<Workflow>(`${WORKFLOWS_PATH}/${workflowId}/activate`);

  return response.data;
}

export async function archiveWorkflow(workflowId: number) {
  const response = await httpClient.post<Workflow>(`${WORKFLOWS_PATH}/${workflowId}/archive`);

  return response.data;
}

export async function restoreWorkflow(workflowId: number) {
  const response = await httpClient.post<Workflow>(`${WORKFLOWS_PATH}/${workflowId}/restore`);

  return response.data;
}

export async function listWorkflowPlugins() {
  const response = await httpClient.get<PluginApiResponse>("/v1/plugins");

  return {
    ...response.data,
    items: response.data.items.map(withPluginCategory),
  } satisfies PluginPageResponse;
}

export async function createWorkflowHTTPTrigger(
  workflowId: number,
  request: CreateWorkflowTriggerRequest,
) {
  const response = await httpClient.post<CreateHTTPTriggerResponse>(
    `${WORKFLOWS_PATH}/${workflowId}/triggers/http`,
    request,
  );

  return response.data;
}

export async function getActiveWorkflowHTTPTrigger(workflowId: number) {
  const response = await httpClient.get<HTTPTrigger>(
    `${WORKFLOWS_PATH}/${workflowId}/triggers/http`,
  );

  return response.data;
}

export async function getWorkflowHTTPTrigger(workflowId: number, triggerId: string) {
  const response = await httpClient.get<HTTPTrigger>(
    `${WORKFLOWS_PATH}/${workflowId}/triggers/http/${encodeURIComponent(triggerId)}`,
  );

  return response.data;
}

export async function disableWorkflowHTTPTrigger(workflowId: number, triggerId: string) {
  const response = await httpClient.post<HTTPTrigger>(
    `${WORKFLOWS_PATH}/${workflowId}/triggers/http/${encodeURIComponent(triggerId)}/disable`,
  );

  return response.data;
}

export async function createWorkflowCronTrigger(
  workflowId: number,
  request: CreateWorkflowTriggerRequest,
) {
  const response = await httpClient.post<CreateCronTriggerResponse>(
    `${WORKFLOWS_PATH}/${workflowId}/triggers/cron`,
    request,
  );

  return response.data;
}

export async function getActiveWorkflowCronTrigger(workflowId: number) {
  const response = await httpClient.get<CronTrigger>(
    `${WORKFLOWS_PATH}/${workflowId}/triggers/cron`,
  );

  return response.data;
}

export async function getWorkflowCronTrigger(workflowId: number, triggerId: string) {
  const response = await httpClient.get<CronTrigger>(
    `${WORKFLOWS_PATH}/${workflowId}/triggers/cron/${encodeURIComponent(triggerId)}`,
  );

  return response.data;
}

export async function disableWorkflowCronTrigger(workflowId: number, triggerId: string) {
  const response = await httpClient.post<CronTrigger>(
    `${WORKFLOWS_PATH}/${workflowId}/triggers/cron/${encodeURIComponent(triggerId)}/disable`,
  );

  return response.data;
}

export async function runWorkflow(
  workflowId: number,
  request: RunWorkflowRequest,
  idempotencyKey: string,
) {
  const response = await httpClient.post<WorkflowExecutionResponse>(
    `${WORKFLOWS_PATH}/${workflowId}/run`,
    request,
    {
      headers: {
        "Idempotency-Key": idempotencyKey,
      },
    },
  );

  return response.data;
}
