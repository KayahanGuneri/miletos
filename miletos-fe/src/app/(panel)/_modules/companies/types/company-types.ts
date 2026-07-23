export type CompanyStatus = "ACTIVE" | "DISABLED";

export interface Company {
  id: number;
  name: string;
  status: CompanyStatus;
  createdAt: string;
  updatedAt: string;
}

export interface CreateCompanyRequest {
  name: string;
}

export interface UpdateCompanyRequest {
  name: string;
  status: CompanyStatus;
}

export interface CompanyPageResponse {
  content: Company[];
  page: number;
  size: number;
  totalElements: number;
  totalPages: number;
  first: boolean;
  last: boolean;
}
