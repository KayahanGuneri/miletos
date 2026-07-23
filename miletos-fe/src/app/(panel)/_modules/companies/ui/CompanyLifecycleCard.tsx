import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import styles from "./CompanyManagementPage.module.css";

export const CompanyLifecycleCard = () => (
  <aside
    className={[styles.companyManagement__card, styles.companyManagement__safetyCard].join(" ")}
  >
    <Box className={styles.companyManagement__cardHeader}>
      <span className={styles.companyManagement__cardIcon}>
        <Icon name="shield" />
      </span>

      <Box>
        <Typography as="p" className={styles.companyManagement__cardEyebrow}>
          Administration safety
        </Typography>

        <Typography as="h2" className={styles.companyManagement__cardTitle}>
          Tenant lifecycle
        </Typography>
      </Box>
    </Box>

    <ul className={styles.companyManagement__guidanceList}>
      <li>
        <Icon name="check" />

        <span>
          <Typography as="h3">Active</Typography>
          <Typography as="p">Company users can operate within the tenant scope.</Typography>
        </span>
      </li>

      <li>
        <Icon name="shield" />

        <span>
          <Typography as="h3">Disabled</Typography>
          <Typography as="p">Use lifecycle status before considering permanent removal.</Typography>
        </span>
      </li>

      <li>
        <Icon name="trash" />

        <span>
          <Typography as="h3">Hard delete</Typography>
          <Typography as="p">Deletion removes the tenant and related company users.</Typography>
        </span>
      </li>
    </ul>
  </aside>
);
