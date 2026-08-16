import { Handle, Position, type NodeProps } from "@xyflow/react";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import {
  PLUGIN_CATEGORY_LABELS,
  resolvePluginCategory,
} from "@/shared/plugins/registry/plugin-palette-registry";
import styles from "../ui/WorkflowEditorPage.module.css";

export interface WorkflowCanvasNodeData extends Record<string, unknown> {
  label: string;
  pluginType: string;
  pluginVersion: string;
  plugin?: WorkflowPlugin;
  readOnly: boolean;
}

export function WorkflowCanvasNode({ data, selected }: NodeProps) {
  const nodeData = data as WorkflowCanvasNodeData;
  const category = resolvePluginCategory(nodeData.pluginType, nodeData.plugin?.category);

  return (
    <Box className={styles.workflowEditor__node} data-selected={selected}>
      {nodeData.plugin?.inputPorts.map((port, index) => {
        const top = 30 + index * 20;
        return (
          <span key={port.name} className={styles.workflowEditor__inputPort} style={{ top }}>
            <Handle
              id={port.name}
              type="target"
              position={Position.Left}
              title={port.description || port.displayName || port.name}
            />
            <span>{port.displayName || port.name}</span>
          </span>
        );
      })}
      <Typography as="span" className={styles.workflowEditor__nodeCategory}>
        {PLUGIN_CATEGORY_LABELS[category]}
      </Typography>
      <Typography as="strong">{nodeData.label}</Typography>
      <Typography as="small">
        {nodeData.pluginType} · {nodeData.pluginVersion}
      </Typography>
      {nodeData.plugin?.outputPorts.map((port, index) => {
        const top = 30 + index * 20;
        return (
          <span key={port.name} className={styles.workflowEditor__outputPort} style={{ top }}>
            <span>{port.displayName || port.name}</span>
            <Handle
              id={port.name}
              type="source"
              position={Position.Right}
              title={port.description || port.displayName || port.name}
            />
          </span>
        );
      })}
    </Box>
  );
}
