import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfigurationDefinition,
  PluginConfigurationEditorProps,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

const messages = pluginMessages.cronTrigger;

interface CronTriggerValues {
  expression: string;
  timezone: string;
}

function CronTriggerConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
}: PluginConfigurationEditorProps<CronTriggerValues>) {
  const expression = typeof initialValues.expression === "string" ? initialValues.expression : "";
  const timezone = typeof initialValues.timezone === "string" ? initialValues.timezone : "UTC";

  return (
    <>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.expressionLabel}</Typography>
        <Input
          disabled={disabled}
          value={expression}
          aria-invalid={Boolean(validationErrors.expression)}
          onChange={(event) =>
            onChange({
              expression: event.target.value,
              timezone,
            })
          }
          placeholder={messages.expressionPlaceholder}
        />
        <Typography as="small">
          {messages.expressionFormatHint}
          <br />
          {messages.expressionExampleHint}
          <br />
          {messages.expressionMeaningHint}
        </Typography>
        {validationErrors.expression ? (
          <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
            {validationErrors.expression}
          </Typography>
        ) : null}
      </label>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.timezoneLabel}</Typography>
        <Input
          disabled={disabled}
          value={timezone}
          aria-invalid={Boolean(validationErrors.timezone)}
          onChange={(event) =>
            onChange({
              expression,
              timezone: event.target.value,
            })
          }
          placeholder={messages.timezonePlaceholder}
        />
        {validationErrors.timezone ? (
          <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
            {validationErrors.timezone}
          </Typography>
        ) : null}
      </label>
    </>
  );
}

function expressionFieldCount(expression: string) {
  return expression.trim().split(/\s+/).filter(Boolean).length;
}

function validateTimezone(timezone: string) {
  try {
    new Intl.DateTimeFormat(undefined, { timeZone: timezone });
    return true;
  } catch (error) {
    if (error instanceof RangeError) {
      return false;
    }
    throw error;
  }
}

export const cronTriggerConfigurationDefinition: PluginConfigurationDefinition<CronTriggerValues> =
  {
    pluginType: "core.cron-trigger",
    createDefaultConfiguration: () => ({ expression: "", timezone: "UTC" }),
    deserialize: (configuration) => {
      const expression = configuration.expression;
      const timezone = configuration.timezone;
      if (expression !== undefined && typeof expression !== "string")
        throw new Error(messages.persistedExpressionNotAString);
      if (timezone !== undefined && typeof timezone !== "string")
        throw new Error(messages.persistedTimezoneNotAString);
      return {
        expression: typeof expression === "string" ? expression : "",
        timezone: typeof timezone === "string" && timezone.trim() ? timezone.trim() : "UTC",
      };
    },
    validate: (values) => {
      const expression = typeof values.expression === "string" ? values.expression : "";
      const timezone = typeof values.timezone === "string" ? values.timezone : "";
      const errors: Record<string, string> = {};
      const trimmedExpression = expression.trim();
      if (!trimmedExpression) {
        errors.expression = messages.expressionRequired;
      } else if (expressionFieldCount(trimmedExpression) !== 5) {
        errors.expression = messages.expressionFieldCountInvalid;
      }
      const trimmedTimezone = timezone.trim();
      if (!trimmedTimezone) errors.timezone = messages.timezoneRequired;
      else if (!validateTimezone(trimmedTimezone)) errors.timezone = messages.timezoneInvalid;
      return { valid: Object.keys(errors).length === 0, errors };
    },
    serialize: (values) => ({
      expression: (typeof values.expression === "string" ? values.expression : "").trim(),
      timezone: (typeof values.timezone === "string" ? values.timezone : "").trim() || "UTC",
    }),
    Editor: CronTriggerConfigurationEditor,
  };
