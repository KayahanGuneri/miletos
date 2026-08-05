import { httpClient } from "@/shared/api/http-client";
import { withPluginCategory } from "@/shared/plugins/registry/plugin-palette-registry";
import {
  type PluginPageResponse,
  type SaveWorkflowRequest,
  type Workflow,
  type WorkflowListParams,
  type WorkflowPageResponse,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";

const WORKFLOWS_PATH = "/workflows";

type PluginApiResponse = Omit<PluginPageResponse, "items"> & {
  items: Array<Omit<PluginPageResponse["items"][number], "category"> & { category?: unknown }>;
};

export async function listWorkflows(params: WorkflowListParams) {
  const response = await httpClient.get<WorkflowPageResponse>(WORKFLOWS_PATH, { params });
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
