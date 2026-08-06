import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import {
  useActivateWorkflowMutation,
  useArchiveWorkflowMutation,
  useRestoreWorkflowMutation,
} from "@/app/(panel)/_modules/workflows/query/workflow-mutations";
import {
  type Workflow,
  type WorkflowSummary,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import styles from "../ui/WorkflowListPage.module.css";

interface WorkflowLifecycleActionsProps {
  workflow: WorkflowSummary;
  onChanged?: (workflow: Workflow) => void;
  disabled?: boolean;
  disabledReason?: string;
}

export function WorkflowLifecycleActions({
  workflow,
  onChanged,
  disabled = false,
  disabledReason,
}: WorkflowLifecycleActionsProps) {
  const activate = useActivateWorkflowMutation();
  const archive = useArchiveWorkflowMutation();
  const restore = useRestoreWorkflowMutation();
  let mutation = restore;
  let label: string = workflowMessages.lifecycle.restore;

  switch (workflow.status) {
    case "DRAFT":
      mutation = activate;
      label = workflowMessages.lifecycle.activate;
      break;
    case "ACTIVE":
      mutation = archive;
      label = workflowMessages.lifecycle.archive;
      break;
    case "ARCHIVED":
      break;
  }

  return (
    <Box className={styles.workflowList__lifecycle}>
      <Button
        type="button"
        variant={ButtonVariant.Secondary}
        disabled={disabled || mutation.isPending}
        onClick={async () => {
          try {
            const changed = await mutation.mutateAsync(workflow.id);
            onChanged?.(changed);
          } catch {
            // The normalized mutation error is rendered below.
          }
        }}
      >
        {mutation.isPending ? workflowMessages.lifecycle.updating : label}
      </Button>
      {disabled && disabledReason ? (
        <Typography as="span" className={styles.workflowList__inlineError} role="status">
          {disabledReason}
        </Typography>
      ) : null}
      {mutation.error ? (
        <Typography as="span" className={styles.workflowList__inlineError} role="alert">
          {mutation.error.message}
        </Typography>
      ) : null}
    </Box>
  );
}
