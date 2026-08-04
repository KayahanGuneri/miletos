"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { deleteWorkflow } from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useDeleteWorkflowMutation() {
  const queryClient = useQueryClient();
  return useMutation<void, ApiError, number>({
    mutationFn: async (workflowId) => {
      try {
        await deleteWorkflow(workflowId);
      } catch (error) {
        throw toApiError(error);
      }
    },
    onSuccess: (_result, workflowId) => {
      queryClient.removeQueries({ queryKey: workflowQueryKeys.detail(workflowId) });
      void queryClient.invalidateQueries({ queryKey: workflowQueryKeys.lists() });
    },
  });
}
