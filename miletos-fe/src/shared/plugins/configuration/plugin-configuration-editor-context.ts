import type {
  PluginConfiguration,
  PluginConfigurationEditorContext,
  PluginIncomingInput,
  PluginWorkflowOption,
} from "@/shared/plugins/contracts/plugin-configuration-interfaces";

export function createPluginConfigurationEditorContext(params: {
  nodeId: string;
  nodes: Array<{
    nodeId: string;
    displayName?: string | null;
    pluginType: string;
    configuration: PluginConfiguration;
  }>;
  edges: Array<{
    edgeId: string;
    sourceNodeId: string;
    targetNodeId: string;
  }>;
  workflowId?: number;
  workflows?: PluginWorkflowOption[];
  sourceLabel: (node: {
    nodeId: string;
    displayName?: string | null;
    pluginType: string;
  }) => string;
}): PluginConfigurationEditorContext {
  const incomingInputs: PluginIncomingInput[] = [];
  params.edges.forEach((edge, edgeOrder) => {
    if (edge.targetNodeId !== params.nodeId) {
      return;
    }
    const source = params.nodes.find((node) => node.nodeId === edge.sourceNodeId);
    incomingInputs.push({
      edgeId: edge.edgeId,
      edgeOrder,
      sourceNodeId: edge.sourceNodeId,
      sourceLabel: source ? params.sourceLabel(source) : edge.sourceNodeId,
      sourcePluginType: source?.pluginType ?? "",
      sourceConfiguration: source?.configuration ?? {},
    });
  });
  return {
    nodeId: params.nodeId,
    incomingInputs,
    workflowId: params.workflowId,
    workflows: params.workflows,
  };
}
