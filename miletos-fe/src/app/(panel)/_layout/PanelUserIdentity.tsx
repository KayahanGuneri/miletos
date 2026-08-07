import { type UserProfile, type UserRole } from "@/shared/session/types/session-user-types";

const USER_ROLE_LABELS: Record<UserRole, string> = {
  ADMIN: "Admin",
  MOD: "Mod",
  USER: "User",
};

interface PanelUserIdentityProps {
  user: UserProfile;
  avatarClassName: string;
  identityClassName: string;
}

function getDisplayName(user: UserProfile) {
  const fullName = [user.firstName, user.lastName].filter(Boolean).join(" ").trim();

  return fullName || "Miletos User";
}

function getInitials(user: UserProfile) {
  const firstInitial = user.firstName?.charAt(0) ?? "M";
  const lastInitial = user.lastName?.charAt(0) ?? "";

  return `${firstInitial}${lastInitial}`.toUpperCase();
}

function getRoleLabel(user: UserProfile) {
  if (user.superAdmin) {
    return "Superadmin";
  }

  if (!user.role) {
    return "User";
  }

  return USER_ROLE_LABELS[user.role];
}

export function PanelUserIdentity({
  user,
  avatarClassName,
  identityClassName,
}: PanelUserIdentityProps) {
  return (
    <>
      <span className={avatarClassName}>{getInitials(user)}</span>

      <span className={identityClassName}>
        <strong>{getDisplayName(user)}</strong>

        <small>{getRoleLabel(user)}</small>
      </span>
    </>
  );
}
