import { CompletePasswordForm } from "@/app/(auth)/_modules/complete-password";

interface FirstPasswordPageProps {
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

export default async function FirstPasswordPage({ searchParams }: FirstPasswordPageProps) {
  const params = await searchParams;
  const token = resolveToken(params.token);

  return (
    <main className="auth-page">
      <section className="auth-page__panel" aria-label="Complete first password">
        <CompletePasswordForm token={token} />
      </section>
    </main>
  );
}
