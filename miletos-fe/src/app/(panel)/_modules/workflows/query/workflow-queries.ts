"use client";

import { useQuery } from "@tanstack/react-query";
import {
  getWorkflow,
  listWorkflowPlugins,
  listWorkflows,
} from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import {
  type PluginPageResponse,
  type Workflow,
  type WorkflowListParams,
  type WorkflowPageResponse,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useWorkflowsQuery(params: WorkflowListParams, enabled = true) {
  return useQuery<WorkflowPageResponse, ApiError>({
    queryKey: workflowQueryKeys.list(params),
    queryFn: async () => {
      try {
        return await listWorkflows(params);
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled,
  });
}

export function useWorkflowQuery(workflowId: number, enabled = true) {
  return useQuery<Workflow, ApiError>({
    queryKey: workflowQueryKeys.detail(workflowId),
    queryFn: async () => {
      try {
        return await getWorkflow(workflowId);
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled: enabled && Number.isSafeInteger(workflowId) && workflowId > 0,
  });
}

export function useWorkflowPluginsQuery(enabled = true) {
  return useQuery<PluginPageResponse, ApiError>({
    queryKey: workflowQueryKeys.plugins(),
    queryFn: async () => {
      try {
        return await listWorkflowPlugins();
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled,
    staleTime: 60_000,
  });
}
