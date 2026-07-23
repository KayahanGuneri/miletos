import { ResetPasswordForm } from "./ResetPasswordForm";

interface ResetPasswordPageViewProps {
  token: string;
}

const ResetPasswordPageView = ({ token }: ResetPasswordPageViewProps) => (
  <main className="auth-page">
    <section className="auth-page__panel" aria-label="Reset password">
      <ResetPasswordForm token={token} />
    </section>
  </main>
);

export default ResetPasswordPageView;
