import type { AriaRole, ReactNode } from "react";

export interface BoxProps {
  children?: ReactNode;
  className?: string;
  role?: AriaRole;
  title?: string;
}

export const Box = ({ children, className, role, title }: BoxProps) => (
  <div className={className} role={role} title={title}>
    {children}
  </div>
);
