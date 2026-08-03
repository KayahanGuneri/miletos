import { MarkerType, type CoordinateExtent } from "@xyflow/react";
import {
  type ExecutionGraphEdge,
  type ExecutionGraphModel,
  type ExecutionGraphNode,
} from "@/app/(panel)/_modules/dashboard/types/execution-graph-types";
import {
  type NodeExecutionResponse,
  type WorkflowDefinition,
} from "@/app/(panel)/_modules/dashboard/types/execution-types";

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

  return {
    id: nodeId,
    type: "executionNode",
    position: { x: 0, y: index * FALLBACK_VERTICAL_GAP },
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

function layoutNodesByTopology(
  nodes: ExecutionGraphNode[],
  edges: ExecutionGraphEdge[],
): ExecutionGraphNode[] {
  const originalOrder = new Map(nodes.map((node, index) => [node.id, index]));
  const indegree = new Map(nodes.map((node) => [node.id, 0]));
  const rank = new Map(nodes.map((node) => [node.id, 0]));
  const successors = new Map(nodes.map((node) => [node.id, [] as string[]]));

  edges.forEach((edge) => {
    successors.get(edge.source)?.push(edge.target);
    indegree.set(edge.target, (indegree.get(edge.target) ?? 0) + 1);
  });

  const compareNodeIds = (first: string, second: string) => {
    const orderDifference =
      (originalOrder.get(first) ?? Number.MAX_SAFE_INTEGER) -
      (originalOrder.get(second) ?? Number.MAX_SAFE_INTEGER);

    return orderDifference || first.localeCompare(second);
  };

  successors.forEach((targets) => {
    targets.sort(compareNodeIds);
  });

  const ready = nodes
    .filter((node) => indegree.get(node.id) === 0)
    .map((node) => node.id)
    .sort(compareNodeIds);
  const visited = new Set<string>();

  while (ready.length > 0) {
    const nodeId = ready.shift();

    if (!nodeId) {
      break;
    }

    visited.add(nodeId);

    for (const targetId of successors.get(nodeId) ?? []) {
      rank.set(targetId, Math.max(rank.get(targetId) ?? 0, (rank.get(nodeId) ?? 0) + 1));
      const remaining = (indegree.get(targetId) ?? 0) - 1;
      indegree.set(targetId, remaining);

      if (remaining === 0) {
        ready.push(targetId);
        ready.sort(compareNodeIds);
      }
    }
  }

  nodes
    .filter((node) => !visited.has(node.id))
    .sort((first, second) => compareNodeIds(first.id, second.id))
    .forEach((node) => {
      rank.set(node.id, 0);
    });

  const nodesByRank = new Map<number, ExecutionGraphNode[]>();

  nodes.forEach((node) => {
    const nodeRank = rank.get(node.id) ?? 0;
    const siblings = nodesByRank.get(nodeRank) ?? [];
    siblings.push(node);
    nodesByRank.set(nodeRank, siblings);
  });

  const positions = new Map<string, { x: number; y: number }>();

  nodesByRank.forEach((siblings, nodeRank) => {
    siblings.sort((first, second) => compareNodeIds(first.id, second.id));
    const totalHeight = (siblings.length - 1) * FALLBACK_VERTICAL_GAP;

    siblings.forEach((node, siblingIndex) => {
      positions.set(node.id, {
        x: nodeRank * FALLBACK_HORIZONTAL_GAP,
        y: siblingIndex * FALLBACK_VERTICAL_GAP - totalHeight / 2,
      });
    });
  });

  return nodes.map((node) => ({
    ...node,
    position: positions.get(node.id) ?? node.position,
  }));
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
    type: "executionEdge",
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
  definition: WorkflowDefinition,
  nodeExecutions: NodeExecutionResponse[] = [],
): ExecutionGraphModel {
  const runtimeNodeIndex = createRuntimeNodeIndex(nodeExecutions);

  const rawNodes = Array.isArray(definition.nodes) ? definition.nodes : [];

  const parsedNodes = rawNodes.flatMap((node, index) => {
    if (!isRecord(node)) {
      return [];
    }

    const nodeId = readString(node.id, "");

    const graphNode = createGraphNode(node, index, runtimeNodeIndex.get(nodeId));

    return graphNode ? [graphNode] : [];
  });

  const nodeIds = new Set(parsedNodes.map((node) => node.id));

  const rawEdges = Array.isArray(definition.edges) ? definition.edges : [];

  const edges = rawEdges.flatMap((edge, index) => {
    const graphEdge = createGraphEdge(edge, index, nodeIds);

    return graphEdge ? [graphEdge] : [];
  });
  const nodes = layoutNodesByTopology(parsedNodes, edges);

  return {
    nodes,
    edges,
    nodeExtent: createNodeExtent(nodes),
  };
}
