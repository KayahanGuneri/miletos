import { ExecutionDetailPage } from "@/app/(panel)/_modules/dashboard";

interface ExecutionDetailRouteProps {
  params: Promise<{
    executionId: string;
  }>;
}

export default async function ExecutionDetailRoute({
  params,
}: ExecutionDetailRouteProps) {
  const { executionId } = await params;

  return <ExecutionDetailPage executionId={executionId} />;
}
