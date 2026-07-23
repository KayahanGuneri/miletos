import Link from "next/link";
import type { Company } from "@/app/(panel)/_modules/companies/types/company-types";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import styles from "./CompanyManagementPage.module.css";

interface CompanyTableRowProps {
  company: Company;
  isDeleting: boolean;
  onRequestDelete: (company: Company) => void;
  onStartEditing: (company: Company) => void;
}

const formatDate = (value: string) =>
  new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));

export function CompanyTableRow({
  company,
  isDeleting,
  onRequestDelete,
  onStartEditing,
}: CompanyTableRowProps) {
  const companyId = String(company.id);
  const shortCompanyId = companyId.length > 12 ? `${companyId.slice(0, 12)}…` : companyId;

  return (
    <tr>
      <td>
        <Box className={styles.companyManagement__companyCell}>
          <span className={styles.companyManagement__companyIcon}>
            <Icon name="building" />
          </span>

          <Box>
            <strong>{company.name}</strong>

            <span className={styles.companyManagement__mutedId} title={companyId}>
              ID {shortCompanyId}
            </span>
          </Box>
        </Box>
      </td>

      <td>
        <span
          className={[
            styles.companyManagement__statusBadge,
            company.status === "ACTIVE"
              ? styles.companyManagement__statusBadgeActive
              : styles.companyManagement__statusBadgeDisabled,
          ].join(" ")}
        >
          <span />
          {company.status === "ACTIVE" ? "Active" : "Disabled"}
        </span>
      </td>

      <td>
        <span className={styles.companyManagement__date}>{formatDate(company.createdAt)}</span>
      </td>

      <td>
        <Link
          className={styles.companyManagement__tableLink}
          href={`/admin/companies/${company.id}/users`}
        >
          <Icon name="team" />
          Manage users
        </Link>
      </td>

      <td>
        <Box className={styles.companyManagement__rowActions}>
          <Button
            disabled={isDeleting}
            type="button"
            variant={ButtonVariant.Secondary}
            onClick={() => onStartEditing(company)}
          >
            <Icon name="edit" />
            Edit
          </Button>

          <Button
            className={styles.companyManagement__dangerGhostButton}
            disabled={isDeleting}
            type="button"
            variant={ButtonVariant.Secondary}
            onClick={() => onRequestDelete(company)}
          >
            <Icon name="trash" />
            Delete
          </Button>
        </Box>
      </td>
    </tr>
  );
}
