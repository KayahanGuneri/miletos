"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createWorkflow } from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import {
  type SaveWorkflowRequest,
  type Workflow,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useCreateWorkflowMutation() {
  const queryClient = useQueryClient();
  return useMutation<Workflow, ApiError, SaveWorkflowRequest>({
    mutationFn: async (request) => {
      try {
        return await createWorkflow(request);
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
