"use client";

import { useEffect, useMemo } from "react";
import {
  Background,
  Controls,
  ReactFlow,
  type NodeTypes,
  useEdgesState,
  useNodesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { mapExecutionDefinitionToGraph } from "@/app/(panel)/_modules/dashboard/model/execution-graph-mapper";
import {
  type ExecutionGraphEdge,
  type ExecutionGraphNode,
} from "@/app/(panel)/_modules/dashboard/model/execution-graph-types";
import { type NodeExecutionResponse } from "@/app/(panel)/_modules/dashboard/model/execution-types";
import { GenericExecutionNode } from "./GenericExecutionNode";
import styles from "./ExecutionGraph.module.css";

interface ExecutionGraphProps {
  definition: Record<string, unknown>;
  nodeExecutions?: NodeExecutionResponse[];
  onNodeSelect?: (nodeId: string | null) => void;
}

const NODE_TYPES: NodeTypes = {
  executionNode: GenericExecutionNode,
};

export function ExecutionGraph({
  definition,
  nodeExecutions = [],
  onNodeSelect,
}: ExecutionGraphProps) {
  const graph = useMemo(
    () => mapExecutionDefinitionToGraph(definition, nodeExecutions),
    [definition, nodeExecutions],
  );

  const [nodes, setNodes, onNodesChange] = useNodesState<ExecutionGraphNode>(
    graph.nodes,
  );

  const [edges, setEdges, onEdgesChange] = useEdgesState<ExecutionGraphEdge>(
    graph.edges,
  );

  useEffect(() => {
    setNodes((currentNodes) =>
      graph.nodes.map((nextNode) => {
        const currentNode = currentNodes.find(
          (node) => node.id === nextNode.id,
        );

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
        <strong>No workflow graph available.</strong>

        <span>
          The execution definition does not contain renderable workflow nodes.
        </span>
      </section>
    );
  }

  return (
    <section className={styles.executionGraph__section}>
      <header className={styles.executionGraph__header}>
        <div>
          <p className={styles.executionGraph__eyebrow}>Workflow topology</p>

          <h2>Execution graph</h2>

          <span>
            Select a node to inspect its runtime details. Dragging changes only
            the local dashboard layout.
          </span>
        </div>

        <div className={styles.executionGraph__summary}>
          <span>{nodes.length} nodes</span>

          <span>{edges.length} edges</span>

          <span>Runtime status</span>
        </div>
      </header>

      <div className={styles.executionGraph__canvas}>
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={NODE_TYPES}
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
