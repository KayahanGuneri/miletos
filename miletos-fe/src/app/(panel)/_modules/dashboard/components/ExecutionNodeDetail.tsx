import { type NodeExecutionResponse } from "@/app/(panel)/_modules/dashboard/types/execution-types";
import {
  formatExecutionDateTime,
  formatExecutionStatus,
} from "@/app/(panel)/_modules/dashboard/utils/execution-formatters";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import styles from "./ExecutionNodeDetail.module.css";
import { JsonDataSection } from "./JsonDataSection";

interface ExecutionNodeDetailProps {
  selectedNodeId: string | null;
  nodeExecution: NodeExecutionResponse | null;
}

export function ExecutionNodeDetail({ selectedNodeId, nodeExecution }: ExecutionNodeDetailProps) {
  if (!selectedNodeId) {
    return (
      <section className={styles.executionNodeDetail__empty}>
        <Typography as="strong">Select a workflow node.</Typography>
        <Typography as="span">
          Click a node in the execution graph to inspect its runtime details.
        </Typography>
      </section>
    );
  }

  if (!nodeExecution) {
    return (
      <section className={styles.executionNodeDetail__empty}>
        <Typography as="strong">{selectedNodeId}</Typography>
        <Typography as="span">
          This node exists in the immutable workflow definition, but no runtime execution record is
          available in the loaded node page.
        </Typography>
      </section>
    );
  }

  return (
    <section className={styles.executionNodeDetail}>
      <header className={styles.executionNodeDetail__header}>
        <Box>
          <Typography as="p">Selected runtime node</Typography>
          <Typography as="h2">{nodeExecution.nodeId}</Typography>
        </Box>
        <Typography
          as="span"
          className={styles.executionNodeDetail__status}
          data-status={nodeExecution.status}
        >
          {formatExecutionStatus(nodeExecution.status)}
        </Typography>
      </header>

      <Box className={styles.executionNodeDetail__grid}>
        <dl>
          <Box>
            <dt>Node execution ID</dt>
            <dd>{nodeExecution.nodeExecutionId}</dd>
          </Box>
          <Box>
            <dt>Plugin</dt>
            <dd>{nodeExecution.pluginType}</dd>
          </Box>
          <Box>
            <dt>Plugin version</dt>
            <dd>{nodeExecution.pluginVersion}</dd>
          </Box>
          <Box>
            <dt>Attempt</dt>
            <dd>#{nodeExecution.attempt}</dd>
          </Box>
        </dl>
        <dl>
          <Box>
            <dt>Created</dt>
            <dd>{formatExecutionDateTime(nodeExecution.createdAt)}</dd>
          </Box>
          <Box>
            <dt>Ready</dt>
            <dd>{formatExecutionDateTime(nodeExecution.readyAt)}</dd>
          </Box>
          <Box>
            <dt>Started</dt>
            <dd>{formatExecutionDateTime(nodeExecution.startedAt)}</dd>
          </Box>
          <Box>
            <dt>Finished</dt>
            <dd>{formatExecutionDateTime(nodeExecution.finishedAt)}</dd>
          </Box>
        </dl>
      </Box>

      <Box className={styles.executionNodeDetail__payloads}>
        <JsonDataSection
          label="Configuration"
          value={nodeExecution.configuration}
          emptyMessage="No configuration was recorded for this node."
        />
        <JsonDataSection
          label="Input"
          value={nodeExecution.inputSummary}
          emptyMessage="Input is not available yet."
        />
        <JsonDataSection
          label="Output"
          value={nodeExecution.outputSummary}
          emptyMessage={
            nodeExecution.status === "PENDING" || nodeExecution.status === "QUEUED"
              ? "Output will appear after the node runs."
              : "No output was recorded."
          }
        />
        <JsonDataSection
          label="Failure"
          value={nodeExecution.failureSummary}
          emptyMessage="No failure was recorded."
        />
      </Box>
    </section>
  );
}
