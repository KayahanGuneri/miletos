import {
  type CursorPageParams,
  type ExecutionListParams,
} from "@/app/(panel)/_modules/dashboard/model/execution-types";

const EXECUTION_QUERY_ROOT = ["executions"] as const;

export const executionQueryKeys = {
  all: EXECUTION_QUERY_ROOT,

  lists: () => [...EXECUTION_QUERY_ROOT, "list"] as const,

  list: (params: ExecutionListParams) =>
    [...EXECUTION_QUERY_ROOT, "list", params] as const,

  details: () => [...EXECUTION_QUERY_ROOT, "detail"] as const,

  detail: (executionId: string) =>
    [...EXECUTION_QUERY_ROOT, "detail", executionId] as const,

  definition: (executionId: string) =>
    [...EXECUTION_QUERY_ROOT, "detail", executionId, "definition"] as const,

  nodes: (executionId: string, params: CursorPageParams) =>
    [...EXECUTION_QUERY_ROOT, "detail", executionId, "nodes", params] as const,

  events: (executionId: string, params: CursorPageParams) =>
    [...EXECUTION_QUERY_ROOT, "detail", executionId, "events", params] as const,

  logs: (executionId: string, params: CursorPageParams) =>
    [...EXECUTION_QUERY_ROOT, "detail", executionId, "logs", params] as const,

  errors: (executionId: string, params: CursorPageParams) =>
    [...EXECUTION_QUERY_ROOT, "detail", executionId, "errors", params] as const,
};
