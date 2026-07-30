"use client";

import { useMutation } from "@tanstack/react-query";
import { runWorkflow } from "../api/execution-api";
import { type RunWorkflowCommand } from "../types/execution-types";

export function useRunWorkflowMutation() {
  return useMutation({
    mutationFn: ({ request, companyId, idempotencyKey }: RunWorkflowCommand) =>
      runWorkflow(request, idempotencyKey, companyId),
  });
}
