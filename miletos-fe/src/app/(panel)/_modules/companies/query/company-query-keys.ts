export const companyQueryKeys = {
  all: ["companies"] as const,
  list: (page: number, size: number) => [...companyQueryKeys.all, "list", page, size] as const,
};
