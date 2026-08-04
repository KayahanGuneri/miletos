import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
} from "@/shared/plugins/contracts/plugin-configuration";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

interface DelayValues {
  delay: string;
}

const GO_DURATION_PART = /([0-9]+(?:\.[0-9]*)?|\.[0-9]+)(ns|us|\u00b5s|\u03bcs|ms|s|m|h)/gy;
const UNIT_MILLISECONDS: Record<string, number> = {
  ns: 0.000001,
  us: 0.001,
  "\u00b5s": 0.001,
  "\u03bcs": 0.001,
  ms: 1,
  s: 1_000,
  m: 60_000,
  h: 3_600_000,
};

function parseGoDurationMilliseconds(value: string) {
  const normalized = value.trim();
  const unsigned = normalized.startsWith("+") ? normalized.slice(1) : normalized;
  if (!unsigned) {
    return null;
  }

  let cursor = 0;
  let total = 0;
  GO_DURATION_PART.lastIndex = 0;
  for (const match of unsigned.matchAll(GO_DURATION_PART)) {
    if (match.index !== cursor) {
      return null;
    }
    total += Number(match[1]) * UNIT_MILLISECONDS[match[2]];
    cursor = match.index + match[0].length;
  }

  return cursor === unsigned.length && Number.isFinite(total) ? total : null;
}

function DelayConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
}: PluginConfigurationEditorProps<DelayValues>) {
  return (
    <label className={styles.pluginConfiguration__field}>
      <Typography as="span">Delay duration</Typography>
      <input
        disabled={disabled}
        placeholder="For example: 500ms, 30s, 1h30m"
        value={initialValues.delay}
        onChange={(event) => onChange({ ...initialValues, delay: event.target.value })}
      />
      <Typography as="small">
        Go duration syntax; greater than zero and no more than 24h.
      </Typography>
      {validationErrors.delay ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {validationErrors.delay}
        </Typography>
      ) : null}
    </label>
  );
}

export const delayConfigurationDefinition: PluginConfigurationDefinition<DelayValues> = {
  pluginType: "core.delay",
  createDefaultConfiguration: () => ({}),
  deserialize: (configuration) => {
    const delay = configuration.delay;
    if (delay === undefined) {
      return { delay: "" };
    }
    if (typeof delay !== "string") {
      throw new Error("The persisted delay configuration is not a string.");
    }
    return { delay };
  },
  validate: (values) => {
    const duration = parseGoDurationMilliseconds(values.delay);
    const valid = duration !== null && duration > 0 && duration <= 86_400_000;
    const errors: Record<string, string> = valid
      ? {}
      : { delay: "Enter a positive duration no greater than 24h." };
    return {
      valid,
      errors,
    };
  },
  serialize: (values) => ({ delay: values.delay.trim() }),
  Editor: DelayConfigurationEditor,
};
