"use client";

import Image from "next/image";
import Link from "next/link";
import DashboardIcon from "./components/DashboardIcon";
import { Box } from "@/components/lib/box/Box";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import {
  canAccessCompanyUsers,
  canCreateCompany,
} from "@/shared/session/permissions/session-permissions";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import styles from "./DashboardPage.module.css";
import { dashboardMessages } from "./messages/dashboard-messages";
import {
  formatSessionLabel,
  resolveSessionRoleLabel,
} from "@/shared/session/utils/session-label-utils";

const DashboardPage = () => {
  const currentUserQuery = useCurrentUserQuery();

  const user = currentUserQuery.data;

  const canOpenCompanyManagement = canCreateCompany(user);

  const companyUsersHref =
    canAccessCompanyUsers(user) && user?.companyId
      ? `/admin/companies/${user.companyId}/users`
      : null;

  const displayName =
    [user?.firstName, user?.lastName].filter(Boolean).join(" ").trim() ||
    dashboardMessages.identity.defaultName;

  const initials = `${user?.firstName?.[0] ?? "M"}${user?.lastName?.[0] ?? ""}`.toUpperCase();

  const statusLabel = formatSessionLabel(user?.status);

  const roleLabel = resolveSessionRoleLabel(user);

  const onboardingLabel = formatSessionLabel(user?.onboardingStatus);

  let companyContext: string = dashboardMessages.identity.noCompany;
  if (user?.superAdmin) {
    companyContext = dashboardMessages.identity.globalContext;
  } else if (user?.companyId) {
    companyContext = dashboardMessages.identity.companyContext;
  }

  return (
    <PageShell
      eyebrow={dashboardMessages.page.eyebrow}
      title={`${dashboardMessages.page.welcome}${user?.firstName ? `, ${user.firstName}` : ""}`}
      description={dashboardMessages.page.description}
    >
      <section className={styles.dashboardHome__hero}>
        <Box className={styles.dashboardHome__heroContent}>
          <Box className={styles.dashboardHome__heroBadge}>
            <DashboardIcon name="sparkles" size={17} />
            Workflow operations center
          </Box>

          <h2 className={styles.dashboardHome__heroTitle}>
            One secure workspace for tenants, teams and workflow operations.
          </h2>

          <p className={styles.dashboardHome__heroDescription}>
            Move between account security, company administration and user onboarding without
            leaving the authenticated Miletos environment.
          </p>

          <Box className={styles.dashboardHome__heroTags}>
            <span>
              <DashboardIcon name="shield" size={16} />
              Protected session
            </span>

            <span>
              <DashboardIcon name="key" size={16} />
              Role-aware access
            </span>

            <span>
              <DashboardIcon name="dashboard" size={16} />
              Unified operations
            </span>
          </Box>
        </Box>

        <figure className={styles.dashboardHome__heroVisual}>
          <span className={styles.dashboardHome__imageGlow} aria-hidden="true" />

          <Image
            className={styles.dashboardHome__heroImage}
            src="/images/illustrations/dashboard-workflow.png"
            alt="Workflow automation dashboard illustration"
            width={1672}
            height={941}
            priority
          />
        </figure>
      </section>

      <section className={styles.dashboardHome__metrics} aria-label="Workspace account summary">
        <article className={styles.dashboardHome__metricCard}>
          <span className={styles.dashboardHome__metricIcon}>
            <DashboardIcon name="check" />
          </span>

          <Box>
            <p className={styles.dashboardHome__metricLabel}>Account status</p>

            <strong className={styles.dashboardHome__metricValue}>{statusLabel}</strong>

            <span className={styles.dashboardHome__metricHint}>Authenticated account state</span>
          </Box>
        </article>

        <article className={styles.dashboardHome__metricCard}>
          <span className={styles.dashboardHome__metricIcon}>
            <DashboardIcon name="shield" />
          </span>

          <Box>
            <p className={styles.dashboardHome__metricLabel}>Access role</p>

            <strong className={styles.dashboardHome__metricValue}>{roleLabel}</strong>

            <span className={styles.dashboardHome__metricHint}>Permission-scoped navigation</span>
          </Box>
        </article>

        <article className={styles.dashboardHome__metricCard}>
          <span className={styles.dashboardHome__metricIcon}>
            <DashboardIcon name="building" />
          </span>

          <Box>
            <p className={styles.dashboardHome__metricLabel}>Company context</p>

            <strong className={styles.dashboardHome__metricValue}>{companyContext}</strong>

            <span className={styles.dashboardHome__metricHint}>
              {user?.companyId
                ? `ID ${String(user.companyId).slice(0, 8)}…`
                : dashboardMessages.identity.noCompanyId}
            </span>
          </Box>
        </article>

        <article className={styles.dashboardHome__metricCard}>
          <span className={styles.dashboardHome__metricIcon}>
            <DashboardIcon name="key" />
          </span>

          <Box>
            <p className={styles.dashboardHome__metricLabel}>Onboarding</p>

            <strong className={styles.dashboardHome__metricValue}>{onboardingLabel}</strong>

            <span className={styles.dashboardHome__metricHint}>Access setup progression</span>
          </Box>
        </article>
      </section>

      <section className={styles.dashboardHome__workspaceGrid}>
        <article className={styles.dashboardHome__actionsCard}>
          <Box className={styles.dashboardHome__cardHeading}>
            <span className={styles.dashboardHome__cardIcon}>
              <DashboardIcon name="dashboard" />
            </span>

            <Box>
              <p className={styles.dashboardHome__cardEyebrow}>Role-aware navigation</p>

              <h2 className={styles.dashboardHome__cardTitle}>Workspace actions</h2>
            </Box>
          </Box>

          <p className={styles.dashboardHome__cardDescription}>
            Open the account and administration areas currently available to your authenticated
            role.
          </p>

          <nav className={styles.dashboardHome__quickActions} aria-label="Dashboard quick actions">
            <Link
              className={styles.dashboardHome__quickAction}
              href="/dashboard/executions"
            >
              <span className={styles.dashboardHome__quickActionIcon}>
                <DashboardIcon name="dashboard" />
              </span>

              <span className={styles.dashboardHome__quickActionCopy}>
                <strong>Workflow executions</strong>

                <small>
                  Monitor runtime status, execution history and observability details.
                </small>
              </span>

              <span className={styles.dashboardHome__quickActionArrow}>
                <DashboardIcon name="arrow" size={18} />
              </span>
            </Link>
            <Link className={styles.dashboardHome__quickAction} href="/profile">
              <span className={styles.dashboardHome__quickActionIcon}>
                <DashboardIcon name="profile" />
              </span>

              <span className={styles.dashboardHome__quickActionCopy}>
                <strong>Manage profile</strong>

                <small>Update your photo, credentials and account security.</small>
              </span>

              <span className={styles.dashboardHome__quickActionArrow}>
                <DashboardIcon name="arrow" size={18} />
              </span>
            </Link>

            {canOpenCompanyManagement ? (
              <Link className={styles.dashboardHome__quickAction} href="/admin/companies">
                <span className={styles.dashboardHome__quickActionIcon}>
                  <DashboardIcon name="building" />
                </span>

                <span className={styles.dashboardHome__quickActionCopy}>
                  <strong>Manage companies</strong>

                  <small>Create tenants and control company lifecycle status.</small>
                </span>

                <span className={styles.dashboardHome__quickActionArrow}>
                  <DashboardIcon name="arrow" size={18} />
                </span>
              </Link>
            ) : null}

            {companyUsersHref ? (
              <Link className={styles.dashboardHome__quickAction} href={companyUsersHref}>
                <span className={styles.dashboardHome__quickActionIcon}>
                  <DashboardIcon name="team" />
                </span>

                <span className={styles.dashboardHome__quickActionCopy}>
                  <strong>Manage company users</strong>

                  <small>Review members, roles and onboarding progress.</small>
                </span>

                <span className={styles.dashboardHome__quickActionArrow}>
                  <DashboardIcon name="arrow" size={18} />
                </span>
              </Link>
            ) : null}
          </nav>
        </article>

        <article className={styles.dashboardHome__identityCard}>
          <Box className={styles.dashboardHome__cardHeading}>
            <span className={styles.dashboardHome__cardIcon}>
              <DashboardIcon name="profile" />
            </span>

            <Box>
              <p className={styles.dashboardHome__cardEyebrow}>Authenticated identity</p>

              <h2 className={styles.dashboardHome__cardTitle}>Account overview</h2>
            </Box>
          </Box>

          <Box className={styles.dashboardHome__identity}>
            <span className={styles.dashboardHome__avatar} aria-hidden="true">
              {initials}
            </span>

            <Box className={styles.dashboardHome__identityCopy}>
              <strong>{displayName}</strong>

              <span>{user?.email}</span>
            </Box>
          </Box>

          <dl className={styles.dashboardHome__details}>
            <Box className={styles.dashboardHome__detailRow}>
              <dt>
                <DashboardIcon name="mail" size={17} />
                Email address
              </dt>

              <dd>{user?.email}</dd>
            </Box>

            <Box className={styles.dashboardHome__detailRow}>
              <dt>
                <DashboardIcon name="shield" size={17} />
                Role
              </dt>

              <dd>
                <span className={styles.dashboardHome__badge}>{roleLabel}</span>
              </dd>
            </Box>

            <Box className={styles.dashboardHome__detailRow}>
              <dt>
                <DashboardIcon name="check" size={17} />
                Status
              </dt>

              <dd>{statusLabel}</dd>
            </Box>

            <Box className={styles.dashboardHome__detailRow}>
              <dt>
                <DashboardIcon name="building" size={17} />
                Company ID
              </dt>

              <dd title={user?.companyId != null ? String(user.companyId) : undefined}>
                {user?.companyId ?? formatSessionLabel(null)}
              </dd>
            </Box>
          </dl>
        </article>
      </section>

      <section className={styles.dashboardHome__securityStrip}>
        <Box className={styles.dashboardHome__securityIntro}>
          <span className={styles.dashboardHome__securityIcon}>
            <DashboardIcon name="shield" size={24} />
          </span>

          <Box>
            <p className={styles.dashboardHome__securityEyebrow}>Workspace safeguards</p>

            <h2 className={styles.dashboardHome__securityTitle}>
              Security follows every operation.
            </h2>
          </Box>
        </Box>

        <Box className={styles.dashboardHome__securityItems}>
          <span>
            <DashboardIcon name="key" size={18} />

            <strong>Session protection</strong>

            <small>Authenticated token lifecycle</small>
          </span>

          <span>
            <DashboardIcon name="team" size={18} />

            <strong>Role boundaries</strong>

            <small>Features shown by permission</small>
          </span>

          <span>
            <DashboardIcon name="building" size={18} />

            <strong>Tenant isolation</strong>

            <small>Company-scoped administration</small>
          </span>
        </Box>
      </section>
    </PageShell>
  );
};

export default DashboardPage;
