"use client";

import { useMutation } from "@tanstack/react-query";
import { requestPasswordReset } from "@/app/(auth)/_api/password-recovery-api";
import { type ForgotPasswordRequest } from "./forgot-password-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useForgotPasswordMutation() {
  return useMutation<void, ApiError, ForgotPasswordRequest>({
    mutationFn: async (request) => {
      try {
        await requestPasswordReset(request);
      } catch (error) {
        throw toApiError(error);
      }
    },
  });
}
