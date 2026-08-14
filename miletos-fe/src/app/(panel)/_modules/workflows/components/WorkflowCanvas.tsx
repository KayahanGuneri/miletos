"use client";

import { useEffect, useMemo, useState, type DragEvent as ReactDragEvent } from "react";
import {
  applyNodeChanges,
  Background,
  Controls,
  ReactFlow,
  type Connection,
  type Edge,
  type Node,
  type NodeTypes,
  type ReactFlowInstance,
  type XYPosition,
  useEdgesState,
  useNodesState,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Typography } from "@/components/lib/typography/Typography";
import { workflowMessages } from "@/app/(panel)/_modules/workflows/messages/workflow-messages";
import {
  type WorkflowEdge,
  type WorkflowNode,
} from "@/app/(panel)/_modules/workflows/types/workflow-interfaces";
import { type WorkflowPlugin } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import { validateWorkflowEdgeConnection } from "@/app/(panel)/_modules/workflows/utils/workflow-edge-validation";
import { createClientId } from "@/app/(panel)/_modules/workflows/utils/workflow-utils";
import {
  PLUGIN_DRAG_DATA_TYPE,
  readPluginDragData,
} from "@/shared/plugins/drag-data/plugin-drag-data";
import { WorkflowCanvasNode, type WorkflowCanvasNodeData } from "./WorkflowCanvasNode";
import styles from "../ui/WorkflowEditorPage.module.css";

interface WorkflowCanvasProps {
  workflowNodes: WorkflowNode[];
  workflowEdges: WorkflowEdge[];
  plugins: WorkflowPlugin[];
  readOnly: boolean;
  selectionLocked: boolean;
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
  onMoveNode: (nodeId: string, x: number, y: number) => void;
  onAddEdge: (edge: WorkflowEdge) => void;
  onDropPlugin: (plugin: WorkflowPlugin, position: XYPosition) => void;
  onDeleteNodes: (nodeIds: string[]) => void;
  onDeleteEdges: (edgeIds: string[]) => void;
}

const NODE_TYPES: NodeTypes = { workflowNode: WorkflowCanvasNode };
type CanvasNode = Node<WorkflowCanvasNodeData, "workflowNode">;

export function WorkflowCanvas({
  workflowNodes,
  workflowEdges,
  plugins,
  readOnly,
  selectionLocked,
  selectedNodeId,
  onSelectNode,
  onMoveNode,
  onAddEdge,
  onDropPlugin,
  onDeleteNodes,
  onDeleteEdges,
}: WorkflowCanvasProps) {
  const pluginByType = useMemo(
    () => new Map(plugins.map((plugin) => [plugin.type, plugin])),
    [plugins],
  );
  const mappedNodes = useMemo<CanvasNode[]>(
    () =>
      workflowNodes.map((node) => {
        const plugin = pluginByType.get(node.pluginType);
        return {
          id: node.nodeId,
          type: "workflowNode",
          position: node.position,
          selected: node.nodeId === selectedNodeId,
          data: {
            label: node.displayName?.trim() || plugin?.displayName || node.pluginType,
            pluginType: node.pluginType,
            pluginVersion: node.pluginVersion,
            plugin,
            readOnly,
          },
        };
      }),
    [pluginByType, readOnly, selectedNodeId, workflowNodes],
  );
  const mappedEdges = useMemo<Edge[]>(
    () =>
      workflowEdges.map((edge) => ({
        id: edge.edgeId,
        source: edge.sourceNodeId,
        sourceHandle: edge.sourceOutputPort,
        target: edge.targetNodeId,
        targetHandle: edge.targetInputPort,
      })),
    [workflowEdges],
  );
  const [nodes, setNodes, onNodesChange] = useNodesState(mappedNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(mappedEdges);
  const [flowInstance, setFlowInstance] = useState<ReactFlowInstance<CanvasNode, Edge> | null>(
    null,
  );
  const [dropActive, setDropActive] = useState(false);

  useEffect(() => setNodes(mappedNodes), [mappedNodes, setNodes]);
  useEffect(() => setEdges(mappedEdges), [mappedEdges, setEdges]);

  function connect(connection: Connection) {
    if (readOnly) {
      return;
    }
    const failure = validateWorkflowEdgeConnection({
      connection,
      nodes: workflowNodes,
      edges: workflowEdges,
      plugins,
    });
    if (failure) {
      return;
    }
    onAddEdge({
      edgeId: createClientId("edge"),
      sourceNodeId: connection.source!,
      sourceOutputPort: connection.sourceHandle!,
      targetNodeId: connection.target!,
      targetInputPort: connection.targetHandle!,
    });
  }

  function handleDragOver(event: ReactDragEvent<HTMLDivElement>) {
    if (
      readOnly ||
      selectionLocked ||
      !Array.from(event.dataTransfer.types).includes(PLUGIN_DRAG_DATA_TYPE)
    ) {
      return;
    }
    event.preventDefault();
    event.dataTransfer.dropEffect = "copy";
    setDropActive(true);
  }

  function handleDrop(event: ReactDragEvent<HTMLDivElement>) {
    setDropActive(false);
    if (readOnly || selectionLocked || !flowInstance) {
      return;
    }

    const dragData = readPluginDragData(event.dataTransfer);
    if (!dragData) {
      return;
    }
    event.preventDefault();

    const plugin = plugins.find((candidate) => candidate.type === dragData.pluginType);
    if (!plugin) {
      return;
    }

    onDropPlugin(plugin, flowInstance.screenToFlowPosition({ x: event.clientX, y: event.clientY }));
  }

  return (
    <section className={styles.workflowEditor__canvasPanel}>
      <header className={styles.workflowEditor__canvasHeader}>
        <Typography as="h2">{workflowMessages.canvas.title}</Typography>
        <Typography as="span">
          {workflowMessages.canvas.graphSummary(workflowNodes.length, workflowEdges.length)}
        </Typography>
      </header>
      {/* React Flow measures this exact native container to calculate its viewport. */}
      <div
        className={styles.workflowEditor__canvas}
        data-drop-active={dropActive}
        onDragOver={handleDragOver}
        onDragLeave={() => setDropActive(false)}
        onDrop={handleDrop}
      >
        <ReactFlow<CanvasNode, Edge>
          nodes={nodes}
          edges={edges}
          nodeTypes={NODE_TYPES}
          onInit={setFlowInstance}
          onNodesChange={(changes) => {
            if (!selectionLocked) {
              onNodesChange(changes);
              return;
            }

            setNodes((current) =>
              applyNodeChanges(
                changes.filter((change) => change.type !== "select"),
                current,
              ).map((node) => ({
                ...node,
                selected: node.id === selectedNodeId,
              })),
            );
          }}
          onEdgesChange={onEdgesChange}
          onConnect={connect}
          onNodeClick={(_event, node) => onSelectNode(node.id)}
          onPaneClick={() => onSelectNode(null)}
          onNodeDragStop={(_event, node) => onMoveNode(node.id, node.position.x, node.position.y)}
          onNodesDelete={(deleted) => onDeleteNodes(deleted.map((node) => node.id))}
          onEdgesDelete={(deleted) => onDeleteEdges(deleted.map((edge) => edge.id))}
          nodesDraggable={!readOnly}
          nodesConnectable={!readOnly}
          elementsSelectable
          deleteKeyCode={readOnly ? null : ["Backspace", "Delete"]}
          fitView
          minZoom={0.25}
          maxZoom={1.8}
        >
          <Background gap={22} size={1} />
          <Controls showInteractive={!readOnly} />
        </ReactFlow>
      </div>
    </section>
  );
}
