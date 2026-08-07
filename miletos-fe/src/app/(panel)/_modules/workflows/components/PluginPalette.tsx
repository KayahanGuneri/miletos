import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { writePluginDragData } from "@/shared/plugins/drag-data/plugin-drag-data";
import {
  PLUGIN_CATEGORY_LABELS,
  PLUGIN_CATEGORY_ORDER,
  resolvePluginCategory,
} from "@/shared/plugins/registry/plugin-palette-registry";
import { type PluginCategory } from "@/shared/plugins/contracts/plugin-configuration-types";
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
          ? workflowMessages.pluginPalette.readOnlyCardLabel(plugin.displayName)
          : workflowMessages.pluginPalette.draggableCardLabel(plugin.displayName)
      }
      title={
        readOnly
          ? workflowMessages.pluginPalette.readOnlyCardTitle
          : workflowMessages.pluginPalette.draggableCardTitle
      }
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
        {workflowMessages.pluginPalette.portSummary(
          plugin.inputPorts.length,
          plugin.outputPorts.length,
        )}
      </Typography>
      <Typography as="small">
        {readOnly
          ? workflowMessages.pluginPalette.readOnlyHint
          : workflowMessages.pluginPalette.dragHint}
      </Typography>
    </article>
  );
}

export function PluginPalette({ plugins, isLoading, errorMessage, readOnly }: PluginPaletteProps) {
  const groups = groupPlugins(plugins);

  return (
    <section className={styles.workflowEditor__sidePanel}>
      <Typography as="p" className={styles.workflowEditor__eyebrow}>
        {workflowMessages.pluginPalette.eyebrow}
      </Typography>
      <Typography as="h2">{workflowMessages.pluginPalette.title}</Typography>
      <Typography as="p" className={styles.workflowEditor__muted}>
        {workflowMessages.pluginPalette.description}
      </Typography>
      {isLoading ? <Typography as="p">{workflowMessages.pluginPalette.loading}</Typography> : null}
      {errorMessage ? (
        <Typography as="p" className={styles.workflowEditor__fieldError} role="alert">
          {errorMessage}
        </Typography>
      ) : null}
      {!isLoading && !errorMessage && plugins.length === 0 ? (
        <Typography as="p" className={styles.workflowEditor__muted}>
          {workflowMessages.pluginPalette.empty}
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
