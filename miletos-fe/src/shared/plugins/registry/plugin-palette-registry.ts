import { type PluginDescriptor } from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { type PluginCategory } from "@/shared/plugins/contracts/plugin-configuration-types";

export const PLUGIN_CATEGORY_ORDER: readonly PluginCategory[] = [
  "trigger",
  "input",
  "flow-control",
  "output",
  "other",
];

export const PLUGIN_CATEGORY_LABELS: Record<PluginCategory, string> = {
  trigger: "Triggers",
  input: "Inputs",
  "flow-control": "Flow Control",
  output: "Outputs",
  other: "Other",
};

const CATEGORY_BY_PLUGIN_TYPE: Readonly<Record<string, PluginCategory>> = {
  "core.http-trigger": "trigger",
  "core.cron-trigger": "trigger",
  "core.file-input": "input",
  "core.excel-input": "input",
  "core.database-input": "input",
  "core.delay": "flow-control",
  "core.if": "flow-control",
  "core.filter": "flow-control",
  "core.terminal": "output",
  "core.rest-output": "output",
  "core.database-output": "output",
  "core.csv-output": "output",
};

function isPluginCategory(value: unknown): value is PluginCategory {
  return typeof value === "string" && PLUGIN_CATEGORY_ORDER.includes(value as PluginCategory);
}

export function resolvePluginCategory(pluginType: string, category?: unknown): PluginCategory {
  if (isPluginCategory(category)) {
    return category;
  }

  return CATEGORY_BY_PLUGIN_TYPE[pluginType] ?? "other";
}

export function withPluginCategory(
  plugin: Omit<PluginDescriptor, "category"> & { category?: unknown },
): PluginDescriptor {
  return {
    ...plugin,
    displayName: plugin.displayName || plugin.type,
    description: plugin.description ?? "",
    inputPorts: plugin.inputPorts.map((port) => ({
      ...port,
      displayName: port.displayName || port.name,
      description: port.description ?? "",
    })),
    outputPorts: plugin.outputPorts.map((port) => ({
      ...port,
      displayName: port.displayName || port.name,
      description: port.description ?? "",
    })),
    connectionRestrictions: plugin.connectionRestrictions ?? [],
    category: resolvePluginCategory(plugin.type, plugin.category),
  };
}
