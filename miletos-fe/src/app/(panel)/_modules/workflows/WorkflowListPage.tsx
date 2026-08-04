"use client";

import Link from "next/link";
import { useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
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
import { useDeleteWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/useDeleteWorkflowMutation";
import { useWorkflowsQuery } from "@/app/(panel)/_modules/workflows/query/useWorkflowsQuery";
import {
  type WorkflowStatus,
  type WorkflowSummary,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";
import {
  formatWorkflowDate,
  formatWorkflowStatus,
  WORKFLOW_STATUSES,
} from "@/app/(panel)/_modules/workflows/utils/workflow-utils";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { canManageWorkflows } from "@/shared/session/permissions/session-permissions";
import styles from "./ui/WorkflowListPage.module.css";

const PAGE_SIZE = 20;

export function WorkflowListPage() {
  const currentUser = useCurrentUserQuery();
  const isAllowed = canManageWorkflows(currentUser.data);
  const [page, setPage] = useState(0);
  const [searchDraft, setSearchDraft] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<WorkflowStatus | undefined>();
  const [deletingWorkflow, setDeletingWorkflow] = useState<WorkflowSummary | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const workflows = useWorkflowsQuery(
    { page, size: PAGE_SIZE, search: search || undefined, status },
    isAllowed,
  );
  const deleteMutation = useDeleteWorkflowMutation();

  return (
    <PageShell
      eyebrow="Workflow control plane"
      title="Workflows"
      description="Create workflow drafts, manage their lifecycle and inspect company-owned definitions."
      actions={
        isAllowed ? (
          <Link className={styles.workflowList__createLink} href="/workflows/new">
            Create workflow
          </Link>
        ) : null
      }
    >
      {!isAllowed ? (
        <Box className={styles.workflowList__state} role="alert">
          <Typography as="strong">Workflow management is unavailable.</Typography>
          <Typography as="span">
            Only active company administrators can access this feature.
          </Typography>
        </Box>
      ) : (
        <section className={styles.workflowList__panel}>
          <header className={styles.workflowList__toolbar}>
            <form
              className={styles.workflowList__searchForm}
              onSubmit={(event) => {
                event.preventDefault();
                setSearch(searchDraft.trim());
                setPage(0);
              }}
            >
              <label className={styles.workflowList__field}>
                <Typography as="span">Search</Typography>
                <input
                  type="search"
                  placeholder="Search workflow names"
                  value={searchDraft}
                  onChange={(event) => setSearchDraft(event.target.value)}
                />
              </label>
              <Button type="submit">Search</Button>
            </form>

            <label className={styles.workflowList__field}>
              <Typography as="span">Status</Typography>
              <select
                value={status ?? ""}
                onChange={(event) => {
                  setStatus((event.target.value || undefined) as WorkflowStatus | undefined);
                  setPage(0);
                }}
              >
                <option value="">All statuses</option>
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
              {workflows.isFetching ? "Refreshing..." : "Refresh"}
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
                  setSuccessMessage(`Workflow ${deletingWorkflow.name} was deleted.`);
                  setDeletingWorkflow(null);
                } catch {
                  // The confirmation panel renders the normalized mutation error.
                }
              }}
            />
          ) : null}

          {workflows.isPending ? (
            <Box className={styles.workflowList__state}>
              <Typography as="strong">Loading workflows...</Typography>
              <Typography as="span">Reading the company workflow directory.</Typography>
            </Box>
          ) : null}

          {workflows.isError ? (
            <Box className={styles.workflowList__state} role="alert">
              <Typography as="strong">Workflows could not be loaded.</Typography>
              <Typography as="span">{workflows.error.message}</Typography>
            </Box>
          ) : null}

          {workflows.isSuccess && workflows.data.content.length === 0 ? (
            <Box className={styles.workflowList__state}>
              <Typography as="strong">No workflows found.</Typography>
              <Typography as="span">Create a draft or adjust the current filters.</Typography>
            </Box>
          ) : null}

          {workflows.isSuccess && workflows.data.content.length > 0 ? (
            <>
              <Box className={styles.workflowList__tableViewport}>
                <Table className={styles.workflowList__table}>
                  <TableHeader>
                    <TableRow>
                      <TableHead>ID</TableHead>
                      <TableHead>Name</TableHead>
                      <TableHead>Description</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Revision</TableHead>
                      <TableHead>Nodes</TableHead>
                      <TableHead>Edges</TableHead>
                      <TableHead>Created</TableHead>
                      <TableHead>Updated</TableHead>
                      <TableHead>Created by</TableHead>
                      <TableHead>Updated by</TableHead>
                      <TableHead>Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {workflows.data.content.map((workflow) => (
                      <TableRow key={workflow.id}>
                        <TableCell>{workflow.id}</TableCell>
                        <TableCell>
                          <Link href={`/workflows/${workflow.id}`}>{workflow.name}</Link>
                        </TableCell>
                        <TableCell>{workflow.description ?? "—"}</TableCell>
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
                        <TableCell>{workflow.createdByEmail}</TableCell>
                        <TableCell>{workflow.updatedByEmail}</TableCell>
                        <TableCell>
                          <Box className={styles.workflowList__rowActions}>
                            <Link href={`/workflows/${workflow.id}`}>Open</Link>
                            <WorkflowLifecycleActions
                              workflow={workflow}
                              onChanged={(changed) =>
                                setSuccessMessage(
                                  `Workflow ${changed.name} is now ${formatWorkflowStatus(changed.status).toLowerCase()}.`,
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
                              Delete
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
                  Page {workflows.data.page + 1} of {Math.max(workflows.data.totalPages, 1)}
                </Typography>
                <Box className={styles.workflowList__actions}>
                  <Button
                    type="button"
                    variant={ButtonVariant.Secondary}
                    disabled={workflows.data.first || workflows.isFetching}
                    onClick={() => setPage((value) => Math.max(value - 1, 0))}
                  >
                    Previous
                  </Button>
                  <Button
                    type="button"
                    variant={ButtonVariant.Secondary}
                    disabled={workflows.data.last || workflows.isFetching}
                    onClick={() => setPage((value) => value + 1)}
                  >
                    Next
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
