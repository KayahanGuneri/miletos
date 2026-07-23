"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { deleteCompany } from "@/app/(panel)/_modules/companies/api/company-api";
import { companyQueryKeys } from "@/app/(panel)/_modules/companies/query/company-query-keys";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useDeleteCompanyMutation() {
  const queryClient = useQueryClient();

  return useMutation<void, ApiError, number>({
    mutationFn: async (companyId) => {
      try {
        await deleteCompany(companyId);
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
