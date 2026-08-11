"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  getActiveWorkflowCronTrigger,
  getActiveWorkflowHTTPTrigger,
  getWorkflow,
  listWorkflowPlugins,
  listWorkflows,
} from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import {
  type CronTrigger,
  type HTTPTrigger,
  type PluginPageResponse,
  type Workflow,
  type WorkflowListParams,
  type WorkflowPageResponse,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type ApiError, toApiError } from "@/shared/api/api-error";

function isMissingActiveTrigger(error: ApiError) {
  return error.status === 404 || error.code === "TRIGGER_NOT_FOUND";
}

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

export function useWorkflowHTTPTriggerQuery(workflowId: number, enabled: boolean) {
  return useQuery<HTTPTrigger | null, ApiError>({
    queryKey: workflowQueryKeys.httpTrigger(workflowId),
    queryFn: async () => {
      try {
        return await getActiveWorkflowHTTPTrigger(workflowId);
      } catch (error) {
        const apiError = toApiError(error);
        if (isMissingActiveTrigger(apiError)) {
          return null;
        }
        throw apiError;
      }
    },
    enabled: enabled && Number.isSafeInteger(workflowId) && workflowId > 0,
  });
}

/**
 * Drops every cached trigger binding of one workflow. A binding belongs to the
 * persisted active revision, so it must not survive the workflow leaving ACTIVE.
 */
export function useResetWorkflowTriggerBindings() {
  const queryClient = useQueryClient();
  return useCallback(
    (workflowId: number) => {
      queryClient.removeQueries({ queryKey: workflowQueryKeys.triggers(workflowId) });
    },
    [queryClient],
  );
}

export function useWorkflowCronTriggerQuery(workflowId: number, enabled: boolean) {
  return useQuery<CronTrigger | null, ApiError>({
    queryKey: workflowQueryKeys.cronTrigger(workflowId),
    queryFn: async () => {
      try {
        return await getActiveWorkflowCronTrigger(workflowId);
      } catch (error) {
        const apiError = toApiError(error);
        if (isMissingActiveTrigger(apiError)) {
          return null;
        }
        throw apiError;
      }
    },
    enabled: enabled && Number.isSafeInteger(workflowId) && workflowId > 0,
  });
}
