import { type ReactNode } from "react";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import styles from "./PageShell.module.css";

interface PageShellProps {
  eyebrow: string;
  title: string;
  description: string;
  actions?: ReactNode;
  children: ReactNode;
  variant?: "default" | "workbench";
}

export function PageShell({
  eyebrow,
  title,
  description,
  actions,
  children,
  variant = "default",
}: PageShellProps) {
  return (
    <main
      className={[styles.pageShell, variant === "workbench" ? styles.pageShellWorkbench : ""]
        .filter(Boolean)
        .join(" ")}
    >
      <Box className={styles.pageShell__inner}>
        <header className={styles.pageShell__header}>
          <Box className={styles.pageShell__heading}>
            <Typography as="p" className={styles.pageShell__eyebrow}>
              {eyebrow}
            </Typography>

            <Typography as="h1" className={styles.pageShell__title}>
              {title}
            </Typography>

            <Typography as="p" className={styles.pageShell__description}>
              {description}
            </Typography>
          </Box>

          {actions ? <Box className={styles.pageShell__actions}>{actions}</Box> : null}
        </header>

        <section className={styles.pageShell__content}>{children}</section>
      </Box>
    </main>
  );
}
