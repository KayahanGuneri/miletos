"use client";

import Link from "next/link";
import { type FormEvent, useState } from "react";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { useForgotPasswordMutation } from "../model/useForgotPasswordMutation";
import styles from "./ForgotPasswordForm.module.css";
import { Box } from "@/components/lib/box/Box";

export function ForgotPasswordForm() {
  const forgotPasswordMutation = useForgotPasswordMutation();
  const [email, setEmail] = useState("");

  const isSubmitting = forgotPasswordMutation.isPending;
  const fieldErrors = forgotPasswordMutation.error?.fieldErrors ?? {};

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    forgotPasswordMutation.mutate({
      email: email.trim(),
    });
  }

  return (
    <form className={styles.forgotPasswordForm} onSubmit={handleSubmit} noValidate>
      <header className={styles.forgotPasswordForm__header}>
        <p className={styles.forgotPasswordForm__eyebrow}>Miletos account recovery</p>
        <h1 className={styles.forgotPasswordForm__title}>Forgot password</h1>
        <p className={styles.forgotPasswordForm__description}>
          Enter your account email. If the account exists, we will send a password reset link.
        </p>
      </header>

      <Box className={styles.forgotPasswordForm__body}>
        {forgotPasswordMutation.error ? (
          <Box className={styles.forgotPasswordForm__error} role="alert">
            {forgotPasswordMutation.error.message}
          </Box>
        ) : null}

        {forgotPasswordMutation.isSuccess ? (
          <Box className={styles.forgotPasswordForm__success} role="status">
            If the account exists, a password reset link has been sent.
          </Box>
        ) : null}

        <Box className={styles.forgotPasswordForm__field}>
          <label className={styles.forgotPasswordForm__label} htmlFor="email">
            Email
          </label>
          <input
            className={styles.forgotPasswordForm__input}
            id="email"
            name="email"
            type="email"
            autoComplete="email"
            value={email}
            disabled={isSubmitting}
            onChange={(event) => setEmail(event.target.value)}
          />
          {fieldErrors.email ? (
            <p className={styles.forgotPasswordForm__fieldError}>{fieldErrors.email}</p>
          ) : null}
        </Box>

        <Button
          className={styles.forgotPasswordForm__submit}
          type="submit"
          variant={ButtonVariant.Primary}
          disabled={isSubmitting}
        >
          {isSubmitting ? "Sending reset link..." : "Send reset link"}
        </Button>

        <Link className={styles.forgotPasswordForm__secondaryAction} href="/auth/login">
          Back to login
        </Link>
      </Box>
    </form>
  );
}
