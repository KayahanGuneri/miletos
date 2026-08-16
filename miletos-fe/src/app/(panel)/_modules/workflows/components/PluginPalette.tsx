"use client";

import { useMemo, useState } from "react";
import { Box } from "@/components/lib/box/Box";
import { Input } from "@/components/lib/input/Input";
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
  const categoryLabel = PLUGIN_CATEGORY_LABELS[resolvePluginCategory(plugin.type, plugin.category)];
  const cardTitle = readOnly
    ? workflowMessages.pluginPalette.readOnlyCardTitle
    : `${workflowMessages.pluginPalette.draggableCardTitle}. ${plugin.description}`;

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
      title={cardTitle}
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
        {categoryLabel} ·{" "}
        {workflowMessages.pluginPalette.portSummary(
          plugin.inputPorts.length,
          plugin.outputPorts.length,
        )}
      </Typography>
    </article>
  );
}

export function PluginPalette({ plugins, isLoading, errorMessage, readOnly }: PluginPaletteProps) {
  const [search, setSearch] = useState("");
  const normalizedSearch = search.trim().toLowerCase();
  const filteredPlugins = useMemo(() => {
    if (!normalizedSearch) {
      return plugins;
    }

    return plugins.filter((plugin) => {
      const category = resolvePluginCategory(plugin.type, plugin.category);
      const categoryLabel = PLUGIN_CATEGORY_LABELS[category];
      return (
        plugin.displayName.toLowerCase().includes(normalizedSearch) ||
        plugin.type.toLowerCase().includes(normalizedSearch) ||
        category.toLowerCase().includes(normalizedSearch) ||
        categoryLabel.toLowerCase().includes(normalizedSearch)
      );
    });
  }, [normalizedSearch, plugins]);
  const groups = groupPlugins(filteredPlugins);

  return (
    <section className={styles.workflowEditor__sidePanel}>
      <Box className={styles.workflowEditor__sideHeader}>
        <Typography as="p" className={styles.workflowEditor__eyebrow}>
          {workflowMessages.pluginPalette.eyebrow}
        </Typography>
        <Typography as="h2">{workflowMessages.pluginPalette.title}</Typography>
        <label className={styles.workflowEditor__pluginSearch}>
          <Typography as="span" className={styles.workflowEditor__eyebrow}>
            {workflowMessages.pluginPalette.searchLabel}
          </Typography>
          <Input
            className={styles.workflowEditor__searchInput}
            type="search"
            value={search}
            placeholder={workflowMessages.pluginPalette.searchPlaceholder}
            aria-label={workflowMessages.pluginPalette.searchLabel}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
      </Box>
      <Box className={styles.workflowEditor__sideScroll}>
        {isLoading ? (
          <Typography as="p">{workflowMessages.pluginPalette.loading}</Typography>
        ) : null}
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
        {!isLoading && !errorMessage && plugins.length > 0 && filteredPlugins.length === 0 ? (
          <Typography as="p" className={styles.workflowEditor__muted}>
            {workflowMessages.pluginPalette.emptyFiltered}
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
      </Box>
    </section>
  );
}
