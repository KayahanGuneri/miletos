import { formatExecutionDateTime } from "@/app/(panel)/_modules/dashboard/model/execution-formatters";
import { formatExecutionStatus } from "@/app/(panel)/_modules/dashboard/model/execution-status";
import { type NodeExecutionResponse } from "@/app/(panel)/_modules/dashboard/model/execution-types";
import styles from "./ExecutionNodeDetail.module.css";

interface ExecutionNodeDetailProps {
  selectedNodeId: string | null;
  nodeExecution: NodeExecutionResponse | null;
}

function formatPayloadSummary(payload: NodeExecutionResponse["inputSummary"] | undefined) {
  if (!payload) {
    return "Not recorded";
  }

  const parts: string[] = [];

  if (payload.source) {
    parts.push(`Source: ${payload.source}`);
  }

  if (payload.contentType) {
    parts.push(`Type: ${payload.contentType}`);
  }

  if (typeof payload.sizeBytes === "number") {
    parts.push(`Size: ${payload.sizeBytes} bytes`);
  }

  return parts.length > 0 ? parts.join(" · ") : "Recorded";
}

export function ExecutionNodeDetail({ selectedNodeId, nodeExecution }: ExecutionNodeDetailProps) {
  if (!selectedNodeId) {
    return (
      <section className={styles.executionNodeDetail__empty}>
        <strong>Select a workflow node.</strong>

        <span>Click a node in the execution graph to inspect its runtime details.</span>
      </section>
    );
  }

  if (!nodeExecution) {
    return (
      <section className={styles.executionNodeDetail__empty}>
        <strong>{selectedNodeId}</strong>

        <span>
          This node exists in the immutable workflow definition, but no runtime execution record is
          available in the loaded node page.
        </span>
      </section>
    );
  }

  return (
    <section className={styles.executionNodeDetail}>
      <header className={styles.executionNodeDetail__header}>
        <div>
          <p>Selected runtime node</p>

          <h2>{nodeExecution.nodeId}</h2>
        </div>

        <span className={styles.executionNodeDetail__status} data-status={nodeExecution.status}>
          {formatExecutionStatus(nodeExecution.status)}
        </span>
      </header>

      <div className={styles.executionNodeDetail__grid}>
        <dl>
          <div>
            <dt>Node execution ID</dt>

            <dd>{nodeExecution.nodeExecutionId}</dd>
          </div>

          <div>
            <dt>Plugin</dt>

            <dd>{nodeExecution.pluginType}</dd>
          </div>

          <div>
            <dt>Plugin version</dt>

            <dd>{nodeExecution.pluginVersion}</dd>
          </div>

          <div>
            <dt>Attempt</dt>

            <dd>#{nodeExecution.attempt}</dd>
          </div>
        </dl>

        <dl>
          <div>
            <dt>Created</dt>

            <dd>{formatExecutionDateTime(nodeExecution.createdAt)}</dd>
          </div>

          <div>
            <dt>Ready</dt>

            <dd>{formatExecutionDateTime(nodeExecution.readyAt)}</dd>
          </div>

          <div>
            <dt>Started</dt>

            <dd>{formatExecutionDateTime(nodeExecution.startedAt)}</dd>
          </div>

          <div>
            <dt>Finished</dt>

            <dd>{formatExecutionDateTime(nodeExecution.finishedAt)}</dd>
          </div>
        </dl>
      </div>

      <div className={styles.executionNodeDetail__payloads}>
        <article>
          <span>Input summary</span>

          <strong>{formatPayloadSummary(nodeExecution.inputSummary)}</strong>
        </article>

        <article>
          <span>Output summary</span>

          <strong>{formatPayloadSummary(nodeExecution.outputSummary)}</strong>
        </article>
      </div>

      {nodeExecution.failureSummary ? (
        <div className={styles.executionNodeDetail__failure}>
          <span>Failure</span>

          <strong>{nodeExecution.failureSummary.code}</strong>

          <p>{nodeExecution.failureSummary.message}</p>

          <small>
            {nodeExecution.failureSummary.category}
            {" · "}
            Retryable: {nodeExecution.failureSummary.retryable ? "Yes" : "No"}
          </small>
        </div>
      ) : null}
    </section>
  );
}
