"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button from "@/components/lib/button/Button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/lib/table/Table";
import { Typography } from "@/components/lib/typography/Typography";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import {
  EXECUTION_STATUSES,
  formatExecutionDateTime,
  formatExecutionStatus,
  shortenExecutionId,
} from "@/app/(panel)/_modules/dashboard/utils/execution-formatters";
import { type ExecutionStatus } from "@/app/(panel)/_modules/dashboard/types/execution-types";
import { useExecutionsQuery } from "@/app/(panel)/_modules/dashboard/query/useExecutionsQuery";
import { useAllCompaniesQuery } from "@/app/(panel)/_modules/companies/query/useAllCompaniesQuery";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import styles from "./ExecutionListPage.module.css";

const EXECUTION_PAGE_SIZE = 20;

type ExecutionStatusFilter = "ALL" | ExecutionStatus;

export function ExecutionListPage() {
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const currentUserQuery = useCurrentUserQuery();
  const isSuperAdmin = currentUserQuery.data?.superAdmin === true;
  const companiesQuery = useAllCompaniesQuery({ enabled: isSuperAdmin });
  const rawCompanyId = searchParams.get("companyId");
  const parsedCompanyId = rawCompanyId === null ? null : Number(rawCompanyId);
  const selectedCompanyId =
    parsedCompanyId !== null && Number.isSafeInteger(parsedCompanyId) && parsedCompanyId > 0
      ? parsedCompanyId
      : null;
  const selectedCompany = companiesQuery.data?.find((company) => company.id === selectedCompanyId);
  const hasValidTenant = !isSuperAdmin || Boolean(selectedCompany);
  const executionCompanyId = isSuperAdmin ? selectedCompany?.id : undefined;
  const [statusFilter, setStatusFilter] = useState<ExecutionStatusFilter>("ALL");

  const [cursorHistory, setCursorHistory] = useState<(string | undefined)[]>([undefined]);

  const currentCursor = cursorHistory[cursorHistory.length - 1];

  const executionsQuery = useExecutionsQuery(
    {
      limit: EXECUTION_PAGE_SIZE,
      after: currentCursor,
      status: statusFilter === "ALL" ? undefined : statusFilter,
    },
    {
      enabled: currentUserQuery.isSuccess && hasValidTenant,
      companyId: executionCompanyId,
    },
  );

  const executions = executionsQuery.data?.items ?? [];

  const canGoPrevious = cursorHistory.length > 1;

  const canGoNext = Boolean(executionsQuery.data?.hasNext && executionsQuery.data.next);

  function handleStatusChange(nextStatus: ExecutionStatusFilter) {
    setStatusFilter(nextStatus);

    setCursorHistory([undefined]);
  }

  function handleNextPage() {
    const nextCursor = executionsQuery.data?.next;

    if (!executionsQuery.data?.hasNext || !nextCursor) {
      return;
    }

    setCursorHistory((current) => [...current, nextCursor]);
  }

  function handlePreviousPage() {
    setCursorHistory((current) => {
      if (current.length <= 1) {
        return current;
      }

      return current.slice(0, -1);
    });
  }

  function handleCompanyChange(value: string) {
    const nextSearchParams = new URLSearchParams(searchParams.toString());

    if (value) {
      nextSearchParams.set("companyId", value);
    } else {
      nextSearchParams.delete("companyId");
    }

    setCursorHistory([undefined]);
    const nextQuery = nextSearchParams.toString();
    router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname);
  }

  return (
    <PageShell
      eyebrow="Workflow observability"
      title="Workflow executions"
      description="Inspect runtime executions, follow their current status and open historical execution details."
      actions={
        <Button
          className={styles.executionListPage__refreshButton}
          type="button"
          disabled={!hasValidTenant || executionsQuery.isFetching}
          onClick={() => {
            if (hasValidTenant) {
              void executionsQuery.refetch();
            }
          }}
        >
          {executionsQuery.isFetching ? "Refreshing…" : "Refresh"}
        </Button>
      }
    >
      <section className={styles.executionListPage__panel} aria-busy={executionsQuery.isFetching}>
        <header className={styles.executionListPage__toolbar}>
          <Box>
            <Typography as="p" className={styles.executionListPage__eyebrow}>
              Runtime activity
            </Typography>

            <Typography as="h2" className={styles.executionListPage__title}>
              Execution history
            </Typography>

            <Typography as="p" className={styles.executionListPage__description}>
              Current execution state comes from the runtime execution read model.
            </Typography>
          </Box>

          <Box className={styles.executionListPage__filters}>
            {isSuperAdmin ? (
              <label className={styles.executionListPage__filter}>
                <Typography as="span">Company</Typography>

                <select
                  value={selectedCompanyId ?? ""}
                  disabled={companiesQuery.isPending || companiesQuery.isError}
                  onChange={(event) => handleCompanyChange(event.target.value)}
                >
                  <option value="">Select a company</option>

                  {companiesQuery.data?.map((company) => (
                    <option key={company.id} value={company.id}>
                      {company.name}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}

            <label className={styles.executionListPage__filter}>
              <Typography as="span">Status</Typography>

              <select
                value={statusFilter}
                onChange={(event) => {
                  handleStatusChange(event.target.value as ExecutionStatusFilter);
                }}
              >
                <option value="ALL">All statuses</option>

                {EXECUTION_STATUSES.map((status) => (
                  <option key={status} value={status}>
                    {formatExecutionStatus(status)}
                  </option>
                ))}
              </select>
            </label>
          </Box>
        </header>

        {isSuperAdmin && companiesQuery.isError ? (
          <Box
            className={[styles.executionListPage__state, styles.executionListPage__stateError].join(
              " ",
            )}
            role="alert"
          >
            <Typography as="strong">Companies could not be loaded.</Typography>

            <Typography as="span">{companiesQuery.error.message}</Typography>
          </Box>
        ) : null}

        {isSuperAdmin && companiesQuery.isPending ? (
          <Box className={styles.executionListPage__state}>
            <Typography as="strong">Loading companies…</Typography>

            <Typography as="span">
              Preparing the tenant selector for workflow execution history.
            </Typography>
          </Box>
        ) : null}

        {isSuperAdmin && companiesQuery.isSuccess && selectedCompanyId === null ? (
          <Box className={styles.executionListPage__state}>
            <Typography as="strong">Select a company.</Typography>

            <Typography as="span">
              Workflow executions are tenant scoped. Choose a company to continue.
            </Typography>
          </Box>
        ) : null}

        {isSuperAdmin &&
        companiesQuery.isSuccess &&
        selectedCompanyId !== null &&
        !selectedCompany ? (
          <Box
            className={[styles.executionListPage__state, styles.executionListPage__stateError].join(
              " ",
            )}
            role="alert"
          >
            <Typography as="strong">Selected company is unavailable.</Typography>

            <Typography as="span">
              Choose another company before loading workflow executions.
            </Typography>
          </Box>
        ) : null}

        {hasValidTenant && executionsQuery.isPending ? (
          <Box className={styles.executionListPage__state}>
            <Typography as="strong">Loading executions…</Typography>

            <Typography as="span">Reading the current workflow execution page.</Typography>
          </Box>
        ) : null}

        {executionsQuery.isError ? (
          <Box
            className={[styles.executionListPage__state, styles.executionListPage__stateError].join(
              " ",
            )}
            role="alert"
          >
            <Typography as="strong">Executions could not be loaded.</Typography>

            <Typography as="span">{executionsQuery.error.message}</Typography>

            <Button
              type="button"
              onClick={() => {
                void executionsQuery.refetch();
              }}
            >
              Try again
            </Button>
          </Box>
        ) : null}

        {executionsQuery.isSuccess && executions.length === 0 ? (
          <Box className={styles.executionListPage__state}>
            <Typography as="strong">No executions found.</Typography>

            <Typography as="span">
              No workflow executions match the selected status on this page.
            </Typography>
          </Box>
        ) : null}

        {executionsQuery.isSuccess && executions.length > 0 ? (
          <>
            <Box className={styles.executionListPage__tableViewport}>
              <Table className={styles.executionListPage__table}>
                <TableHeader>
                  <TableRow>
                    <TableHead>Execution</TableHead>
                    <TableHead>Workflow</TableHead>
                    <TableHead>Revision</TableHead>
                    <TableHead>Mode</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead>Updated</TableHead>
                    <TableHead>Runtime</TableHead>
                  </TableRow>
                </TableHeader>

                <TableBody>
                  {executions.map((execution) => (
                    <TableRow key={execution.executionId}>
                      <TableCell>
                        <Link
                          className={styles.executionListPage__executionLink}
                          href={
                            executionCompanyId
                              ? `/dashboard/executions/${execution.executionId}?companyId=${executionCompanyId}`
                              : `/dashboard/executions/${execution.executionId}`
                          }
                          title={execution.executionId}
                        >
                          {shortenExecutionId(execution.executionId)}
                        </Link>

                        <Typography
                          as="span"
                          className={styles.executionListPage__correlation}
                          title={execution.correlationId}
                        >
                          Corr. {shortenExecutionId(execution.correlationId)}
                        </Typography>
                      </TableCell>

                      <TableCell>
                        <Typography
                          as="span"
                          className={styles.executionListPage__workflow}
                          title={execution.workflowId}
                        >
                          {shortenExecutionId(execution.workflowId)}
                        </Typography>
                      </TableCell>

                      <TableCell>
                        <Typography as="span">#{execution.workflowRevision}</Typography>
                      </TableCell>

                      <TableCell>
                        <Typography as="span" className={styles.executionListPage__mode}>
                          {execution.mode}
                        </Typography>
                      </TableCell>

                      <TableCell>
                        <Typography
                          as="span"
                          className={styles.executionListPage__status}
                          data-status={execution.status}
                        >
                          {formatExecutionStatus(execution.status)}
                        </Typography>
                      </TableCell>

                      <TableCell>{formatExecutionDateTime(execution.createdAt)}</TableCell>

                      <TableCell>{formatExecutionDateTime(execution.updatedAt)}</TableCell>

                      <TableCell>
                        {execution.isStalled ? (
                          <Typography as="span" className={styles.executionListPage__stalled}>
                            Stalled
                          </Typography>
                        ) : (
                          <Typography as="span" className={styles.executionListPage__healthy}>
                            Normal
                          </Typography>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Box>

            <footer className={styles.executionListPage__pagination}>
              <Typography as="span">Page {cursorHistory.length}</Typography>

              <Box>
                <Button
                  type="button"
                  disabled={!canGoPrevious || executionsQuery.isFetching}
                  onClick={handlePreviousPage}
                >
                  Previous
                </Button>

                <Button
                  type="button"
                  disabled={!canGoNext || executionsQuery.isFetching}
                  onClick={handleNextPage}
                >
                  Next
                </Button>
              </Box>
            </footer>
          </>
        ) : null}
      </section>
    </PageShell>
  );
}
