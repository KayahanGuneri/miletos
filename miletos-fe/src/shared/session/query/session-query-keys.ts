export const sessionQueryKeys = {
  all: ["auth"] as const,
  currentUser: () => [...sessionQueryKeys.all, "current-user"] as const,
};
