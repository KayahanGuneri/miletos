import { type WorkflowListParams } from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";

export const workflowQueryKeys = {
  all: ["workflows"] as const,
  lists: () => [...workflowQueryKeys.all, "list"] as const,
  list: (params: WorkflowListParams) => [...workflowQueryKeys.lists(), params] as const,
  details: () => [...workflowQueryKeys.all, "detail"] as const,
  detail: (workflowId: number) => [...workflowQueryKeys.details(), workflowId] as const,
  plugins: () => [...workflowQueryKeys.all, "plugins"] as const,
  triggers: (workflowId: number) => [...workflowQueryKeys.detail(workflowId), "triggers"] as const,
  httpTrigger: (workflowId: number) => [...workflowQueryKeys.triggers(workflowId), "http"] as const,
  cronTrigger: (workflowId: number) => [...workflowQueryKeys.triggers(workflowId), "cron"] as const,
};
