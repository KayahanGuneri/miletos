"use client";

import { useState } from "react";
import {
  type CompanyDirectoryState,
  CompanyDirectoryStatus,
} from "@/app/(panel)/_modules/companies/types/company-management-types";
import { useCompaniesQuery } from "@/app/(panel)/_modules/companies/query/useCompaniesQuery";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import { canCreateCompany } from "@/shared/session/permissions/session-permissions";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { CompanyDirectory } from "./ui/CompanyDirectory";
import { CompanyLifecycleCard } from "./ui/CompanyLifecycleCard";
import { CompanyManagementHeader } from "./ui/CompanyManagementHeader";
import { CompanySummary } from "./ui/CompanySummary";
import { CreateCompanyCard } from "./ui/CreateCompanyCard";
import styles from "./ui/CompanyManagementPage.module.css";

const PAGE_SIZE = 10;

export default function CompanyManagementPage() {
  const currentUserQuery = useCurrentUserQuery();
  const [page, setPage] = useState(0);
  const isAllowed = canCreateCompany(currentUserQuery.data);
  const companiesQuery = useCompaniesQuery(page, PAGE_SIZE, { enabled: isAllowed });

  let directoryState: CompanyDirectoryState;

  if (companiesQuery.isError) {
    directoryState = {
      status: CompanyDirectoryStatus.Error,
      message: companiesQuery.error.message,
    };
  } else if (!companiesQuery.data) {
    directoryState = { status: CompanyDirectoryStatus.Loading };
  } else if (companiesQuery.data.content.length === 0) {
    directoryState = { status: CompanyDirectoryStatus.Empty, page: companiesQuery.data };
  } else {
    directoryState = { status: CompanyDirectoryStatus.Ready, page: companiesQuery.data };
  }

  return (
    <main className={styles.companyManagement}>
      <Box className={styles.companyManagement__shell}>
        <CompanyManagementHeader />

        {!isAllowed ? (
          <section className={styles.companyManagement__card}>
            <Typography as="p" className={styles.companyManagement__error} role="alert">
              You are not allowed to manage companies. This page is only available for active
              superadmin users.
            </Typography>
          </section>
        ) : (
          <>
            <CompanySummary page={companiesQuery.data} pageSize={PAGE_SIZE} />

            <section className={styles.companyManagement__workspaceGrid}>
              <CreateCompanyCard onCreated={() => setPage(0)} />
              <CompanyLifecycleCard />
            </section>

            <CompanyDirectory
              isRefreshing={companiesQuery.isFetching}
              state={directoryState}
              onPageChange={setPage}
              onRefresh={() => void companiesQuery.refetch()}
            />
          </>
        )}
      </Box>
    </main>
  );
}
