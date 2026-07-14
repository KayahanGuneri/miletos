import type { CompanyPageResponse } from "@/app/(panel)/_modules/companies/types/company-types";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import styles from "./CompanyManagementPage.module.css";

interface CompanySummaryProps {
  page?: CompanyPageResponse;
  pageSize: number;
}

export function CompanySummary({ page, pageSize }: CompanySummaryProps) {
  const companies = page?.content ?? [];
  const activeOnPage = companies.filter((company) => company.status === "ACTIVE").length;
  const disabledOnPage = companies.filter((company) => company.status === "DISABLED").length;
  const totalPages = Math.max(page?.totalPages ?? 1, 1);

  return (
    <section aria-label="Company summary" className={styles.companyManagement__metrics}>
      <article>
        <span>
          <Icon name="building" />
        </span>

        <Box>
          <Typography as="p">Total companies</Typography>
          <strong>{page?.totalElements ?? "-"}</strong>
          <small>Across all result pages</small>
        </Box>
      </article>

      <article>
        <span>
          <Icon name="check" />
        </span>

        <Box>
          <Typography as="p">Active on page</Typography>
          <strong>{activeOnPage}</strong>
          <small>Currently enabled tenants</small>
        </Box>
      </article>

      <article>
        <span>
          <Icon name="shield" />
        </span>

        <Box>
          <Typography as="p">Disabled on page</Typography>
          <strong>{disabledOnPage}</strong>
          <small>Restricted tenant access</small>
        </Box>
      </article>

      <article>
        <span>
          <Icon name="team" />
        </span>

        <Box>
          <Typography as="p">Current page</Typography>
          <strong>{page ? `${page.page + 1}/${totalPages}` : "-"}</strong>
          <small>{page?.size ?? pageSize} tenants per page</small>
        </Box>
      </article>
    </section>
  );
}
