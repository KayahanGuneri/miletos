import { type WorkflowListParams } from "@/app/(panel)/_modules/workflows/types/workflow-types";

export const workflowQueryKeys = {
  all: ["workflows"] as const,
  lists: () => [...workflowQueryKeys.all, "list"] as const,
  list: (params: WorkflowListParams) => [...workflowQueryKeys.lists(), params] as const,
  details: () => [...workflowQueryKeys.all, "detail"] as const,
  detail: (workflowId: number) => [...workflowQueryKeys.details(), workflowId] as const,
  plugins: () => [...workflowQueryKeys.all, "plugins"] as const,
};
