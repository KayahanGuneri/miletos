import { type NodeExecutionResponse } from "@/app/(panel)/_modules/dashboard/types/execution-types";
import {
  formatExecutionDateTime,
  formatExecutionStatus,
} from "@/app/(panel)/_modules/dashboard/utils/execution-formatters";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Dialog } from "@/components/lib/dialog/Dialog";
import { Typography } from "@/components/lib/typography/Typography";
import { JsonDataSection } from "./JsonDataSection";
import styles from "./RuntimeNodeDetailsDialog.module.css";

interface RuntimeNodeDetailsDialogProps {
  selectedNodeId: string;
  nodeExecution: NodeExecutionResponse | null;
  onClose: () => void;
}

export function RuntimeNodeDetailsDialog({
  selectedNodeId,
  nodeExecution,
  onClose,
}: RuntimeNodeDetailsDialogProps) {
  if (!nodeExecution) {
    return (
      <Dialog
        title="Runtime node details"
        description={selectedNodeId}
        size="large"
        onClose={onClose}
        footer={
          <Button type="button" variant={ButtonVariant.Secondary} onClick={onClose}>
            Close
          </Button>
        }
      >
        <Box className={styles.runtimeNodeDetails__empty}>
          <Typography as="strong">No runtime record is available for this node.</Typography>
          <Typography as="span">
            The node exists in the immutable workflow definition, but it has not produced a node
            execution in the loaded runtime page.
          </Typography>
        </Box>
      </Dialog>
    );
  }

  return (
    <Dialog
      title="Runtime node details"
      description={`${nodeExecution.pluginType} / ${nodeExecution.pluginVersion}`}
      size="large"
      onClose={onClose}
      footer={
        <Button type="button" variant={ButtonVariant.Secondary} onClick={onClose}>
          Close
        </Button>
      }
    >
      <header className={styles.runtimeNodeDetails__header}>
        <Box>
          <Typography as="p">Selected runtime node</Typography>
          <Typography as="h2">{nodeExecution.nodeId}</Typography>
        </Box>
        <Typography
          as="span"
          className={styles.runtimeNodeDetails__status}
          data-status={nodeExecution.status}
        >
          {formatExecutionStatus(nodeExecution.status)}
        </Typography>
      </header>

      <Box className={styles.runtimeNodeDetails__grid}>
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

      <Box className={styles.runtimeNodeDetails__payloads}>
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
    </Dialog>
  );
}
