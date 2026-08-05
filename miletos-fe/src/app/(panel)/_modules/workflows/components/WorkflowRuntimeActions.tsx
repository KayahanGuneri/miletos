"use client";

import { useEffect, useMemo } from "react";
import { Typography } from "@/components/lib/typography/Typography";
import { WorkflowCronTriggerCard } from "@/app/(panel)/_modules/workflows/components/WorkflowCronTriggerCard";
import { WorkflowHTTPTriggerCard } from "@/app/(panel)/_modules/workflows/components/WorkflowHTTPTriggerCard";
import { WorkflowManualRunCard } from "@/app/(panel)/_modules/workflows/components/WorkflowManualRunCard";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import { useResetWorkflowTriggerBindings } from "@/app/(panel)/_modules/workflows/query/workflow-queries";
import { type Workflow } from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import styles from "../ui/WorkflowRuntimeActions.module.css";

const messages = workflowMessages.runtime;

interface WorkflowRuntimeActionsProps {
  workflow: Workflow;
  plugins: WorkflowPlugin[];
  canManage: boolean;
}

function inactiveStatusReason(status: Workflow["status"]) {
  if (status === "ACTIVE") {
    return undefined;
  }
  if (status === "DRAFT") {
    return messages.draftStatusReason;
  }
  return messages.archivedStatusReason;
}

function WorkflowRuntimeActionsState({
  workflow,
  plugins,
  canManage,
}: WorkflowRuntimeActionsProps) {
  const resetTriggerBindings = useResetWorkflowTriggerBindings();

  const roots = useMemo(() => {
    const targeted = new Set(workflow.edges.map((edge) => edge.targetNodeId));
    return workflow.nodes.filter((node) => !targeted.has(node.nodeId));
  }, [workflow.edges, workflow.nodes]);
  const descriptors = roots.map((node) => ({
    node,
    plugin: plugins.find(
      (plugin) => plugin.type === node.pluginType && plugin.version === node.pluginVersion,
    ),
  }));
  const onlyRoot = descriptors.length === 1 ? descriptors[0] : undefined;
  const httpRoot =
    onlyRoot?.plugin?.allowedRootOrigins.includes("HTTP_WEBHOOK") &&
    onlyRoot.plugin.contextProvider === "http-trigger"
      ? onlyRoot.node
      : undefined;
  const cronRoot =
    onlyRoot?.plugin?.allowedRootOrigins.includes("CRON") &&
    onlyRoot.plugin.contextProvider === "cron-trigger"
      ? onlyRoot.node
      : undefined;
  // The runtime rejects a manual execution unless every root is a known plugin
  // that declares MANUAL_DIRECT, and it only accepts initial variables when at
  // least one of those roots can receive entry input.
  const manualCompatible =
    descriptors.length > 0 &&
    descriptors.every(({ plugin }) =>
      Boolean(plugin?.allowedRootOrigins.includes("MANUAL_DIRECT")),
    );
  const acceptsInitialVariables =
    manualCompatible && descriptors.some(({ plugin }) => plugin?.acceptsInitialVariables);

  const active = workflow.status === "ACTIVE";
  const statusReason = inactiveStatusReason(workflow.status);

  useEffect(() => {
    if (active) {
      return;
    }
    resetTriggerBindings(workflow.id);
  }, [active, resetTriggerBindings, workflow.id]);

  if (!httpRoot && !cronRoot && !manualCompatible) return null;

  return (
    <section className={styles.workflowRuntime__panel}>
      <header>
        <Typography as="h2">{messages.title}</Typography>
        <Typography as="p">{messages.description}</Typography>
      </header>

      {httpRoot ? (
        <WorkflowHTTPTriggerCard
          workflowId={workflow.id}
          triggerNodeId={httpRoot.nodeId}
          canManage={canManage}
          active={active}
          statusReason={statusReason}
        />
      ) : null}

      {cronRoot ? (
        <WorkflowCronTriggerCard
          workflowId={workflow.id}
          triggerNodeId={cronRoot.nodeId}
          canManage={canManage}
          active={active}
          statusReason={statusReason}
        />
      ) : null}

      {manualCompatible ? (
        <WorkflowManualRunCard
          workflowId={workflow.id}
          acceptsInitialVariables={acceptsInitialVariables}
          canManage={canManage}
          active={active}
          statusReason={statusReason}
        />
      ) : null}
    </section>
  );
}

export function WorkflowRuntimeActions(props: WorkflowRuntimeActionsProps) {
  // Runtime state belongs to one persisted revision. Remounting on a changed
  // workflow, revision or status clears the trigger bindings, the one-time
  // public URL, the success and error state and the open manual run dialog
  // together with its idempotency key.
  const { workflow } = props;
  return (
    <WorkflowRuntimeActionsState
      key={`${workflow.id}:${workflow.revision}:${workflow.status}`}
      {...props}
    />
  );
}
