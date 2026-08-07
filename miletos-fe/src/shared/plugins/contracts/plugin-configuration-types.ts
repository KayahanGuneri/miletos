export type PluginConfigurationValue =
  | string
  | number
  | boolean
  | null
  | { [key: string]: PluginConfigurationValue }
  | PluginConfigurationValue[];

export type PluginCategory = "trigger" | "input" | "flow-control" | "output" | "other";
