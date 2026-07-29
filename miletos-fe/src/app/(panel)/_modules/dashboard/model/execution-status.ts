import { type ExecutionStatus } from "@/app/(panel)/_modules/dashboard/model/execution-types";

export const EXECUTION_STATUSES: ExecutionStatus[] = [
  "CREATED",
  "VALIDATING",
  "REJECTED",
  "QUEUED",
  "RUNNING",
  "SUCCEEDED",
  "FAILED",
  "CANCELLED",
  "TIMED_OUT",
];

const TERMINAL_EXECUTION_STATUSES = new Set<ExecutionStatus>([
  "REJECTED",
  "SUCCEEDED",
  "FAILED",
  "CANCELLED",
  "TIMED_OUT",
]);

export function formatExecutionStatus(status: string) {
  return status
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

export function isTerminalExecutionStatus(status: ExecutionStatus) {
  return TERMINAL_EXECUTION_STATUSES.has(status);
}
