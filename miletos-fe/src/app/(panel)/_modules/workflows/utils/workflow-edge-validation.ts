import type {
  WorkflowEdge,
  WorkflowNode,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import type { WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";

export type EdgeValidationFailure =
  "missing-endpoints" | "self-edge" | "duplicate" | "unavailable-handle";

function pluginForNode(node: WorkflowNode, plugins: WorkflowPlugin[]) {
  return plugins.find(
    (plugin) => plugin.type === node.pluginType && plugin.version === node.pluginVersion,
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

  return null;
}
