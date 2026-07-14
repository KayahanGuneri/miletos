"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { inviteUser } from "../api/company-users-api";
import { companyUserQueryKeys } from "@/app/(panel)/_modules/companies/users/query/company-user-query-keys";
import {
  type InviteUserRequest,
  type InviteUserResponse,
} from "@/app/(panel)/_modules/companies/users/types/invite-user-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useInviteUserMutation() {
  const queryClient = useQueryClient();

  return useMutation<InviteUserResponse, ApiError, InviteUserRequest>({
    mutationFn: async (request) => {
      try {
        return await inviteUser(request);
      } catch (error) {
        throw toApiError(error);
      }
    },
    onSuccess: (_response, request) => {
      void queryClient.invalidateQueries({
        queryKey: companyUserQueryKeys.byCompany(request.companyId),
      });
    },
  });
}
