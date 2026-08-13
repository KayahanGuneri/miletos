import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import { WorkflowHTTPTriggerCard } from "@/app/(panel)/_modules/workflows/components/WorkflowHTTPTriggerCard";
import {
  workflowMessages,
  workflowStatusReason,
} from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import { type WorkflowNode } from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { supportsExecutionOrigin } from "@/shared/plugins/capabilities/plugin-capabilities";
import styles from "../ui/WorkflowEditorPage.module.css";

interface NodePropertiesPanelProps {
  node: WorkflowNode | null;
  plugin?: WorkflowPlugin;
  readOnly: boolean;
  workflowId?: number;
  workflowStatus?: "DRAFT" | "ACTIVE" | "ARCHIVED";
  canManageTriggers?: boolean;
  onDisplayNameChange: (displayName: string) => void;
  onConfigure: () => void;
  onDelete: () => void;
}

export function NodePropertiesPanel({
  node,
  plugin,
  readOnly,
  workflowId,
  workflowStatus,
  canManageTriggers = false,
  onDisplayNameChange,
  onConfigure,
  onDelete,
}: NodePropertiesPanelProps) {
  const showHttpTrigger =
    supportsExecutionOrigin(plugin, "HTTP_WEBHOOK") &&
    typeof workflowId === "number" &&
    workflowId > 0;

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
          <label className={styles.workflowEditor__field}>
            <Typography as="span">{workflowMessages.nodeProperties.displayName}</Typography>
            <Input
              maxLength={120}
              readOnly={readOnly}
              value={node.displayName ?? ""}
              placeholder={plugin?.displayName ?? node.pluginType}
              onChange={(event) => onDisplayNameChange(event.target.value)}
            />
          </label>
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
          {showHttpTrigger ? (
            <WorkflowHTTPTriggerCard
              workflowId={workflowId}
              triggerNodeId={node.nodeId}
              canManage={canManageTriggers}
              active={workflowStatus === "ACTIVE"}
              statusReason={workflowStatus ? workflowStatusReason(workflowStatus) : undefined}
              compact
            />
          ) : null}
        </Box>
      )}
    </section>
  );
}
