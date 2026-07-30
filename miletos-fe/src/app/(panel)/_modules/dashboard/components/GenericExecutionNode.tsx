"use client";

import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Typography } from "@/components/lib/typography/Typography";
import { formatExecutionStatus } from "@/app/(panel)/_modules/dashboard/utils/execution-formatters";
import { type ExecutionGraphNode } from "@/app/(panel)/_modules/dashboard/types/execution-graph-types";
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
        <Typography as="span" className={styles.genericExecutionNode__type}>
          {data.pluginType}
        </Typography>

        {runtimeStatus ? (
          <Typography as="span" className={styles.genericExecutionNode__status}>
            {formatExecutionStatus(runtimeStatus)}
          </Typography>
        ) : (
          <Typography as="span" className={styles.genericExecutionNode__statusMuted}>
            Definition
          </Typography>
        )}
      </header>

      <Typography as="strong" className={styles.genericExecutionNode__name} title={data.nodeId}>
        {data.label}
      </Typography>

      <footer className={styles.genericExecutionNode__footer}>
        <Typography as="span">Version</Typography>

        <Typography as="strong">{data.pluginVersion}</Typography>

        {typeof data.attempt === "number" ? (
          <>
            <Typography as="span">Attempt</Typography>

            <Typography as="strong">#{data.attempt}</Typography>
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
