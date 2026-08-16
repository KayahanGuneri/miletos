"use client";

import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfiguration,
  PluginConfigurationDefinition,
  PluginConfigurationEditorContext,
  PluginConfigurationEditorProps,
  PluginConfigurationValidationResult,
  PluginWorkflowOption,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";
import styles from "../../configuration/PluginConfigurationDialog.module.css";

const messages = pluginMessages.subflow;

interface SubflowValues {
  workflowId: string;
}

function SubflowConfigurationEditor({
  initialValues,
  disabled,
  validationErrors,
  onChange,
  editorContext,
}: PluginConfigurationEditorProps<SubflowValues>) {
  const options = selectableWorkflows(editorContext);

  return (
    <section className={styles.pluginConfiguration__formSection}>
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{messages.workflowLabel}</Typography>
        <select
          disabled={disabled || options.length === 0}
          value={initialValues.workflowId}
          aria-invalid={Boolean(validationErrors.workflowId)}
          onChange={(event) => onChange({ workflowId: event.target.value })}
        >
          <option value="">{messages.selectWorkflowPlaceholder}</option>
          {options.map((workflow) => (
            <option key={workflow.id} value={String(workflow.id)}>
              {workflow.name}
            </option>
          ))}
        </select>
        {editorContext?.workflows === undefined ? (
          <Typography as="small">{messages.loadingWorkflows}</Typography>
        ) : null}
        {validationErrors.workflowId ? (
          <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
            {validationErrors.workflowId}
          </Typography>
        ) : null}
      </label>
    </section>
  );
}

function selectableWorkflows(editorContext?: PluginConfigurationEditorContext) {
  const workflows = editorContext?.workflows ?? [];
  return workflows.filter((workflow) => {
    if (workflow.status !== "ACTIVE") {
      return false;
    }
    if (editorContext?.workflowId !== undefined && workflow.id === editorContext.workflowId) {
      return false;
    }
    return true;
  });
}

function validateSubflowValues(
  values: SubflowValues,
  editorContext?: PluginConfigurationEditorContext,
): PluginConfigurationValidationResult {
  const errors: Record<string, string> = {};
  const workflowId = values.workflowId.trim();
  if (!workflowId) {
    errors.workflowId = messages.workflowRequired;
    return { valid: false, errors };
  }
  if (editorContext?.workflowId !== undefined && workflowId === String(editorContext.workflowId)) {
    errors.workflowId = messages.workflowSelfReference;
  }
  if (editorContext?.workflows) {
    const selected = editorContext.workflows.find(
      (workflow: PluginWorkflowOption) => String(workflow.id) === workflowId,
    );
    if (!selected || selected.status !== "ACTIVE") {
      errors.workflowId = messages.workflowMissing;
    }
  }
  return { valid: Object.keys(errors).length === 0, errors };
}

export const subflowConfigurationDefinition: PluginConfigurationDefinition<SubflowValues> = {
  pluginType: "core.subflow",
  createDefaultConfiguration: () => ({}),
  deserialize: (configuration) => {
    const workflowId = configuration.workflowId;
    if (
      workflowId !== undefined &&
      typeof workflowId !== "string" &&
      typeof workflowId !== "number"
    ) {
      throw new Error(messages.persistedWorkflowInvalid);
    }
    return {
      workflowId:
        typeof workflowId === "number"
          ? String(workflowId)
          : typeof workflowId === "string"
            ? workflowId.trim()
            : "",
    };
  },
  validate: validateSubflowValues,
  serialize: (values): PluginConfiguration => {
    const workflowId = values.workflowId.trim();

    if (!workflowId) {
      return {};
    }

    return { workflowId };
  },
  Editor: SubflowConfigurationEditor,
};
