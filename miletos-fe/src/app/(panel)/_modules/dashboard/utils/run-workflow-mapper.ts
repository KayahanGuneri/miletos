import { type WorkflowDefinition } from "../types/execution-types";
import { type RunWorkflowRequest } from "../types/execution-types";

export function mapDefinitionToRunWorkflowRequest(
  definition: WorkflowDefinition,
): RunWorkflowRequest {
  return {
    definition: {
      id: definition.id,
      name: definition.name,
      revision: definition.revision,
      nodes: definition.nodes.map((node) => ({
        ...node,
        configuration: node.configuration ? { ...node.configuration } : undefined,
        position: node.position ? { ...node.position } : undefined,
      })),
      edges: definition.edges.map((edge) => ({ ...edge })),
      metadata: definition.metadata ? { ...definition.metadata } : undefined,
    },
  };
}
