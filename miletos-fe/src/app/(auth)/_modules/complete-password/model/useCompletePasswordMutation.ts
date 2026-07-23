"use client";

import { useMutation } from "@tanstack/react-query";
import { completePassword } from "@/app/(auth)/_api/complete-password-api";
import { type ApiError, toApiError } from "@/shared/api/api-error";
import {
  type CompletePasswordRequest,
  type CompletePasswordResponse,
} from "./complete-password-types";

export function useCompletePasswordMutation() {
  return useMutation<CompletePasswordResponse, ApiError, CompletePasswordRequest>({
    mutationFn: async (request) => {
      try {
        return await completePassword(request);
      } catch (error) {
        throw toApiError(error);
      }
    },
  });
}
