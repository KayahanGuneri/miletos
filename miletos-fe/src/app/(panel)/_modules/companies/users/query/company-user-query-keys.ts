export const companyUserQueryKeys = {
  all: ["company-users"] as const,
  byCompany: (companyId: number) => [...companyUserQueryKeys.all, companyId] as const,
};
