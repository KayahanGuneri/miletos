"use client";

import Image from "next/image";
import Link from "next/link";
import { useState } from "react";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import {
  canAccessCompanyUsers,
  canCreateCompany,
  canInviteUsers,
} from "@/shared/session/permissions/session-permissions";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { InviteUserForm } from "./components/InviteUserForm";
import { useCompanyUsersQuery } from "@/app/(panel)/_modules/companies/users/query/useCompanyUsersQuery";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import styles from "./CompanyUsersPage.module.css";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/lib/table/Table";
import {
  CompanyUserRoleFilter,
  INITIAL_COMPANY_USER_FILTERS,
} from "@/app/(panel)/_modules/companies/users/types/company-user-filter-types";
import {
  CompanyUserBadgeTone,
  filterCompanyUsers,
  formatCompanyUserLabel,
  resolveCompanyUserRole,
  resolveOnboardingTone,
  resolveStatusTone,
} from "./utils/company-user-utils";
import { companyUsersMessages } from "./messages/company-users-messages";

interface CompanyUsersPageProps {
  companyId: number;
}

export function CompanyUsersPage({ companyId }: CompanyUsersPageProps) {
  const currentUserQuery = useCurrentUserQuery();
  const user = currentUserQuery.data;
  const isAllowedToViewUsers = canAccessCompanyUsers(user, companyId);
  const isAllowedToInviteUsers = canInviteUsers(user, companyId);
  const canOpenCompanyManagement = canCreateCompany(user);
  const [filters, setFilters] = useState(INITIAL_COMPANY_USER_FILTERS);

  const companyUsersQuery = useCompanyUsersQuery({ companyId, enabled: isAllowedToViewUsers });
  const users = companyUsersQuery.data ?? [];
  const filteredUsers = filterCompanyUsers(users, filters);
  const badgeToneClass = {
    [CompanyUserBadgeTone.Muted]: styles.companyUsers__badgeMuted,
    [CompanyUserBadgeTone.Success]: styles.companyUsers__badgeSuccess,
    [CompanyUserBadgeTone.Warning]: styles.companyUsers__badgeWarning,
  };

  const activeUsers = users.filter((companyUser) => companyUser.status === "ACTIVE").length;
  const pendingUsers = users.filter(
    (companyUser) =>
      companyUser.status === "PENDING" || companyUser.onboardingStatus !== "COMPLETED",
  ).length;
  const adminUsers = users.filter(
    (companyUser) => companyUser.superAdmin || companyUser.role === "ADMIN",
  ).length;

  return (
    <PageShell
      eyebrow="Company administration"
      title="Company users"
      description="Invite users into the correct tenant context, review role assignments and follow onboarding state from one administration workspace."
      actions={
        <>
          <Link className={styles.companyUsers__link} href="/profile">
            <Icon name="user" />
            Profile
          </Link>
          {canOpenCompanyManagement ? (
            <Link className={styles.companyUsers__link} href="/admin/companies">
              <Icon name="building" />
              Company management
            </Link>
          ) : null}
          <Link className={styles.companyUsers__link} href="/dashboard">
            <Icon name="arrow-left" />
            Back to dashboard
          </Link>
        </>
      }
    >
      <section className={styles.companyUsers__intro}>
        <Box className={styles.companyUsers__introContent}>
          <Box className={styles.companyUsers__introBadge}>
            <Icon name="team" />
            Tenant user onboarding
          </Box>
          <Typography as="h2" className={styles.companyUsers__introTitle}>
            Put every person in the right company, role and access boundary.
          </Typography>
          <Typography as="p" className={styles.companyUsers__introDescription}>
            Assign ADMIN, MOD or USER access and keep membership aligned with the company-level
            authorization model.
          </Typography>
          <Box className={styles.companyUsers__introTags}>
            <span>
              <Icon name="mail" />
              Invitation flow
            </span>
            <span>
              <Icon name="shield" />
              Role boundaries
            </span>
            <span>
              <Icon name="check" />
              Onboarding visibility
            </span>
          </Box>
        </Box>
        <Box className={styles.companyUsers__introImageFrame}>
          <Image
            className={styles.companyUsers__introImage}
            src="/images/illustrations/company-users-team.png"
            alt="Company users and team onboarding illustration"
            width={900}
            height={675}
            priority
          />
        </Box>
      </section>

      <section className={styles.companyUsers__metrics} aria-label="Company user summary">
        <article>
          <span>
            <Icon name="team" />
          </span>
          <Box>
            <Typography as="p">Total users</Typography>
            <strong>{users.length}</strong>
            <small>Loaded company members</small>
          </Box>
        </article>
        <article>
          <span>
            <Icon name="check" />
          </span>
          <Box>
            <Typography as="p">Active</Typography>
            <strong>{activeUsers}</strong>
            <small>Operational accounts</small>
          </Box>
        </article>
        <article>
          <span>
            <Icon name="clock" />
          </span>
          <Box>
            <Typography as="p">Pending</Typography>
            <strong>{pendingUsers}</strong>
            <small>Awaiting activation or completion</small>
          </Box>
        </article>
        <article>
          <span>
            <Icon name="shield" />
          </span>
          <Box>
            <Typography as="p">Administrators</Typography>
            <strong>{adminUsers}</strong>
            <small>Tenant management access</small>
          </Box>
        </article>
      </section>

      <InviteUserForm companyId={companyId} isAllowed={isAllowedToInviteUsers} />

      <section className={styles.companyUsers__card}>
        <Box className={styles.companyUsers__cardHeader}>
          <Box className={styles.companyUsers__headingGroup}>
            <span className={styles.companyUsers__cardIcon}>
              <Icon name="team" />
            </span>
            <Box>
              <Typography as="p" className={styles.companyUsers__cardEyebrow}>
                Member directory
              </Typography>
              <Typography as="h2" className={styles.companyUsers__cardTitle}>
                Users
              </Typography>
              <span className={styles.companyUsers__companyId} title={String(companyId)}>
                Company ID · {String(companyId).slice(0, 12)}…
              </span>
            </Box>
          </Box>
          <Button
            type="button"
            variant={ButtonVariant.Secondary}
            disabled={companyUsersQuery.isFetching}
            onClick={() => void companyUsersQuery.refetch()}
          >
            <Icon name="refresh" />
            {companyUsersQuery.isFetching ? "Refreshing..." : "Refresh"}
          </Button>
        </Box>

        <Box className={styles.companyUsers__toolbar}>
          <label className={styles.companyUsers__searchField}>
            <Icon name="search" />
            <input
              type="search"
              aria-label={companyUsersMessages.filters.searchLabel}
              placeholder={companyUsersMessages.filters.searchPlaceholder}
              value={filters.searchTerm}
              onChange={(event) =>
                setFilters((current) => ({ ...current, searchTerm: event.target.value }))
              }
            />
          </label>
          <label className={styles.companyUsers__filterField}>
            <span>{companyUsersMessages.filters.role}</span>
            <select
              value={filters.role}
              onChange={(event) =>
                setFilters((current) => ({
                  ...current,
                  role: event.target.value as CompanyUserRoleFilter,
                }))
              }
            >
              <option value={CompanyUserRoleFilter.All}>
                {companyUsersMessages.filters.allRoles}
              </option>
              <option value={CompanyUserRoleFilter.Admin}>Admin</option>
              <option value={CompanyUserRoleFilter.Moderator}>Moderator</option>
              <option value={CompanyUserRoleFilter.User}>User</option>
              <option value={CompanyUserRoleFilter.Superadmin}>Superadmin</option>
            </select>
          </label>
          <span className={styles.companyUsers__resultCount}>{filteredUsers.length} visible</span>
        </Box>

        {!isAllowedToViewUsers ? (
          <Box className={styles.companyUsers__error} role="alert">
            You are not allowed to view company users.
          </Box>
        ) : null}
        {companyUsersQuery.isLoading || companyUsersQuery.isPending ? (
          <Box className={styles.companyUsers__state}>Loading company users...</Box>
        ) : null}
        {companyUsersQuery.isError ? (
          <Box className={styles.companyUsers__error} role="alert">
            {companyUsersQuery.error.message}
          </Box>
        ) : null}

        {companyUsersQuery.isSuccess && users.length === 0 ? (
          <Box className={styles.companyUsers__emptyState}>
            <Icon name="team" size={30} />
            <Typography as="h3" className={styles.companyUsers__emptyTitle}>
              No users added yet
            </Typography>
            <Typography as="p" className={styles.companyUsers__emptyDescription}>
              Invite the first user for this tenant to start onboarding company members.
            </Typography>
          </Box>
        ) : null}

        {companyUsersQuery.isSuccess && users.length > 0 ? (
          <>
            <Box className={styles.companyUsers__tableWrapper}>
              <Table className={styles.companyUsers__table}>
                <TableHeader>
                  <TableRow>
                    <TableHead>User</TableHead>
                    <TableHead>Email</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Onboarding</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredUsers.map((companyUser) => {
                    const roleLabel = formatCompanyUserLabel(resolveCompanyUserRole(companyUser));
                    const initials = `${companyUser.firstName?.[0] ?? "U"}${companyUser.lastName?.[0] ?? ""}`;
                    return (
                      <TableRow key={companyUser.id}>
                        <TableCell>
                          <Box className={styles.companyUsers__userCell}>
                            <span className={styles.companyUsers__avatar}>{initials}</span>
                            <Box>
                              <strong>
                                {companyUser.firstName} {companyUser.lastName}
                              </strong>
                              <small title={String(companyUser.id)}>
                                ID {String(companyUser.id).slice(0, 12)}…
                              </small>
                            </Box>
                          </Box>
                        </TableCell>
                        <TableCell>
                          <span className={styles.companyUsers__email}>
                            <Icon name="mail" size={16} />
                            {companyUser.email}
                          </span>
                        </TableCell>
                        <TableCell>
                          <span
                            className={`${styles.companyUsers__badge} ${styles.companyUsers__badgeRole}`}
                          >
                            <Icon name="shield" size={14} />
                            {roleLabel}
                          </span>
                        </TableCell>
                        <TableCell>
                          <span
                            className={`${styles.companyUsers__badge} ${badgeToneClass[resolveStatusTone(companyUser.status)]}`}
                          >
                            <span className={styles.companyUsers__statusDot} />
                            {formatCompanyUserLabel(companyUser.status)}
                          </span>
                        </TableCell>
                        <TableCell>
                          <span
                            className={`${styles.companyUsers__badge} ${badgeToneClass[resolveOnboardingTone(companyUser.onboardingStatus)]}`}
                          >
                            {formatCompanyUserLabel(companyUser.onboardingStatus)}
                          </span>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </Box>
            {filteredUsers.length === 0 ? (
              <Box className={styles.companyUsers__emptyState}>
                <Icon name="search" size={28} />
                <Typography as="h3" className={styles.companyUsers__emptyTitle}>
                  No matching users
                </Typography>
                <Typography as="p" className={styles.companyUsers__emptyDescription}>
                  Clear the search or choose another role filter.
                </Typography>
              </Box>
            ) : null}
          </>
        ) : null}
      </section>
    </PageShell>
  );
}
