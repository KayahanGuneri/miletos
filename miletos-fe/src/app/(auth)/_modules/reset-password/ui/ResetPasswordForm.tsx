"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { useResetPasswordMutation } from "../model/useResetPasswordMutation";
import styles from "./ResetPasswordForm.module.css";
import { Box } from "@/components/lib/box/Box";

interface ResetPasswordFormProps {
  token: string;
}

export function ResetPasswordForm({ token }: ResetPasswordFormProps) {
  const router = useRouter();
  const resetPasswordMutation = useResetPasswordMutation();

  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [clientError, setClientError] = useState<string | null>(null);

  const isSubmitting = resetPasswordMutation.isPending;
  const fieldErrors = resetPasswordMutation.error?.fieldErrors ?? {};
  const hasToken = token.trim().length > 0;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setClientError(null);

    if (!hasToken) {
      setClientError("Reset token is missing. Please use the link from your password reset email.");
      return;
    }

    if (password !== confirmPassword) {
      setClientError("Passwords do not match.");
      return;
    }

    resetPasswordMutation.mutate(
      {
        token,
        password,
      },
      {
        onSuccess: () => {
          router.push("/auth/login");
        },
      },
    );
  }

  return (
    <form className={styles.resetPasswordForm} onSubmit={handleSubmit} noValidate>
      <header className={styles.resetPasswordForm__header}>
        <p className={styles.resetPasswordForm__eyebrow}>Miletos account recovery</p>
        <h1 className={styles.resetPasswordForm__title}>Reset password</h1>
        <p className={styles.resetPasswordForm__description}>
          Create a new password for your account using the reset link from your email.
        </p>
      </header>

      <Box className={styles.resetPasswordForm__body}>
        {!hasToken ? (
          <Box className={styles.resetPasswordForm__error} role="alert">
            Reset token is missing. Please open the full password reset link from your email.
          </Box>
        ) : null}

        {clientError ? (
          <Box className={styles.resetPasswordForm__error} role="alert">
            {clientError}
          </Box>
        ) : null}

        {resetPasswordMutation.error ? (
          <Box className={styles.resetPasswordForm__error} role="alert">
            {resetPasswordMutation.error.message}
          </Box>
        ) : null}

        {resetPasswordMutation.isSuccess ? (
          <Box className={styles.resetPasswordForm__success} role="status">
            Password reset successfully. Redirecting to login...
          </Box>
        ) : null}

        <Box className={styles.resetPasswordForm__field}>
          <label className={styles.resetPasswordForm__label} htmlFor="password">
            New password
          </label>
          <input
            className={styles.resetPasswordForm__input}
            id="password"
            name="password"
            type="password"
            autoComplete="new-password"
            value={password}
            disabled={isSubmitting || !hasToken}
            onChange={(event) => setPassword(event.target.value)}
          />
          {fieldErrors.password ? (
            <p className={styles.resetPasswordForm__fieldError}>{fieldErrors.password}</p>
          ) : null}
        </Box>

        <Box className={styles.resetPasswordForm__field}>
          <label className={styles.resetPasswordForm__label} htmlFor="confirmPassword">
            Confirm password
          </label>
          <input
            className={styles.resetPasswordForm__input}
            id="confirmPassword"
            name="confirmPassword"
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            disabled={isSubmitting || !hasToken}
            onChange={(event) => setConfirmPassword(event.target.value)}
          />
        </Box>

        <Button
          className={styles.resetPasswordForm__submit}
          type="submit"
          variant={ButtonVariant.Primary}
          disabled={isSubmitting || !hasToken}
        >
          {isSubmitting ? "Resetting password..." : "Reset password"}
        </Button>

        <Link className={styles.resetPasswordForm__secondaryAction} href="/auth/login">
          Back to login
        </Link>
      </Box>
    </form>
  );
}
