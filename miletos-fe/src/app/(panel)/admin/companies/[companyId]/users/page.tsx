import { notFound } from "next/navigation";
import { CompanyUsersPage as CompanyUsersView } from "@/app/(panel)/_modules/companies/users";

interface CompanyUsersPageProps {
  params: Promise<{
    companyId: string;
  }>;
}

export default async function CompanyUsersRoute({ params }: CompanyUsersPageProps) {
  const { companyId } = await params;

  const parsedCompanyId = Number(companyId);

  if (!Number.isSafeInteger(parsedCompanyId) || parsedCompanyId <= 0) {
    notFound();
  }

  return <CompanyUsersView companyId={parsedCompanyId} />;
}
