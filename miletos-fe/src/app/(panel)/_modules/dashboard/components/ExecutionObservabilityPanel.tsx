"use client";

import { useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import { formatExecutionDateTime } from "@/app/(panel)/_modules/dashboard/utils/execution-formatters";
import { useExecutionErrorsQuery } from "@/app/(panel)/_modules/dashboard/query/useExecutionErrorsQuery";
import { useExecutionEventsQuery } from "@/app/(panel)/_modules/dashboard/query/useExecutionEventsQuery";
import { useExecutionLogsQuery } from "@/app/(panel)/_modules/dashboard/query/useExecutionLogsQuery";
import styles from "./ExecutionObservabilityPanel.module.css";

type ObservabilityTab = "events" | "logs" | "errors";

interface ExecutionObservabilityPanelProps {
  executionId: string;
  pollingEnabled: boolean;
  companyId?: number;
}

const OBSERVABILITY_PAGE_SIZE = 50;

function formatMetadata(metadata: Record<string, unknown>) {
  const entries = Object.entries(metadata);

  if (entries.length === 0) {
    return null;
  }

  return JSON.stringify(metadata, null, 2);
}

export function ExecutionObservabilityPanel({
  executionId,
  pollingEnabled,
  companyId,
}: ExecutionObservabilityPanelProps) {
  const [activeTab, setActiveTab] = useState<ObservabilityTab>("events");

  const eventsQuery = useExecutionEventsQuery(
    executionId,
    {
      limit: OBSERVABILITY_PAGE_SIZE,
    },
    {
      enabled: activeTab === "events",
      pollingEnabled,
      companyId,
    },
  );

  const logsQuery = useExecutionLogsQuery(
    executionId,
    {
      limit: OBSERVABILITY_PAGE_SIZE,
    },
    {
      enabled: activeTab === "logs",
      pollingEnabled,
      companyId,
    },
  );

  const errorsQuery = useExecutionErrorsQuery(
    executionId,
    {
      limit: OBSERVABILITY_PAGE_SIZE,
    },
    {
      enabled: activeTab === "errors",
      pollingEnabled,
      companyId,
    },
  );

  return (
    <section className={styles.executionObservability}>
      <header className={styles.executionObservability__header}>
        <Box>
          <Typography as="p">Runtime observability</Typography>

          <Typography as="h2">Execution activity</Typography>

          <Typography as="span">
            Inspect lifecycle events, safe runtime logs and structured execution errors.
          </Typography>
        </Box>
      </header>

      <nav className={styles.executionObservability__tabs} aria-label="Execution observability">
        <Button
          type="button"
          aria-pressed={activeTab === "events"}
          onClick={() => setActiveTab("events")}
        >
          Events
        </Button>

        <Button
          type="button"
          aria-pressed={activeTab === "logs"}
          onClick={() => setActiveTab("logs")}
        >
          Logs
        </Button>

        <Button
          type="button"
          aria-pressed={activeTab === "errors"}
          onClick={() => setActiveTab("errors")}
        >
          Errors
        </Button>
      </nav>

      {activeTab === "events" ? (
        <Box className={styles.executionObservability__content}>
          {eventsQuery.isPending ? (
            <ObservabilityState message="Loading execution events…" />
          ) : null}

          {eventsQuery.isError ? (
            <ObservabilityState error message={eventsQuery.error.message} />
          ) : null}

          {eventsQuery.isSuccess && eventsQuery.data.items.length === 0 ? (
            <ObservabilityState message="No execution events recorded." />
          ) : null}

          {eventsQuery.isSuccess && eventsQuery.data.items.length > 0 ? (
            <Box className={styles.executionObservability__list}>
              {eventsQuery.data.items.map((event) => {
                const metadata = formatMetadata(event.metadata);

                return (
                  <article key={event.eventId} className={styles.executionObservability__item}>
                    <Box className={styles.executionObservability__itemHeader}>
                      <Box>
                        <Typography as="span">Event #{event.sequenceNumber}</Typography>

                        <Typography as="strong">{event.type}</Typography>
                      </Box>

                      <time>{formatExecutionDateTime(event.createdAt)}</time>
                    </Box>

                    {event.previousStatus || event.newStatus ? (
                      <Typography as="p" className={styles.executionObservability__transition}>
                        {event.previousStatus ?? "—"}
                        {" → "}
                        {event.newStatus ?? "—"}
                      </Typography>
                    ) : null}

                    {event.safeMessage ? <Typography as="p">{event.safeMessage}</Typography> : null}

                    {event.nodeExecutionId ? (
                      <small>Node execution: {event.nodeExecutionId}</small>
                    ) : null}

                    {metadata ? (
                      <details>
                        <summary>Metadata</summary>

                        <pre>{metadata}</pre>
                      </details>
                    ) : null}
                  </article>
                );
              })}
            </Box>
          ) : null}
        </Box>
      ) : null}

      {activeTab === "logs" ? (
        <Box className={styles.executionObservability__content}>
          {logsQuery.isPending ? <ObservabilityState message="Loading runtime logs…" /> : null}

          {logsQuery.isError ? (
            <ObservabilityState error message={logsQuery.error.message} />
          ) : null}

          {logsQuery.isSuccess && logsQuery.data.items.length === 0 ? (
            <ObservabilityState message="No runtime logs recorded." />
          ) : null}

          {logsQuery.isSuccess && logsQuery.data.items.length > 0 ? (
            <Box className={styles.executionObservability__list}>
              {logsQuery.data.items.map((log) => {
                const metadata = formatMetadata(log.metadata);

                return (
                  <article key={log.logId} className={styles.executionObservability__item}>
                    <Box className={styles.executionObservability__itemHeader}>
                      <Box>
                        <Typography as="span">Log #{log.sequenceNumber}</Typography>

                        <Typography as="strong" data-level={log.level}>
                          {log.level}
                        </Typography>
                      </Box>

                      <time>{formatExecutionDateTime(log.createdAt)}</time>
                    </Box>

                    <Typography as="p" className={styles.executionObservability__message}>
                      {log.message}
                    </Typography>

                    {log.nodeExecutionId ? (
                      <small>Node execution: {log.nodeExecutionId}</small>
                    ) : null}

                    {metadata ? (
                      <details>
                        <summary>Metadata</summary>

                        <pre>{metadata}</pre>
                      </details>
                    ) : null}
                  </article>
                );
              })}
            </Box>
          ) : null}
        </Box>
      ) : null}

      {activeTab === "errors" ? (
        <Box className={styles.executionObservability__content}>
          {errorsQuery.isPending ? (
            <ObservabilityState message="Loading structured errors…" />
          ) : null}

          {errorsQuery.isError ? (
            <ObservabilityState error message={errorsQuery.error.message} />
          ) : null}

          {errorsQuery.isSuccess && errorsQuery.data.items.length === 0 ? (
            <ObservabilityState message="No structured errors recorded." />
          ) : null}

          {errorsQuery.isSuccess && errorsQuery.data.items.length > 0 ? (
            <Box className={styles.executionObservability__list}>
              {errorsQuery.data.items.map((error) => (
                <article
                  key={error.errorId}
                  className={[
                    styles.executionObservability__item,
                    styles.executionObservability__errorItem,
                  ].join(" ")}
                >
                  <Box className={styles.executionObservability__itemHeader}>
                    <Box>
                      <Typography as="span">{error.category}</Typography>

                      <Typography as="strong">{error.code}</Typography>
                    </Box>

                    <time>{formatExecutionDateTime(error.createdAt)}</time>
                  </Box>

                  <Typography as="p" className={styles.executionObservability__message}>
                    {error.safeMessage}
                  </Typography>

                  <Box className={styles.executionObservability__badges}>
                    <Typography as="span">Retryable: {error.retryable ? "Yes" : "No"}</Typography>

                    {error.nodeExecutionId ? <Typography as="span">Node linked</Typography> : null}

                    {error.relatedEventId ? <Typography as="span">Event linked</Typography> : null}
                  </Box>

                  {Object.keys(error.details).length > 0 ? (
                    <details>
                      <summary>Safe details</summary>

                      <dl className={styles.executionObservability__details}>
                        {Object.entries(error.details).map(([key, value]) => (
                          <Box key={key}>
                            <dt>{key}</dt>

                            <dd>{value}</dd>
                          </Box>
                        ))}
                      </dl>
                    </details>
                  ) : null}
                </article>
              ))}
            </Box>
          ) : null}
        </Box>
      ) : null}
    </section>
  );
}

interface ObservabilityStateProps {
  message: string;
  error?: boolean;
}

function ObservabilityState({ message, error = false }: ObservabilityStateProps) {
  return (
    <Box
      className={[
        styles.executionObservability__state,
        error ? styles.executionObservability__stateError : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <Typography as="span">{message}</Typography>
    </Box>
  );
}
