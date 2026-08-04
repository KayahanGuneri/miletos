import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import {
  type WorkflowNode,
  type WorkflowPlugin,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";
import styles from "../ui/WorkflowEditorPage.module.css";

interface NodePropertiesPanelProps {
  node: WorkflowNode | null;
  plugin?: WorkflowPlugin;
  readOnly: boolean;
  onConfigure: () => void;
  onDelete: () => void;
}

export function NodePropertiesPanel({
  node,
  plugin,
  readOnly,
  onConfigure,
  onDelete,
}: NodePropertiesPanelProps) {
  return (
    <section className={styles.workflowEditor__sidePanel}>
      <Typography as="p" className={styles.workflowEditor__eyebrow}>
        Properties
      </Typography>
      <Typography as="h2">Selected node</Typography>
      {!node ? (
        <Typography as="p" className={styles.workflowEditor__muted}>
          Select a node on the canvas to inspect its configuration.
        </Typography>
      ) : (
        <Box className={styles.workflowEditor__properties}>
          <Typography as="span">
            <Typography as="strong">Node ID</Typography>
            {node.nodeId}
          </Typography>
          <Typography as="span">
            <Typography as="strong">Plugin type</Typography>
            {node.pluginType}
          </Typography>
          <Typography as="span">
            <Typography as="strong">Plugin version</Typography>
            {node.pluginVersion}
          </Typography>
          {plugin ? (
            <Typography as="span">
              <Typography as="strong">Plugin</Typography>
              {plugin.displayName}
            </Typography>
          ) : null}
          <Box className={styles.workflowEditor__propertyActions}>
            <Button type="button" variant={ButtonVariant.Secondary} onClick={onConfigure}>
              {readOnly ? "View configuration" : "Configure"}
            </Button>
            <Button
              type="button"
              variant={ButtonVariant.Secondary}
              disabled={readOnly}
              onClick={onDelete}
            >
              Delete node
            </Button>
          </Box>
        </Box>
      )}
    </section>
  );
}
