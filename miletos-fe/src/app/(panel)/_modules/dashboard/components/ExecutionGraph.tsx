"use client";

import { useEffect, useMemo, useState } from "react";
import {
  Background,
  Controls,
  ReactFlow,
  type EdgeTypes,
  type NodeTypes,
  useEdgesState,
  useNodesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import { useExecutionNodesQuery } from "@/app/(panel)/_modules/dashboard/query/useExecutionNodesQuery";
import { mapExecutionDefinitionToGraph } from "@/app/(panel)/_modules/dashboard/utils/execution-graph-mapper";
import {
  type ExecutionGraphEdge,
  type ExecutionGraphNode,
} from "@/app/(panel)/_modules/dashboard/types/execution-graph-types";
import {
  type NodeExecutionResponse,
  type WorkflowDefinition,
} from "@/app/(panel)/_modules/dashboard/types/execution-types";
import { ExecutionEdge } from "./ExecutionEdge";
import { GenericExecutionNode } from "./GenericExecutionNode";
import { ExecutionNodeDetail } from "./ExecutionNodeDetail";
import styles from "./ExecutionGraph.module.css";

interface ExecutionFlowGraphProps {
  definition: WorkflowDefinition;
  nodeExecutions?: NodeExecutionResponse[];
  onNodeSelect?: (nodeId: string | null) => void;
}

interface ExecutionGraphProps {
  executionId: string;
  definition: WorkflowDefinition;
  pollingEnabled: boolean;
  companyId?: number;
}

const RUNTIME_NODE_PAGE_SIZE = 100;

const NODE_TYPES: NodeTypes = {
  executionNode: GenericExecutionNode,
};

const EDGE_TYPES: EdgeTypes = {
  executionEdge: ExecutionEdge,
};

function ExecutionFlowGraph({
  definition,
  nodeExecutions = [],
  onNodeSelect,
}: ExecutionFlowGraphProps) {
  const graph = useMemo(
    () => mapExecutionDefinitionToGraph(definition, nodeExecutions),
    [definition, nodeExecutions],
  );

  const [nodes, setNodes, onNodesChange] = useNodesState<ExecutionGraphNode>(graph.nodes);

  const [edges, setEdges, onEdgesChange] = useEdgesState<ExecutionGraphEdge>(graph.edges);

  useEffect(() => {
    setNodes((currentNodes) =>
      graph.nodes.map((nextNode) => {
        const currentNode = currentNodes.find((node) => node.id === nextNode.id);

        if (!currentNode) {
          return nextNode;
        }

        return {
          ...nextNode,
          position: currentNode.position,
        };
      }),
    );

    setEdges(graph.edges);
  }, [graph, setEdges, setNodes]);

  if (nodes.length === 0) {
    return (
      <section className={styles.executionGraph__empty}>
        <Typography as="strong">No workflow graph available.</Typography>

        <Typography as="span">
          The execution definition does not contain renderable workflow nodes.
        </Typography>
      </section>
    );
  }

  return (
    <section className={styles.executionGraph__section}>
      <header className={styles.executionGraph__header}>
        <Box>
          <Typography as="p" className={styles.executionGraph__eyebrow}>
            Workflow topology
          </Typography>

          <Typography as="h2">Execution graph</Typography>

          <Typography as="span">
            Select a node to inspect its runtime details. Dragging changes only the local dashboard
            layout.
          </Typography>
        </Box>

        <Box className={styles.executionGraph__summary}>
          <Typography as="span">{nodes.length} nodes</Typography>

          <Typography as="span">{edges.length} edges</Typography>

          <Typography as="span">Runtime status</Typography>
        </Box>
      </header>

      {/* React Flow measures this exact canvas element to calculate its viewport. */}
      <div className={styles.executionGraph__canvas}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={NODE_TYPES}
          edgeTypes={EDGE_TYPES}
          nodeExtent={graph.nodeExtent}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeClick={(_event, node) => {
            onNodeSelect?.(node.id);
          }}
          onPaneClick={() => {
            onNodeSelect?.(null);
          }}
          nodesDraggable
          nodesConnectable={false}
          elementsSelectable
          deleteKeyCode={null}
          fitView
          minZoom={0.25}
          maxZoom={1.8}
          fitViewOptions={{
            padding: 0.2,
          }}
        >
          <Background gap={22} size={1} />

          <Controls showInteractive={false} />
        </ReactFlow>
      </div>
    </section>
  );
}

export function ExecutionGraph({
  executionId,
  definition,
  pollingEnabled,
  companyId,
}: ExecutionGraphProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const nodesQuery = useExecutionNodesQuery(
    executionId,
    { limit: RUNTIME_NODE_PAGE_SIZE },
    { pollingEnabled, companyId },
  );
  const nodeExecutions = useMemo(() => nodesQuery.data?.items ?? [], [nodesQuery.data?.items]);
  const selectedNodeExecution = useMemo(() => {
    if (!selectedNodeId) {
      return null;
    }

    return (
      nodeExecutions
        .filter((nodeExecution) => nodeExecution.nodeId === selectedNodeId)
        .sort((first, second) => second.attempt - first.attempt)[0] ?? null
    );
  }, [nodeExecutions, selectedNodeId]);

  return (
    <>
      <ExecutionFlowGraph
        definition={definition}
        nodeExecutions={nodeExecutions}
        onNodeSelect={setSelectedNodeId}
      />
      <ExecutionNodeDetail selectedNodeId={selectedNodeId} nodeExecution={selectedNodeExecution} />
    </>
  );
}
