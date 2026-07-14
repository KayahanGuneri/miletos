import type { UserProfile } from "@/shared/session/types/session-user-types";
import { formatSessionLabel } from "@/shared/session/utils/session-label-utils";
import { CompanyUserRoleFilter, type CompanyUserFilters } from "../types/company-user-filter-types";

export enum CompanyUserBadgeTone {
  Muted = "MUTED",
  Success = "SUCCESS",
  Warning = "WARNING",
}

export const formatCompanyUserLabel = formatSessionLabel;

export const resolveCompanyUserRole = (user: UserProfile) =>
  user.superAdmin ? CompanyUserRoleFilter.Superadmin : user.role;

export const filterCompanyUsers = (users: UserProfile[], filters: CompanyUserFilters) => {
  const searchTerm = filters.searchTerm.trim().toLowerCase();
  return users.filter((user) => {
    const fullName = `${user.firstName} ${user.lastName}`.toLowerCase();
    const matchesSearch =
      !searchTerm ||
      fullName.includes(searchTerm) ||
      user.email.toLowerCase().includes(searchTerm) ||
      String(user.id).includes(searchTerm);
    const role = resolveCompanyUserRole(user);
    return matchesSearch && (filters.role === CompanyUserRoleFilter.All || role === filters.role);
  });
};

export const resolveStatusTone = (status: UserProfile["status"]) => {
  if (status === "ACTIVE") return CompanyUserBadgeTone.Success;
  if (status === "PENDING") return CompanyUserBadgeTone.Warning;
  return CompanyUserBadgeTone.Muted;
};

export const resolveOnboardingTone = (status: UserProfile["onboardingStatus"]) =>
  status === "COMPLETED" || status === "NOT_REQUIRED"
    ? CompanyUserBadgeTone.Success
    : CompanyUserBadgeTone.Warning;
