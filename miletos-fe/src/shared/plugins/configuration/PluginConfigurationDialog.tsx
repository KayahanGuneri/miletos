"use client";

import { useEffect, useMemo, useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Dialog } from "@/components/lib/dialog/Dialog";
import { Typography } from "@/components/lib/typography/Typography";
import type {
  PluginConfiguration,
  PluginConfigurationEditorContext,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";
import { FormBuilder } from "@/shared/plugins/form/FormBuilder";
import {
  deserializeFormConfiguration,
  serializeFormConfiguration,
  validateFormConfiguration,
} from "@/shared/plugins/form/form-configuration";
import type { FlatFormSchema, FormConfiguration } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";
import { getPluginConfigurationDefinition } from "@/shared/plugins/registry/plugin-configuration-registry";
import styles from "./PluginConfigurationDialog.module.css";

interface PluginConfigurationDialogProps {
  displayName: string;
  pluginType: string;
  pluginVersion: string;
  configuration: PluginConfiguration;
  readOnly: boolean;
  editorContext?: PluginConfigurationEditorContext;
  onValidityChange: (valid: boolean) => void;
  onClose: () => void;
  onSave: (configuration: PluginConfiguration) => void;
  onUploadFile?: (file: File) => Promise<string>;
  uploadPending?: boolean;
  uploadError?: boolean;
}

interface DialogState {
  values?: unknown;
  schema?: FlatFormSchema;
  error?: string;
  embedded?: boolean;
  mode: "legacy" | "schema" | "unsupported";
}

function readEmbeddedFormSchema(configuration: PluginConfiguration): FlatFormSchema | null {
  const schema = configuration.formSchema;
  if (!schema || typeof schema !== "object" || Array.isArray(schema)) {
    return null;
  }
  if (!Array.isArray((schema as { fields?: unknown }).fields)) {
    return null;
  }
  return schema as unknown as FlatFormSchema;
}

function loadErrorMessage(error: unknown) {
  return error instanceof Error
    ? error.message
    : pluginMessages.configurationDialog.loadErrorFallback;
}

function mergeEmbeddedConfiguration(
  configuration: PluginConfiguration,
  schema: FlatFormSchema,
  values: FormConfiguration,
): PluginConfiguration {
  const next: PluginConfiguration = { ...configuration };
  for (const field of schema.fields) {
    delete next[field.key];
  }
  return {
    ...next,
    ...serializeFormConfiguration(schema, values),
  };
}

function loadDialogState(pluginType: string, configuration: PluginConfiguration): DialogState {
  const embeddedSchema = readEmbeddedFormSchema(configuration);
  if (embeddedSchema) {
    try {
      return {
        mode: "schema",
        embedded: true,
        schema: embeddedSchema,
        values: deserializeFormConfiguration(embeddedSchema, configuration),
      };
    } catch (error) {
      return {
        mode: "schema",
        embedded: true,
        schema: embeddedSchema,
        error: loadErrorMessage(error),
      };
    }
  }

  const definition = getPluginConfigurationDefinition(pluginType);
  if (definition?.formSchema) {
    try {
      return {
        mode: "schema",
        schema: definition.formSchema,
        values: definition.deserialize(configuration),
      };
    } catch (error) {
      return {
        mode: "schema",
        schema: definition.formSchema,
        error: loadErrorMessage(error),
      };
    }
  }
  if (definition?.Editor) {
    try {
      return {
        mode: "legacy",
        values: definition.deserialize(configuration),
      };
    } catch (error) {
      return {
        mode: "legacy",
        error: loadErrorMessage(error),
      };
    }
  }
  return { mode: "unsupported" };
}

export function PluginConfigurationDialog({
  displayName,
  pluginType,
  pluginVersion,
  configuration,
  readOnly,
  editorContext,
  onValidityChange,
  onClose,
  onSave,
  onUploadFile,
  uploadPending,
  uploadError,
}: PluginConfigurationDialogProps) {
  const definition = getPluginConfigurationDefinition(pluginType);
  const [dialogState, setDialogState] = useState<DialogState>(() =>
    loadDialogState(pluginType, configuration),
  );
  const [saveAttempted, setSaveAttempted] = useState(false);

  const validation = useMemo(() => {
    if (dialogState.error || dialogState.values === undefined) {
      return { valid: false, errors: {} };
    }
    if (dialogState.mode === "schema" && dialogState.schema) {
      if (dialogState.embedded) {
        return validateFormConfiguration(
          dialogState.schema,
          dialogState.values as FormConfiguration,
        );
      }
      if (definition) {
        return definition.validate(dialogState.values, editorContext);
      }
    }
    if (dialogState.mode === "legacy" && definition) {
      return definition.validate(dialogState.values, editorContext);
    }
    return { valid: false, errors: {} };
  }, [definition, dialogState, editorContext]);

  useEffect(() => {
    onValidityChange(validation.valid && !dialogState.error);
  }, [dialogState.error, onValidityChange, validation.valid]);

  const Editor = definition?.Editor;

  function resetToDefaultConfiguration() {
    if (!definition) {
      return;
    }
    setDialogState(loadDialogState(pluginType, definition.createDefaultConfiguration()));
    setSaveAttempted(false);
  }

  function saveConfiguration() {
    setSaveAttempted(true);
    if (dialogState.values === undefined || !validation.valid) {
      return;
    }
    const serialized =
      dialogState.mode === "schema" && dialogState.schema && dialogState.embedded
        ? mergeEmbeddedConfiguration(
            configuration,
            dialogState.schema,
            dialogState.values as FormConfiguration,
          )
        : definition?.serialize(dialogState.values);
    if (!serialized) {
      return;
    }
    if (JSON.stringify(serialized) === JSON.stringify(configuration)) {
      onClose();
      return;
    }
    onSave(serialized);
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
            {readOnly
              ? pluginMessages.configurationDialog.close
              : pluginMessages.configurationDialog.cancel}
          </Button>
          {!readOnly && dialogState.mode !== "unsupported" && !dialogState.error ? (
            <Button
              type="button"
              disabled={dialogState.values === undefined}
              onClick={saveConfiguration}
            >
              {pluginMessages.configurationDialog.save}
            </Button>
          ) : null}
        </>
      }
    >
      <Box className={styles.pluginConfiguration__identity}>
        <Typography as="span">{pluginMessages.configurationDialog.identityLabel}</Typography>
        <Typography as="strong">
          {pluginType} {"\u00B7"} {pluginVersion}
        </Typography>
      </Box>

      {dialogState.mode === "unsupported" ? (
        <Typography as="p" className={styles.pluginConfiguration__notice}>
          {pluginMessages.configurationDialog.unsupportedNotice}
        </Typography>
      ) : null}

      {dialogState.error ? (
        <Box className={styles.pluginConfiguration__errorPanel} role="alert">
          <Typography as="strong">{pluginMessages.configurationDialog.loadErrorTitle}</Typography>
          <Typography as="span">{dialogState.error}</Typography>
          {!readOnly && definition ? (
            <Button
              type="button"
              variant={ButtonVariant.Secondary}
              onClick={resetToDefaultConfiguration}
            >
              {pluginMessages.configurationDialog.resetToBlank}
            </Button>
          ) : null}
        </Box>
      ) : null}

      {dialogState.mode === "schema" &&
      dialogState.schema &&
      dialogState.values !== undefined &&
      !dialogState.error ? (
        <>
          <FormBuilder
            schema={dialogState.schema}
            values={dialogState.values as FormConfiguration}
            disabled={readOnly}
            validationErrors={saveAttempted ? validation.errors : {}}
            onChange={(values) => {
              setSaveAttempted(false);
              setDialogState((current) => ({ ...current, values, error: undefined }));
            }}
            uploadPending={uploadPending}
            uploadError={uploadError}
            onUploadFile={
              dialogState.schema.fields.some(
                (field) =>
                  field.dataType === "STRING" &&
                  field.renderType === "INPUT" &&
                  field.inputType === "file",
              )
                ? onUploadFile
                : undefined
            }
          />
          {!dialogState.embedded && definition && Editor ? (
            <Editor
              initialValues={dialogState.values}
              disabled={readOnly}
              validationErrors={saveAttempted ? validation.errors : {}}
              editorContext={editorContext}
              onChange={(values) => {
                setSaveAttempted(false);
                setDialogState((current) => ({ ...current, values, error: undefined }));
              }}
            />
          ) : null}
        </>
      ) : null}

      {dialogState.mode === "legacy" &&
      definition &&
      Editor &&
      dialogState.values !== undefined &&
      !dialogState.error ? (
        <Editor
          initialValues={dialogState.values}
          disabled={readOnly}
          validationErrors={saveAttempted ? validation.errors : {}}
          editorContext={editorContext}
          onChange={(values) => {
            setSaveAttempted(false);
            setDialogState({ mode: "legacy", values });
          }}
          onUploadFile={onUploadFile}
          uploadPending={uploadPending}
          uploadError={uploadError}
        />
      ) : null}
    </Dialog>
  );
}
