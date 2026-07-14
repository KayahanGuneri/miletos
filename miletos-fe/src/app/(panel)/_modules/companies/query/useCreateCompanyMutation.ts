"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  type Company,
  type CreateCompanyRequest,
} from "@/app/(panel)/_modules/companies/types/company-types";
import { createCompany } from "@/app/(panel)/_modules/companies/api/company-api";
import { companyQueryKeys } from "@/app/(panel)/_modules/companies/query/company-query-keys";
import { type ApiError, toApiError } from "@/shared/api/api-error";

export function useCreateCompanyMutation() {
  const queryClient = useQueryClient();

  return useMutation<Company, ApiError, CreateCompanyRequest>({
    mutationFn: async (request) => {
      try {
        return await createCompany(request);
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
