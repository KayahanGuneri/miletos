import { WorkflowEditorPage } from "@/app/(panel)/_modules/workflows";

interface WorkflowDetailRouteProps {
  params: Promise<{ workflowId: string }>;
}

export default async function WorkflowDetailRoute({ params }: WorkflowDetailRouteProps) {
  const { workflowId: rawWorkflowId } = await params;
  const workflowId = Number(rawWorkflowId);
  return <WorkflowEditorPage workflowId={workflowId} />;
}
