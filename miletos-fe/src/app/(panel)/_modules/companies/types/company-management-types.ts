import type {
  CompanyPageResponse,
  CompanyStatus,
} from "@/app/(panel)/_modules/companies/types/company-types";

export enum CompanyStatusFilter {
  ALL = "ALL",
  ACTIVE = "ACTIVE",
  DISABLED = "DISABLED",
}

export const COMPANY_STATUSES: CompanyStatus[] = ["ACTIVE", "DISABLED"];

export interface CompanyFilterState {
  searchTerm: string;
  status: CompanyStatusFilter;
}

export enum CompanyDirectoryStatus {
  Loading = "LOADING",
  Error = "ERROR",
  Empty = "EMPTY",
  Ready = "READY",
}

export type CompanyDirectoryState =
  | { status: CompanyDirectoryStatus.Loading }
  | { status: CompanyDirectoryStatus.Error; message: string }
  | { status: CompanyDirectoryStatus.Empty; page: CompanyPageResponse }
  | { status: CompanyDirectoryStatus.Ready; page: CompanyPageResponse };

export const INITIAL_FILTERS: CompanyFilterState = {
  searchTerm: "",
  status: CompanyStatusFilter.ALL,
};
