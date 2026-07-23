"use client";

import { useMutation } from "@tanstack/react-query";
import { resetPassword } from "@/app/(auth)/_api/password-recovery-api";
import { type ResetPasswordRequest, type ResetPasswordResponse } from "./reset-password-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useResetPasswordMutation() {
  return useMutation<ResetPasswordResponse, ApiError, ResetPasswordRequest>({
    mutationFn: async (request) => {
      try {
        return await resetPassword(request);
      } catch (error) {
        throw toApiError(error);
      }
    },
  });
}
