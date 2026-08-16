"use client";

import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { DescriptionListItem } from "@/components/lib/description-list-item/DescriptionListItem";
import { Typography } from "@/components/lib/typography/Typography";
import {
  runtimeErrorMessage,
  runtimeStatusLabel,
  workflowMessages,
} from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import {
  useCreateWorkflowCronTriggerMutation,
  useDisableWorkflowCronTriggerMutation,
} from "@/app/(panel)/_modules/workflows/query/workflow-mutations";
import { useWorkflowCronTriggerQuery } from "@/app/(panel)/_modules/workflows/query/workflow-queries";
import styles from "../ui/WorkflowRuntimeActions.module.css";

const messages = workflowMessages.runtime.cron;

interface WorkflowCronTriggerCardProps {
  workflowId: number;
  triggerNodeId: string;
  canManage: boolean;
  active: boolean;
  statusReason?: string;
}

function displayTime(value?: string) {
  if (!value) {
    return workflowMessages.runtime.notYet;
  }
  const timestamp = new Date(value);
  return Number.isNaN(timestamp.getTime()) ? value : timestamp.toLocaleString();
}

export function WorkflowCronTriggerCard({
  workflowId,
  triggerNodeId,
  canManage,
  active,
  statusReason,
}: WorkflowCronTriggerCardProps) {
  const binding = useWorkflowCronTriggerQuery(workflowId, triggerNodeId, active);
  const createTrigger = useCreateWorkflowCronTriggerMutation();
  const disableTrigger = useDisableWorkflowCronTriggerMutation();

  const trigger = active ? (binding.data ?? null) : null;
  const busy =
    createTrigger.isPending || disableTrigger.isPending || binding.isLoading || binding.isFetching;
  const error = runtimeErrorMessage(createTrigger.error ?? disableTrigger.error ?? binding.error);

  async function create() {
    try {
      await createTrigger.mutateAsync({ workflowId, triggerNodeId });
    } catch {}
  }

  async function disable(triggerId: string) {
    try {
      await disableTrigger.mutateAsync({ workflowId, triggerId, triggerNodeId });
    } catch {}
  }

  return (
    <Box className={styles.workflowRuntime__card}>
      <Typography as="h3">{messages.title}</Typography>
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
      {trigger?.status === "ACTIVE" ? (
        <dl className={styles.workflowRuntime__details}>
          <DescriptionListItem term={messages.status}>
            {runtimeStatusLabel(trigger.status)}
          </DescriptionListItem>
          <DescriptionListItem term={messages.expression}>
            {trigger.cronExpression}
          </DescriptionListItem>
          <DescriptionListItem term={messages.timezone}>{trigger.timezone}</DescriptionListItem>
          <DescriptionListItem term={messages.nextFire}>
            {displayTime(trigger.nextFireAt)}
          </DescriptionListItem>
          <DescriptionListItem term={messages.lastScheduled}>
            {displayTime(trigger.lastScheduledAt)}
          </DescriptionListItem>
          <DescriptionListItem term={messages.lastFired}>
            {displayTime(trigger.lastFiredAt)}
          </DescriptionListItem>
        </dl>
      ) : null}
      {error ? (
        <Typography as="p" className={styles.workflowRuntime__error} role="alert">
          {error}
        </Typography>
      ) : null}
    </Box>
  );
}
