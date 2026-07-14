import { profileMessages } from "../messages/profile-messages";

export const formatProfileLabel = (value: string | null | undefined) => {
  if (!value) {
    return profileMessages.common.notAssigned;
  }

  return value
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
};

export const getProfileDisplayName = (
  firstName: string | null | undefined,
  lastName: string | null | undefined,
) => [firstName, lastName].filter(Boolean).join(" ").trim() || "Miletos User";

export const getProfileInitials = (
  firstName: string | null | undefined,
  lastName: string | null | undefined,
) => `${firstName?.charAt(0) ?? "M"}${lastName?.charAt(0) ?? ""}`.toUpperCase();
