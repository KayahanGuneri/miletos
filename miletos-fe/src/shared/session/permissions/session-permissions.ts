import { type AuthenticatedUser, type UserRole } from "@/shared/session/types/session-user-types";

export const INVITABLE_USER_ROLES: UserRole[] = ["ADMIN", "MOD", "USER"];

export function isActiveCompletedUser(user: AuthenticatedUser | null | undefined) {
  return (
    user?.status === "ACTIVE" &&
    (user.onboardingStatus === "COMPLETED" || user.onboardingStatus === "NOT_REQUIRED")
  );
}

export function canCreateCompany(user: AuthenticatedUser | null | undefined) {
  return Boolean(user?.superAdmin && isActiveCompletedUser(user));
}

export function canManageWorkflows(user: AuthenticatedUser | null | undefined) {
  return Boolean(
    user &&
    isActiveCompletedUser(user) &&
    !user.superAdmin &&
    user.role === "ADMIN" &&
    user.companyId,
  );
}

export function canAccessCompanyUsers(
  user: AuthenticatedUser | null | undefined,
  companyId?: number,
) {
  if (!user || !isActiveCompletedUser(user)) {
    return false;
  }

  if (user.superAdmin) {
    return true;
  }

  return (
    user.role === "ADMIN" && Boolean(user.companyId) && (!companyId || user.companyId === companyId)
  );
}

export function canInviteUsers(user: AuthenticatedUser | null | undefined, companyId?: number) {
  return canAccessCompanyUsers(user, companyId);
}

export function canChangeOwnPassword(user: AuthenticatedUser | null | undefined) {
  return isActiveCompletedUser(user);
}

export function canUpdateOwnProfilePhoto(user: AuthenticatedUser | null | undefined) {
  return isActiveCompletedUser(user);
}
