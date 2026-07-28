"use client";

import { useQuery } from "@tanstack/react-query";
import { getExecutionDefinition } from "@/app/(panel)/_modules/dashboard/api/execution-api";
import { executionQueryKeys } from "@/app/(panel)/_modules/dashboard/model/execution-query-keys";
import { type ExecutionDefinitionResponse } from "@/app/(panel)/_modules/dashboard/model/execution-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseExecutionDefinitionQueryOptions {
  enabled?: boolean;
}

export function useExecutionDefinitionQuery(
  executionId: string,
  { enabled = true }: UseExecutionDefinitionQueryOptions = {},
) {
  return useQuery<ExecutionDefinitionResponse, ApiError>({
    queryKey: executionQueryKeys.definition(executionId),
    queryFn: async () => {
      try {
        return await getExecutionDefinition(executionId);
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled: enabled && executionId.trim().length > 0,
  });
}
