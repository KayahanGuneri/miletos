"use client";

import { useQuery } from "@tanstack/react-query";
import { listExecutionNodes } from "@/app/(panel)/_modules/dashboard/api/execution-api";
import { executionQueryKeys } from "@/app/(panel)/_modules/dashboard/model/execution-query-keys";
import {
  type CursorPageParams,
  type NodeExecutionPageResponse,
} from "@/app/(panel)/_modules/dashboard/model/execution-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseExecutionNodesQueryOptions {
  enabled?: boolean;
  pollingEnabled?: boolean;
}

const EXECUTION_RUNTIME_POLL_INTERVAL_MS = 3_000;

export function useExecutionNodesQuery(
  executionId: string,
  params: CursorPageParams = {},
  {
    enabled = true,
    pollingEnabled = false,
  }: UseExecutionNodesQueryOptions = {},
) {
  return useQuery<NodeExecutionPageResponse, ApiError>({
    queryKey: executionQueryKeys.nodes(executionId, params),

    queryFn: async () => {
      try {
        return await listExecutionNodes(executionId, params);
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
