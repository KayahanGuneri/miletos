"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { useCompletePasswordMutation } from "../model/useCompletePasswordMutation";
import styles from "./CompletePasswordForm.module.css";
import { Box } from "@/components/lib/box/Box";

interface CompletePasswordFormProps {
  token: string;
}

export function CompletePasswordForm({ token }: CompletePasswordFormProps) {
  const router = useRouter();
  const completePasswordMutation = useCompletePasswordMutation();

  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [clientError, setClientError] = useState<string | null>(null);

  const isSubmitting = completePasswordMutation.isPending;
  const fieldErrors = completePasswordMutation.error?.fieldErrors ?? {};
  const hasToken = token.trim().length > 0;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setClientError(null);

    if (!hasToken) {
      setClientError("Invite token is missing. Please use the link from your invitation email.");
      return;
    }

    if (password !== confirmPassword) {
      setClientError("Passwords do not match.");
      return;
    }

    completePasswordMutation.mutate(
      {
        token,
        password,
      },
      {
        onSuccess: () => {
          router.push("/login");
        },
      },
    );
  }

  return (
    <form className={styles.completePasswordForm} onSubmit={handleSubmit} noValidate>
      <header className={styles.completePasswordForm__header}>
        <p className={styles.completePasswordForm__eyebrow}>Miletos onboarding</p>

        <h1 className={styles.completePasswordForm__title}>Set your password</h1>

        <p className={styles.completePasswordForm__description}>
          Create your first password to activate your invited account.
        </p>
      </header>

      <Box className={styles.completePasswordForm__body}>
        {!hasToken ? (
          <Box className={styles.completePasswordForm__error} role="alert">
            Invite token is missing. Please open the full invitation link from your email.
          </Box>
        ) : null}

        {clientError ? (
          <Box className={styles.completePasswordForm__error} role="alert">
            {clientError}
          </Box>
        ) : null}

        {completePasswordMutation.error ? (
          <Box className={styles.completePasswordForm__error} role="alert">
            {completePasswordMutation.error.message}
          </Box>
        ) : null}

        {completePasswordMutation.isSuccess ? (
          <Box className={styles.completePasswordForm__success} role="status">
            Password created successfully. Redirecting to login...
          </Box>
        ) : null}

        <Box className={styles.completePasswordForm__field}>
          <label className={styles.completePasswordForm__label} htmlFor="password">
            New password
          </label>

          <input
            className={styles.completePasswordForm__input}
            id="password"
            name="password"
            type="password"
            autoComplete="new-password"
            value={password}
            disabled={isSubmitting || !hasToken}
            onChange={(event) => setPassword(event.target.value)}
          />

          {fieldErrors.password ? (
            <p className={styles.completePasswordForm__fieldError}>{fieldErrors.password}</p>
          ) : null}
        </Box>

        <Box className={styles.completePasswordForm__field}>
          <label className={styles.completePasswordForm__label} htmlFor="confirmPassword">
            Confirm password
          </label>

          <input
            className={styles.completePasswordForm__input}
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
          className={styles.completePasswordForm__submit}
          type="submit"
          variant={ButtonVariant.Primary}
          disabled={isSubmitting || !hasToken}
        >
          {isSubmitting ? "Setting password..." : "Set password"}
        </Button>

        <Link className={styles.completePasswordForm__secondaryAction} href="/login">
          Back to login
        </Link>
      </Box>
    </form>
  );
}
