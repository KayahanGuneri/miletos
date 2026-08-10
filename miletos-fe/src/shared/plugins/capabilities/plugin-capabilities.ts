import { type PluginDescriptor } from "@/shared/plugins/contracts/plugin-configuration-interfaces";

export function supportsExecutionOrigin(
  plugin: Pick<PluginDescriptor, "allowedRootOrigins"> | null | undefined,
  origin: string,
): boolean {
  return Boolean(plugin?.allowedRootOrigins.includes(origin));
}
