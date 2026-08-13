import type { PluginDescriptor } from "@/shared/plugins/contracts/plugin-configuration-interfaces";

export const PLUGIN_DRAG_DATA_TYPE = "application/x-miletos-workflow-plugin";

export interface PluginDragData {
  pluginType: string;
  pluginVersion: string;
}

export function writePluginDragData(dataTransfer: DataTransfer, plugin: PluginDescriptor) {
  const payload: PluginDragData = {
    pluginType: plugin.type,
    pluginVersion: plugin.version,
  };
  dataTransfer.effectAllowed = "copy";
  dataTransfer.setData(PLUGIN_DRAG_DATA_TYPE, JSON.stringify(payload));
}

export function readPluginDragData(dataTransfer: DataTransfer): PluginDragData | null {
  const rawPayload = dataTransfer.getData(PLUGIN_DRAG_DATA_TYPE);
  if (!rawPayload) {
    return null;
  }

  try {
    const payload: unknown = JSON.parse(rawPayload);
    if (
      typeof payload === "object" &&
      payload !== null &&
      "pluginType" in payload &&
      "pluginVersion" in payload &&
      typeof payload.pluginType === "string" &&
      typeof payload.pluginVersion === "string"
    ) {
      return {
        pluginType: payload.pluginType,
        pluginVersion: payload.pluginVersion,
      };
    }
  } catch {
    return null;
  }

  return null;
}
