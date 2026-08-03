import { ExecutionDetailPage } from "@/app/(panel)/_modules/dashboard";

interface ExecutionDetailRouteProps {
  params: Promise<{
    executionId: string;
  }>;
  searchParams: Promise<{
    companyId?: string;
  }>;
}

export default async function ExecutionDetailRoute({
  params,
  searchParams,
}: ExecutionDetailRouteProps) {
  const { executionId } = await params;
  const { companyId: rawCompanyId } = await searchParams;
  const parsedCompanyId = rawCompanyId ? Number(rawCompanyId) : undefined;
  const companyId =
    parsedCompanyId && Number.isSafeInteger(parsedCompanyId) && parsedCompanyId > 0
      ? parsedCompanyId
      : undefined;

  return <ExecutionDetailPage executionId={executionId} companyId={companyId} />;
}
