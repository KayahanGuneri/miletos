import type { Company } from "@/app/(panel)/_modules/companies/types/company-types";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import styles from "./CompanyManagementPage.module.css";

interface CompanyDeleteConfirmationProps {
  company: Company;
  isDeleting: boolean;
  onCancel: () => void;
  onConfirm: (company: Company) => void;
}

export const CompanyDeleteConfirmation = ({
  company,
  isDeleting,
  onCancel,
  onConfirm,
}: CompanyDeleteConfirmationProps) => (
  <Box className={styles.companyManagement__deleteBox} role="alert">
    <span className={styles.companyManagement__deleteIcon}>
      <Icon name="trash" />
    </span>

    <Box>
      <Typography as="h3">Delete {company.name}?</Typography>

      <Typography as="p">
        This permanently removes the company and related company users. This action cannot be undone
        from the UI.
      </Typography>
    </Box>

    <Box className={styles.companyManagement__rowActions}>
      <Button
        disabled={isDeleting}
        type="button"
        variant={ButtonVariant.Secondary}
        onClick={onCancel}
      >
        Cancel
      </Button>

      <Button
        className={styles.companyManagement__dangerButton}
        disabled={isDeleting}
        type="button"
        variant={ButtonVariant.Primary}
        onClick={() => onConfirm(company)}
      >
        <Icon name="trash" />

        {isDeleting ? "Deleting..." : "Delete company"}
      </Button>
    </Box>
  </Box>
);
