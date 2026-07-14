import Image from "next/image";
import Link from "next/link";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import styles from "./CompanyManagementPage.module.css";

export const CompanyManagementHeader = () => (
  <>
    <header className={styles.companyManagement__header}>
      <Box>
        <Typography as="p" className={styles.companyManagement__eyebrow}>
          Superadmin operations
        </Typography>

        <Typography as="h1" className={styles.companyManagement__title}>
          Company management
        </Typography>

        <Typography as="p" className={styles.companyManagement__description}>
          Create and maintain tenant workspaces, control lifecycle status and open company-level
          user administration.
        </Typography>
      </Box>

      <Box className={styles.companyManagement__actions}>
        <Link className={styles.companyManagement__link} href="/profile">
          <Icon name="shield" />
          Profile
        </Link>

        <Link className={styles.companyManagement__link} href="/dashboard">
          <Icon name="arrow-left" />
          Back to dashboard
        </Link>
      </Box>
    </header>

    <section className={styles.companyManagement__hero}>
      <Box className={styles.companyManagement__heroContent}>
        <Box className={styles.companyManagement__heroBadge}>
          <Icon name="building" />
          Tenant orchestration
        </Box>

        <Typography as="h2" className={styles.companyManagement__heroTitle}>
          Build a clear operating model for every company workspace.
        </Typography>

        <Typography as="p" className={styles.companyManagement__heroDescription}>
          Keep tenant creation, status changes and user administration connected in one protected
          superadmin flow.
        </Typography>

        <Box className={styles.companyManagement__heroTags}>
          <span>
            <Icon name="check" />
            Lifecycle control
          </span>

          <span>
            <Icon name="shield" />
            Isolated tenants
          </span>

          <span>
            <Icon name="team" />
            Connected user access
          </span>
        </Box>
      </Box>

      <Box className={styles.companyManagement__heroImageFrame}>
        <Image
          priority
          alt="Company management workflow illustration"
          className={styles.companyManagement__heroImage}
          height={600}
          src="/images/illustrations/company-management-workflow.png"
          width={900}
        />
      </Box>
    </section>
  </>
);
