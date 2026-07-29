"use client";

import { useState } from "react";
import { formatExecutionDateTime } from "@/app/(panel)/_modules/dashboard/model/execution-formatters";
import { useExecutionErrorsQuery } from "@/app/(panel)/_modules/dashboard/model/useExecutionErrorsQuery";
import { useExecutionEventsQuery } from "@/app/(panel)/_modules/dashboard/model/useExecutionEventsQuery";
import { useExecutionLogsQuery } from "@/app/(panel)/_modules/dashboard/model/useExecutionLogsQuery";
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
        <div>
          <p>Runtime observability</p>

          <h2>Execution activity</h2>

          <span>Inspect lifecycle events, safe runtime logs and structured execution errors.</span>
        </div>
      </header>

      <nav className={styles.executionObservability__tabs} aria-label="Execution observability">
        <button
          type="button"
          aria-pressed={activeTab === "events"}
          onClick={() => setActiveTab("events")}
        >
          Events
        </button>

        <button
          type="button"
          aria-pressed={activeTab === "logs"}
          onClick={() => setActiveTab("logs")}
        >
          Logs
        </button>

        <button
          type="button"
          aria-pressed={activeTab === "errors"}
          onClick={() => setActiveTab("errors")}
        >
          Errors
        </button>
      </nav>

      {activeTab === "events" ? (
        <div className={styles.executionObservability__content}>
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
            <div className={styles.executionObservability__list}>
              {eventsQuery.data.items.map((event) => {
                const metadata = formatMetadata(event.metadata);

                return (
                  <article key={event.eventId} className={styles.executionObservability__item}>
                    <div className={styles.executionObservability__itemHeader}>
                      <div>
                        <span>Event #{event.sequenceNumber}</span>

                        <strong>{event.type}</strong>
                      </div>

                      <time>{formatExecutionDateTime(event.createdAt)}</time>
                    </div>

                    {event.previousStatus || event.newStatus ? (
                      <p className={styles.executionObservability__transition}>
                        {event.previousStatus ?? "—"}
                        {" → "}
                        {event.newStatus ?? "—"}
                      </p>
                    ) : null}

                    {event.safeMessage ? <p>{event.safeMessage}</p> : null}

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
            </div>
          ) : null}
        </div>
      ) : null}

      {activeTab === "logs" ? (
        <div className={styles.executionObservability__content}>
          {logsQuery.isPending ? <ObservabilityState message="Loading runtime logs…" /> : null}

          {logsQuery.isError ? (
            <ObservabilityState error message={logsQuery.error.message} />
          ) : null}

          {logsQuery.isSuccess && logsQuery.data.items.length === 0 ? (
            <ObservabilityState message="No runtime logs recorded." />
          ) : null}

          {logsQuery.isSuccess && logsQuery.data.items.length > 0 ? (
            <div className={styles.executionObservability__list}>
              {logsQuery.data.items.map((log) => {
                const metadata = formatMetadata(log.metadata);

                return (
                  <article key={log.logId} className={styles.executionObservability__item}>
                    <div className={styles.executionObservability__itemHeader}>
                      <div>
                        <span>Log #{log.sequenceNumber}</span>

                        <strong data-level={log.level}>{log.level}</strong>
                      </div>

                      <time>{formatExecutionDateTime(log.createdAt)}</time>
                    </div>

                    <p className={styles.executionObservability__message}>{log.message}</p>

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
            </div>
          ) : null}
        </div>
      ) : null}

      {activeTab === "errors" ? (
        <div className={styles.executionObservability__content}>
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
            <div className={styles.executionObservability__list}>
              {errorsQuery.data.items.map((error) => (
                <article
                  key={error.errorId}
                  className={[
                    styles.executionObservability__item,
                    styles.executionObservability__errorItem,
                  ].join(" ")}
                >
                  <div className={styles.executionObservability__itemHeader}>
                    <div>
                      <span>{error.category}</span>

                      <strong>{error.code}</strong>
                    </div>

                    <time>{formatExecutionDateTime(error.createdAt)}</time>
                  </div>

                  <p className={styles.executionObservability__message}>{error.safeMessage}</p>

                  <div className={styles.executionObservability__badges}>
                    <span>Retryable: {error.retryable ? "Yes" : "No"}</span>

                    {error.nodeExecutionId ? <span>Node linked</span> : null}

                    {error.relatedEventId ? <span>Event linked</span> : null}
                  </div>

                  {Object.keys(error.details).length > 0 ? (
                    <details>
                      <summary>Safe details</summary>

                      <dl className={styles.executionObservability__details}>
                        {Object.entries(error.details).map(([key, value]) => (
                          <div key={key}>
                            <dt>{key}</dt>

                            <dd>{value}</dd>
                          </div>
                        ))}
                      </dl>
                    </details>
                  ) : null}
                </article>
              ))}
            </div>
          ) : null}
        </div>
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
    <div
      className={[
        styles.executionObservability__state,
        error ? styles.executionObservability__stateError : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <span>{message}</span>
    </div>
  );
}
