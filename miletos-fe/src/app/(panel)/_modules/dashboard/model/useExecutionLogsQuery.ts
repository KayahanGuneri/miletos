"use client";

import { useQuery } from "@tanstack/react-query";
import { listExecutionLogs } from "@/app/(panel)/_modules/dashboard/api/execution-api";
import { executionQueryKeys } from "@/app/(panel)/_modules/dashboard/model/execution-query-keys";
import {
  type CursorPageParams,
  type ExecutionLogPageResponse,
} from "@/app/(panel)/_modules/dashboard/model/execution-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseExecutionLogsQueryOptions {
  enabled?: boolean;
  pollingEnabled?: boolean;
}

const EXECUTION_RUNTIME_POLL_INTERVAL_MS = 3_000;

export function useExecutionLogsQuery(
  executionId: string,
  params: CursorPageParams = {},
  { enabled = true, pollingEnabled = false }: UseExecutionLogsQueryOptions = {},
) {
  return useQuery<ExecutionLogPageResponse, ApiError>({
    queryKey: executionQueryKeys.logs(executionId, params),

    queryFn: async () => {
      try {
        return await listExecutionLogs(executionId, params);
      } catch (error) {
        throw toApiError(error);
      }
    },

    enabled: enabled && executionId.trim().length > 0,

    refetchInterval: pollingEnabled
      ? EXECUTION_RUNTIME_POLL_INTERVAL_MS
      : false,

    refetchIntervalInBackground: false,
  });
}
