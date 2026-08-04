"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { XYPosition } from "@xyflow/react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import { JsonObjectEditor } from "@/app/(panel)/_modules/workflows/components/JsonObjectEditor";
import { NodePropertiesPanel } from "@/app/(panel)/_modules/workflows/components/NodePropertiesPanel";
import { PluginPalette } from "@/app/(panel)/_modules/workflows/components/PluginPalette";
import { WorkflowCanvas } from "@/app/(panel)/_modules/workflows/components/WorkflowCanvas";
import { WorkflowDeleteConfirmation } from "@/app/(panel)/_modules/workflows/components/WorkflowDeleteConfirmation";
import { WorkflowLifecycleActions } from "@/app/(panel)/_modules/workflows/components/WorkflowLifecycleActions";
import { useCreateWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/useCreateWorkflowMutation";
import { useDeleteWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/useDeleteWorkflowMutation";
import { useUpdateWorkflowMutation } from "@/app/(panel)/_modules/workflows/query/useUpdateWorkflowMutation";
import { useWorkflowPluginsQuery } from "@/app/(panel)/_modules/workflows/query/useWorkflowPluginsQuery";
import { useWorkflowQuery } from "@/app/(panel)/_modules/workflows/query/useWorkflowQuery";
import {
  type JsonObject,
  type SaveWorkflowRequest,
  type Workflow,
  type WorkflowEdge,
  type WorkflowNode,
  type WorkflowPlugin,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";
import {
  createClientId,
  formatWorkflowStatus,
} from "@/app/(panel)/_modules/workflows/utils/workflow-utils";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { canManageWorkflows } from "@/shared/session/permissions/session-permissions";
import { PluginConfigurationDialog } from "@/shared/plugins/configuration/PluginConfigurationDialog";
import {
  createDefaultPluginConfiguration,
  isPluginConfigurationValid,
} from "@/shared/plugins/registry/plugin-configuration-registry";
import styles from "./ui/WorkflowEditorPage.module.css";

interface WorkflowEditorPageProps {
  workflowId?: number;
}

const INVALID_CONFIGURATION_MESSAGE =
  "Correct the selected node configuration or delete the node to discard it.";

function toEditableRequest(workflow: Workflow): SaveWorkflowRequest {
  return {
    name: workflow.name.trim(),
    description: workflow.description?.trim() || null,
    nodes: workflow.nodes,
    edges: workflow.edges,
    metadata: workflow.metadata,
  };
}

function editableRequestsMatch(current: SaveWorkflowRequest, persisted: SaveWorkflowRequest) {
  return JSON.stringify(current) === JSON.stringify(persisted);
}

export function WorkflowEditorPage({ workflowId }: WorkflowEditorPageProps) {
  const router = useRouter();
  const currentUser = useCurrentUserQuery();
  const isAllowed = canManageWorkflows(currentUser.data);
  const isNew = workflowId === undefined;
  const hasValidWorkflowId = isNew || (Number.isSafeInteger(workflowId) && (workflowId ?? 0) > 0);
  const detail = useWorkflowQuery(workflowId ?? 0, isAllowed && !isNew && hasValidWorkflowId);
  const plugins = useWorkflowPluginsQuery(isAllowed);
  const createMutation = useCreateWorkflowMutation();
  const updateMutation = useUpdateWorkflowMutation();
  const deleteMutation = useDeleteWorkflowMutation();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [nodes, setNodes] = useState<WorkflowNode[]>([]);
  const [edges, setEdges] = useState<WorkflowEdge[]>([]);
  const [metadata, setMetadata] = useState<JsonObject>({});
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [metadataValid, setMetadataValid] = useState(true);
  const [configurationValid, setConfigurationValid] = useState(true);
  const [configurationNodeId, setConfigurationNodeId] = useState<string | null>(null);
  const [localError, setLocalError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [showDelete, setShowDelete] = useState(false);
  const [persistedBaseline, setPersistedBaseline] = useState<SaveWorkflowRequest | null>(null);
  const [editorResetVersion, setEditorResetVersion] = useState(0);
  const [initializedWorkflowId, setInitializedWorkflowId] = useState<number | null>(null);

  const loadServerWorkflow = useCallback((workflow: Workflow) => {
    const editable = toEditableRequest(workflow);
    setName(editable.name);
    setDescription(editable.description ?? "");
    setNodes(editable.nodes);
    setEdges(editable.edges);
    setMetadata(editable.metadata);
    setPersistedBaseline(editable);
    setInitializedWorkflowId(workflow.id);
    setSelectedNodeId(null);
    setConfigurationNodeId(null);
    setMetadataValid(true);
    setConfigurationValid(true);
    setLocalError(null);
    setEditorResetVersion((current) => current + 1);
  }, []);

  useEffect(() => {
    if (!detail.data || initializedWorkflowId === detail.data.id) {
      return;
    }

    // Server detail intentionally initializes the local editor exactly once per workflow ID.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    loadServerWorkflow(detail.data);
  }, [detail.data, initializedWorkflowId, loadServerWorkflow]);

  const selectedNode = useMemo(
    () => nodes.find((node) => node.nodeId === selectedNodeId) ?? null,
    [nodes, selectedNodeId],
  );
  const selectedPlugin = useMemo(
    () =>
      selectedNode
        ? plugins.data?.items.find(
            (plugin) =>
              plugin.type === selectedNode.pluginType &&
              plugin.version === selectedNode.pluginVersion,
          )
        : undefined,
    [plugins.data?.items, selectedNode],
  );
  const configurationNode = useMemo(
    () => nodes.find((node) => node.nodeId === configurationNodeId) ?? null,
    [configurationNodeId, nodes],
  );
  const configurationPlugin = useMemo(
    () =>
      configurationNode
        ? plugins.data?.items.find(
            (plugin) =>
              plugin.type === configurationNode.pluginType &&
              plugin.version === configurationNode.pluginVersion,
          )
        : undefined,
    [configurationNode, plugins.data?.items],
  );
  const hasInvalidStoredConfiguration = useMemo(
    () => nodes.some((node) => !isPluginConfigurationValid(node.pluginType, node.configuration)),
    [nodes],
  );
  const readOnly = !isNew && detail.data?.status !== "DRAFT";
  const saving = createMutation.isPending || updateMutation.isPending;
  const mutationError = createMutation.error?.message ?? updateMutation.error?.message;
  const editableRequest = useMemo<SaveWorkflowRequest>(
    () => ({
      name: name.trim(),
      description: description.trim() || null,
      nodes,
      edges,
      metadata,
    }),
    [description, edges, metadata, name, nodes],
  );
  const isDirty =
    isNew ||
    (persistedBaseline !== null && !editableRequestsMatch(editableRequest, persistedBaseline));
  const metadataValidityChange = useCallback((valid: boolean) => setMetadataValid(valid), []);
  const configurationValidityChange = useCallback((valid: boolean) => {
    setConfigurationValid(valid);
    if (valid) {
      setLocalError((current) => (current === INVALID_CONFIGURATION_MESSAGE ? null : current));
    }
  }, []);

  const lifecyclePending = saving || deleteMutation.isPending;
  let lifecycleDisabled = lifecyclePending;
  let lifecycleDisabledReason = lifecyclePending
    ? "Wait for the current workflow operation to finish."
    : undefined;

  if (detail.data?.status === "DRAFT") {
    if (isDirty) {
      lifecycleDisabled = true;
      lifecycleDisabledReason = "Save the current draft before activating it.";
    } else if (!metadataValid || !configurationValid || hasInvalidStoredConfiguration) {
      lifecycleDisabled = true;
      lifecycleDisabledReason =
        "Resolve metadata and node configuration errors before activating it.";
    }
  }

  function dropPlugin(plugin: WorkflowPlugin, position: XYPosition) {
    if (!configurationValid) {
      setLocalError(INVALID_CONFIGURATION_MESSAGE);
      return;
    }

    const nodeId = createClientId("node");
    const configuration = createDefaultPluginConfiguration(plugin.type);
    setNodes((current) => [
      ...current,
      {
        nodeId,
        pluginType: plugin.type,
        pluginVersion: plugin.version,
        configuration,
        position,
      },
    ]);
    setSelectedNodeId(nodeId);
    setConfigurationNodeId(nodeId);
    setConfigurationValid(isPluginConfigurationValid(plugin.type, configuration));
  }

  function deleteNodes(nodeIds: string[]) {
    const ids = new Set(nodeIds);
    setNodes((current) => current.filter((node) => !ids.has(node.nodeId)));
    setEdges((current) =>
      current.filter((edge) => !ids.has(edge.sourceNodeId) && !ids.has(edge.targetNodeId)),
    );
    if (selectedNodeId && ids.has(selectedNodeId)) {
      setSelectedNodeId(null);
      setConfigurationValid(true);
      setLocalError((current) => (current === INVALID_CONFIGURATION_MESSAGE ? null : current));
    }
    if (configurationNodeId && ids.has(configurationNodeId)) {
      setConfigurationNodeId(null);
      setConfigurationValid(true);
    }
  }

  function selectNode(nodeId: string | null) {
    if (nodeId === selectedNodeId) {
      return;
    }

    if (!configurationValid) {
      setLocalError(INVALID_CONFIGURATION_MESSAGE);
      return;
    }

    setSelectedNodeId(nodeId);
    setConfigurationValid(true);
  }

  async function saveWorkflow() {
    const normalizedName = name.trim();
    if (normalizedName.length < 3 || normalizedName.length > 120) {
      setLocalError("Workflow name must contain between 3 and 120 characters.");
      return;
    }
    if (description.trim().length > 1000) {
      setLocalError("Description must not exceed 1000 characters.");
      return;
    }
    if (!metadataValid || !configurationValid) {
      setLocalError("Resolve JSON editor errors before saving.");
      return;
    }
    setLocalError(null);
    setSuccessMessage(null);
    const request: SaveWorkflowRequest = {
      ...editableRequest,
      name: normalizedName,
    };
    try {
      if (isNew) {
        const created = await createMutation.mutateAsync(request);
        router.replace(`/workflows/${created.id}`);
      } else if (workflowId) {
        const updated = await updateMutation.mutateAsync({ workflowId, request });
        loadServerWorkflow(updated);
        setSuccessMessage("Workflow draft saved.");
      }
    } catch {
      // The normalized mutation error is rendered below.
    }
  }

  if (!isAllowed) {
    return (
      <PageShell
        eyebrow="Workflow control plane"
        title="Workflow access unavailable"
        description="Only active company administrators can manage workflows."
      >
        <Box className={styles.workflowEditor__state} role="alert">
          <Typography as="strong">You do not have permission to access this page.</Typography>
        </Box>
      </PageShell>
    );
  }

  if (!hasValidWorkflowId) {
    return (
      <PageShell
        eyebrow="Workflow control plane"
        title="Workflow unavailable"
        description="The workflow identifier is invalid."
      >
        <Box className={styles.workflowEditor__state} role="alert">
          <Typography as="strong">Open a workflow from the company directory.</Typography>
        </Box>
      </PageShell>
    );
  }

  if (!isNew && detail.isPending) {
    return (
      <PageShell
        eyebrow="Workflow control plane"
        title="Loading workflow"
        description="Reading the company-owned workflow definition."
      >
        <Box className={styles.workflowEditor__state}>
          <Typography as="strong">Loading workflow...</Typography>
        </Box>
      </PageShell>
    );
  }

  if (!isNew && detail.isError) {
    return (
      <PageShell
        eyebrow="Workflow control plane"
        title="Workflow unavailable"
        description="The workflow could not be loaded."
      >
        <Box className={styles.workflowEditor__state} role="alert">
          <Typography as="strong">{detail.error.message}</Typography>
        </Box>
      </PageShell>
    );
  }

  if (!isNew && (persistedBaseline === null || initializedWorkflowId !== detail.data?.id)) {
    return (
      <PageShell
        eyebrow="Workflow control plane"
        title="Loading workflow"
        description="Preparing the editable workflow state."
      >
        <Box className={styles.workflowEditor__state}>
          <Typography as="strong">Preparing workflow editor...</Typography>
        </Box>
      </PageShell>
    );
  }

  return (
    <PageShell
      eyebrow="Workflow control plane"
      title={isNew ? "Create workflow" : name || "Workflow editor"}
      description={
        isNew
          ? "Build a local draft and save it when ready."
          : `Revision ${detail.data?.revision ?? "—"} · ${formatWorkflowStatus(detail.data?.status ?? "DRAFT")}`
      }
      actions={
        <>
          <Link className={styles.workflowEditor__backLink} href="/workflows">
            Back to workflows
          </Link>
          {detail.data ? (
            <WorkflowLifecycleActions
              workflow={detail.data}
              disabled={lifecycleDisabled}
              disabledReason={lifecycleDisabledReason}
              onChanged={(workflow) => {
                loadServerWorkflow(workflow);
                setSuccessMessage(
                  `Workflow is now ${formatWorkflowStatus(workflow.status).toLowerCase()}.`,
                );
              }}
            />
          ) : null}
        </>
      }
    >
      <section className={styles.workflowEditor__metadataPanel}>
        <Box className={styles.workflowEditor__formGrid}>
          <label className={styles.workflowEditor__field}>
            <Typography as="span">Name</Typography>
            <input
              maxLength={120}
              readOnly={readOnly}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          <label className={styles.workflowEditor__field}>
            <Typography as="span">Description</Typography>
            <textarea
              maxLength={1000}
              rows={3}
              readOnly={readOnly}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </label>
        </Box>
        {readOnly ? (
          <Typography as="p" className={styles.workflowEditor__readOnlyNotice}>
            Active and archived workflows are read-only. Restore an archived workflow to edit it.
          </Typography>
        ) : null}
        {localError || mutationError ? (
          <Typography as="p" className={styles.workflowEditor__fieldError} role="alert">
            {localError ?? mutationError}
          </Typography>
        ) : null}
        {successMessage ? (
          <Typography as="p" className={styles.workflowEditor__success} role="status">
            {successMessage}
          </Typography>
        ) : null}
      </section>

      <Box className={styles.workflowEditor__workspace}>
        <PluginPalette
          plugins={plugins.data?.items ?? []}
          isLoading={plugins.isPending}
          errorMessage={plugins.error?.message}
          readOnly={readOnly || !configurationValid}
        />
        <WorkflowCanvas
          workflowNodes={nodes}
          workflowEdges={edges}
          plugins={plugins.data?.items ?? []}
          readOnly={readOnly}
          selectionLocked={!configurationValid}
          selectedNodeId={selectedNodeId}
          onSelectNode={selectNode}
          onMoveNode={(nodeId, x, y) =>
            setNodes((current) =>
              current.map((node) =>
                node.nodeId === nodeId ? { ...node, position: { x, y } } : node,
              ),
            )
          }
          onAddEdge={(edge) => setEdges((current) => [...current, edge])}
          onDropPlugin={dropPlugin}
          onDeleteNodes={deleteNodes}
          onDeleteEdges={(edgeIds) => {
            const ids = new Set(edgeIds);
            setEdges((current) => current.filter((edge) => !ids.has(edge.edgeId)));
          }}
        />
        <NodePropertiesPanel
          node={selectedNode}
          plugin={selectedPlugin}
          readOnly={readOnly}
          onConfigure={() => {
            if (!selectedNode) {
              return;
            }
            setConfigurationNodeId(selectedNode.nodeId);
            setConfigurationValid(
              isPluginConfigurationValid(selectedNode.pluginType, selectedNode.configuration),
            );
          }}
          onDelete={() => {
            if (selectedNodeId) deleteNodes([selectedNodeId]);
          }}
        />
      </Box>

      <section className={styles.workflowEditor__advancedPanel}>
        <Typography as="h2">Advanced metadata</Typography>
        <JsonObjectEditor
          label="Workflow metadata JSON"
          value={metadata}
          readOnly={readOnly}
          resetKey={editorResetVersion}
          onChange={setMetadata}
          onValidityChange={metadataValidityChange}
        />
      </section>

      <footer className={styles.workflowEditor__footer}>
        <Button
          type="button"
          disabled={readOnly || saving || !metadataValid || !configurationValid}
          onClick={() => void saveWorkflow()}
        >
          {saving ? "Saving..." : "Save draft"}
        </Button>
        {!isNew && detail.data && detail.data.status !== "ACTIVE" ? (
          <Button
            type="button"
            variant={ButtonVariant.Secondary}
            onClick={() => {
              setShowDelete(true);
              deleteMutation.reset();
            }}
          >
            Delete workflow
          </Button>
        ) : null}
      </footer>

      {showDelete && detail.data ? (
        <WorkflowDeleteConfirmation
          workflow={detail.data}
          isDeleting={deleteMutation.isPending}
          errorMessage={deleteMutation.error?.message}
          onCancel={() => setShowDelete(false)}
          onConfirm={async () => {
            try {
              await deleteMutation.mutateAsync(detail.data.id);
              router.replace("/workflows");
            } catch {
              // The confirmation renders the normalized mutation error.
            }
          }}
        />
      ) : null}

      {configurationNode ? (
        <PluginConfigurationDialog
          displayName={configurationPlugin?.displayName ?? configurationNode.pluginType}
          pluginType={configurationNode.pluginType}
          pluginVersion={configurationNode.pluginVersion}
          configuration={configurationNode.configuration}
          readOnly={readOnly}
          onValidityChange={configurationValidityChange}
          onClose={() => {
            setConfigurationNodeId(null);
            setConfigurationValid(true);
          }}
          onSave={(configuration) => {
            setNodes((current) =>
              current.map((node) =>
                node.nodeId === configurationNode.nodeId ? { ...node, configuration } : node,
              ),
            );
            setConfigurationNodeId(null);
            setConfigurationValid(true);
          }}
        />
      ) : null}
    </PageShell>
  );
}
