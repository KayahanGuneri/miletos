"use client";

import { useQuery } from "@tanstack/react-query";
import { listCompanies } from "@/app/(panel)/_modules/companies/api/company-api";
import { companyQueryKeys } from "@/app/(panel)/_modules/companies/query/company-query-keys";
import { type Company } from "@/app/(panel)/_modules/companies/types/company-types";
import { type ApiError, toApiError } from "@/shared/api/api-error";

interface UseAllCompaniesQueryOptions {
  enabled?: boolean;
}

const COMPANY_PAGE_SIZE = 100;

export function useAllCompaniesQuery({ enabled = true }: UseAllCompaniesQueryOptions = {}) {
  return useQuery<Company[], ApiError>({
    queryKey: [...companyQueryKeys.all, "options"],
    queryFn: async () => {
      try {
        const companies: Company[] = [];
        let page = 0;
        let isLastPage = false;

        while (!isLastPage) {
          const response = await listCompanies({ page, size: COMPANY_PAGE_SIZE });
          companies.push(...response.content);
          isLastPage = response.last;
          page += 1;
        }

        return companies;
      } catch (error) {
        throw toApiError(error);
      }
    },
    enabled,
  });
}
