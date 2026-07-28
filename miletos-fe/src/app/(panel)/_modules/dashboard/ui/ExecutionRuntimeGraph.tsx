"use client";

import { useMemo, useState } from "react";
import { useExecutionNodesQuery } from "@/app/(panel)/_modules/dashboard/model/useExecutionNodesQuery";
import { ExecutionGraph } from "./ExecutionGraph";
import { ExecutionNodeDetail } from "./ExecutionNodeDetail";

interface ExecutionRuntimeGraphProps {
  executionId: string;
  definition: Record<string, unknown>;
  pollingEnabled: boolean;
}

const RUNTIME_NODE_PAGE_SIZE = 100;

export function ExecutionRuntimeGraph({
  executionId,
  definition,
  pollingEnabled,
}: ExecutionRuntimeGraphProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  const nodesQuery = useExecutionNodesQuery(
    executionId,
    {
      limit: RUNTIME_NODE_PAGE_SIZE,
    },
    {
      pollingEnabled,
    },
  );

  const nodeExecutions = useMemo(
    () => nodesQuery.data?.items ?? [],
    [nodesQuery.data?.items],
  );

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
      <ExecutionGraph
        definition={definition}
        nodeExecutions={nodeExecutions}
        onNodeSelect={setSelectedNodeId}
      />

      <ExecutionNodeDetail
        selectedNodeId={selectedNodeId}
        nodeExecution={selectedNodeExecution}
      />
    </>
  );
}
