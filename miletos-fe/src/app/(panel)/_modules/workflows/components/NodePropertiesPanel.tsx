import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import { type WorkflowNode } from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
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
        {workflowMessages.nodeProperties.eyebrow}
      </Typography>
      <Typography as="h2">{workflowMessages.nodeProperties.title}</Typography>
      {!node ? (
        <Typography as="p" className={styles.workflowEditor__muted}>
          {workflowMessages.nodeProperties.empty}
        </Typography>
      ) : (
        <Box className={styles.workflowEditor__properties}>
          <Typography as="span">
            <Typography as="strong">{workflowMessages.nodeProperties.nodeId}</Typography>
            {node.nodeId}
          </Typography>
          <Typography as="span">
            <Typography as="strong">{workflowMessages.nodeProperties.pluginType}</Typography>
            {node.pluginType}
          </Typography>
          <Typography as="span">
            <Typography as="strong">{workflowMessages.nodeProperties.pluginVersion}</Typography>
            {node.pluginVersion}
          </Typography>
          {plugin ? (
            <Typography as="span">
              <Typography as="strong">{workflowMessages.nodeProperties.plugin}</Typography>
              {plugin.displayName}
            </Typography>
          ) : null}
          <Box className={styles.workflowEditor__propertyActions}>
            <Button type="button" variant={ButtonVariant.Secondary} onClick={onConfigure}>
              {readOnly
                ? workflowMessages.nodeProperties.viewConfiguration
                : workflowMessages.nodeProperties.configure}
            </Button>
            <Button
              type="button"
              variant={ButtonVariant.Secondary}
              disabled={readOnly}
              onClick={onDelete}
            >
              {workflowMessages.nodeProperties.delete}
            </Button>
          </Box>
        </Box>
      )}
    </section>
  );
}
