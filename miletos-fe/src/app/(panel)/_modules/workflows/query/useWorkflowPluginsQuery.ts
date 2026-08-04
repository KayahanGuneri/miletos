"use client";

import { useQuery } from "@tanstack/react-query";
import { listWorkflowPlugins } from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import { type PluginPageResponse } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

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
