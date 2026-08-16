"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  activateWorkflow,
  archiveWorkflow,
  createWorkflow,
  createWorkflowCronTrigger,
  createWorkflowHTTPTrigger,
  deleteWorkflow,
  disableWorkflowCronTrigger,
  disableWorkflowHTTPTrigger,
  restoreWorkflow,
  runWorkflow,
  updateWorkflow,
  uploadWorkflowInputFile,
} from "@/app/(panel)/_modules/workflows/api/workflow-api";
import { workflowQueryKeys } from "@/app/(panel)/_modules/workflows/query/workflow-query-keys";
import {
  type CreateCronTriggerResponse,
  type CreateHTTPTriggerResponse,
  type CreateTriggerVariables,
  type CronTrigger,
  type DisableTriggerVariables,
  type HTTPTrigger,
  type RunWorkflowVariables,
  type SaveWorkflowRequest,
  type UpdateWorkflowVariables,
  type Workflow,
  type WorkflowExecutionResponse,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type ApiError, toApiError } from "@/shared/api/api-error";

function normalizeError(error: unknown): never {
  throw toApiError(error);
}

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

export function useCreateWorkflowHTTPTriggerMutation() {
  const queryClient = useQueryClient();
  return useMutation<CreateHTTPTriggerResponse, ApiError, CreateTriggerVariables>({
    mutationFn: async ({ workflowId, triggerNodeId }) => {
      try {
        return await createWorkflowHTTPTrigger(workflowId, { triggerNodeId });
      } catch (error) {
        return normalizeError(error);
      }
    },
    onSuccess: (created, variables) => {
      queryClient.setQueryData(
        workflowQueryKeys.httpTrigger(variables.workflowId, variables.triggerNodeId),
        created.trigger,
      );
    },
  });
}

export function useDisableWorkflowHTTPTriggerMutation() {
  const queryClient = useQueryClient();
  return useMutation<HTTPTrigger, ApiError, DisableTriggerVariables & { triggerNodeId: string }>({
    mutationFn: async ({ workflowId, triggerId }) => {
      try {
        return await disableWorkflowHTTPTrigger(workflowId, triggerId);
      } catch (error) {
        return normalizeError(error);
      }
    },
    onSuccess: (_, variables) => {
      queryClient.setQueryData(
        workflowQueryKeys.httpTrigger(variables.workflowId, variables.triggerNodeId),
        null,
      );
    },
  });
}

export function useCreateWorkflowCronTriggerMutation() {
  const queryClient = useQueryClient();
  return useMutation<CreateCronTriggerResponse, ApiError, CreateTriggerVariables>({
    mutationFn: async ({ workflowId, triggerNodeId }) => {
      try {
        return await createWorkflowCronTrigger(workflowId, { triggerNodeId });
      } catch (error) {
        return normalizeError(error);
      }
    },
    onSuccess: (created, variables) => {
      queryClient.setQueryData(
        workflowQueryKeys.cronTrigger(variables.workflowId, variables.triggerNodeId),
        created.trigger,
      );
    },
  });
}

export function useDisableWorkflowCronTriggerMutation() {
  const queryClient = useQueryClient();
  return useMutation<CronTrigger, ApiError, DisableTriggerVariables & { triggerNodeId: string }>({
    mutationFn: async ({ workflowId, triggerId }) => {
      try {
        return await disableWorkflowCronTrigger(workflowId, triggerId);
      } catch (error) {
        return normalizeError(error);
      }
    },
    onSuccess: (_, variables) => {
      queryClient.setQueryData(
        workflowQueryKeys.cronTrigger(variables.workflowId, variables.triggerNodeId),
        null,
      );
    },
  });
}

export function useRunWorkflowMutation() {
  return useMutation<WorkflowExecutionResponse, ApiError, RunWorkflowVariables>({
    mutationFn: async ({ workflowId, initialVariables, idempotencyKey }) => {
      try {
        return await runWorkflow(workflowId, { initialVariables }, idempotencyKey);
      } catch (error) {
        return normalizeError(error);
      }
    },
  });
}

export function useUploadWorkflowInputFileMutation() {
  return useMutation<{ fileName: string }, ApiError, File>({
    mutationFn: async (file) => {
      try {
        return await uploadWorkflowInputFile(file);
      } catch (error) {
        throw toApiError(error);
      }
    },
  });
}
