import { CompletePasswordForm } from "@/app/(auth)/_modules/complete-password";

interface CompletePasswordPageProps {
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

export default async function CompletePasswordPage({ searchParams }: CompletePasswordPageProps) {
  const params = await searchParams;
  const token = resolveToken(params.token);

  return (
    <main className="auth-page">
      <section className="auth-page__panel" aria-label="Complete password">
        <CompletePasswordForm token={token} />
      </section>
    </main>
  );
}
