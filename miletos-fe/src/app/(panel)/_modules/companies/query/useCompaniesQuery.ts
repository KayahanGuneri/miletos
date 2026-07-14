"use client";

import { useQuery } from "@tanstack/react-query";
import { type CompanyPageResponse } from "@/app/(panel)/_modules/companies/types/company-types";
import { listCompanies } from "@/app/(panel)/_modules/companies/api/company-api";
import { companyQueryKeys } from "@/app/(panel)/_modules/companies/query/company-query-keys";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseCompaniesQueryOptions {
  enabled?: boolean;
}

export function useCompaniesQuery(
  page: number,
  size: number,
  { enabled = true }: UseCompaniesQueryOptions = {},
) {
  return useQuery<CompanyPageResponse, ApiError>({
    queryKey: companyQueryKeys.list(page, size),
    queryFn: async () => {
      try {
        return await listCompanies({ page, size });
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled,
  });
}
