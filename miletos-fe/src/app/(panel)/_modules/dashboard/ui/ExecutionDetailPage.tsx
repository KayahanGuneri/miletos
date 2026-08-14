"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import {
  formatExecutionDateTime,
  formatExecutionDuration,
  formatExecutionStatus,
  isTerminalExecutionStatus,
} from "@/app/(panel)/_modules/dashboard/utils/execution-formatters";
import { useExecutionDefinitionQuery } from "@/app/(panel)/_modules/dashboard/query/useExecutionDefinitionQuery";
import { useExecutionDetailQuery } from "@/app/(panel)/_modules/dashboard/query/useExecutionDetailQuery";
import { useExecutionNodesQuery } from "@/app/(panel)/_modules/dashboard/query/useExecutionNodesQuery";
import { useRunWorkflowMutation } from "@/app/(panel)/_modules/dashboard/query/useRunWorkflowMutation";
import { mapDefinitionToRunWorkflowRequest } from "@/app/(panel)/_modules/dashboard/utils/run-workflow-mapper";
import { useAllCompaniesQuery } from "@/app/(panel)/_modules/companies/query/useAllCompaniesQuery";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { ExecutionObservabilityPanel } from "../components/ExecutionObservabilityPanel";
import { ExecutionGraph } from "../components/ExecutionGraph";
import { RuntimeNodeDetailsDialog } from "../components/RuntimeNodeDetailsDialog";
import styles from "./ExecutionDetailPage.module.css";

interface ExecutionDetailPageProps {
  executionId: string;
  companyId?: number;
}

const RUNTIME_NODE_PAGE_SIZE = 100;

