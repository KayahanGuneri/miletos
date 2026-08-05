"use client";

import { useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import { ManualWorkflowRunDialog } from "@/app/(panel)/_modules/workflows/components/ManualWorkflowRunDialog";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import styles from "../ui/WorkflowRuntimeActions.module.css";

const messages = workflowMessages.runtime.manual;

interface WorkflowManualRunCardProps {
  workflowId: number;
  acceptsInitialVariables: boolean;
  canManage: boolean;
  active: boolean;
  statusReason?: string;
}

export function WorkflowManualRunCard({
  workflowId,
  acceptsInitialVariables,
  canManage,
  active,
  statusReason,
}: WorkflowManualRunCardProps) {
  const [dialogOpen, setDialogOpen] = useState(false);

  return (
    <Box className={styles.workflowRuntime__card}>
      <Typography as="h3">{messages.title}</Typography>
      <Typography as="p">{messages.description}</Typography>
      <Button type="button" disabled={!canManage || !active} onClick={() => setDialogOpen(true)}>
        {messages.open}
      </Button>
      {statusReason ? (
        <Typography as="p" className={styles.workflowRuntime__muted}>
          {statusReason}
        </Typography>
      ) : null}
      {dialogOpen ? (
        <ManualWorkflowRunDialog
          workflowId={workflowId}
          acceptsInitialVariables={acceptsInitialVariables}
          onClose={() => setDialogOpen(false)}
        />
      ) : null}
    </Box>
  );
}
