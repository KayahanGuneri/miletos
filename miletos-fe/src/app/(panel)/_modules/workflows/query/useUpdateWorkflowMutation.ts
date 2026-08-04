"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { updateWorkflow } from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import {
  type SaveWorkflowRequest,
  type Workflow,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UpdateWorkflowVariables {
  workflowId: number;
  request: SaveWorkflowRequest;
}

export function useUpdateWorkflowMutation() {
  const queryClient = useQueryClient();
  return useMutation<Workflow, ApiError, UpdateWorkflowVariables>({
    mutationFn: async ({ workflowId, request }) => {
      try {
        return await updateWorkflow(workflowId, request);
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
