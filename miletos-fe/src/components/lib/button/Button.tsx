import { type ButtonHTMLAttributes, type ReactNode } from "react";
import styles from "./Button.module.css";

export enum ButtonVariant {
  Primary = "primary",
  Secondary = "secondary",
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  children: ReactNode;
  variant?: ButtonVariant;
}

function Button({
  children,
  variant = ButtonVariant.Primary,
  className = "",
  ...props
}: ButtonProps) {
  const classes = [styles.button, styles[`button--${variant}`], className]
    .filter(Boolean)
    .join(" ");

  return (
    <button className={classes} {...props}>
      {children}
    </button>
  );
}

export default Button;
