"use client";

import { useEffect, useMemo, useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Dialog } from "@/components/lib/dialog/Dialog";
import { Typography } from "@/components/lib/typography/Typography";
import type { PluginConfiguration } from "@/shared/plugins/contracts/plugin-configuration";
import { getPluginConfigurationDefinition } from "@/shared/plugins/registry/plugin-configuration-registry";
import styles from "./PluginConfigurationDialog.module.css";

interface PluginConfigurationDialogProps {
  displayName: string;
  pluginType: string;
  pluginVersion: string;
  configuration: PluginConfiguration;
  readOnly: boolean;
  onValidityChange: (valid: boolean) => void;
  onClose: () => void;
  onSave: (configuration: PluginConfiguration) => void;
}

interface InitialDialogState {
  values?: unknown;
  error?: string;
}

function loadValues(pluginType: string, configuration: PluginConfiguration): InitialDialogState {
  const definition = getPluginConfigurationDefinition(pluginType);
  if (!definition) {
    return {};
  }

  try {
    return { values: definition.deserialize(configuration) };
  } catch (error) {
    return {
      error:
        error instanceof Error
          ? error.message
          : "The persisted plugin configuration could not be loaded.",
    };
  }
}

export function PluginConfigurationDialog({
  displayName,
  pluginType,
  pluginVersion,
  configuration,
  readOnly,
  onValidityChange,
  onClose,
  onSave,
}: PluginConfigurationDialogProps) {
  const definition = getPluginConfigurationDefinition(pluginType);
  const [dialogState, setDialogState] = useState<InitialDialogState>(() =>
    loadValues(pluginType, configuration),
  );
  const validation = useMemo(() => {
    if (!definition || dialogState.values === undefined || dialogState.error) {
      return { valid: !definition, errors: {} };
    }
    return definition.validate(dialogState.values);
  }, [definition, dialogState.error, dialogState.values]);

  useEffect(() => {
    onValidityChange(validation.valid && !dialogState.error);
  }, [dialogState.error, onValidityChange, validation.valid]);

  const Editor = definition?.Editor;

  function resetToDefaultConfiguration() {
    if (!definition) {
      return;
    }
    setDialogState(loadValues(pluginType, definition.createDefaultConfiguration()));
  }

  return (
    <Dialog
      title={displayName}
      description={definition?.dialogDescription ?? `${pluginType} / ${pluginVersion}`}
      size={definition?.dialogSize}
      onClose={onClose}
      footer={
        <>
          <Button type="button" variant={ButtonVariant.Secondary} onClick={onClose}>
            {readOnly ? "Close" : "Cancel"}
          </Button>
          {!readOnly && definition && !dialogState.error ? (
            <Button
              type="button"
              disabled={!validation.valid || dialogState.values === undefined}
              onClick={() => {
                if (dialogState.values === undefined || !validation.valid) {
                  return;
                }
                const serialized = definition.serialize(dialogState.values);
                if (JSON.stringify(serialized) === JSON.stringify(configuration)) {
                  onClose();
                  return;
                }
                onSave(serialized);
              }}
            >
              Save configuration
            </Button>
          ) : null}
        </>
      }
    >
      <Box className={styles.pluginConfiguration__identity}>
        <Typography as="span">Plugin identity</Typography>
        <Typography as="strong">
          {pluginType} Â· {pluginVersion}
        </Typography>
      </Box>

      {!definition ? (
        <Typography as="p" className={styles.pluginConfiguration__notice}>
          No configuration editor is registered for this plugin. The existing configuration will be
          preserved.
        </Typography>
      ) : null}

      {dialogState.error ? (
        <Box className={styles.pluginConfiguration__errorPanel} role="alert">
          <Typography as="strong">Configuration could not be loaded.</Typography>
          <Typography as="span">{dialogState.error}</Typography>
          {!readOnly ? (
            <Button
              type="button"
              variant={ButtonVariant.Secondary}
              onClick={resetToDefaultConfiguration}
            >
              Start with blank configuration
            </Button>
          ) : null}
        </Box>
      ) : null}

      {definition && Editor && dialogState.values !== undefined && !dialogState.error ? (
        <Editor
          initialValues={dialogState.values}
          disabled={readOnly}
          validationErrors={validation.errors}
          onChange={(values) => setDialogState({ values })}
        />
      ) : null}
    </Dialog>
  );
}
