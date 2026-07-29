"use client";

import { useQuery } from "@tanstack/react-query";
import { listExecutionErrors } from "@/app/(panel)/_modules/dashboard/api/execution-api";
import { executionQueryKeys } from "@/app/(panel)/_modules/dashboard/model/execution-query-keys";
import {
  type CursorPageParams,
  type ExecutionErrorPageResponse,
} from "@/app/(panel)/_modules/dashboard/model/execution-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseExecutionErrorsQueryOptions {
  enabled?: boolean;
  pollingEnabled?: boolean;
  companyId?: number;
}

const EXECUTION_RUNTIME_POLL_INTERVAL_MS = 3_000;

export function useExecutionErrorsQuery(
  executionId: string,
  params: CursorPageParams = {},
  { enabled = true, pollingEnabled = false, companyId }: UseExecutionErrorsQueryOptions = {},
) {
  return useQuery<ExecutionErrorPageResponse, ApiError>({
    queryKey: executionQueryKeys.errors(executionId, params, companyId),

    queryFn: async () => {
      try {
        return await listExecutionErrors(executionId, params, companyId);
      } catch (error) {
        throw toApiError(error);
      }
    },

    enabled: enabled && executionId.trim().length > 0,

    refetchInterval: pollingEnabled ? EXECUTION_RUNTIME_POLL_INTERVAL_MS : false,

    refetchIntervalInBackground: false,
  });
}
