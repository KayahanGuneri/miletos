import { ResetPasswordPageView } from "@/app/(auth)/_modules/reset-password";

interface ResetPasswordPageProps {
  searchParams: Promise<{
    token?: string | string[];
  }>;
}

function resolveToken(token: string | string[] | undefined) {
  if (Array.isArray(token)) {
    return token[0] ?? "";
  }

  return token ?? "";
}

export default async function ResetPasswordPage({ searchParams }: ResetPasswordPageProps) {
  const params = await searchParams;
  const token = resolveToken(params.token);

  return <ResetPasswordPageView token={token} />;
}
