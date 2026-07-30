"use client";

import Image from "next/image";
import Link from "next/link";
import { Box } from "@/components/lib/box/Box";
import Icon from "@/components/lib/icon/Icon";
import { Typography } from "@/components/lib/typography/Typography";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import {
  canAccessCompanyUsers,
  canCreateCompany,
} from "@/shared/session/permissions/session-permissions";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import styles from "./DashboardPage.module.css";
import {
  formatSessionLabel,
  resolveSessionRoleLabel,
} from "@/shared/session/utils/session-label-utils";

const dashboardMessages = {
  page: {
    eyebrow: "Miletos workspace",
    welcome: "Welcome back",
    description:
      "Monitor your authenticated workspace, open the administration areas available to your role and keep account operations under control.",
  },
  identity: {
    defaultName: "Miletos User",
    globalContext: "Global tenant access",
    companyContext: "Company workspace",
    noCompany: "No company assigned",
    noCompanyId: "Tenant scope resolved by role",
  },
} as const;

function dashboardIconSource(name: string) {
  return `/icons/dashboard/${name}.svg`;
}

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
            <Icon src={dashboardIconSource("sparkles")} size={17} />
            Workflow operations center
          </Box>

          <Typography as="h2" className={styles.dashboardHome__heroTitle}>
            One secure workspace for tenants, teams and workflow operations.
          </Typography>

          <Typography as="p" className={styles.dashboardHome__heroDescription}>
            Move between account security, company administration and user onboarding without
            leaving the authenticated Miletos environment.
          </Typography>

          <Box className={styles.dashboardHome__heroTags}>
            <Typography as="span">
              <Icon src={dashboardIconSource("shield")} size={16} />
              Protected session
            </Typography>

            <Typography as="span">
              <Icon src={dashboardIconSource("key")} size={16} />
              Role-aware access
            </Typography>

            <Typography as="span">
              <Icon src={dashboardIconSource("dashboard")} size={16} />
              Unified operations
            </Typography>
          </Box>
        </Box>

        <figure className={styles.dashboardHome__heroVisual}>
          <Typography as="span" className={styles.dashboardHome__imageGlow} aria-hidden="true" />

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
          <Typography as="span" className={styles.dashboardHome__metricIcon}>
            <Icon src={dashboardIconSource("check")} />
          </Typography>

          <Box>
            <Typography as="p" className={styles.dashboardHome__metricLabel}>
              Account status
            </Typography>

            <Typography as="strong" className={styles.dashboardHome__metricValue}>
              {statusLabel}
            </Typography>

            <Typography as="span" className={styles.dashboardHome__metricHint}>
              Authenticated account state
            </Typography>
          </Box>
        </article>

        <article className={styles.dashboardHome__metricCard}>
          <Typography as="span" className={styles.dashboardHome__metricIcon}>
            <Icon src={dashboardIconSource("shield")} />
          </Typography>

          <Box>
            <Typography as="p" className={styles.dashboardHome__metricLabel}>
              Access role
            </Typography>

            <Typography as="strong" className={styles.dashboardHome__metricValue}>
              {roleLabel}
            </Typography>

            <Typography as="span" className={styles.dashboardHome__metricHint}>
              Permission-scoped navigation
            </Typography>
          </Box>
        </article>

        <article className={styles.dashboardHome__metricCard}>
          <Typography as="span" className={styles.dashboardHome__metricIcon}>
            <Icon src={dashboardIconSource("building")} />
          </Typography>

          <Box>
            <Typography as="p" className={styles.dashboardHome__metricLabel}>
              Company context
            </Typography>

            <Typography as="strong" className={styles.dashboardHome__metricValue}>
              {companyContext}
            </Typography>

            <Typography as="span" className={styles.dashboardHome__metricHint}>
              {user?.companyId
                ? `ID ${String(user.companyId).slice(0, 8)}…`
                : dashboardMessages.identity.noCompanyId}
            </Typography>
          </Box>
        </article>

        <article className={styles.dashboardHome__metricCard}>
          <Typography as="span" className={styles.dashboardHome__metricIcon}>
            <Icon src={dashboardIconSource("key")} />
          </Typography>

          <Box>
            <Typography as="p" className={styles.dashboardHome__metricLabel}>
              Onboarding
            </Typography>

            <Typography as="strong" className={styles.dashboardHome__metricValue}>
              {onboardingLabel}
            </Typography>

            <Typography as="span" className={styles.dashboardHome__metricHint}>
              Access setup progression
            </Typography>
          </Box>
        </article>
      </section>

      <section className={styles.dashboardHome__workspaceGrid}>
        <article className={styles.dashboardHome__actionsCard}>
          <Box className={styles.dashboardHome__cardHeading}>
            <Typography as="span" className={styles.dashboardHome__cardIcon}>
              <Icon src={dashboardIconSource("dashboard")} />
            </Typography>

            <Box>
              <Typography as="p" className={styles.dashboardHome__cardEyebrow}>
                Role-aware navigation
              </Typography>

              <Typography as="h2" className={styles.dashboardHome__cardTitle}>
                Workspace actions
              </Typography>
            </Box>
          </Box>

          <Typography as="p" className={styles.dashboardHome__cardDescription}>
            Open the account and administration areas currently available to your authenticated
            role.
          </Typography>

          <nav className={styles.dashboardHome__quickActions} aria-label="Dashboard quick actions">
            <Link className={styles.dashboardHome__quickAction} href="/dashboard/executions">
              <Typography as="span" className={styles.dashboardHome__quickActionIcon}>
                <Icon src={dashboardIconSource("dashboard")} />
              </Typography>

              <Typography as="span" className={styles.dashboardHome__quickActionCopy}>
                <Typography as="strong">Workflow executions</Typography>

                <Typography as="small">
                  Monitor runtime status, execution history and observability details.
                </Typography>
              </Typography>

              <Typography as="span" className={styles.dashboardHome__quickActionArrow}>
                <Icon src={dashboardIconSource("arrow")} size={18} />
              </Typography>
            </Link>
            <Link className={styles.dashboardHome__quickAction} href="/profile">
              <Typography as="span" className={styles.dashboardHome__quickActionIcon}>
                <Icon src={dashboardIconSource("profile")} />
              </Typography>

              <Typography as="span" className={styles.dashboardHome__quickActionCopy}>
                <Typography as="strong">Manage profile</Typography>

                <Typography as="small">
                  Update your photo, credentials and account security.
                </Typography>
              </Typography>

              <Typography as="span" className={styles.dashboardHome__quickActionArrow}>
                <Icon src={dashboardIconSource("arrow")} size={18} />
              </Typography>
            </Link>

            {canOpenCompanyManagement ? (
              <Link className={styles.dashboardHome__quickAction} href="/admin/companies">
                <Typography as="span" className={styles.dashboardHome__quickActionIcon}>
                  <Icon src={dashboardIconSource("building")} />
                </Typography>

                <Typography as="span" className={styles.dashboardHome__quickActionCopy}>
                  <Typography as="strong">Manage companies</Typography>

                  <Typography as="small">
                    Create tenants and control company lifecycle status.
                  </Typography>
                </Typography>

                <Typography as="span" className={styles.dashboardHome__quickActionArrow}>
                  <Icon src={dashboardIconSource("arrow")} size={18} />
                </Typography>
              </Link>
            ) : null}

            {companyUsersHref ? (
              <Link className={styles.dashboardHome__quickAction} href={companyUsersHref}>
                <Typography as="span" className={styles.dashboardHome__quickActionIcon}>
                  <Icon src={dashboardIconSource("team")} />
                </Typography>

                <Typography as="span" className={styles.dashboardHome__quickActionCopy}>
                  <Typography as="strong">Manage company users</Typography>

                  <Typography as="small">Review members, roles and onboarding progress.</Typography>
                </Typography>

                <Typography as="span" className={styles.dashboardHome__quickActionArrow}>
                  <Icon src={dashboardIconSource("arrow")} size={18} />
                </Typography>
              </Link>
            ) : null}
          </nav>
        </article>

        <article className={styles.dashboardHome__identityCard}>
          <Box className={styles.dashboardHome__cardHeading}>
            <Typography as="span" className={styles.dashboardHome__cardIcon}>
              <Icon src={dashboardIconSource("profile")} />
            </Typography>

            <Box>
              <Typography as="p" className={styles.dashboardHome__cardEyebrow}>
                Authenticated identity
              </Typography>

              <Typography as="h2" className={styles.dashboardHome__cardTitle}>
                Account overview
              </Typography>
            </Box>
          </Box>

          <Box className={styles.dashboardHome__identity}>
            <Typography as="span" className={styles.dashboardHome__avatar} aria-hidden="true">
              {initials}
            </Typography>

            <Box className={styles.dashboardHome__identityCopy}>
              <Typography as="strong">{displayName}</Typography>

              <Typography as="span">{user?.email}</Typography>
            </Box>
          </Box>

          <dl className={styles.dashboardHome__details}>
            <Box className={styles.dashboardHome__detailRow}>
              <dt>
                <Icon src={dashboardIconSource("mail")} size={17} />
                Email address
              </dt>

              <dd>{user?.email}</dd>
            </Box>

            <Box className={styles.dashboardHome__detailRow}>
              <dt>
                <Icon src={dashboardIconSource("shield")} size={17} />
                Role
              </dt>

              <dd>
                <Typography as="span" className={styles.dashboardHome__badge}>
                  {roleLabel}
                </Typography>
              </dd>
            </Box>

            <Box className={styles.dashboardHome__detailRow}>
              <dt>
                <Icon src={dashboardIconSource("check")} size={17} />
                Status
              </dt>

              <dd>{statusLabel}</dd>
            </Box>

            <Box className={styles.dashboardHome__detailRow}>
              <dt>
                <Icon src={dashboardIconSource("building")} size={17} />
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
          <Typography as="span" className={styles.dashboardHome__securityIcon}>
            <Icon src={dashboardIconSource("shield")} size={24} />
          </Typography>

          <Box>
            <Typography as="p" className={styles.dashboardHome__securityEyebrow}>
              Workspace safeguards
            </Typography>

            <Typography as="h2" className={styles.dashboardHome__securityTitle}>
              Security follows every operation.
            </Typography>
          </Box>
        </Box>

        <Box className={styles.dashboardHome__securityItems}>
          <Typography as="span">
            <Icon src={dashboardIconSource("key")} size={18} />

            <Typography as="strong">Session protection</Typography>

            <Typography as="small">Authenticated token lifecycle</Typography>
          </Typography>

          <Typography as="span">
            <Icon src={dashboardIconSource("team")} size={18} />

            <Typography as="strong">Role boundaries</Typography>

            <Typography as="small">Features shown by permission</Typography>
          </Typography>

          <Typography as="span">
            <Icon src={dashboardIconSource("building")} size={18} />

            <Typography as="strong">Tenant isolation</Typography>

            <Typography as="small">Company-scoped administration</Typography>
          </Typography>
        </Box>
      </section>
    </PageShell>
  );
};

export default DashboardPage;
