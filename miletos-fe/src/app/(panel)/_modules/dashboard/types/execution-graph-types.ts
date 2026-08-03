import { type CoordinateExtent, type Edge, type Node } from "@xyflow/react";

export interface ExecutionGraphNodeData extends Record<string, unknown> {
  label: string;
  nodeId: string;
  pluginType: string;
  pluginVersion: string;
  configuration: Record<string, unknown>;
  nodeExecutionId?: string;
  runtimeStatus?: string;
  attempt?: number;
}

export type ExecutionGraphNode = Node<ExecutionGraphNodeData, "executionNode">;

export type ExecutionGraphEdge = Edge;

export interface ExecutionGraphModel {
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
  nodeExtent: CoordinateExtent;
}
