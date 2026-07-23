"use client";

import { useQuery } from "@tanstack/react-query";
import { getCurrentUser } from "@/shared/session/api/session-api";
import { sessionQueryKeys } from "@/shared/session/query/session-query-keys";
import { type UserProfile } from "@/shared/session/types/session-user-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";
import { hasAccessToken } from "@/shared/session/storage/access-token-storage";

interface UseCurrentUserQueryOptions {
  enabled?: boolean;
}

export function useCurrentUserQuery({ enabled = true }: UseCurrentUserQueryOptions = {}) {
  return useQuery<UserProfile, ApiError>({
    queryKey: sessionQueryKeys.currentUser(),
    queryFn: async () => {
      try {
        return await getCurrentUser();
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled: enabled && hasAccessToken(),
    retry: (failureCount, error) => {
      if (error.isUnauthorized || error.isForbidden) {
        return false;
      }

      return failureCount < 1;
    },
  });
}
