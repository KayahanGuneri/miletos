import { useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import { type WorkflowSummary } from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import styles from "../ui/WorkflowListPage.module.css";

interface WorkflowDeleteConfirmationProps {
  workflow: WorkflowSummary;
  isDeleting: boolean;
  errorMessage?: string;
  onCancel: () => void;
  onConfirm: () => void;
}

export function WorkflowDeleteConfirmation({
  workflow,
  isDeleting,
  errorMessage,
  onCancel,
  onConfirm,
}: WorkflowDeleteConfirmationProps) {
  const [confirmationName, setConfirmationName] = useState("");
  const matches = confirmationName === workflow.name;

  return (
    <section
      className={styles.workflowList__deletePanel}
      aria-label={workflowMessages.deleteConfirmation.panelLabel}
    >
      <Typography as="h2">{workflowMessages.deleteConfirmation.title(workflow.name)}</Typography>
      <Typography as="p">{workflowMessages.deleteConfirmation.description}</Typography>

      <label className={styles.workflowList__field}>
        <Typography as="span">{workflowMessages.deleteConfirmation.nameLabel}</Typography>
        <Input
          autoFocus
          value={confirmationName}
          onChange={(event) => setConfirmationName(event.target.value)}
        />
      </label>

      {errorMessage ? (
        <Typography as="p" className={styles.workflowList__error} role="alert">
          {errorMessage}
        </Typography>
      ) : null}

      <Box className={styles.workflowList__actions}>
        <Button
          type="button"
          variant={ButtonVariant.Secondary}
          disabled={isDeleting}
          onClick={onCancel}
        >
          {workflowMessages.deleteConfirmation.cancel}
        </Button>
        <Button
          type="button"
          className={styles.workflowList__dangerButton}
          disabled={!matches || isDeleting}
          onClick={onConfirm}
        >
          {isDeleting
            ? workflowMessages.deleteConfirmation.deleting
            : workflowMessages.deleteConfirmation.confirm}
        </Button>
      </Box>
    </section>
  );
}
