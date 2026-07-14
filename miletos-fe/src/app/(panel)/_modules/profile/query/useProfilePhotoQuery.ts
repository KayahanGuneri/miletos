"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getOwnProfilePhoto } from "../api/profile-api";
import { profileQueryKeys } from "./profile-query-keys";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseProfilePhotoQueryOptions {
  enabled?: boolean;
}

export function useProfilePhotoQuery({ enabled = true }: UseProfilePhotoQueryOptions = {}) {
  const query = useQuery<Blob, ApiError>({
    queryKey: profileQueryKeys.currentUserProfilePhoto(),
    queryFn: async () => {
      try {
        return await getOwnProfilePhoto();
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled,
    retry: (failureCount, error) =>
      error.isUnauthorized || error.isForbidden ? false : failureCount < 1,
  });
  const [profilePhotoUrl, setProfilePhotoUrl] = useState<string | null>(null);

  useEffect(() => {
    if (!query.data) {
      // The URL mirrors an external browser resource and must be cleared when that resource is absent.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setProfilePhotoUrl(null);
      return undefined;
    }

    const objectUrl = URL.createObjectURL(query.data);
    // Object URL creation and revocation belong to the same resource-lifecycle effect.
    setProfilePhotoUrl(objectUrl);
    return () => URL.revokeObjectURL(objectUrl);
  }, [query.data]);

  return { ...query, profilePhotoUrl };
}
