"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { useLoginMutation } from "../model/useLoginMutation";
import styles from "./LoginForm.module.css";
import { Box } from "@/components/lib/box/Box";
import Icon from "@/components/lib/icon/Icon";

export function LoginForm() {
  const router = useRouter();
  const loginMutation = useLoginMutation();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const fieldErrors = loginMutation.error?.fieldErrors ?? {};
  const isSubmitting = loginMutation.isPending;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    loginMutation.mutate(
      {
        email: email.trim(),
        password,
      },
      {
        onSuccess: () => {
          router.push("/dashboard");
        },
      },
    );
  }

  return (
    <Box className={styles.loginExperience}>
      <Box className={styles.loginExperience__securityBadge}>
        <span className={styles.loginExperience__securityIcon} aria-hidden="true">
          <Icon size={18} src="/icons/dashboard/shield.svg" />
        </span>

        <span>
          <strong>Secure workspace access</strong>
          <small>Protected authentication for Miletos operations</small>
        </span>
      </Box>

      <form className={styles.loginForm} onSubmit={handleSubmit} noValidate>
        <header className={styles.loginForm__header}>
          <p className={styles.loginForm__eyebrow}>Miletos workspace</p>

          <h1 className={styles.loginForm__title}>Sign in</h1>

          <p className={styles.loginForm__description}>
            Use your company account to access workflow operations, tenant administration and secure
            execution tools.
          </p>
        </header>

        <Box className={styles.loginForm__body}>
          {loginMutation.error ? (
            <Box className={styles.loginForm__error} role="alert">
              {loginMutation.error.message}
            </Box>
          ) : null}

          <Box className={styles.loginForm__field}>
            <label className={styles.loginForm__label} htmlFor="email">
              Email
            </label>

            <input
              className={styles.loginForm__input}
              id="email"
              name="email"
              type="email"
              autoComplete="email"
              value={email}
              disabled={isSubmitting}
              onChange={(event) => setEmail(event.target.value)}
            />

            {fieldErrors.email ? (
              <p className={styles.loginForm__fieldError}>{fieldErrors.email}</p>
            ) : null}
          </Box>

          <Box className={styles.loginForm__field}>
            <label className={styles.loginForm__label} htmlFor="password">
              Password
            </label>

            <input
              className={styles.loginForm__input}
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              value={password}
              disabled={isSubmitting}
              onChange={(event) => setPassword(event.target.value)}
            />

            {fieldErrors.password ? (
              <p className={styles.loginForm__fieldError}>{fieldErrors.password}</p>
            ) : null}
          </Box>

          <Box className={styles.loginForm__actions}>
            <Button
              className={styles.loginForm__submit}
              type="submit"
              variant={ButtonVariant.Primary}
              disabled={isSubmitting}
            >
              {isSubmitting ? "Signing in..." : "Sign in"}
            </Button>

            <Link className={styles.loginForm__secondaryAction} href="/forgot-password">
              Forgot password?
            </Link>
          </Box>
        </Box>
      </form>

      <section
        className={styles.loginExperience__features}
        aria-label="Workspace security capabilities"
      >
        <article className={styles.loginExperience__feature}>
          <span className={styles.loginExperience__featureIndicator} aria-hidden="true" />

          <span>
            <strong>Secure sessions</strong>
            <small>Protected token lifecycle</small>
          </span>
        </article>

        <article className={styles.loginExperience__feature}>
          <span className={styles.loginExperience__featureIndicator} aria-hidden="true" />

          <span>
            <strong>Role-based access</strong>
            <small>Permissions aligned by role</small>
          </span>
        </article>

        <article className={styles.loginExperience__feature}>
          <span className={styles.loginExperience__featureIndicator} aria-hidden="true" />

          <span>
            <strong>Tenant isolation</strong>
            <small>Company-scoped operations</small>
          </span>
        </article>
      </section>

      <p className={styles.loginExperience__footer}>
        Encrypted access for company administrators and workflow teams.
      </p>
    </Box>
  );
}
