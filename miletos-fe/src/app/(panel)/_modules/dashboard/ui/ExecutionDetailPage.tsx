"use client";

import Link from "next/link";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import {
  formatExecutionDateTime,
  formatExecutionDuration,
} from "@/app/(panel)/_modules/dashboard/model/execution-formatters";
import {
  formatExecutionStatus,
  isTerminalExecutionStatus,
} from "@/app/(panel)/_modules/dashboard/model/execution-status";
import { useExecutionDefinitionQuery } from "@/app/(panel)/_modules/dashboard/model/useExecutionDefinitionQuery";
import { useExecutionDetailQuery } from "@/app/(panel)/_modules/dashboard/model/useExecutionDetailQuery";
import { useAllCompaniesQuery } from "@/app/(panel)/_modules/companies/query/useAllCompaniesQuery";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { ExecutionObservabilityPanel } from "./ExecutionObservabilityPanel";
import { ExecutionRuntimeGraph } from "./ExecutionRuntimeGraph";
import styles from "./ExecutionDetailPage.module.css";

interface ExecutionDetailPageProps {
  executionId: string;
  companyId?: number;
}

export function ExecutionDetailPage({ executionId, companyId }: ExecutionDetailPageProps) {
  const currentUserQuery = useCurrentUserQuery();
  const isSuperAdmin = currentUserQuery.data?.superAdmin === true;
  const companiesQuery = useAllCompaniesQuery({ enabled: isSuperAdmin });
  const selectedCompany = companiesQuery.data?.find((company) => company.id === companyId);
  const hasValidTenant = !isSuperAdmin || Boolean(selectedCompany);
  const effectiveCompanyId = isSuperAdmin ? selectedCompany?.id : undefined;
  const queryEnabled = currentUserQuery.isSuccess && hasValidTenant;
  const executionsHref = effectiveCompanyId
    ? `/dashboard/executions?companyId=${effectiveCompanyId}`
    : "/dashboard/executions";
  const executionQuery = useExecutionDetailQuery(executionId, {
    enabled: queryEnabled,
    companyId: effectiveCompanyId,
  });

  const definitionQuery = useExecutionDefinitionQuery(executionId, {
    enabled: queryEnabled && executionQuery.isSuccess,
    companyId: effectiveCompanyId,
  });

  if (currentUserQuery.isPending || (isSuperAdmin && companiesQuery.isPending)) {
    return (
      <PageShell
        eyebrow="Workflow observability"
        title="Execution detail"
        description="Loading runtime execution state."
      >
        <div className={styles.executionDetailPage__state}>
          <strong>Loading execution…</strong>

          <span>Reading the authoritative execution state from the runtime read model.</span>
        </div>
      </PageShell>
    );
  }

  if (isSuperAdmin && (!companyId || (companiesQuery.isSuccess && !selectedCompany))) {
    return (
      <PageShell
        eyebrow="Workflow observability"
        title="Execution detail"
        description="Workflow executions require a valid tenant selection."
        actions={
          <Link className={styles.executionDetailPage__backLink} href="/dashboard/executions">
            Back to executions
          </Link>
        }
      >
        <div className={styles.executionDetailPage__state} role="alert">
          <strong>
            {companyId ? "Selected company is unavailable." : "Select a company first."}
          </strong>

          <span>Return to workflow executions and choose an available company.</span>
        </div>
      </PageShell>
    );
  }

  if (isSuperAdmin && companiesQuery.isError) {
    return (
      <PageShell
        eyebrow="Workflow observability"
        title="Execution detail"
        description="The selected tenant could not be verified."
        actions={
          <Link className={styles.executionDetailPage__backLink} href="/dashboard/executions">
            Back to executions
          </Link>
        }
      >
        <div className={styles.executionDetailPage__state} role="alert">
          <strong>Companies could not be loaded.</strong>

          <span>{companiesQuery.error.message}</span>
        </div>
      </PageShell>
    );
  }

  if (executionQuery.isPending) {
    return (
      <PageShell
        eyebrow="Workflow observability"
        title="Execution detail"
        description="Loading runtime execution state."
      >
        <div className={styles.executionDetailPage__state}>
          <strong>Loading executionâ€¦</strong>

          <span>Reading the authoritative execution state from the runtime read model.</span>
        </div>
      </PageShell>
    );
  }

  if (executionQuery.isError) {
    return (
      <PageShell
        eyebrow="Workflow observability"
        title="Execution detail"
        description="The requested execution could not be loaded."
        actions={
          <Link className={styles.executionDetailPage__backLink} href={executionsHref}>
            Back to executions
          </Link>
        }
      >
        <div
          className={[
            styles.executionDetailPage__state,
            styles.executionDetailPage__stateError,
          ].join(" ")}
          role="alert"
        >
          <strong>Execution could not be loaded.</strong>

          <span>{executionQuery.error.message}</span>

          <button
            type="button"
            onClick={() => {
              void executionQuery.refetch();
            }}
          >
            Try again
          </button>
        </div>
      </PageShell>
    );
  }

  const execution = executionQuery.data;

  const pollingEnabled = !isTerminalExecutionStatus(execution.status);

  const duration = formatExecutionDuration(
    execution.startedAt ?? execution.createdAt,
    execution.finishedAt,
  );

  return (
    <PageShell
      eyebrow="Workflow observability"
      title="Execution detail"
      description="Inspect the authoritative runtime state and the immutable workflow definition captured for this execution."
      actions={
        <Link className={styles.executionDetailPage__backLink} href={executionsHref}>
          Back to executions
        </Link>
      }
    >
      <section className={styles.executionDetailPage__hero}>
        <div className={styles.executionDetailPage__heroCopy}>
          <span className={styles.executionDetailPage__eyebrow}>Execution</span>

          <strong className={styles.executionDetailPage__executionId}>
            {execution.executionId}
          </strong>

          <p>
            Workflow <strong>{execution.workflowId}</strong> revision{" "}
            <strong>#{execution.workflowRevision}</strong>
          </p>
        </div>

        <div className={styles.executionDetailPage__heroStatus}>
          <span className={styles.executionDetailPage__status} data-status={execution.status}>
            {formatExecutionStatus(execution.status)}
          </span>

          {execution.isStalled ? (
            <span className={styles.executionDetailPage__stalled}>Stalled</span>
          ) : null}
        </div>
      </section>

      <section className={styles.executionDetailPage__metrics} aria-label="Execution summary">
        <article>
          <span>Status</span>

          <strong>{formatExecutionStatus(execution.status)}</strong>
        </article>

        <article>
          <span>Mode</span>

          <strong>{execution.mode}</strong>
        </article>

        <article>
          <span>Revision</span>

          <strong>#{execution.workflowRevision}</strong>
        </article>

        <article>
          <span>Duration</span>

          <strong>{duration}</strong>
        </article>
      </section>

      <section className={styles.executionDetailPage__grid}>
        <article className={styles.executionDetailPage__card}>
          <header className={styles.executionDetailPage__cardHeader}>
            <div>
              <p>Runtime identity</p>

              <h2>Execution context</h2>
            </div>
          </header>

          <dl className={styles.executionDetailPage__details}>
            <div>
              <dt>Execution ID</dt>

              <dd>{execution.executionId}</dd>
            </div>

            <div>
              <dt>Workflow ID</dt>

              <dd>{execution.workflowId}</dd>
            </div>

            <div>
              <dt>Correlation ID</dt>

              <dd>{execution.correlationId}</dd>
            </div>

            <div>
              <dt>Mode</dt>

              <dd>{execution.mode}</dd>
            </div>

            <div>
              <dt>Stalled</dt>

              <dd>{execution.isStalled ? "Yes" : "No"}</dd>
            </div>
          </dl>
        </article>

        <article className={styles.executionDetailPage__card}>
          <header className={styles.executionDetailPage__cardHeader}>
            <div>
              <p>Lifecycle</p>

              <h2>Runtime timestamps</h2>
            </div>
          </header>

          <dl className={styles.executionDetailPage__details}>
            <div>
              <dt>Created</dt>

              <dd>{formatExecutionDateTime(execution.createdAt)}</dd>
            </div>

            <div>
              <dt>Validating</dt>

              <dd>{formatExecutionDateTime(execution.validatingAt)}</dd>
            </div>

            <div>
              <dt>Queued</dt>

              <dd>{formatExecutionDateTime(execution.queuedAt)}</dd>
            </div>

            <div>
              <dt>Started</dt>

              <dd>{formatExecutionDateTime(execution.startedAt)}</dd>
            </div>

            <div>
              <dt>Finished</dt>

              <dd>{formatExecutionDateTime(execution.finishedAt)}</dd>
            </div>

            <div>
              <dt>Updated</dt>

              <dd>{formatExecutionDateTime(execution.updatedAt)}</dd>
            </div>
          </dl>
        </article>
      </section>

      <section className={styles.executionDetailPage__definition}>
        <header className={styles.executionDetailPage__cardHeader}>
          <div>
            <p>Historical execution truth</p>

            <h2>Definition snapshot</h2>
          </div>
        </header>

        {definitionQuery.isPending ? (
          <div className={styles.executionDetailPage__definitionState}>
            Loading immutable definition snapshot…
          </div>
        ) : null}

        {definitionQuery.isError ? (
          <div
            className={[
              styles.executionDetailPage__definitionState,
              styles.executionDetailPage__definitionStateError,
            ].join(" ")}
          >
            <strong>Definition snapshot could not be loaded.</strong>

            <span>{definitionQuery.error.message}</span>
          </div>
        ) : null}

        {definitionQuery.isSuccess ? (
          <div className={styles.executionDetailPage__definitionGrid}>
            <div>
              <span>Workflow name</span>

              <strong>{definitionQuery.data.workflowName}</strong>
            </div>

            <div>
              <span>Snapshot ID</span>

              <strong title={definitionQuery.data.snapshotId}>
                {definitionQuery.data.snapshotId}
              </strong>
            </div>

            <div>
              <span>Revision</span>

              <strong>#{definitionQuery.data.workflowRevision}</strong>
            </div>

            <div>
              <span>Captured at</span>

              <strong>{formatExecutionDateTime(definitionQuery.data.createdAt)}</strong>
            </div>
          </div>
        ) : null}
      </section>

      {definitionQuery.isSuccess ? (
        <ExecutionRuntimeGraph
          executionId={execution.executionId}
          definition={definitionQuery.data.definition}
          pollingEnabled={pollingEnabled}
          companyId={effectiveCompanyId}
        />
      ) : null}

      <ExecutionObservabilityPanel
        executionId={execution.executionId}
        pollingEnabled={pollingEnabled}
        companyId={effectiveCompanyId}
      />
    </PageShell>
  );
}
