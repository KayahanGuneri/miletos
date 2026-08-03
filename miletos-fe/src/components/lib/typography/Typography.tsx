import { createElement, type ComponentPropsWithoutRef } from "react";

type TypographyElement =
  "h1" | "h2" | "h3" | "h4" | "h5" | "h6" | "p" | "span" | "strong" | "small";

type TypographyProps<Element extends TypographyElement> = {
  as: Element;
} & ComponentPropsWithoutRef<Element>;

export function Typography<Element extends TypographyElement>({
  as,
  ...props
}: TypographyProps<Element>) {
  return createElement(as, props);
}
