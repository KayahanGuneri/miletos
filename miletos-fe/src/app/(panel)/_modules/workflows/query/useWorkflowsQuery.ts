"use client";

import { useQuery } from "@tanstack/react-query";
import { listWorkflows } from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import {
  type WorkflowListParams,
  type WorkflowPageResponse,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";
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
