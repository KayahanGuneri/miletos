import type { ComponentPropsWithoutRef } from "react";

export type BoxProps = ComponentPropsWithoutRef<"div">;

export const Box = (props: BoxProps) => <div {...props} />;
