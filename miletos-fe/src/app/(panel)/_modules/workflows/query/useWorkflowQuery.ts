"use client";

import { useQuery } from "@tanstack/react-query";
import { getWorkflow } from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import { type Workflow } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

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
