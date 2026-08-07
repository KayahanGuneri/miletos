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
  "core.static-input": "input",
  "core.delay": "flow-control",
  "core.join": "flow-control",
  "core.pass-through": "flow-control",
  "core.terminal": "output",
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
    category: resolvePluginCategory(plugin.type, plugin.category),
  };
}
