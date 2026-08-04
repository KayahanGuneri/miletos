"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { activateWorkflow } from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import { type Workflow } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useActivateWorkflowMutation() {
  const queryClient = useQueryClient();
  return useMutation<Workflow, ApiError, number>({
    mutationFn: async (workflowId) => {
      try {
        return await activateWorkflow(workflowId);
      } catch (error) {
        throw toApiError(error);
      }
    },
    onSuccess: (workflow) => {
      queryClient.setQueryData(workflowQueryKeys.detail(workflow.id), workflow);
      void queryClient.invalidateQueries({ queryKey: workflowQueryKeys.lists() });
    },
  });
}
