import { createElement, type ComponentPropsWithoutRef } from "react";

type TypographyElement = "h1" | "h2" | "h3" | "p" | "span";

type TypographyProps<Element extends TypographyElement> = {
  as: Element;
} & ComponentPropsWithoutRef<Element>;

export function Typography<Element extends TypographyElement>({
  as,
  ...props
}: TypographyProps<Element>) {
  return createElement(as, props);
}
