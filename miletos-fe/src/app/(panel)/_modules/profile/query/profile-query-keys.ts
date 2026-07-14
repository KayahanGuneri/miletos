export const profileQueryKeys = {
  all: ["auth"] as const,
  currentUserProfilePhoto: () => [...profileQueryKeys.all, "current-user-profile-photo"] as const,
};
