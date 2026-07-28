"use client";

import Link from "next/link";
import { useState } from "react";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import {
  formatExecutionDateTime,
  shortenExecutionId,
} from "@/app/(panel)/_modules/dashboard/model/execution-formatters";
import {
  EXECUTION_STATUSES,
  formatExecutionStatus,
} from "@/app/(panel)/_modules/dashboard/model/execution-status";
import { type ExecutionStatus } from "@/app/(panel)/_modules/dashboard/model/execution-types";
import { useExecutionsQuery } from "@/app/(panel)/_modules/dashboard/model/useExecutionsQuery";
import styles from "./ExecutionListPage.module.css";

const EXECUTION_PAGE_SIZE = 20;

type ExecutionStatusFilter = "ALL" | ExecutionStatus;

export function ExecutionListPage() {
  const [statusFilter, setStatusFilter] =
    useState<ExecutionStatusFilter>("ALL");

  const [cursorHistory, setCursorHistory] = useState<(string | undefined)[]>([
    undefined,
  ]);

  const currentCursor = cursorHistory[cursorHistory.length - 1];

  const executionsQuery = useExecutionsQuery({
    limit: EXECUTION_PAGE_SIZE,
    after: currentCursor,
    status: statusFilter === "ALL" ? undefined : statusFilter,
  });

  const executions = executionsQuery.data?.items ?? [];

  const canGoPrevious = cursorHistory.length > 1;

  const canGoNext = Boolean(
    executionsQuery.data?.hasNext && executionsQuery.data.next,
  );

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

  return (
    <PageShell
      eyebrow="Workflow observability"
      title="Workflow executions"
      description="Inspect runtime executions, follow their current status and open historical execution details."
      actions={
        <button
          className={styles.executionListPage__refreshButton}
          type="button"
          disabled={executionsQuery.isFetching}
          onClick={() => {
            void executionsQuery.refetch();
          }}
        >
          {executionsQuery.isFetching ? "Refreshing…" : "Refresh"}
        </button>
      }
    >
      <section
        className={styles.executionListPage__panel}
        aria-busy={executionsQuery.isFetching}
      >
        <header className={styles.executionListPage__toolbar}>
          <div>
            <p className={styles.executionListPage__eyebrow}>
              Runtime activity
            </p>

            <h2 className={styles.executionListPage__title}>
              Execution history
            </h2>

            <p className={styles.executionListPage__description}>
              Current execution state comes from the runtime execution read
              model.
            </p>
          </div>

          <label className={styles.executionListPage__filter}>
            <span>Status</span>

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
        </header>

        {executionsQuery.isPending ? (
          <div className={styles.executionListPage__state}>
            <strong>Loading executions…</strong>

            <span>Reading the current workflow execution page.</span>
          </div>
        ) : null}

        {executionsQuery.isError ? (
          <div
            className={[
              styles.executionListPage__state,
              styles.executionListPage__stateError,
            ].join(" ")}
            role="alert"
          >
            <strong>Executions could not be loaded.</strong>

            <span>{executionsQuery.error.message}</span>

            <button
              type="button"
              onClick={() => {
                void executionsQuery.refetch();
              }}
            >
              Try again
            </button>
          </div>
        ) : null}

        {executionsQuery.isSuccess && executions.length === 0 ? (
          <div className={styles.executionListPage__state}>
            <strong>No executions found.</strong>

            <span>
              No workflow executions match the selected status on this page.
            </span>
          </div>
        ) : null}

        {executionsQuery.isSuccess && executions.length > 0 ? (
          <>
            <div className={styles.executionListPage__tableViewport}>
              <table className={styles.executionListPage__table}>
                <thead>
                  <tr>
                    <th>Execution</th>
                    <th>Workflow</th>
                    <th>Revision</th>
                    <th>Mode</th>
                    <th>Status</th>
                    <th>Created</th>
                    <th>Updated</th>
                    <th>Runtime</th>
                  </tr>
                </thead>

                <tbody>
                  {executions.map((execution) => (
                    <tr key={execution.executionId}>
                      <td>
                        <Link
                          className={styles.executionListPage__executionLink}
                          href={`/dashboard/executions/${execution.executionId}`}
                          title={execution.executionId}
                        >
                          {shortenExecutionId(execution.executionId)}
                        </Link>

                        <span
                          className={styles.executionListPage__correlation}
                          title={execution.correlationId}
                        >
                          Corr. {shortenExecutionId(execution.correlationId)}
                        </span>
                      </td>

                      <td>
                        <span
                          className={styles.executionListPage__workflow}
                          title={execution.workflowId}
                        >
                          {shortenExecutionId(execution.workflowId)}
                        </span>
                      </td>

                      <td>
                        <span>#{execution.workflowRevision}</span>
                      </td>

                      <td>
                        <span className={styles.executionListPage__mode}>
                          {execution.mode}
                        </span>
                      </td>

                      <td>
                        <span
                          className={styles.executionListPage__status}
                          data-status={execution.status}
                        >
                          {formatExecutionStatus(execution.status)}
                        </span>
                      </td>

                      <td>{formatExecutionDateTime(execution.createdAt)}</td>

                      <td>{formatExecutionDateTime(execution.updatedAt)}</td>

                      <td>
                        {execution.isStalled ? (
                          <span className={styles.executionListPage__stalled}>
                            Stalled
                          </span>
                        ) : (
                          <span className={styles.executionListPage__healthy}>
                            Normal
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <footer className={styles.executionListPage__pagination}>
              <span>Page {cursorHistory.length}</span>

              <div>
                <button
                  type="button"
                  disabled={!canGoPrevious || executionsQuery.isFetching}
                  onClick={handlePreviousPage}
                >
                  Previous
                </button>

                <button
                  type="button"
                  disabled={!canGoNext || executionsQuery.isFetching}
                  onClick={handleNextPage}
                >
                  Next
                </button>
              </div>
            </footer>
          </>
        ) : null}
      </section>
    </PageShell>
  );
}
