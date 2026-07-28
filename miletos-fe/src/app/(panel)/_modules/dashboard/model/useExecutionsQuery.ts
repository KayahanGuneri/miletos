"use client";

import { useQuery } from "@tanstack/react-query";
import { listExecutions } from "@/app/(panel)/_modules/dashboard/api/execution-api";
import { executionQueryKeys } from "@/app/(panel)/_modules/dashboard/model/execution-query-keys";
import { isTerminalExecutionStatus } from "@/app/(panel)/_modules/dashboard/model/execution-status";
import {
  type ExecutionListParams,
  type ExecutionPageResponse,
} from "@/app/(panel)/_modules/dashboard/model/execution-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseExecutionsQueryOptions {
  enabled?: boolean;
}

const EXECUTION_LIST_POLL_INTERVAL_MS = 5_000;

export function useExecutionsQuery(
  params: ExecutionListParams = {},
  { enabled = true }: UseExecutionsQueryOptions = {},
) {
  return useQuery<ExecutionPageResponse, ApiError>({
    queryKey: executionQueryKeys.list(params),

    queryFn: async () => {
      try {
        return await listExecutions(params);
      } catch (error) {
        throw toApiError(error);
      }
    },

    enabled,

    refetchInterval: (query) => {
      const page = query.state.data;

      if (!page) {
        return false;
      }

      const hasActiveExecution = page.items.some(
        (execution) => !isTerminalExecutionStatus(execution.status),
      );

      return hasActiveExecution ? EXECUTION_LIST_POLL_INTERVAL_MS : false;
    },

    refetchIntervalInBackground: false,
  });
}
