export enum CompanyUserRoleFilter {
  All = "ALL",
  Admin = "ADMIN",
  Moderator = "MOD",
  User = "USER",
  Superadmin = "SUPERADMIN",
}

export interface CompanyUserFilters {
  role: CompanyUserRoleFilter;
  searchTerm: string;
}

export const INITIAL_COMPANY_USER_FILTERS: CompanyUserFilters = {
  role: CompanyUserRoleFilter.All,
  searchTerm: "",
};
