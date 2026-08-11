import type { ReactNode } from "react";
import { Box } from "@/components/lib/box/Box";

interface DescriptionListItemProps {
  term: ReactNode;
  children: ReactNode;
}

export function DescriptionListItem({ term, children }: DescriptionListItemProps) {
  return (
    <Box>
      <dt>{term}</dt>
      <dd>{children}</dd>
    </Box>
  );
}
