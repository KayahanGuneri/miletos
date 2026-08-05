import {
  type Company,
  type CompanyPageResponse,
  type CreateCompanyRequest,
  type UpdateCompanyRequest,
} from "@/app/(panel)/_modules/companies/types/company-types";
import { httpClient } from "@/shared/api/http-client";

export interface ListCompaniesParams {
  page?: number;
  size?: number;
}

export async function listCompanies({ page = 0, size = 20 }: ListCompaniesParams = {}) {
  const response = await httpClient.get<CompanyPageResponse>("/companies", {
    params: {
      page,
      size,
    },
  });

  return response.data;
}

export async function createCompany(request: CreateCompanyRequest) {
  const response = await httpClient.post<Company>("/companies", request);

  return response.data;
}

export async function updateCompany(companyId: number, request: UpdateCompanyRequest) {
  const response = await httpClient.put<Company>(`/companies/${companyId}`, request);

  return response.data;
}

export async function deleteCompany(companyId: number) {
  await httpClient.delete(`/companies/${companyId}`);
}
