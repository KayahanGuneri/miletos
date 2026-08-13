"use client";

import { useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import {
  runtimeErrorMessage,
  workflowMessages,
} from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import {
  useCreateWorkflowHTTPTriggerMutation,
  useDisableWorkflowHTTPTriggerMutation,
} from "@/app/(panel)/_modules/workflows/query/workflow-mutations";
import { useWorkflowHTTPTriggerQuery } from "@/app/(panel)/_modules/workflows/query/workflow-queries";
import styles from "../ui/WorkflowRuntimeActions.module.css";

const messages = workflowMessages.runtime.http;

interface WorkflowHTTPTriggerCardProps {
  workflowId: number;
  triggerNodeId: string;
  canManage: boolean;
  active: boolean;
  statusReason?: string;
  compact?: boolean;
}

export function WorkflowHTTPTriggerCard({
  workflowId,
  triggerNodeId,
  canManage,
  active,
  statusReason,
  compact = false,
}: WorkflowHTTPTriggerCardProps) {
  const binding = useWorkflowHTTPTriggerQuery(workflowId, triggerNodeId, active);
  const createTrigger = useCreateWorkflowHTTPTriggerMutation();
  const disableTrigger = useDisableWorkflowHTTPTriggerMutation();
  const [publicUrl, setPublicUrl] = useState<string | null>(null);

  const trigger = active ? (binding.data ?? null) : null;
  const busy =
    createTrigger.isPending || disableTrigger.isPending || binding.isLoading || binding.isFetching;
  const error = runtimeErrorMessage(createTrigger.error ?? disableTrigger.error ?? binding.error);

  async function create() {
    try {
      const created = await createTrigger.mutateAsync({ workflowId, triggerNodeId });
      setPublicUrl(created.publicUrl);
    } catch {}
  }

  async function disable(triggerId: string) {
    try {
      await disableTrigger.mutateAsync({ workflowId, triggerId, triggerNodeId });
      setPublicUrl(null);
    } catch {}
  }

  return (
    <Box className={compact ? undefined : styles.workflowRuntime__card}>
      {!compact ? <Typography as="h3">{messages.title}</Typography> : null}
      <Typography as="p">{messages.description(triggerNodeId)}</Typography>
      <Box className={styles.workflowRuntime__actions}>
        <Button
          type="button"
          disabled={!canManage || !active || busy || trigger?.status === "ACTIVE"}
          onClick={() => void create()}
        >
          {createTrigger.isPending ? messages.creating : messages.create}
        </Button>
        {trigger?.status === "ACTIVE" ? (
          <Button
            type="button"
            variant={ButtonVariant.Secondary}
            disabled={!canManage || busy}
            onClick={() => void disable(trigger.triggerId)}
          >
            {disableTrigger.isPending ? messages.disabling : messages.disable}
          </Button>
        ) : null}
      </Box>
      {statusReason ? (
        <Typography as="p" className={styles.workflowRuntime__muted}>
          {statusReason}
        </Typography>
      ) : null}
      {trigger?.status === "ACTIVE" && !publicUrl ? (
        <Typography as="p" className={styles.workflowRuntime__muted}>
          {messages.boundNotice}
        </Typography>
      ) : null}
      {publicUrl ? (
        <Box className={styles.workflowRuntime__secret}>
          <Typography as="strong">{messages.secretTitle}</Typography>
          <Typography as="p">{messages.secretDescription}</Typography>
          <Input aria-label={messages.publicUrlLabel} readOnly value={publicUrl} />
        </Box>
      ) : null}
      {error ? (
        <Typography as="p" className={styles.workflowRuntime__error} role="alert">
          {error}
        </Typography>
      ) : null}
    </Box>
  );
}
