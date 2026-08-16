"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { XYPosition } from "@xyflow/react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import { PageShell } from "@/components/layout/page-shell/PageShell";
import { JsonObjectEditor } from "@/app/(panel)/_modules/workflows/components/JsonObjectEditor";
import { NodePropertiesPanel } from "@/app/(panel)/_modules/workflows/components/NodePropertiesPanel";
import { PluginPalette } from "@/app/(panel)/_modules/workflows/components/PluginPalette";
import { WorkflowCanvas } from "@/app/(panel)/_modules/workflows/components/WorkflowCanvas";
import { WorkflowDeleteConfirmation } from "@/app/(panel)/_modules/workflows/components/WorkflowDeleteConfirmation";
import { WorkflowLifecycleActions } from "@/app/(panel)/_modules/workflows/components/WorkflowLifecycleActions";
import { WorkflowRuntimeActions } from "@/app/(panel)/_modules/workflows/components/WorkflowRuntimeActions";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import {
  useCreateWorkflowMutation,
  useDeleteWorkflowMutation,
  useUpdateWorkflowMutation,
  useUploadWorkflowInputFileMutation,
} from "@/app/(panel)/_modules/workflows/query/workflow-mutations";
import {
  useAllWorkflowsQuery,
  useWorkflowPluginsQuery,
  useWorkflowQuery,
} from "@/app/(panel)/_modules/workflows/query/workflow-queries";
import {
  type SaveWorkflowRequest,
  type Workflow,
  type WorkflowEdge,
  type WorkflowNode,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import {
  type JsonObject,
  type WorkflowPlugin,
} from "@/app/(panel)/_modules/workflows/types/workflow-types";
import {
  createClientId,
  formatWorkflowStatus,
} from "@/app/(panel)/_modules/workflows/utils/workflow-utils";
import { useCurrentUserQuery } from "@/shared/session/hooks/useCurrentUserQuery";
import { canManageWorkflows } from "@/shared/session/permissions/session-permissions";
import { PluginConfigurationDialog } from "@/shared/plugins/configuration/PluginConfigurationDialog";
import { createPluginConfigurationEditorContext } from "@/shared/plugins/configuration/plugin-configuration-editor-context";
import {
  createDefaultPluginConfiguration,
  isPluginConfigurationValid,
} from "@/shared/plugins/registry/plugin-configuration-registry";
import styles from "./ui/WorkflowEditorPage.module.css";

interface WorkflowEditorPageProps {
  workflowId?: number;
}

function pluginForWorkflowNode(node: WorkflowNode | null, plugins: WorkflowPlugin[] | undefined) {
  if (!node) {
    return undefined;
  }
  return plugins?.find((plugin) => plugin.type === node.pluginType);
}

function editorContextForNode(
  nodeId: string,
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  plugins: WorkflowPlugin[] | undefined,
  workflowId?: number,
  workflows?: Array<{ id: number; name: string; status: string }>,
) {
  return createPluginConfigurationEditorContext({
    nodeId,
    nodes,
    edges,
    workflowId,
    workflows,
    sourceLabel: (node) =>
      node.displayName?.trim() ||
      plugins?.find((plugin) => plugin.type === node.pluginType)?.displayName ||
      node.pluginType,
  });
}

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
  const workflowOptions = useAllWorkflowsQuery(isAllowed);
  const createMutation = useCreateWorkflowMutation();
  const updateMutation = useUpdateWorkflowMutation();
  const deleteMutation = useDeleteWorkflowMutation();
  const uploadMutation = useUploadWorkflowInputFileMutation();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [nodes, setNodes] = useState<WorkflowNode[]>([]);
  const [edges, setEdges] = useState<WorkflowEdge[]>([]);
  const [metadata, setMetadata] = useState<JsonObject>({});
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [metadataValid, setMetadataValid] = useState(true);
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
  const configurationNode = useMemo(
    () => nodes.find((node) => node.nodeId === configurationNodeId) ?? null,
    [configurationNodeId, nodes],
  );
  const selectedPlugin = useMemo(
    () => pluginForWorkflowNode(selectedNode, plugins.data?.items),
    [plugins.data?.items, selectedNode],
  );
  const configurationPlugin = useMemo(
    () => pluginForWorkflowNode(configurationNode, plugins.data?.items),
    [configurationNode, plugins.data?.items],
  );
  const editorWorkflows = workflowOptions.data?.map((workflow) => ({
    id: workflow.id,
    name: workflow.name,
    status: workflow.status,
  }));
  const hasInvalidStoredConfiguration = useMemo(
    () =>
      nodes.some(
        (node) =>
          !isPluginConfigurationValid(
            node.pluginType,
            node.configuration,
            editorContextForNode(
              node.nodeId,
              nodes,
              edges,
              plugins.data?.items,
              workflowId,
              editorWorkflows,
            ),
          ),
      ),
    [edges, editorWorkflows, nodes, plugins.data?.items, workflowId],
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
  const ignoreConfigurationValidity = useCallback(() => {}, []);

  const lifecyclePending = saving || deleteMutation.isPending;
  let lifecycleDisabled = lifecyclePending;
  let lifecycleDisabledReason: string | undefined = lifecyclePending
    ? workflowMessages.editor.operationPending
    : undefined;

  if (detail.data?.status === "DRAFT") {
    if (isDirty) {
      lifecycleDisabled = true;
      lifecycleDisabledReason = workflowMessages.editor.unsavedDraft;
    } else if (!metadataValid || hasInvalidStoredConfiguration) {
      lifecycleDisabled = true;
      lifecycleDisabledReason = workflowMessages.editor.unresolvedErrors;
    }
  }

  function dropPlugin(plugin: WorkflowPlugin, position: XYPosition) {
    const nodeId = createClientId("node");
    const configuration = createDefaultPluginConfiguration(plugin.type);
    setNodes((current) => [
      ...current,
      {
        nodeId,
        displayName: plugin.displayName,
        pluginType: plugin.type,
        pluginVersion: plugin.version,
        configuration,
        position,
      },
    ]);
    setSelectedNodeId(nodeId);
  }

  function deleteNodes(nodeIds: string[]) {
    const ids = new Set(nodeIds);
    setNodes((current) => current.filter((node) => !ids.has(node.nodeId)));
    setEdges((current) =>
      current.filter((edge) => !ids.has(edge.sourceNodeId) && !ids.has(edge.targetNodeId)),
    );
    if (selectedNodeId && ids.has(selectedNodeId)) {
      setSelectedNodeId(null);
    }
    if (configurationNodeId && ids.has(configurationNodeId)) {
      setConfigurationNodeId(null);
    }
  }

  function selectNode(nodeId: string | null) {
    if (nodeId === selectedNodeId) {
      return;
    }

    setSelectedNodeId(nodeId);
  }

  async function saveWorkflow() {
    const normalizedName = name.trim();
    if (normalizedName.length < 3 || normalizedName.length > 120) {
      setLocalError(workflowMessages.editor.invalidName);
      return;
    }
    if (description.trim().length > 1000) {
      setLocalError(workflowMessages.editor.invalidDescription);
      return;
    }
    if (!metadataValid) {
      setLocalError(workflowMessages.editor.invalidJson);
      return;
    }
    const firstInvalidNode = nodes.find(
      (node) =>
        !isPluginConfigurationValid(
          node.pluginType,
          node.configuration,
          editorContextForNode(
            node.nodeId,
            nodes,
            edges,
            plugins.data?.items,
            workflowId,
            editorWorkflows,
          ),
        ),
    );
    if (firstInvalidNode) {
      setSelectedNodeId(firstInvalidNode.nodeId);
      setLocalError(
        workflowMessages.editor.invalidNodeConfiguration(
          firstInvalidNode.displayName?.trim() ||
            pluginForWorkflowNode(firstInvalidNode, plugins.data?.items)?.displayName ||
            firstInvalidNode.pluginType,
        ),
      );
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
        setSuccessMessage(workflowMessages.editor.saved);
      }
    } catch {
      // The normalized mutation error is rendered below.
    }
  }

  if (!isAllowed) {
    return (
      <PageShell
        eyebrow={workflowMessages.common.eyebrow}
        title={workflowMessages.editor.forbiddenTitle}
        description={workflowMessages.editor.forbiddenDescription}
      >
        <Box className={styles.workflowEditor__state} role="alert">
          <Typography as="strong">{workflowMessages.editor.forbiddenState}</Typography>
        </Box>
      </PageShell>
    );
  }

  if (!hasValidWorkflowId) {
    return (
      <PageShell
        eyebrow={workflowMessages.common.eyebrow}
        title={workflowMessages.editor.unavailableTitle}
        description={workflowMessages.editor.invalidIdentifierDescription}
      >
        <Box className={styles.workflowEditor__state} role="alert">
          <Typography as="strong">{workflowMessages.editor.invalidIdentifierState}</Typography>
        </Box>
      </PageShell>
    );
  }

  if (!isNew && detail.isPending) {
    return (
      <PageShell
        eyebrow={workflowMessages.common.eyebrow}
        title={workflowMessages.editor.loadingTitle}
        description={workflowMessages.editor.loadingDescription}
      >
        <Box className={styles.workflowEditor__state}>
          <Typography as="strong">{workflowMessages.editor.loadingState}</Typography>
        </Box>
      </PageShell>
    );
  }

  if (!isNew && detail.isError) {
    return (
      <PageShell
        eyebrow={workflowMessages.common.eyebrow}
        title={workflowMessages.editor.unavailableTitle}
        description={workflowMessages.editor.loadErrorDescription}
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
        eyebrow={workflowMessages.common.eyebrow}
        title={workflowMessages.editor.loadingTitle}
        description={workflowMessages.editor.preparingDescription}
      >
        <Box className={styles.workflowEditor__state}>
          <Typography as="strong">{workflowMessages.editor.preparingState}</Typography>
        </Box>
      </PageShell>
    );
  }

  return (
    <PageShell
      variant="workbench"
      eyebrow={workflowMessages.common.eyebrow}
      title={
        isNew ? workflowMessages.editor.createTitle : name || workflowMessages.editor.fallbackTitle
      }
      description={
        isNew
          ? workflowMessages.editor.createDescription
          : workflowMessages.editor.revisionDescription(
              detail.data?.revision.toString() ?? workflowMessages.common.emptyValue,
              formatWorkflowStatus(detail.data?.status ?? "DRAFT"),
            )
      }
      actions={
        <>
          <Link className={styles.workflowEditor__backLink} href="/workflows">
            {workflowMessages.editor.back}
          </Link>
          {detail.data ? (
            <WorkflowLifecycleActions
              workflow={detail.data}
              disabled={lifecycleDisabled}
              disabledReason={lifecycleDisabledReason}
              onChanged={(workflow) => {
                loadServerWorkflow(workflow);
                setSuccessMessage(
                  workflowMessages.editor.statusChanged(
                    formatWorkflowStatus(workflow.status).toLowerCase(),
                  ),
                );
              }}
            />
          ) : null}
          <Button
            type="button"
            disabled={readOnly || saving || !metadataValid}
            onClick={() => void saveWorkflow()}
          >
            {saving ? workflowMessages.editor.saving : workflowMessages.editor.save}
          </Button>
          {!isNew && detail.data && detail.data.status !== "ACTIVE" ? (
            <Button
              type="button"
              variant={ButtonVariant.Danger}
              onClick={() => {
                setShowDelete(true);
                deleteMutation.reset();
              }}
            >
              {workflowMessages.editor.delete}
            </Button>
          ) : null}
        </>
      }
    >
      <div className={styles.workflowEditor}>
        <section className={styles.workflowEditor__toolbar} data-create={isNew}>
          {!isNew ? (
            <Box className={styles.workflowEditor__toolbarHeader}>
              <Typography as="p" className={styles.workflowEditor__eyebrow}>
                {workflowMessages.editor.detailsEyebrow}
              </Typography>
              <Typography as="h2">{workflowMessages.editor.detailsTitle}</Typography>
            </Box>
          ) : null}
          <Box className={styles.workflowEditor__formGrid}>
            <label className={styles.workflowEditor__field}>
              <Typography as="span">{workflowMessages.editor.nameLabel}</Typography>
              <Input
                className={styles.workflowEditor__nameInput}
                maxLength={120}
                readOnly={readOnly}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </label>
            <label className={styles.workflowEditor__field}>
              <Typography as="span">{workflowMessages.editor.descriptionLabel}</Typography>
              <textarea
                maxLength={1000}
                rows={4}
                readOnly={readOnly}
                value={description}
                onChange={(event) => setDescription(event.target.value)}
              />
            </label>
          </Box>
          {readOnly ? (
            <Typography as="p" className={styles.workflowEditor__readOnlyNotice}>
              {workflowMessages.editor.readOnlyNotice}
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

        {detail.data ? (
          <WorkflowRuntimeActions
            workflow={detail.data}
            plugins={plugins.data?.items ?? []}
            canManage={isAllowed}
          />
        ) : null}

        <Box className={styles.workflowEditor__workspace}>
          <PluginPalette
            plugins={plugins.data?.items ?? []}
            isLoading={plugins.isPending}
            errorMessage={plugins.error?.message}
            readOnly={readOnly}
          />
          <WorkflowCanvas
            workflowNodes={nodes}
            workflowEdges={edges}
            plugins={plugins.data?.items ?? []}
            readOnly={readOnly}
            selectionLocked={false}
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
            workflowId={detail.data?.id}
            workflowStatus={detail.data?.status}
            canManageTriggers={isAllowed}
            onDisplayNameChange={(displayName) =>
              setNodes((current) =>
                current.map((node) =>
                  node.nodeId === selectedNode?.nodeId ? { ...node, displayName } : node,
                ),
              )
            }
            onConfigure={() => {
              if (!selectedNode) {
                return;
              }
              setConfigurationNodeId(selectedNode.nodeId);
            }}
            onDelete={() => {
              if (selectedNodeId) deleteNodes([selectedNodeId]);
            }}
          />
        </Box>

        <details className={styles.workflowEditor__secondaryPanel}>
          <summary>{workflowMessages.editor.advancedMetadataTitle}</summary>
          <Box className={styles.workflowEditor__secondaryBody}>
            <JsonObjectEditor
              label={workflowMessages.editor.metadataLabel}
              value={metadata}
              readOnly={readOnly}
              resetKey={editorResetVersion}
              onChange={setMetadata}
              onValidityChange={metadataValidityChange}
            />
          </Box>
        </details>
      </div>

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
          key={configurationNode.nodeId}
          displayName={configurationPlugin?.displayName ?? configurationNode.pluginType}
          pluginType={configurationNode.pluginType}
          pluginVersion={configurationNode.pluginVersion}
          configuration={configurationNode.configuration}
          readOnly={readOnly}
          editorContext={editorContextForNode(
            configurationNode.nodeId,
            nodes,
            edges,
            plugins.data?.items,
            workflowId,
            editorWorkflows,
          )}
          uploadPending={uploadMutation.isPending}
          uploadError={uploadMutation.isError}
          onUploadFile={async (file) => {
            uploadMutation.reset();
            const uploaded = await uploadMutation.mutateAsync(file);
            return uploaded.fileName;
          }}
          onValidityChange={ignoreConfigurationValidity}
          onClose={() => {
            setConfigurationNodeId(null);
          }}
          onSave={(configuration) => {
            setNodes((current) =>
              current.map((node) =>
                node.nodeId === configurationNode.nodeId ? { ...node, configuration } : node,
              ),
            );
            setConfigurationNodeId(null);
          }}
        />
      ) : null}
    </PageShell>
  );
}
