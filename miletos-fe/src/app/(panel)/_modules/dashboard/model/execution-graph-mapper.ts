import { MarkerType, type CoordinateExtent, type XYPosition } from "@xyflow/react";
import {
  type ExecutionGraphEdge,
  type ExecutionGraphModel,
  type ExecutionGraphNode,
} from "@/app/(panel)/_modules/dashboard/model/execution-graph-types";
import { type NodeExecutionResponse } from "@/app/(panel)/_modules/dashboard/model/execution-types";

const FALLBACK_COLUMN_COUNT = 4;

const FALLBACK_HORIZONTAL_GAP = 320;

const FALLBACK_VERTICAL_GAP = 190;

const GRAPH_PADDING = 320;

const ESTIMATED_NODE_WIDTH = 280;

const ESTIMATED_NODE_HEIGHT = 150;

const MIN_GRAPH_WIDTH = 1200;

const MIN_GRAPH_HEIGHT = 800;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readString(value: unknown, fallback: string) {
  return typeof value === "string" && value.trim().length > 0 ? value : fallback;
}

function readPosition(value: unknown): XYPosition | null {
  if (!isRecord(value)) {
    return null;
  }

  const x = value.x;
  const y = value.y;

  if (
    typeof x !== "number" ||
    typeof y !== "number" ||
    !Number.isFinite(x) ||
    !Number.isFinite(y)
  ) {
    return null;
  }

  return {
    x,
    y,
  };
}

function createFallbackPosition(index: number): XYPosition {
  const column = index % FALLBACK_COLUMN_COUNT;

  const row = Math.floor(index / FALLBACK_COLUMN_COUNT);

  return {
    x: column * FALLBACK_HORIZONTAL_GAP,
    y: row * FALLBACK_VERTICAL_GAP,
  };
}

function createRuntimeNodeIndex(nodeExecutions: NodeExecutionResponse[]) {
  const runtimeNodeIndex = new Map<string, NodeExecutionResponse>();

  nodeExecutions.forEach((nodeExecution) => {
    const current = runtimeNodeIndex.get(nodeExecution.nodeId);

    if (!current || nodeExecution.attempt >= current.attempt) {
      runtimeNodeIndex.set(nodeExecution.nodeId, nodeExecution);
    }
  });

  return runtimeNodeIndex;
}

function createGraphNode(
  value: unknown,
  index: number,
  runtimeNode: NodeExecutionResponse | undefined,
): ExecutionGraphNode | null {
  if (!isRecord(value)) {
    return null;
  }

  const nodeId = readString(value.id, "");

  if (!nodeId) {
    return null;
  }

  const pluginType = readString(value.pluginType, "unknown-plugin");

  const pluginVersion = readString(value.pluginVersion, "unknown");

  const configuration = isRecord(value.configuration) ? value.configuration : {};

  const position = readPosition(value.position) ?? createFallbackPosition(index);

  return {
    id: nodeId,
    type: "executionNode",
    position,
    draggable: true,
    selectable: true,
    deletable: false,
    connectable: false,
    ariaLabel: `Workflow node ${nodeId}`,
    data: {
      label: nodeId,
      nodeId,
      pluginType,
      pluginVersion,
      configuration,
      nodeExecutionId: runtimeNode?.nodeExecutionId,
      runtimeStatus: runtimeNode?.status,
      attempt: runtimeNode?.attempt,
    },
  };
}

function createGraphEdge(
  value: unknown,
  index: number,
  nodeIds: Set<string>,
): ExecutionGraphEdge | null {
  if (!isRecord(value)) {
    return null;
  }

  const sourceNodeId = readString(value.sourceNodeId, "");

  const targetNodeId = readString(value.targetNodeId, "");

  if (!sourceNodeId || !targetNodeId || !nodeIds.has(sourceNodeId) || !nodeIds.has(targetNodeId)) {
    return null;
  }

  const sourceOutputPort = readString(value.sourceOutputPort, "");

  const targetInputPort = readString(value.targetInputPort, "");

  const edgeId = readString(value.id, `${sourceNodeId}-${targetNodeId}-${index}`);

  const portLabel =
    sourceOutputPort && targetInputPort ? `${sourceOutputPort} → ${targetInputPort}` : undefined;

  return {
    id: edgeId,
    source: sourceNodeId,
    target: targetNodeId,
    label: portLabel,
    deletable: false,
    selectable: true,
    markerEnd: {
      type: MarkerType.ArrowClosed,
    },
  };
}

function createNodeExtent(nodes: ExecutionGraphNode[]): CoordinateExtent {
  if (nodes.length === 0) {
    return [
      [0, 0],
      [MIN_GRAPH_WIDTH, MIN_GRAPH_HEIGHT],
    ];
  }

  const xPositions = nodes.map((node) => node.position.x);

  const yPositions = nodes.map((node) => node.position.y);

  const minimumX = Math.min(0, ...xPositions);

  const minimumY = Math.min(0, ...yPositions);

  const maximumX = Math.max(MIN_GRAPH_WIDTH, ...xPositions);

  const maximumY = Math.max(MIN_GRAPH_HEIGHT, ...yPositions);

  return [
    [minimumX - GRAPH_PADDING, minimumY - GRAPH_PADDING],
    [
      maximumX + ESTIMATED_NODE_WIDTH + GRAPH_PADDING,
      maximumY + ESTIMATED_NODE_HEIGHT + GRAPH_PADDING,
    ],
  ];
}

export function mapExecutionDefinitionToGraph(
  definition: Record<string, unknown>,
  nodeExecutions: NodeExecutionResponse[] = [],
): ExecutionGraphModel {
  const runtimeNodeIndex = createRuntimeNodeIndex(nodeExecutions);

  const rawNodes = Array.isArray(definition.nodes) ? definition.nodes : [];

  const nodes = rawNodes.flatMap((node, index) => {
    if (!isRecord(node)) {
      return [];
    }

    const nodeId = readString(node.id, "");

    const graphNode = createGraphNode(node, index, runtimeNodeIndex.get(nodeId));

    return graphNode ? [graphNode] : [];
  });

  const nodeIds = new Set(nodes.map((node) => node.id));

  const rawEdges = Array.isArray(definition.edges) ? definition.edges : [];

  const edges = rawEdges.flatMap((edge, index) => {
    const graphEdge = createGraphEdge(edge, index, nodeIds);

    return graphEdge ? [graphEdge] : [];
  });

  return {
    nodes,
    edges,
    nodeExtent: createNodeExtent(nodes),
  };
}
