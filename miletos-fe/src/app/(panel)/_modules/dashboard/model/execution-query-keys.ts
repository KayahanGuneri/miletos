import {
  type CursorPageParams,
  type ExecutionListParams,
} from "@/app/(panel)/_modules/dashboard/model/execution-types";

const EXECUTION_QUERY_ROOT = ["executions"] as const;

export const executionQueryKeys = {
  all: EXECUTION_QUERY_ROOT,

  lists: () => [...EXECUTION_QUERY_ROOT, "list"] as const,

  list: (params: ExecutionListParams, companyId?: number) =>
    [...EXECUTION_QUERY_ROOT, "list", companyId ?? "current", params] as const,

  details: () => [...EXECUTION_QUERY_ROOT, "detail"] as const,

  detail: (executionId: string, companyId?: number) =>
    [...EXECUTION_QUERY_ROOT, "detail", companyId ?? "current", executionId] as const,

  definition: (executionId: string, companyId?: number) =>
    [...EXECUTION_QUERY_ROOT, "detail", companyId ?? "current", executionId, "definition"] as const,

  nodes: (executionId: string, params: CursorPageParams, companyId?: number) =>
    [...EXECUTION_QUERY_ROOT, "detail", companyId ?? "current", executionId, "nodes", params] as const,

  events: (executionId: string, params: CursorPageParams, companyId?: number) =>
    [...EXECUTION_QUERY_ROOT, "detail", companyId ?? "current", executionId, "events", params] as const,

  logs: (executionId: string, params: CursorPageParams, companyId?: number) =>
    [...EXECUTION_QUERY_ROOT, "detail", companyId ?? "current", executionId, "logs", params] as const,

  errors: (executionId: string, params: CursorPageParams, companyId?: number) =>
    [...EXECUTION_QUERY_ROOT, "detail", companyId ?? "current", executionId, "errors", params] as const,
};
