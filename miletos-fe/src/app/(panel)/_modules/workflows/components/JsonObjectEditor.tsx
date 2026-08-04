"use client";

import { useEffect, useState } from "react";
import { Typography } from "@/components/lib/typography/Typography";
import { type JsonObject } from "@/app/(panel)/_modules/workflows/types/workflow-types";
import styles from "../ui/WorkflowEditorPage.module.css";

interface JsonObjectEditorProps {
  label: string;
  value: JsonObject;
  readOnly: boolean;
  resetKey: string | number;
  onChange: (value: JsonObject) => void;
  onValidityChange: (valid: boolean) => void;
}

function isJsonObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function JsonObjectEditorState({
  label,
  value,
  readOnly,
  onChange,
  onValidityChange,
}: JsonObjectEditorProps) {
  const [draft, setDraft] = useState(() => JSON.stringify(value, null, 2));
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    onValidityChange(true);
  }, [onValidityChange]);

  return (
    <label className={styles.workflowEditor__jsonField}>
      <Typography as="span">{label}</Typography>
      <textarea
        rows={10}
        readOnly={readOnly}
        value={draft}
        onChange={(event) => {
          const nextDraft = event.target.value;
          setDraft(nextDraft);
          try {
            const parsed: unknown = JSON.parse(nextDraft);
            if (!isJsonObject(parsed)) {
              setError("Enter a JSON object. Arrays and primitive values are not allowed.");
              onValidityChange(false);
              return;
            }
            setError(null);
            onValidityChange(true);
            onChange(parsed);
          } catch {
            setError("Enter valid JSON before saving.");
            onValidityChange(false);
          }
        }}
      />
      {error ? (
        <Typography as="span" className={styles.workflowEditor__fieldError} role="alert">
          {error}
        </Typography>
      ) : null}
    </label>
  );
}

export function JsonObjectEditor(props: JsonObjectEditorProps) {
  return <JsonObjectEditorState key={props.resetKey} {...props} />;
}
