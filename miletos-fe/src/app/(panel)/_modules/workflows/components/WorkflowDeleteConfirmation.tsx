import { useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import { type WorkflowSummary } from "@/app/(panel)/_modules/workflows/types/workflow-types";
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
    <section className={styles.workflowList__deletePanel} aria-label="Delete workflow">
      <Typography as="h2">Delete {workflow.name}</Typography>
      <Typography as="p">
        This permanently removes the workflow. Type the exact workflow name to continue.
      </Typography>

      <label className={styles.workflowList__field}>
        <Typography as="span">Workflow name</Typography>
        <input
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
          Cancel
        </Button>
        <Button
          type="button"
          className={styles.workflowList__dangerButton}
          disabled={!matches || isDeleting}
          onClick={onConfirm}
        >
          {isDeleting ? "Deleting..." : "Delete workflow"}
        </Button>
      </Box>
    </section>
  );
}
