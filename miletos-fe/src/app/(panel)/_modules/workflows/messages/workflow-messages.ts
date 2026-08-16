import { type ApiError } from "@/shared/api/api-error";
import { type WorkflowStatus } from "@/app/(panel)/_modules/workflows/types/workflow-types";

export const workflowMessages = {
  common: {
    eyebrow: "Workflow control plane",
    emptyValue: "—",
  },
  list: {
    title: "Workflows",
    description:
      "Create workflow drafts, manage their lifecycle and inspect company-owned definitions.",
    create: "Create workflow",
    directoryEyebrow: "Directory",
    directoryTitle: "Company workflows",
    forbiddenTitle: "Workflow management is unavailable.",
    forbiddenDescription: "Only active company administrators can access this feature.",
    searchLabel: "Search",
    searchPlaceholder: "Search workflow names",
    searchSubmit: "Search",
    statusLabel: "Status",
    allStatuses: "All statuses",
    refresh: "Refresh",
    refreshing: "Refreshing...",
    loadingTitle: "Loading workflows...",
    loadingDescription: "Reading the company workflow directory.",
    errorTitle: "Workflows could not be loaded.",
    emptyTitle: "No workflows found.",
    emptyDescription: "Create a draft or adjust the current filters.",
    columns: {
      id: "ID",
      name: "Name",
      description: "Description",
      status: "Status",
      revision: "Revision",
      nodes: "Nodes",
      edges: "Edges",
      created: "Created",
      updated: "Updated",
      createdBy: "Created by",
      updatedBy: "Updated by",
      actions: "Actions",
    },
    open: "Open",
    delete: "Delete",
    deleted: (workflowName: string) => `Workflow ${workflowName} was deleted.`,
    statusChanged: (workflowName: string, statusLabel: string) =>
      `Workflow ${workflowName} is now ${statusLabel}.`,
    pagination: (page: number, totalPages: number) => `Page ${page} of ${totalPages}`,
    previous: "Previous",
    next: "Next",
  },
  editor: {
    forbiddenTitle: "Workflow access unavailable",
    forbiddenDescription: "Only active company administrators can manage workflows.",
    forbiddenState: "You do not have permission to access this page.",
    unavailableTitle: "Workflow unavailable",
    invalidIdentifierDescription: "The workflow identifier is invalid.",
    invalidIdentifierState: "Open a workflow from the company directory.",
    loadErrorDescription: "The workflow could not be loaded.",
    loadingTitle: "Loading workflow",
    loadingDescription: "Reading the company-owned workflow definition.",
    loadingState: "Loading workflow...",
    preparingDescription: "Preparing the editable workflow state.",
    preparingState: "Preparing workflow editor...",
    createTitle: "Create workflow",
    fallbackTitle: "Workflow editor",
    createDescription:
      "Name the workflow, describe its purpose, then assemble the draft on the canvas.",
    revisionDescription: (revision: string, statusLabel: string) =>
      `Revision ${revision} · ${statusLabel}`,
    back: "Back to workflows",
    statusChanged: (statusLabel: string) => `Workflow is now ${statusLabel}.`,
    detailsEyebrow: "Definition",
    detailsTitle: "Workflow details",
    nameLabel: "Name",
    descriptionLabel: "Description",
    readOnlyNotice:
      "Active and archived workflows are read-only. Restore an archived workflow to edit it.",
    advancedMetadataTitle: "Advanced metadata",
    metadataLabel: "Workflow metadata JSON",
    save: "Save draft",
    saving: "Saving...",
    delete: "Delete workflow",
    saved: "Workflow draft saved.",
    invalidConfiguration:
      "Correct the selected node configuration or delete the node to discard it.",
    invalidNodeConfiguration: (nodeName: string) =>
      `Configure “${nodeName}” before saving this workflow.`,
    operationPending: "Wait for the current workflow operation to finish.",
    unsavedDraft: "Save the current draft before activating it.",
    unresolvedErrors: "Resolve metadata and node configuration errors before activating it.",
    invalidName: "Workflow name must contain between 3 and 120 characters.",
    invalidDescription: "Description must not exceed 1000 characters.",
    invalidJson: "Resolve JSON editor errors before saving.",
  },
  jsonEditor: {
    notAnObject: "Enter a JSON object. Arrays and primitive values are not allowed.",
    invalidJson: "Enter valid JSON before saving.",
  },
  deleteConfirmation: {
    panelLabel: "Delete workflow",
    title: (workflowName: string) => `Delete ${workflowName}`,
    description: "This permanently removes the workflow. Type the exact workflow name to continue.",
    nameLabel: "Workflow name",
    cancel: "Cancel",
    confirm: "Delete workflow",
    deleting: "Deleting...",
  },
  lifecycle: {
    activate: "Activate",
    archive: "Archive",
    restore: "Restore",
    updating: "Updating...",
  },
  nodeProperties: {
    eyebrow: "Properties",
    title: "Selected node",
    emptyTitle: "No node selected",
    empty: "Select a node on the canvas to inspect its configuration.",
    displayName: "Node name",
    nodeId: "Node ID",
    pluginType: "Plugin type",
    pluginVersion: "Plugin version",
    plugin: "Plugin",
    configure: "Configure",
    viewConfiguration: "View configuration",
    delete: "Delete node",
  },
  pluginPalette: {
    eyebrow: "Plugin palette",
    title: "Nodes",
    description: "Drag an installed runtime plugin onto the workflow canvas.",
    loading: "Loading plugins...",
    empty: "No runtime plugins are currently available.",
    readOnlyCardLabel: (displayName: string) => `${displayName}; workflow is read-only`,
    draggableCardLabel: (displayName: string) => `Drag ${displayName} onto the workflow canvas`,
    readOnlyCardTitle: "Workflow is read-only",
    draggableCardTitle: "Drag onto the canvas to add this node",
    portSummary: (inputCount: number, outputCount: number) =>
      `${inputCount} inputs · ${outputCount} outputs`,
    readOnlyHint: "Available for inspection only",
    dragHint: "Drag to canvas",
    searchLabel: "Search plugins",
    searchPlaceholder: "Filter by name, type or category",
    emptyFiltered: "No plugins match the current search.",
  },
  canvas: {
    title: "Workflow canvas",
    graphSummary: (nodeCount: number, edgeCount: number) =>
      `${nodeCount} nodes · ${edgeCount} edges`,
  },
  runtime: {
    title: "Runtime actions",
    description:
      "Bind triggers or run the persisted revision without sending the editable browser graph.",
    statusReasons: {
      DRAFT: "Activate the workflow before creating triggers or running it.",
      ARCHIVED: "Restore and activate the workflow before creating triggers or running it.",
    } as Partial<Record<WorkflowStatus, string>>,
    notYet: "Not yet",
    yes: "Yes",
    no: "No",
    statusLabels: {
      ACTIVE: "Active",
      DISABLED: "Disabled",
      VALIDATING: "Validating",
      QUEUED: "Queued",
      RUNNING: "Running",
      SUCCEEDED: "Succeeded",
      FAILED: "Failed",
    } as Record<string, string>,
    modeLabels: {
      ASYNC: "Asynchronous",
      SYNC: "Synchronous",
    } as Record<string, string>,
    originLabels: {
      MANUAL_DIRECT: "Manual",
      HTTP_WEBHOOK: "HTTP webhook",
      CRON: "Cron",
      DATA_ARRIVAL: "Data arrival",
    } as Record<string, string>,
    errors: {
      fallback: "The runtime operation could not be completed. Try again.",
      byCode: {
        WORKFLOW_NOT_FOUND: "This workflow no longer exists.",
        WORKFLOW_INVALID_STATE: "Activate the workflow before binding a trigger or running it.",
        INVALID_WORKFLOW_DEFINITION: "The workflow definition is invalid.",
        WORKFLOW_TRIGGER_ROOT_REQUIRED: "The selected node must be a workflow root.",
        WORKFLOW_TRIGGER_NODE_NOT_FOUND:
          "The trigger node is not part of the persisted workflow revision.",
        WORKFLOW_TRIGGER_TYPE_INVALID: "The root plugin does not support this trigger type.",
        HTTP_TRIGGER_CONFIGURATION_INVALID:
          "The HTTP trigger node needs a valid HTTP method. Configure the node and save the workflow.",
        CRON_TRIGGER_CONFIGURATION_INVALID:
          "The cron trigger node needs a valid expression. Configure the node and save the workflow.",
        MANUAL_WORKFLOW_EXECUTION_INVALID:
          "The manual run request is invalid. Reopen the dialog and run it again.",
        TRIGGER_NOT_FOUND: "The trigger binding no longer exists.",
        TRIGGER_RUNTIME_UNAVAILABLE:
          "The workflow runtime is currently unavailable. Try again shortly.",
        WORKFLOW_VALIDATION_FAILED: "The persisted workflow failed runtime validation.",
        INVALID_EXECUTION_ORIGIN: "The workflow root plugins do not support this execution origin.",
        INVALID_EXECUTION_REQUEST: "The runtime rejected the request as invalid.",
        IDEMPOTENCY_KEY_REUSED: "This submission key was already used for a different request.",
        EXECUTION_UNAVAILABLE: "Workflow execution is currently unavailable. Try again shortly.",
        FAILED_PRECONDITION: "The operation conflicts with the current runtime state.",
        NOT_FOUND: "The requested runtime resource was not found.",
      } as Record<string, string>,
    },
    http: {
      title: "HTTP webhook",
      description: (rootNodeId: string) =>
        `Create an asynchronous public binding for root ${rootNodeId}.`,
      create: "Create HTTP trigger",
      creating: "Creating...",
      disable: "Disable HTTP trigger",
      disabling: "Disabling...",
      boundNotice:
        "An active HTTP trigger is bound. The one-time public URL is shown only at creation.",
      secretTitle: "Save this secret URL now",
      secretDescription:
        "It contains the webhook secret and is returned only once. It is kept only in this page memory and is not written to browser storage.",
      publicUrlLabel: "HTTP trigger public URL",
    },
    cron: {
      title: "Cron schedule",
      description: (rootNodeId: string) =>
        `Create a schedule from root ${rootNodeId}'s persisted configuration.`,
      create: "Create cron trigger",
      creating: "Creating...",
      disable: "Disable cron trigger",
      disabling: "Disabling...",
      status: "Status",
      expression: "Expression",
      timezone: "Timezone",
      nextFire: "Next fire",
      lastScheduled: "Last scheduled",
      lastFired: "Last fired",
    },
    manual: {
      title: "Manual run",
      description: "Run this persisted active revision with a fresh idempotency key.",
      open: "Run workflow",
      dialogTitle: "Run workflow",
      dialogDescription: "Start this persisted active revision with a new idempotent submission.",
      close: "Close",
      run: "Run workflow",
      running: "Running...",
      initialVariablesLabel: "Initial variables JSON (optional)",
      initialVariablesUnsupported:
        "The compatible root plugins do not accept initial variables. This run will use an empty object.",
      executionStarted: "Execution started",
      executionId: "Execution ID",
      executionCount: (count: number) => `${count} record executions`,
      status: "Status",
      mode: "Mode",
      origin: "Origin",
      replayed: "Replayed",
      scheduledRoots: "Scheduled roots",
    },
  },
} as const;

export function runtimeErrorMessage(error: ApiError | null | undefined) {
  if (!error) {
    return undefined;
  }
  return (
    workflowMessages.runtime.errors.byCode[error.code] ?? workflowMessages.runtime.errors.fallback
  );
}

export function runtimeStatusLabel(status: string) {
  return workflowMessages.runtime.statusLabels[status] ?? status;
}

export function runtimeModeLabel(mode: string) {
  return workflowMessages.runtime.modeLabels[mode] ?? mode;
}

export function runtimeOriginLabel(origin: string) {
  return workflowMessages.runtime.originLabels[origin] ?? origin;
}

export function workflowStatusReason(status: WorkflowStatus) {
  return workflowMessages.runtime.statusReasons[status];
}
