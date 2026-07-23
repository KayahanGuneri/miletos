"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { updateOwnProfilePhoto } from "../api/profile-api";
import { profileQueryKeys } from "./profile-query-keys";
import { type UserProfile } from "@/shared/session/types/session-user-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";
import { sessionQueryKeys } from "@/shared/session/query/session-query-keys";

export function useUpdateProfilePhotoMutation() {
  const queryClient = useQueryClient();

  return useMutation<UserProfile, ApiError, File>({
    mutationFn: async (file) => {
      try {
        return await updateOwnProfilePhoto(file);
      } catch (error) {
        throw toApiError(error);
      }
    },
    onSuccess: (updatedUser) => {
      queryClient.setQueryData(sessionQueryKeys.currentUser(), updatedUser);
      void queryClient.invalidateQueries({
        queryKey: profileQueryKeys.currentUserProfilePhoto(),
      });
    },
  });
}