export function ExecutionDetailPage({ executionId, companyId }: ExecutionDetailPageProps) {
  const router = useRouter();
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [isNodeDialogOpen, setIsNodeDialogOpen] = useState(false);
  const runWorkflowMutation = useRunWorkflowMutation();
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
  const pollingEnabled = executionQuery.data
    ? !isTerminalExecutionStatus(executionQuery.data.status)
    : false;
  const nodesQuery = useExecutionNodesQuery(
    executionId,
    { limit: RUNTIME_NODE_PAGE_SIZE },
    {
      enabled: queryEnabled && definitionQuery.isSuccess && executionQuery.isSuccess,
      pollingEnabled,
      companyId: effectiveCompanyId,
    },
  );
  const nodeExecutions = useMemo(() => nodesQuery.data?.items ?? [], [nodesQuery.data?.items]);
  const selectedNodeExecution = useMemo(() => {
    if (!selectedNodeId) {
      return null;
    }

    return (
      nodeExecutions
        .filter((nodeExecution) => nodeExecution.nodeId === selectedNodeId)
        .sort((first, second) => second.attempt - first.attempt)[0] ?? null
    );
  }, [nodeExecutions, selectedNodeId]);

  useEffect(() => {
    // A route identity change invalidates selection and dialog state from the previous execution.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setSelectedNodeId(null);
    setIsNodeDialogOpen(false);
  }, [executionId]);

  function selectRuntimeNode(nodeId: string | null) {
    setSelectedNodeId(nodeId);

    if (!nodeId) {
      setIsNodeDialogOpen(false);
      return;
    }

    setIsNodeDialogOpen(true);
  }

  if (currentUserQuery.isPending || (isSuperAdmin && companiesQuery.isPending)) {
    return (
      <PageShell
        eyebrow="Workflow observability"
        title="Execution detail"
        description="Loading runtime execution state."
      >
        <Box className={styles.executionDetailPage__state}>
          <Typography as="strong">Loading execution…</Typography>

          <Typography as="span">
            Reading the authoritative execution state from the runtime read model.
          </Typography>
        </Box>
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
        <Box className={styles.executionDetailPage__state} role="alert">
          <Typography as="strong">
            {companyId ? "Selected company is unavailable." : "Select a company first."}
          </Typography>

          <Typography as="span">
            Return to workflow executions and choose an available company.
          </Typography>
        </Box>
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
        <Box className={styles.executionDetailPage__state} role="alert">
          <Typography as="strong">Companies could not be loaded.</Typography>

          <Typography as="span">{companiesQuery.error.message}</Typography>
        </Box>
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
        <Box className={styles.executionDetailPage__state}>
          <Typography as="strong">Loading execution…</Typography>

          <Typography as="span">
            Reading the authoritative execution state from the runtime read model.
          </Typography>
        </Box>
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
        <Box
          className={[
            styles.executionDetailPage__state,
            styles.executionDetailPage__stateError,
          ].join(" ")}
          role="alert"
        >
          <Typography as="strong">Execution could not be loaded.</Typography>

          <Typography as="span">{executionQuery.error.message}</Typography>

          <Button
            type="button"
            onClick={() => {
              void executionQuery.refetch();
            }}
          >
            Try again
          </Button>
        </Box>
      </PageShell>
    );
  }

  const execution = executionQuery.data;

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
        <Box className={styles.executionDetailPage__actions}>
          <Button
            type="button"
            disabled={!definitionQuery.isSuccess || runWorkflowMutation.isPending}
            onClick={() => {
              if (!definitionQuery.isSuccess) {
                return;
              }
              runWorkflowMutation.mutate(
                {
                  request: mapDefinitionToRunWorkflowRequest(definitionQuery.data.definition),
                  companyId: effectiveCompanyId,
                  idempotencyKey: crypto.randomUUID(),
                },
                {
                  onSuccess: (result) => {
                    const companyQuery = effectiveCompanyId
                      ? `?companyId=${effectiveCompanyId}`
                      : "";
                    router.push(`/dashboard/executions/${result.executionId}${companyQuery}`);
                  },
                },
              );
            }}
          >
            {runWorkflowMutation.isPending ? "Running…" : "Run"}
          </Button>

          <Link className={styles.executionDetailPage__backLink} href={executionsHref}>
            Back to executions
          </Link>
        </Box>
      }
    >
      {runWorkflowMutation.isError ? (
        <Box className={styles.executionDetailPage__runError} role="alert">
          <Typography as="strong">Workflow could not be started.</Typography>

          <Typography as="span">{runWorkflowMutation.error.message}</Typography>
        </Box>
      ) : null}

      <section className={styles.executionDetailPage__hero}>
        <Box className={styles.executionDetailPage__heroCopy}>
          <Typography as="span" className={styles.executionDetailPage__eyebrow}>
            Execution
          </Typography>

          <Typography as="strong" className={styles.executionDetailPage__executionId}>
            {execution.executionId}
          </Typography>

          <Typography as="p">
            Workflow <Typography as="strong">{execution.workflowId}</Typography> revision{" "}
            <Typography as="strong">#{execution.workflowRevision}</Typography>
          </Typography>
        </Box>

        <Box className={styles.executionDetailPage__heroStatus}>
          <Typography
            as="span"
            className={styles.executionDetailPage__status}
            data-status={execution.status}
          >
            {formatExecutionStatus(execution.status)}
          </Typography>

          {execution.isStalled ? (
            <Typography as="span" className={styles.executionDetailPage__stalled}>
              Stalled
            </Typography>
          ) : null}
        </Box>
      </section>

      <section className={styles.executionDetailPage__metrics} aria-label="Execution summary">
        <article>
          <Typography as="span">Status</Typography>

          <Typography as="strong">{formatExecutionStatus(execution.status)}</Typography>
        </article>

        <article>
          <Typography as="span">Mode</Typography>

          <Typography as="strong">{execution.mode}</Typography>
        </article>

        <article>
          <Typography as="span">Revision</Typography>

          <Typography as="strong">#{execution.workflowRevision}</Typography>
        </article>

        <article>
          <Typography as="span">Duration</Typography>

          <Typography as="strong">{duration}</Typography>
        </article>
      </section>

      {definitionQuery.isSuccess ? (
        <ExecutionGraph
          definition={definitionQuery.data.definition}
          nodeExecutions={nodeExecutions}
          selectedNodeId={selectedNodeId}
          onNodeSelect={selectRuntimeNode}
        />
      ) : null}

      <section className={styles.executionDetailPage__grid}>
        <article className={styles.executionDetailPage__card}>
          <header className={styles.executionDetailPage__cardHeader}>
            <Box>
              <Typography as="p">Runtime identity</Typography>

              <Typography as="h2">Execution context</Typography>
            </Box>
          </header>

          <dl className={styles.executionDetailPage__details}>
            <Box>
              <dt>Execution ID</dt>

              <dd>{execution.executionId}</dd>
            </Box>

            <Box>
              <dt>Workflow ID</dt>

              <dd>{execution.workflowId}</dd>
            </Box>

            <Box>
              <dt>Correlation ID</dt>

              <dd>{execution.correlationId}</dd>
            </Box>

            <Box>
              <dt>Mode</dt>

              <dd>{execution.mode}</dd>
            </Box>

            <Box>
              <dt>Stalled</dt>

              <dd>{execution.isStalled ? "Yes" : "No"}</dd>
            </Box>
          </dl>
        </article>

        <article className={styles.executionDetailPage__card}>
          <header className={styles.executionDetailPage__cardHeader}>
            <Box>
              <Typography as="p">Lifecycle</Typography>

              <Typography as="h2">Runtime timestamps</Typography>
            </Box>
          </header>

          <dl className={styles.executionDetailPage__details}>
            <Box>
              <dt>Created</dt>

              <dd>{formatExecutionDateTime(execution.createdAt)}</dd>
            </Box>

            <Box>
              <dt>Validating</dt>

              <dd>{formatExecutionDateTime(execution.validatingAt)}</dd>
            </Box>

            <Box>
              <dt>Queued</dt>

              <dd>{formatExecutionDateTime(execution.queuedAt)}</dd>
            </Box>

            <Box>
              <dt>Started</dt>

              <dd>{formatExecutionDateTime(execution.startedAt)}</dd>
            </Box>

            <Box>
              <dt>Finished</dt>

              <dd>{formatExecutionDateTime(execution.finishedAt)}</dd>
            </Box>

            <Box>
              <dt>Updated</dt>

              <dd>{formatExecutionDateTime(execution.updatedAt)}</dd>
            </Box>
          </dl>
        </article>
      </section>

      <section className={styles.executionDetailPage__definition}>
        <header className={styles.executionDetailPage__cardHeader}>
          <Box>
            <Typography as="p">Historical execution truth</Typography>

            <Typography as="h2">Definition snapshot</Typography>
          </Box>
        </header>

        {definitionQuery.isPending ? (
          <Box className={styles.executionDetailPage__definitionState}>
            Loading immutable definition snapshot…
          </Box>
        ) : null}

        {definitionQuery.isError ? (
          <Box
            className={[
              styles.executionDetailPage__definitionState,
              styles.executionDetailPage__definitionStateError,
            ].join(" ")}
          >
            <Typography as="strong">Definition snapshot could not be loaded.</Typography>

            <Typography as="span">{definitionQuery.error.message}</Typography>
          </Box>
        ) : null}

        {definitionQuery.isSuccess ? (
          <Box className={styles.executionDetailPage__definitionGrid}>
            <Box>
              <Typography as="span">Workflow name</Typography>

              <Typography as="strong">{definitionQuery.data.workflowName}</Typography>
            </Box>

            <Box>
              <Typography as="span">Snapshot ID</Typography>

              <Typography as="strong" title={definitionQuery.data.snapshotId}>
                {definitionQuery.data.snapshotId}
              </Typography>
            </Box>

            <Box>
              <Typography as="span">Revision</Typography>

              <Typography as="strong">#{definitionQuery.data.workflowRevision}</Typography>
            </Box>

            <Box>
              <Typography as="span">Captured at</Typography>

              <Typography as="strong">
                {formatExecutionDateTime(definitionQuery.data.createdAt)}
              </Typography>
            </Box>
          </Box>
        ) : null}
      </section>

      {isNodeDialogOpen && selectedNodeId ? (
        <RuntimeNodeDetailsDialog
          selectedNodeId={selectedNodeId}
          nodeExecution={selectedNodeExecution}
          onClose={() => setIsNodeDialogOpen(false)}
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
