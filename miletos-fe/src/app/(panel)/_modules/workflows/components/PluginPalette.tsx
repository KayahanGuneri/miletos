import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { writePluginDragData } from "@/shared/plugins/drag-data/plugin-drag-data";
import styles from "../ui/WorkflowEditorPage.module.css";

interface PluginPaletteProps {
  plugins: WorkflowPlugin[];
  isLoading: boolean;
  errorMessage?: string;
  readOnly: boolean;
}

export function PluginPalette({ plugins, isLoading, errorMessage, readOnly }: PluginPaletteProps) {
  return (
    <section className={styles.workflowEditor__sidePanel}>
      <Typography as="p" className={styles.workflowEditor__eyebrow}>
        Plugin palette
      </Typography>
      <Typography as="h2">Nodes</Typography>
      <Typography as="p" className={styles.workflowEditor__muted}>
        Drag an installed runtime plugin onto the workflow canvas.
      </Typography>
      {isLoading ? <Typography as="p">Loading plugins...</Typography> : null}
      {errorMessage ? (
        <Typography as="p" className={styles.workflowEditor__fieldError} role="alert">
          {errorMessage}
        </Typography>
      ) : null}
      {!isLoading && !errorMessage && plugins.length === 0 ? (
        <Typography as="p" className={styles.workflowEditor__muted}>
          No runtime plugins are currently available.
        </Typography>
      ) : null}
      <Box className={styles.workflowEditor__pluginList}>
        {plugins.map((plugin) => (
          <article
            key={`${plugin.type}:${plugin.version}`}
            className={styles.workflowEditor__pluginCard}
            draggable={!readOnly}
            tabIndex={0}
            data-disabled={readOnly}
            aria-label={
              readOnly
                ? `${plugin.displayName}; workflow is read-only`
                : `Drag ${plugin.displayName} onto the workflow canvas`
            }
            title={readOnly ? "Workflow is read-only" : "Drag onto the canvas to add this node"}
            onDragStart={(event) => {
              if (readOnly) {
                event.preventDefault();
                return;
              }
              writePluginDragData(event.dataTransfer, plugin);
            }}
          >
            <Typography as="strong">{plugin.displayName}</Typography>
            <Typography as="span">
              {plugin.type} · {plugin.version}
            </Typography>
            <Typography as="p">{plugin.description}</Typography>
            <Typography as="small">
              {plugin.inputPorts.length} inputs · {plugin.outputPorts.length} outputs
            </Typography>
            <Typography as="small">
              {readOnly ? "Available for inspection only" : "Drag to canvas"}
            </Typography>
          </article>
        ))}
      </Box>
    </section>
  );
}
