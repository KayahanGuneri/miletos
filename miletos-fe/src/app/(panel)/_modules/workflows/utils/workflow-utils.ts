import { type WorkflowStatus } from "@/app/(panel)/_modules/workflows/types/workflow-types";

export const WORKFLOW_STATUSES: WorkflowStatus[] = ["DRAFT", "ACTIVE", "ARCHIVED"];

export function formatWorkflowStatus(status: WorkflowStatus) {
  return status.charAt(0) + status.slice(1).toLowerCase();
}

export function formatWorkflowDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function createClientId(prefix: string) {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return `${prefix}-${crypto.randomUUID()}`;
  }
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
}
