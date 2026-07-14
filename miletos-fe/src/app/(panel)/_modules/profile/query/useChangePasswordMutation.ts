"use client";

import { useMutation } from "@tanstack/react-query";
import { changePassword } from "../api/profile-api";
import { type ChangePasswordRequest } from "../types/profile-api-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useChangePasswordMutation() {
  return useMutation<void, ApiError, ChangePasswordRequest>({
    mutationFn: async (request) => {
      try {
        await changePassword(request);
      } catch (error) {
        throw toApiError(error);
      }
    },
  });
}
