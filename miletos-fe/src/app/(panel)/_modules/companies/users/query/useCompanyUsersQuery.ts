"use client";

import { useQuery } from "@tanstack/react-query";
import { type UserProfile } from "@/shared/session/types/session-user-types";
import { getCompanyUsers } from "../api/company-users-api";
import { companyUserQueryKeys } from "./company-user-query-keys";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseCompanyUsersQueryOptions {
  companyId: number;
  enabled?: boolean;
}

export function useCompanyUsersQuery({ companyId, enabled = true }: UseCompanyUsersQueryOptions) {
  return useQuery<UserProfile[], ApiError>({
    queryKey: companyUserQueryKeys.byCompany(companyId),
    queryFn: async () => {
      try {
        return await getCompanyUsers(companyId);
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled: enabled && Number.isSafeInteger(companyId) && companyId > 0,
    retry: (failureCount, error) => {
      if (error.isUnauthorized || error.isForbidden) {
        return false;
      }

      return failureCount < 1;
    },
  });
}
