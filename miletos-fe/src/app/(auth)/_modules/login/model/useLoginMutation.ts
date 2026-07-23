"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { login } from "@/app/(auth)/_api/login-api";
import { sessionQueryKeys } from "@/shared/session/query/session-query-keys";
import { type LoginRequest, type LoginResponse } from "./login-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";
import { setAccessToken } from "@/shared/session/storage/access-token-storage";

export function useLoginMutation() {
  const queryClient = useQueryClient();

  return useMutation<LoginResponse, ApiError, LoginRequest>({
    mutationFn: async (request) => {
      try {
        return await login(request);
      } catch (error) {
        throw toApiError(error);
      }
    },
    onSuccess: (response) => {
      setAccessToken(response.accessToken);
      void queryClient.invalidateQueries({ queryKey: sessionQueryKeys.currentUser() });
    },
  });
}
