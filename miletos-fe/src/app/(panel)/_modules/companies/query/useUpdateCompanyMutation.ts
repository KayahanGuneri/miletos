"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  type Company,
  type UpdateCompanyRequest,
} from "@/app/(panel)/_modules/companies/types/company-types";
import { updateCompany } from "@/app/(panel)/_modules/companies/api/company-api";
import { companyQueryKeys } from "@/app/(panel)/_modules/companies/query/company-query-keys";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UpdateCompanyVariables {
  companyId: number;
  request: UpdateCompanyRequest;
}

export function useUpdateCompanyMutation() {
  const queryClient = useQueryClient();

  return useMutation<Company, ApiError, UpdateCompanyVariables>({
    mutationFn: async ({ companyId, request }) => {
      try {
        return await updateCompany(companyId, request);
      } catch (error) {
        throw toApiError(error);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: companyQueryKeys.all,
      });
    },
  });
}
