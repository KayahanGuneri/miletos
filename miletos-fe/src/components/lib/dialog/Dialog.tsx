"use client";

import { useEffect, type ReactNode } from "react";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import styles from "./Dialog.module.css";

interface DialogProps {
  title: string;
  description?: string;
  children: ReactNode;
  footer: ReactNode;
  onClose: () => void;
}

export function Dialog({ title, description, children, footer, onClose }: DialogProps) {
  useEffect(() => {
    function closeOnEscape(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
      }
    }

    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [onClose]);

  return (
    <Box
      className={styles.dialog__backdrop}
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) {
          onClose();
        }
      }}
    >
      <section className={styles.dialog__panel} role="dialog" aria-modal="true" aria-label={title}>
        <header className={styles.dialog__header}>
          <Typography as="h2">{title}</Typography>
          {description ? <Typography as="p">{description}</Typography> : null}
        </header>
        <Box className={styles.dialog__body}>{children}</Box>
        <footer className={styles.dialog__footer}>{footer}</footer>
      </section>
    </Box>
  );
}
