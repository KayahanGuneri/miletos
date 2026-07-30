"use client";

import { useQuery } from "@tanstack/react-query";
import { getExecutionDefinition } from "@/app/(panel)/_modules/dashboard/api/execution-api";
import { executionQueryKeys } from "@/app/(panel)/_modules/dashboard/query/execution-query-keys";
import { type ExecutionDefinitionResponse } from "@/app/(panel)/_modules/dashboard/types/execution-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseExecutionDefinitionQueryOptions {
  enabled?: boolean;
  companyId?: number;
}

export function useExecutionDefinitionQuery(
  executionId: string,
  { enabled = true, companyId }: UseExecutionDefinitionQueryOptions = {},
) {
  return useQuery<ExecutionDefinitionResponse, ApiError>({
    queryKey: executionQueryKeys.definition(executionId, companyId),
    queryFn: async () => {
      try {
        return await getExecutionDefinition(executionId, companyId);
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled: enabled && executionId.trim().length > 0,
  });
}
