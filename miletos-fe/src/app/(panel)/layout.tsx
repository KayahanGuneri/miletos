import { type ReactNode } from "react";
import { PanelGuard } from "@/app/(panel)/_layout/PanelGuard";

interface PanelLayoutProps {
  children: ReactNode;
}

export default function PanelLayout({ children }: PanelLayoutProps) {
  return <PanelGuard>{children}</PanelGuard>;
}
