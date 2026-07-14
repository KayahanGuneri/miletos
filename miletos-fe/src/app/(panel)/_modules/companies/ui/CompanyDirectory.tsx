"use client";

import { useState } from "react";
import type { Company } from "@/app/(panel)/_modules/companies/types/company-types";
import {
  COMPANY_STATUSES,
  type CompanyDirectoryState,
  CompanyDirectoryStatus,
  CompanyStatusFilter,
  INITIAL_FILTERS,
} from "@/app/(panel)/_modules/companies/types/company-management-types";
import { useDeleteCompanyMutation } from "@/app/(panel)/_modules/companies/query/useDeleteCompanyMutation";
import { useUpdateCompanyMutation } from "@/app/(panel)/_modules/companies/query/useUpdateCompanyMutation";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import { CompanyDeleteConfirmation } from "./CompanyDeleteConfirmation";
import { CompanyForm, CompanyFormMode } from "./CompanyForm";
import { CompanyTableRow } from "./CompanyTableRow";
import styles from "./CompanyManagementPage.module.css";

interface CompanyDirectoryProps {
  isRefreshing: boolean;
  state: CompanyDirectoryState;
  onPageChange: (page: number) => void;
  onRefresh: () => void;
}

export function CompanyDirectory({
  isRefreshing,
  state,
  onPageChange,
  onRefresh,
}: CompanyDirectoryProps) {
  const updateCompanyMutation = useUpdateCompanyMutation();
  const deleteCompanyMutation = useDeleteCompanyMutation();
  const [filters, setFilters] = useState(INITIAL_FILTERS);
  const [editingCompanyId, setEditingCompanyId] = useState<number | null>(null);
  const [deletingCompany, setDeletingCompany] = useState<Company | null>(null);

  const page =
    state.status === CompanyDirectoryStatus.Empty || state.status === CompanyDirectoryStatus.Ready
      ? state.page
      : undefined;
  const companies = page?.content ?? [];
  const normalizedSearch = filters.searchTerm.trim().toLowerCase();
  const visibleCompanies = companies.filter((company) => {
    const matchesSearch =
      !normalizedSearch ||
      company.name.toLowerCase().includes(normalizedSearch) ||
      String(company.id).toLowerCase().includes(normalizedSearch);
    const matchesStatus =
      filters.status === CompanyStatusFilter.ALL || company.status === filters.status;

    return matchesSearch && matchesStatus;
  });

  function startEditing(company: Company) {
    setEditingCompanyId(company.id);
    updateCompanyMutation.reset();
  }

  function cancelEditing() {
    setEditingCompanyId(null);
    updateCompanyMutation.reset();
  }

  function requestDelete(company: Company) {
    setDeletingCompany(company);
    deleteCompanyMutation.reset();
  }

  function changePage(nextPage: number) {
    cancelEditing();
    setDeletingCompany(null);
    deleteCompanyMutation.reset();
    onPageChange(nextPage);
  }

  return (
    <section
      className={[styles.companyManagement__card, styles.companyManagement__tableCard].join(" ")}
    >
      <Box className={styles.companyManagement__sectionHeader}>
        <Box>
          <Typography as="p" className={styles.companyManagement__cardEyebrow}>
            Tenant directory
          </Typography>

          <Typography as="h2" className={styles.companyManagement__cardTitle}>
            Companies
          </Typography>

          <Typography as="p" className={styles.companyManagement__sectionDescription}>
            Search the current page, filter lifecycle state and open company users.
          </Typography>
        </Box>

        <Button
          disabled={isRefreshing}
          type="button"
          variant={ButtonVariant.Secondary}
          onClick={onRefresh}
        >
          <Icon name="refresh" />
          {isRefreshing ? "Refreshing..." : "Refresh"}
        </Button>
      </Box>

      <Box className={styles.companyManagement__toolbar}>
        <label className={styles.companyManagement__searchField}>
          <Icon name="search" />

          <input
            aria-label="Search companies on current page"
            placeholder="Search by company name or ID"
            type="search"
            value={filters.searchTerm}
            onChange={(event) =>
              setFilters((current) => ({ ...current, searchTerm: event.target.value }))
            }
          />
        </label>

        <label className={styles.companyManagement__filterField}>
          <span>Status</span>

          <select
            value={filters.status}
            onChange={(event) =>
              setFilters((current) => ({
                ...current,
                status: event.target.value as CompanyStatusFilter,
              }))
            }
          >
            <option value={CompanyStatusFilter.ALL}>All statuses</option>

            {COMPANY_STATUSES.map((status) => (
              <option key={status} value={status}>
                {status}
              </option>
            ))}
          </select>
        </label>

        <span className={styles.companyManagement__resultCount}>
          {visibleCompanies.length} visible on this page
        </span>
      </Box>

      {state.status === CompanyDirectoryStatus.Loading ? (
        <Typography as="p" className={styles.companyManagement__notice}>
          Loading companies...
        </Typography>
      ) : null}

      {state.status === CompanyDirectoryStatus.Error ? (
        <Typography as="p" className={styles.companyManagement__error} role="alert">
          {state.message}
        </Typography>
      ) : null}

      {deleteCompanyMutation.error ? (
        <Typography as="p" className={styles.companyManagement__error} role="alert">
          {deleteCompanyMutation.error.message}
        </Typography>
      ) : null}

      {deletingCompany ? (
        <CompanyDeleteConfirmation
          company={deletingCompany}
          isDeleting={deleteCompanyMutation.isPending}
          onCancel={() => {
            setDeletingCompany(null);
            deleteCompanyMutation.reset();
          }}
          onConfirm={async (company) => {
            try {
              await deleteCompanyMutation.mutateAsync(company.id);

              if (page?.content.length === 1 && page.page > 0) {
                onPageChange(page.page - 1);
              }

              setDeletingCompany(null);
            } catch {
              // Mutation state renders the normalized API error above.
            }
          }}
        />
      ) : null}

      {state.status === CompanyDirectoryStatus.Empty ? (
        <Box className={styles.companyManagement__emptyState}>
          <Icon name="building" size={28} />
          <Typography as="h3">No companies found</Typography>
          <Typography as="p">Create the first tenant from the form above.</Typography>
        </Box>
      ) : null}

      {state.status === CompanyDirectoryStatus.Ready ? (
        <>
          <Box className={styles.companyManagement__tableWrap}>
            <table className={styles.companyManagement__table}>
              <thead>
                <tr>
                  <th>Company</th>
                  <th>Status</th>
                  <th>Created</th>
                  <th>Users</th>
                  <th>Actions</th>
                </tr>
              </thead>

              <tbody>
                {visibleCompanies.map((company) =>
                  editingCompanyId === company.id ? (
                    <tr key={company.id}>
                      <td colSpan={5}>
                        <CompanyForm
                          key={company.id}
                          errorMessage={updateCompanyMutation.error?.message}
                          initialValues={{ name: company.name, status: company.status }}
                          isSubmitting={updateCompanyMutation.isPending}
                          mode={CompanyFormMode.Edit}
                          nameError={updateCompanyMutation.error?.fieldErrors?.name}
                          statusError={updateCompanyMutation.error?.fieldErrors?.status}
                          onCancel={cancelEditing}
                          onSubmit={async (values) => {
                            await updateCompanyMutation.mutateAsync({
                              companyId: company.id,
                              request: values,
                            });
                            cancelEditing();
                          }}
                        />
                      </td>
                    </tr>
                  ) : (
                    <CompanyTableRow
                      key={company.id}
                      company={company}
                      isDeleting={deleteCompanyMutation.isPending}
                      onRequestDelete={requestDelete}
                      onStartEditing={startEditing}
                    />
                  ),
                )}
              </tbody>
            </table>
          </Box>

          {visibleCompanies.length === 0 ? (
            <Box className={styles.companyManagement__emptyState}>
              <Icon name="search" size={26} />
              <Typography as="h3">No matching companies</Typography>
              <Typography as="p">Clear the search or select another status filter.</Typography>
            </Box>
          ) : null}

          <Box className={styles.companyManagement__pagination}>
            <Button
              disabled={state.page.first || isRefreshing}
              type="button"
              variant={ButtonVariant.Secondary}
              onClick={() => changePage(Math.max(state.page.page - 1, 0))}
            >
              <Icon name="arrow-left" />
              Previous
            </Button>

            <span className={styles.companyManagement__paginationText}>
              Page <strong>{state.page.page + 1}</strong> of{" "}
              <strong>{Math.max(state.page.totalPages, 1)}</strong>
            </span>

            <Button
              disabled={state.page.last || isRefreshing}
              type="button"
              variant={ButtonVariant.Secondary}
              onClick={() => changePage(state.page.page + 1)}
            >
              Next
              <Icon name="arrow-right" />
            </Button>
          </Box>
        </>
      ) : null}
    </section>
  );
}
