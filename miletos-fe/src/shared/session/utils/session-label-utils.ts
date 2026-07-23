import type { AuthenticatedUser } from "../types/session-user-types";

export const formatSessionLabel = (value: string | null | undefined) => {
  if (!value) return "Not assigned";
  if (value === "MOD") return "Moderator";
  if (value === "SUPERADMIN") return "Superadmin";

  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
};

export const resolveSessionRoleLabel = (user: AuthenticatedUser | undefined) =>
  user?.superAdmin ? "Superadmin" : formatSessionLabel(user?.role);
