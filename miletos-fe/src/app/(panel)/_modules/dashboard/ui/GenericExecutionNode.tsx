"use client";

import { Handle, Position, type NodeProps } from "@xyflow/react";
import { formatExecutionStatus } from "@/app/(panel)/_modules/dashboard/model/execution-status";
import { type ExecutionGraphNode } from "@/app/(panel)/_modules/dashboard/model/execution-graph-types";
import styles from "./GenericExecutionNode.module.css";

export function GenericExecutionNode({ data, selected }: NodeProps<ExecutionGraphNode>) {
  const runtimeStatus = data.runtimeStatus;

  return (
    <article
      className={[styles.genericExecutionNode, selected ? styles.genericExecutionNodeSelected : ""]
        .filter(Boolean)
        .join(" ")}
      data-status={runtimeStatus ?? "UNKNOWN"}
    >
      <Handle
        className={styles.genericExecutionNode__handle}
        type="target"
        position={Position.Left}
        isConnectable={false}
      />

      <header className={styles.genericExecutionNode__header}>
        <span className={styles.genericExecutionNode__type}>{data.pluginType}</span>

        {runtimeStatus ? (
          <span className={styles.genericExecutionNode__status}>
            {formatExecutionStatus(runtimeStatus)}
          </span>
        ) : (
          <span className={styles.genericExecutionNode__statusMuted}>Definition</span>
        )}
      </header>

      <strong className={styles.genericExecutionNode__name} title={data.nodeId}>
        {data.label}
      </strong>

      <footer className={styles.genericExecutionNode__footer}>
        <span>Version</span>

        <strong>{data.pluginVersion}</strong>

        {typeof data.attempt === "number" ? (
          <>
            <span>Attempt</span>

            <strong>#{data.attempt}</strong>
          </>
        ) : null}
      </footer>

      <Handle
        className={styles.genericExecutionNode__handle}
        type="source"
        position={Position.Right}
        isConnectable={false}
      />
    </article>
  );
}
