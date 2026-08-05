import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { writePluginDragData } from "@/shared/plugins/drag-data/plugin-drag-data";
import {
  PLUGIN_CATEGORY_LABELS,
  PLUGIN_CATEGORY_ORDER,
  resolvePluginCategory,
} from "@/shared/plugins/registry/plugin-palette-registry";
import { type PluginCategory } from "@/shared/plugins/contracts/plugin-configuration";
import styles from "../ui/WorkflowEditorPage.module.css";

interface PluginPaletteProps {
  plugins: WorkflowPlugin[];
  isLoading: boolean;
  errorMessage?: string;
  readOnly: boolean;
}

interface PluginPaletteCardProps {
  plugin: WorkflowPlugin;
  readOnly: boolean;
}

function comparePlugins(first: WorkflowPlugin, second: WorkflowPlugin) {
  return (
    first.displayName.localeCompare(second.displayName, undefined, { sensitivity: "base" }) ||
    first.type.localeCompare(second.type) ||
    first.version.localeCompare(second.version)
  );
}

function groupPlugins(plugins: WorkflowPlugin[]) {
  const pluginsByCategory = PLUGIN_CATEGORY_ORDER.reduce<Record<PluginCategory, WorkflowPlugin[]>>(
    (groups, category) => ({ ...groups, [category]: [] }),
    {} as Record<PluginCategory, WorkflowPlugin[]>,
  );

  plugins.forEach((plugin) => {
    const category = resolvePluginCategory(plugin.type, plugin.category);
    pluginsByCategory[category].push(plugin);
  });

  return PLUGIN_CATEGORY_ORDER.map((category) => ({
    category,
    plugins: pluginsByCategory[category].toSorted(comparePlugins),
  })).filter((group) => group.plugins.length > 0);
}

function PluginPaletteCard({ plugin, readOnly }: PluginPaletteCardProps) {
  return (
    <article
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
  );
}

export function PluginPalette({ plugins, isLoading, errorMessage, readOnly }: PluginPaletteProps) {
  const groups = groupPlugins(plugins);

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
      {!isLoading && !errorMessage ? (
        <Box className={styles.workflowEditor__pluginGroups}>
          {groups.map((group) => (
            <section key={group.category} className={styles.workflowEditor__pluginGroup}>
              <Typography as="h3">{PLUGIN_CATEGORY_LABELS[group.category]}</Typography>
              <Box className={styles.workflowEditor__pluginList}>
                {group.plugins.map((plugin) => (
                  <PluginPaletteCard
                    key={`${plugin.type}:${plugin.version}`}
                    plugin={plugin}
                    readOnly={readOnly}
                  />
                ))}
              </Box>
            </section>
          ))}
        </Box>
      ) : null}
    </section>
  );
}
