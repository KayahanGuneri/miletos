import { Typography } from "@/components/lib/typography/Typography";

interface JsonDataSectionProps {
  label: string;
  value?: unknown;
  emptyMessage: string;
}

export function JsonDataSection({ label, value, emptyMessage }: JsonDataSectionProps) {
  const hasValue =
    value !== undefined &&
    value !== null &&
    (!Array.isArray(value) || value.length > 0) &&
    (typeof value !== "object" || Array.isArray(value) || Object.keys(value).length > 0);

  return (
    <article>
      <Typography as="span">{label}</Typography>

      {hasValue ? (
        <pre>{JSON.stringify(value, null, 2)}</pre>
      ) : (
        <Typography as="p">{emptyMessage}</Typography>
      )}
    </article>
  );
}
