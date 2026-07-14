"use client";

import { useCreateCompanyMutation } from "@/app/(panel)/_modules/companies/query/useCreateCompanyMutation";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import { CompanyForm, CompanyFormMode } from "./CompanyForm";
import styles from "./CompanyManagementPage.module.css";

interface CreateCompanyCardProps {
  onCreated: () => void;
}

export function CreateCompanyCard({ onCreated }: CreateCompanyCardProps) {
  const createCompanyMutation = useCreateCompanyMutation();

  return (
    <article className={styles.companyManagement__card}>
      <Box className={styles.companyManagement__cardHeader}>
        <span className={styles.companyManagement__cardIcon}>
          <Icon name="plus" />
        </span>

        <Box>
          <Typography as="p" className={styles.companyManagement__cardEyebrow}>
            New tenant
          </Typography>

          <Typography as="h2" className={styles.companyManagement__cardTitle}>
            Create company
          </Typography>
        </Box>
      </Box>

      <Typography as="p" className={styles.companyManagement__sectionDescription}>
        Create the tenant record first, then open its user management flow.
      </Typography>

      {createCompanyMutation.isSuccess ? (
        <Typography as="p" className={styles.companyManagement__success} role="status">
          Company created successfully: {createCompanyMutation.data.name}
        </Typography>
      ) : null}

      <CompanyForm
        errorMessage={createCompanyMutation.error?.message}
        isSubmitting={createCompanyMutation.isPending}
        mode={CompanyFormMode.Create}
        nameError={createCompanyMutation.error?.fieldErrors?.name}
        onSubmit={async ({ name }) => {
          await createCompanyMutation.mutateAsync({ name });
          onCreated();
        }}
      />
    </article>
  );
}
