"use client";

import { useCallback, useRef, useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { DescriptionListItem } from "@/components/lib/description-list-item/DescriptionListItem";
import { Dialog } from "@/components/lib/dialog/Dialog";
import { Typography } from "@/components/lib/typography/Typography";
import { JsonObjectEditor } from "@/app/(panel)/_modules/workflows/components/JsonObjectEditor";
import {
  runtimeErrorMessage,
  runtimeModeLabel,
  runtimeOriginLabel,
  runtimeStatusLabel,
  workflowMessages,
} from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import { useRunWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/workflow-mutations";
import { type JsonObject } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import styles from "../ui/WorkflowRuntimeActions.module.css";

const messages = workflowMessages.runtime.manual;

interface ManualWorkflowRunDialogProps {
  workflowId: number;
  acceptsInitialVariables: boolean;
  onClose: () => void;
}

export function ManualWorkflowRunDialog({
  workflowId,
  acceptsInitialVariables,
  onClose,
}: ManualWorkflowRunDialogProps) {
  const runMutation = useRunWorkflowMutation();
  const [initialVariables, setInitialVariables] = useState<JsonObject>({});
  const [variablesValid, setVariablesValid] = useState(true);
  const validityChange = useCallback((valid: boolean) => setVariablesValid(valid), []);
  const idempotencyKey = useRef<string | null>(null);

  function changeInitialVariables(value: JsonObject) {
    if (JSON.stringify(value) !== JSON.stringify(initialVariables)) {
      idempotencyKey.current = null;
    }
    setInitialVariables(value);
  }

  async function submit() {
    if (!variablesValid || runMutation.isPending) return;
    idempotencyKey.current ??= crypto.randomUUID();
    try {
      await runMutation.mutateAsync({
        workflowId,
        initialVariables: acceptsInitialVariables ? initialVariables : {},
        idempotencyKey: idempotencyKey.current,
      });
      idempotencyKey.current = null;
    } catch {}
  }

  return (
    <Dialog
      title={messages.dialogTitle}
      description={messages.dialogDescription}
      onClose={() => {
        if (!runMutation.isPending) onClose();
      }}
      footer={
        <>
          <Button
            type="button"
            variant={ButtonVariant.Secondary}
            disabled={runMutation.isPending}
            onClick={onClose}
          >
            {messages.close}
          </Button>
          <Button
            type="button"
            disabled={!variablesValid || runMutation.isPending}
            onClick={() => void submit()}
          >
            {runMutation.isPending ? messages.running : messages.run}
          </Button>
        </>
      }
    >
      {acceptsInitialVariables ? (
        <JsonObjectEditor
          label={messages.initialVariablesLabel}
          value={initialVariables}
          readOnly={runMutation.isPending}
          resetKey={workflowId}
          onChange={changeInitialVariables}
          onValidityChange={validityChange}
        />
      ) : (
        <Typography as="p" className={styles.workflowRuntime__muted}>
          {messages.initialVariablesUnsupported}
        </Typography>
      )}
      {runMutation.error ? (
        <Typography as="p" className={styles.workflowRuntime__error} role="alert">
          {runtimeErrorMessage(runMutation.error)}
        </Typography>
      ) : null}
      {runMutation.data ? (
        <Box className={styles.workflowRuntime__result} role="status">
          <Typography as="strong">{messages.executionStarted}</Typography>
          <dl>
            <DescriptionListItem term={messages.executionId}>
              {runMutation.data.executionId}
            </DescriptionListItem>
            <DescriptionListItem term={messages.status}>
              {runtimeStatusLabel(runMutation.data.status)}
            </DescriptionListItem>
            <DescriptionListItem term={messages.mode}>
              {runtimeModeLabel(runMutation.data.mode)}
            </DescriptionListItem>
            <DescriptionListItem term={messages.origin}>
              {runtimeOriginLabel(runMutation.data.executionOrigin)}
            </DescriptionListItem>
            <DescriptionListItem term={messages.replayed}>
              {runMutation.data.replayed
                ? workflowMessages.runtime.yes
                : workflowMessages.runtime.no}
            </DescriptionListItem>
            <DescriptionListItem term={messages.scheduledRoots}>
              {runMutation.data.scheduledRoots}
            </DescriptionListItem>
          </dl>
        </Box>
      ) : null}
    </Dialog>
  );
}
