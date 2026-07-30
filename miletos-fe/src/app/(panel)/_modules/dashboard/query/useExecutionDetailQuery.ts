"use client";

import { useQuery } from "@tanstack/react-query";
import { getExecution } from "@/app/(panel)/_modules/dashboard/api/execution-api";
import { executionQueryKeys } from "@/app/(panel)/_modules/dashboard/query/execution-query-keys";
import { type ExecutionSummaryResponse } from "@/app/(panel)/_modules/dashboard/types/execution-types";
import { isTerminalExecutionStatus } from "@/app/(panel)/_modules/dashboard/utils/execution-formatters";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseExecutionDetailQueryOptions {
  enabled?: boolean;
  companyId?: number;
}

const EXECUTION_DETAIL_POLL_INTERVAL_MS = 3_000;

export function useExecutionDetailQuery(
  executionId: string,
  { enabled = true, companyId }: UseExecutionDetailQueryOptions = {},
) {
  return useQuery<ExecutionSummaryResponse, ApiError>({
    queryKey: executionQueryKeys.detail(executionId, companyId),

    queryFn: async () => {
      try {
        return await getExecution(executionId, companyId);
      } catch (error) {
        throw toApiError(error);
      }
    },

    enabled: enabled && executionId.trim().length > 0,

    refetchInterval: (query) => {
      const execution = query.state.data;

      if (!execution) {
        return false;
      }

      return isTerminalExecutionStatus(execution.status)
        ? false
        : EXECUTION_DETAIL_POLL_INTERVAL_MS;
    },

    refetchIntervalInBackground: false,
  });
}
