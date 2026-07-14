import { LoginForm } from "@/app/(auth)/_modules/login";

export default function LoginPage() {
  return (
    <main className="auth-page">
      <section className="auth-page__panel" aria-label="Login">
        <LoginForm />
      </section>
    </main>
  );
}
