import { type WorkflowListParams } from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";

export const workflowQueryKeys = {
  all: ["workflows"] as const,
  lists: () => [...workflowQueryKeys.all, "list"] as const,
  list: (params: WorkflowListParams) => [...workflowQueryKeys.lists(), params] as const,
  options: () => [...workflowQueryKeys.all, "options"] as const,
  details: () => [...workflowQueryKeys.all, "detail"] as const,
  detail: (workflowId: number) => [...workflowQueryKeys.details(), workflowId] as const,
  plugins: () => [...workflowQueryKeys.all, "plugins"] as const,
  triggers: (workflowId: number) => [...workflowQueryKeys.detail(workflowId), "triggers"] as const,
  httpTrigger: (workflowId: number, triggerNodeId: string) =>
    [...workflowQueryKeys.triggers(workflowId), "http", triggerNodeId] as const,
  cronTrigger: (workflowId: number, triggerNodeId: string) =>
    [...workflowQueryKeys.triggers(workflowId), "cron", triggerNodeId] as const,
};
