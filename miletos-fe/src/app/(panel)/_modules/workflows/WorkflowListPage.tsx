"use client";

import Link from "next/link";
import { useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
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
import { WorkflowDeleteConfirmation } from "@/app/(panel)/_modules/workflows/components/WorkflowDeleteConfirmation";
import { WorkflowLifecycleActions } from "@/app/(panel)/_modules/workflows/components/WorkflowLifecycleActions";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import { useDeleteWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/workflow-mutations";
import { useWorkflowsQuery } from "@/app/(panel)/_modules/workflows/query/workflow-queries";
import {
  type WorkflowFilterState,
  type WorkflowSummary,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type WorkflowStatus } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import {
  formatWorkflowDate,
  formatWorkflowStatus,
  WORKFLOW_STATUSES,
} from "@/app/(panel)/_modules/workflows/utils/workflow-utils";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { canManageWorkflows } from "@/shared/session/permissions/session-permissions";
import styles from "./ui/WorkflowListPage.module.css";

const PAGE_SIZE = 20;

const INITIAL_FILTERS: WorkflowFilterState = {
  searchDraft: "",
  search: "",
};

export function WorkflowListPage() {
  const currentUser = useCurrentUserQuery();
  const isAllowed = canManageWorkflows(currentUser.data);
  const [page, setPage] = useState(0);
  const [filters, setFilters] = useState<WorkflowFilterState>(INITIAL_FILTERS);
  const [deletingWorkflow, setDeletingWorkflow] = useState<WorkflowSummary | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const workflows = useWorkflowsQuery(
    {
      page,
      size: PAGE_SIZE,
      search: filters.search || undefined,
      status: filters.status,
    },
    isAllowed,
  );
  const deleteMutation = useDeleteWorkflowMutation();

  return (
    <PageShell
      eyebrow={workflowMessages.common.eyebrow}
      title={workflowMessages.list.title}
      description={workflowMessages.list.description}
      actions={
        isAllowed ? (
          <Link className={styles.workflowList__createLink} href="/workflows/new">
            {workflowMessages.list.create}
          </Link>
        ) : null
      }
    >
      {!isAllowed ? (
        <Box className={styles.workflowList__state} role="alert">
          <Typography as="strong">{workflowMessages.list.forbiddenTitle}</Typography>
          <Typography as="span">{workflowMessages.list.forbiddenDescription}</Typography>
        </Box>
      ) : (
        <section className={styles.workflowList__panel}>
          <Box className={styles.workflowList__directoryHeader}>
            <Typography as="p" className={styles.workflowList__eyebrow}>
              {workflowMessages.list.directoryEyebrow}
            </Typography>
            <Typography as="h2">{workflowMessages.list.directoryTitle}</Typography>
          </Box>
          <header className={styles.workflowList__toolbar}>
            <form
              className={styles.workflowList__searchForm}
              onSubmit={(event) => {
                event.preventDefault();
                setFilters((current) => ({ ...current, search: current.searchDraft.trim() }));
                setPage(0);
              }}
            >
              <label className={styles.workflowList__field}>
                <Typography as="span">{workflowMessages.list.searchLabel}</Typography>
                <Input
                  type="search"
                  placeholder={workflowMessages.list.searchPlaceholder}
                  value={filters.searchDraft}
                  onChange={(event) =>
                    setFilters((current) => ({ ...current, searchDraft: event.target.value }))
                  }
                />
              </label>
              <Button type="submit">{workflowMessages.list.searchSubmit}</Button>
            </form>

            <label className={styles.workflowList__field}>
              <Typography as="span">{workflowMessages.list.statusLabel}</Typography>
              <select
                value={filters.status ?? ""}
                onChange={(event) => {
                  const status = (event.target.value || undefined) as WorkflowStatus | undefined;
                  setFilters((current) => ({ ...current, status }));
                  setPage(0);
                }}
              >
                <option value="">{workflowMessages.list.allStatuses}</option>
                {WORKFLOW_STATUSES.map((value) => (
                  <option key={value} value={value}>
                    {formatWorkflowStatus(value)}
                  </option>
                ))}
              </select>
            </label>

            <Button
              type="button"
              variant={ButtonVariant.Secondary}
              disabled={workflows.isFetching}
              onClick={() => void workflows.refetch()}
            >
              {workflows.isFetching
                ? workflowMessages.list.refreshing
                : workflowMessages.list.refresh}
            </Button>
          </header>

          {successMessage ? (
            <Typography as="p" className={styles.workflowList__success} role="status">
              {successMessage}
            </Typography>
          ) : null}

          {deletingWorkflow ? (
            <WorkflowDeleteConfirmation
              workflow={deletingWorkflow}
              isDeleting={deleteMutation.isPending}
              errorMessage={deleteMutation.error?.message}
              onCancel={() => {
                setDeletingWorkflow(null);
                deleteMutation.reset();
              }}
              onConfirm={async () => {
                try {
                  await deleteMutation.mutateAsync(deletingWorkflow.id);
                  setSuccessMessage(workflowMessages.list.deleted(deletingWorkflow.name));
                  setDeletingWorkflow(null);
                } catch {
                  // The confirmation panel renders the normalized mutation error.
                }
              }}
            />
          ) : null}

          {workflows.isPending ? (
            <Box className={styles.workflowList__state}>
              <Typography as="strong">{workflowMessages.list.loadingTitle}</Typography>
              <Typography as="span">{workflowMessages.list.loadingDescription}</Typography>
            </Box>
          ) : null}

          {workflows.isError ? (
            <Box className={styles.workflowList__state} role="alert">
              <Typography as="strong">{workflowMessages.list.errorTitle}</Typography>
              <Typography as="span">{workflows.error.message}</Typography>
            </Box>
          ) : null}

          {workflows.isSuccess && workflows.data.content.length === 0 ? (
            <Box className={styles.workflowList__state}>
              <Typography as="strong">{workflowMessages.list.emptyTitle}</Typography>
              <Typography as="span">{workflowMessages.list.emptyDescription}</Typography>
            </Box>
          ) : null}

          {workflows.isSuccess && workflows.data.content.length > 0 ? (
            <>
              <Box className={styles.workflowList__tableViewport}>
                <Table className={styles.workflowList__table}>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{workflowMessages.list.columns.id}</TableHead>
                      <TableHead>{workflowMessages.list.columns.name}</TableHead>
                      <TableHead>{workflowMessages.list.columns.description}</TableHead>
                      <TableHead>{workflowMessages.list.columns.status}</TableHead>
                      <TableHead>{workflowMessages.list.columns.revision}</TableHead>
                      <TableHead>{workflowMessages.list.columns.nodes}</TableHead>
                      <TableHead>{workflowMessages.list.columns.edges}</TableHead>
                      <TableHead>{workflowMessages.list.columns.created}</TableHead>
                      <TableHead>{workflowMessages.list.columns.updated}</TableHead>
                      <TableHead>{workflowMessages.list.columns.createdBy}</TableHead>
                      <TableHead>{workflowMessages.list.columns.updatedBy}</TableHead>
                      <TableHead>{workflowMessages.list.columns.actions}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {workflows.data.content.map((workflow) => (
                      <TableRow key={workflow.id}>
                        <TableCell>{workflow.id}</TableCell>
                        <TableCell>
                          <Link href={`/workflows/${workflow.id}`}>{workflow.name}</Link>
                        </TableCell>
                        <TableCell>
                          {workflow.description ?? workflowMessages.common.emptyValue}
                        </TableCell>
                        <TableCell>
                          <Typography
                            as="span"
                            className={styles.workflowList__status}
                            data-status={workflow.status}
                          >
                            {formatWorkflowStatus(workflow.status)}
                          </Typography>
                        </TableCell>
                        <TableCell>{workflow.revision}</TableCell>
                        <TableCell>{workflow.nodeCount}</TableCell>
                        <TableCell>{workflow.edgeCount}</TableCell>
                        <TableCell>{formatWorkflowDate(workflow.createdAt)}</TableCell>
                        <TableCell>{formatWorkflowDate(workflow.updatedAt)}</TableCell>
                        <TableCell>{workflow.createdBy.email}</TableCell>
                        <TableCell>{workflow.updatedBy.email}</TableCell>
                        <TableCell>
                          <Box className={styles.workflowList__rowActions}>
                            <Link href={`/workflows/${workflow.id}`}>
                              {workflowMessages.list.open}
                            </Link>
                            <WorkflowLifecycleActions
                              workflow={workflow}
                              onChanged={(changed) =>
                                setSuccessMessage(
                                  workflowMessages.list.statusChanged(
                                    changed.name,
                                    formatWorkflowStatus(changed.status).toLowerCase(),
                                  ),
                                )
                              }
                            />
                            <Button
                              type="button"
                              variant={ButtonVariant.Secondary}
                              disabled={workflow.status === "ACTIVE"}
                              onClick={() => {
                                setSuccessMessage(null);
                                setDeletingWorkflow(workflow);
                                deleteMutation.reset();
                              }}
                            >
                              {workflowMessages.list.delete}
                            </Button>
                          </Box>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Box>
              <footer className={styles.workflowList__pagination}>
                <Typography as="span">
                  {workflowMessages.list.pagination(
                    workflows.data.page + 1,
                    Math.max(workflows.data.totalPages, 1),
                  )}
                </Typography>
                <Box className={styles.workflowList__actions}>
                  <Button
                    type="button"
                    variant={ButtonVariant.Secondary}
                    disabled={workflows.data.first || workflows.isFetching}
                    onClick={() => setPage((value) => Math.max(value - 1, 0))}
                  >
                    {workflowMessages.list.previous}
                  </Button>
                  <Button
                    type="button"
                    variant={ButtonVariant.Secondary}
                    disabled={workflows.data.last || workflows.isFetching}
                    onClick={() => setPage((value) => value + 1)}
                  >
                    {workflowMessages.list.next}
                  </Button>
                </Box>
              </footer>
            </>
          ) : null}
        </section>
      )}
    </PageShell>
  );
}
