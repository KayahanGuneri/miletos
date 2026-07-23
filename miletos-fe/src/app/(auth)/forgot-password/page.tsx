import { ForgotPasswordForm } from "@/app/(auth)/_modules/forgot-password";

export default function ForgotPasswordPage() {
  return (
    <main className="auth-page">
      <section className="auth-page__panel" aria-label="Forgot password">
        <ForgotPasswordForm />
      </section>
    </main>
  );
}
