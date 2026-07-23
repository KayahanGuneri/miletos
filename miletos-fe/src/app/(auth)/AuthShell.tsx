"use client";

import Image from "next/image";
import { usePathname } from "next/navigation";
import { type ReactNode } from "react";
import { Box } from "@/components/lib/box/Box";
import styles from "./auth-layout.module.css";

interface AuthShellProps {
  children: ReactNode;
}

type AuthImageMode = "wide" | "portrait";

interface AuthVisualContent {
  imageSrc: string;
  imageAlt: string;
  imageMode: AuthImageMode;
  eyebrow: string;
  title: string;
  description: string;
  highlights: readonly string[];
}

const loginVisual: AuthVisualContent = {
  imageSrc: "/images/illustrations/dashboard-workflow.png",
  imageAlt: "Miletos workflow orchestration illustration",
  imageMode: "wide",
  eyebrow: "Workflow automation platform",
  title: "Turn complex operations into clear, repeatable workflows.",
  description:
    "Connect data, coordinate teams and manage every execution from one secure workspace.",
  highlights: ["Visual orchestration", "Reliable execution", "Operational visibility"],
};

const recoveryVisual: AuthVisualContent = {
  imageSrc: "/images/illustrations/auth-security.png",
  imageAlt: "Miletos secure account recovery illustration",
  imageMode: "portrait",
  eyebrow: "Secure account recovery",
  title: "Restore access without losing control.",
  description: "Recover your account through a focused and protected password recovery flow.",
  highlights: ["Verified recovery", "Protected access", "Account continuity"],
};

const resetPasswordVisual: AuthVisualContent = {
  imageSrc: "/images/illustrations/auth-security.png",
  imageAlt: "Miletos protected credential reset illustration",
  imageMode: "portrait",
  eyebrow: "Protected credentials",
  title: "Set a new credential with confidence.",
  description: "Complete the password reset process and return securely to your Miletos workspace.",
  highlights: ["Secure reset link", "Credential protection", "Controlled access"],
};

const onboardingVisual: AuthVisualContent = {
  imageSrc: "/images/illustrations/auth-security.png",
  imageAlt: "Miletos secure account onboarding illustration",
  imageMode: "portrait",
  eyebrow: "Secure onboarding",
  title: "Activate your Miletos account securely.",
  description:
    "Complete your invitation, define your first password and enter your company workspace.",
  highlights: ["Invitation validation", "Secure activation", "Company access"],
};

const visualByPath: Record<string, AuthVisualContent> = {
  "/login": loginVisual,
  "/forgot-password": recoveryVisual,
  "/reset-password": resetPasswordVisual,
  "/complete-password": onboardingVisual,
  "/first-password": onboardingVisual,
};

function resolveVisual(pathname: string) {
  return visualByPath[pathname] ?? onboardingVisual;
}

export function AuthShell({ children }: AuthShellProps) {
  const pathname = usePathname();
  const visual = resolveVisual(pathname);

  const imageStageClassName = [
    styles.authLayout__imageStage,
    visual.imageMode === "wide"
      ? styles.authLayout__imageStageWide
      : styles.authLayout__imageStagePortrait,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <main className={styles.authLayout}>
      <section className={styles.authLayout__split}>
        <aside className={styles.authLayout__visual} aria-label="Miletos platform introduction">
          <Box className={styles.authLayout__brand}>
            <span className={styles.authLayout__brandMark}>M</span>

            <span className={styles.authLayout__brandCopy}>
              <strong>Miletos</strong>

              <small>Workflow automation</small>
            </span>
          </Box>

          <Box className={styles.authLayout__copy}>
            <p className={styles.authLayout__eyebrow}>{visual.eyebrow}</p>

            <h2 className={styles.authLayout__title}>{visual.title}</h2>

            <p className={styles.authLayout__description}>{visual.description}</p>
          </Box>

          <figure className={imageStageClassName}>
            <span className={styles.authLayout__imageGlow} aria-hidden="true" />

            <Image
              className={styles.authLayout__image}
              src={visual.imageSrc}
              alt={visual.imageAlt}
              fill
              priority
              sizes="
                (max-width: 1000px) 100vw,
                52vw
              "
            />
          </figure>

          <ul className={styles.authLayout__highlights}>
            {visual.highlights.map((highlight) => (
              <li className={styles.authLayout__highlight} key={highlight}>
                <span className={styles.authLayout__highlightIndicator} aria-hidden="true" />

                {highlight}
              </li>
            ))}
          </ul>
        </aside>

        <Box className={styles.authLayout__content}>
          <span className={styles.authLayout__contentArc} aria-hidden="true" />

          <span className={styles.authLayout__contentPattern} aria-hidden="true" />

          <span className={styles.authLayout__contentDotTop} aria-hidden="true" />

          <span className={styles.authLayout__contentDotBottom} aria-hidden="true" />

          <Box className={styles.authLayout__panel}>{children}</Box>
        </Box>
      </section>
    </main>
  );
}
