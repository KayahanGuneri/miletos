"use client";

import { useEffect, useMemo } from "react";
import { Typography } from "@/components/lib/typography/Typography";
import { WorkflowCronTriggerCard } from "@/app/(panel)/_modules/workflows/components/WorkflowCronTriggerCard";
import { WorkflowManualRunCard } from "@/app/(panel)/_modules/workflows/components/WorkflowManualRunCard";
import {
  workflowMessages,
  workflowStatusReason,
} from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import { useResetWorkflowTriggerBindings } from "@/app/(panel)/_modules/workflows/query/workflow-queries";
import { type Workflow } from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { supportsExecutionOrigin } from "@/shared/plugins/capabilities/plugin-capabilities";
import styles from "../ui/WorkflowRuntimeActions.module.css";

const messages = workflowMessages.runtime;

interface WorkflowRuntimeActionsProps {
  workflow: Workflow;
  plugins: WorkflowPlugin[];
  canManage: boolean;
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
  const cronRoot = descriptors.find(({ plugin }) => supportsExecutionOrigin(plugin, "CRON"))?.node;
  const manualCompatible =
    descriptors.length > 0 &&
    descriptors.every(({ plugin }) => supportsExecutionOrigin(plugin, "MANUAL_DIRECT"));
  const acceptsInitialVariables =
    manualCompatible && descriptors.some(({ plugin }) => plugin?.acceptsInitialVariables);

  const active = workflow.status === "ACTIVE";
  const statusReason = workflowStatusReason(workflow.status);

  useEffect(() => {
    if (active) {
      return;
    }
    resetTriggerBindings(workflow.id);
  }, [active, resetTriggerBindings, workflow.id]);

  if (!cronRoot && !manualCompatible) return null;

  return (
    <section className={styles.workflowRuntime__panel}>
      <header>
        <Typography as="h2">{messages.title}</Typography>
        <Typography as="p">{messages.description}</Typography>
      </header>

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
  const { workflow } = props;
  return (
    <WorkflowRuntimeActionsState
      key={`${workflow.id}:${workflow.revision}:${workflow.status}`}
      {...props}
    />
  );
}
