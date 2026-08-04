import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import { useActivateWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/useActivateWorkflowMutation";
import { useArchiveWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/useArchiveWorkflowMutation";
import { useRestoreWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/useRestoreWorkflowMutation";
import {
  type Workflow,
  type WorkflowSummary,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";
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
  let label = "Restore";

  switch (workflow.status) {
    case "DRAFT":
      mutation = activate;
      label = "Activate";
      break;
    case "ACTIVE":
      mutation = archive;
      label = "Archive";
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
        {mutation.isPending ? "Updating..." : label}
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
