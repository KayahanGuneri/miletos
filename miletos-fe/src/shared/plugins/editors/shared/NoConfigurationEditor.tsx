import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfiguration,
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

interface NoConfigurationValues {
  configuration: PluginConfiguration;
}

function NoConfigurationEditor({
  disabled,
}: PluginConfigurationEditorProps<NoConfigurationValues>) {
  return (
    <Typography as="p" className={styles.pluginConfiguration__notice} aria-disabled={disabled}>
      This plugin does not require configuration.
    </Typography>
  );
}

export function createNoConfigurationDefinition(
  pluginType: string,
): PluginConfigurationDefinition<NoConfigurationValues> {
  return {
    pluginType,
    createDefaultConfiguration: () => ({}),
    deserialize: (configuration) => ({ configuration }),
    validate: () => ({ valid: true, errors: {} }),
    serialize: (values) => values.configuration,
    Editor: NoConfigurationEditor,
  };
}
