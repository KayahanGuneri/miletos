import type {
  WorkflowEdge,
  WorkflowNode,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import type { WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import type { PluginConnectionRestriction } from "@/shared/plugins/contracts/plugin-configuration-interfaces";

export type EdgeValidationFailure =
  | "missing-endpoints"
  | "self-edge"
  | "duplicate"
  | "unavailable-handle"
  | "occupied-input-port"
  | "restricted-connection";

function pluginForNode(node: WorkflowNode, plugins: WorkflowPlugin[]) {
  return plugins.find((plugin) => plugin.type === node.pluginType);
}

function restrictionMatches(
  restriction: PluginConnectionRestriction,
  sourcePlugin: WorkflowPlugin,
  targetPlugin: WorkflowPlugin,
  targetHandle: string,
) {
  if (restriction.from !== sourcePlugin.type || restriction.to !== targetPlugin.type) {
    return false;
  }
  if (restriction.selector === "PRIMARY") {
    return targetPlugin.inputPorts[0]?.name === targetHandle;
  }
  if (restriction.selector === "NOT_PRIMARY") {
    return targetPlugin.inputPorts.slice(1).some((port) => port.name === targetHandle);
  }
  return (
    restriction.selector === "POSITION" &&
    Number.isInteger(restriction.position) &&
    (restriction.position ?? 0) > 0 &&
    targetPlugin.inputPorts[(restriction.position ?? 0) - 1]?.name === targetHandle
  );
}

export function validateWorkflowEdgeConnection(params: {
  connection: {
    source: string | null;
    target: string | null;
    sourceHandle: string | null;
    targetHandle: string | null;
  };
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  plugins: WorkflowPlugin[];
}): EdgeValidationFailure | null {
  const { connection, nodes, edges, plugins } = params;
  if (
    !connection.source ||
    !connection.target ||
    !connection.sourceHandle ||
    !connection.targetHandle
  ) {
    return "missing-endpoints";
  }
  if (connection.source === connection.target) {
    return "self-edge";
  }

  const duplicate = edges.some(
    (edge) =>
      edge.sourceNodeId === connection.source &&
      edge.sourceOutputPort === connection.sourceHandle &&
      edge.targetNodeId === connection.target &&
      edge.targetInputPort === connection.targetHandle,
  );
  if (duplicate) {
    return "duplicate";
  }

  const sourceNode = nodes.find((node) => node.nodeId === connection.source);
  const targetNode = nodes.find((node) => node.nodeId === connection.target);
  if (!sourceNode || !targetNode) {
    return "missing-endpoints";
  }

  const sourcePlugin = pluginForNode(sourceNode, plugins);
  const targetPlugin = pluginForNode(targetNode, plugins);
  if (
    sourcePlugin &&
    !sourcePlugin.outputPorts.some((port) => port.name === connection.sourceHandle)
  ) {
    return "unavailable-handle";
  }
  if (
    targetPlugin &&
    !targetPlugin.inputPorts.some((port) => port.name === connection.targetHandle)
  ) {
    return "unavailable-handle";
  }

  const fixedMultiInput =
    targetPlugin?.inputMode === "MULTI" &&
    !targetPlugin.inputEdgeConstraint.unlimited &&
    targetPlugin.inputEdgeConstraint.minimum === targetPlugin.inputEdgeConstraint.maximum &&
    targetPlugin.inputEdgeConstraint.minimum === targetPlugin.inputPorts.length;
  const portMaximum = targetPlugin?.inputPorts.find((port) => port.name === connection.targetHandle)
    ?.edgeConstraint?.maximum;
  const hasPortMaximum = typeof portMaximum === "number";
  const occupiedPort = edges.filter(
    (edge) =>
      edge.targetNodeId === connection.target && edge.targetInputPort === connection.targetHandle,
  ).length;
  if (occupiedPort > 0 && ((hasPortMaximum && occupiedPort >= portMaximum) || fixedMultiInput)) {
    return "occupied-input-port";
  }

  if (
    sourcePlugin &&
    targetPlugin &&
    sourcePlugin.connectionRestrictions.some((restriction) =>
      restrictionMatches(restriction, sourcePlugin, targetPlugin, connection.targetHandle!),
    )
  ) {
    return "restricted-connection";
  }

  return null;
}
