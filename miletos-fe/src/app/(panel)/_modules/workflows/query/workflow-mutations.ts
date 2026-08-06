"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  activateWorkflow,
  archiveWorkflow,
  createWorkflow,
  deleteWorkflow,
  restoreWorkflow,
  updateWorkflow,
} from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import {
  type SaveWorkflowRequest,
  type UpdateWorkflowVariables,
  type Workflow,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
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

export function useArchiveWorkflowMutation() {
  const queryClient = useQueryClient();
  return useMutation<Workflow, ApiError, number>({
    mutationFn: async (workflowId) => {
      try {
        return await archiveWorkflow(workflowId);
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

export function useRestoreWorkflowMutation() {
  const queryClient = useQueryClient();
  return useMutation<Workflow, ApiError, number>({
    mutationFn: async (workflowId) => {
      try {
        return await restoreWorkflow(workflowId);
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
