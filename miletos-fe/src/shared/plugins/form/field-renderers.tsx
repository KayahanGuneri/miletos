"use client";

import { useId, useRef, useState } from "react";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Input } from "@/components/lib/input/Input";
import { Typography } from "@/components/lib/typography/Typography";
import type { FormField, FormPrimitive } from "@/shared/plugins/form/form-schema-types";
import { formBuilderMessages } from "@/shared/plugins/messages/form-builder-messages";
import styles from "@/shared/plugins/configuration/PluginConfigurationDialog.module.css";

export interface FieldRendererProps {
  field: FormField;
  value: FormPrimitive | undefined;
  disabled: boolean;
  error?: string;
  onChange: (value: FormPrimitive | undefined) => void;
  onUploadFile?: (file: File) => Promise<string>;
  uploadPending?: boolean;
  uploadError?: boolean;
}

export function InputRenderer({ field, value, disabled, error, onChange }: FieldRendererProps) {
  if (field.dataType === "NUMBER" && field.renderType === "INPUT") {
    return (
      <label className={styles.pluginConfiguration__field}>
        <Typography as="span">{field.label}</Typography>
        <Input
          type="number"
          disabled={disabled}
          value={typeof value === "number" ? String(value) : ""}
          aria-invalid={Boolean(error)}
          min={field.min}
          max={field.max}
          placeholder={field.placeholder}
          onChange={(event) => {
            const next = event.target.value;
            onChange(next === "" ? undefined : Number(next));
          }}
        />
        {error ? (
          <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
            {error}
          </Typography>
        ) : null}
      </label>
    );
  }

  return (
    <label className={styles.pluginConfiguration__field}>
      <Typography as="span">{field.label}</Typography>
      <Input
        disabled={disabled}
        value={typeof value === "string" ? value : ""}
        aria-invalid={Boolean(error)}
        placeholder={field.placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
      {error ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {error}
        </Typography>
      ) : null}
    </label>
  );
}

export function PasswordRenderer({ field, value, disabled, error, onChange }: FieldRendererProps) {
  return (
    <label className={styles.pluginConfiguration__field}>
      <Typography as="span">{field.label}</Typography>
      <Input
        type="password"
        autoComplete="new-password"
        disabled={disabled}
        value={typeof value === "string" ? value : ""}
        aria-invalid={Boolean(error)}
        placeholder={field.placeholder ?? formBuilderMessages.passwordUnset}
        onChange={(event) => onChange(event.target.value)}
      />
      <Typography as="small">{formBuilderMessages.passwordUnset}</Typography>
      {error ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {error}
        </Typography>
      ) : null}
    </label>
  );
}

export function DropdownRenderer({ field, value, disabled, error, onChange }: FieldRendererProps) {
  const options =
    field.renderType === "DROPDOWN" && "options" in field && field.options ? field.options : [];

  return (
    <label className={styles.pluginConfiguration__field}>
      <Typography as="span">{field.label}</Typography>
      <select
        disabled={disabled}
        value={value === undefined || value === null ? "" : String(value)}
        aria-invalid={Boolean(error)}
        onChange={(event) => {
          const selected = event.target.value;
          if (selected === "") {
            onChange(undefined);
            return;
          }
          if (field.dataType === "BOOLEAN") {
            onChange(selected === "true");
            return;
          }
          if (field.dataType === "NUMBER") {
            onChange(Number(selected));
            return;
          }
          onChange(selected);
        }}
      >
        {!field.required ? <option value="">{formBuilderMessages.selectPlaceholder}</option> : null}
        {options.map((option) => (
          <option key={String(option.value)} value={String(option.value)}>
            {option.label}
          </option>
        ))}
      </select>
      {error ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {error}
        </Typography>
      ) : null}
    </label>
  );
}

export function CheckboxRenderer({ field, value, disabled, error, onChange }: FieldRendererProps) {
  return (
    <label className={styles.pluginConfiguration__field}>
      <Typography as="span">{field.label}</Typography>
      <input
        type="checkbox"
        disabled={disabled}
        checked={value === true}
        onChange={(event) => onChange(event.target.checked)}
      />
      {error ? (
        <Typography as="span" className={styles.pluginConfiguration__error} role="alert">
          {error}
        </Typography>
      ) : null}
    </label>
  );
}

export function FileUploadRenderer({
  field,
  value,
  disabled,
  error,
  onChange,
  onUploadFile,
  uploadPending,
  uploadError,
}: FieldRendererProps) {
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const [selectedFileName, setSelectedFileName] = useState("");
  const accept =
    field.dataType === "STRING" && field.renderType === "FILE_UPLOAD" ? field.accept : undefined;
  const acceptedTypes = Array.from(
    new Set(
      accept
        ?.split(",")
        .map((item) => item.trim())
        .filter((item) => /^\.[a-z0-9]+$/i.test(item))
        .map((item) => item.slice(1).toUpperCase()) ?? [],
    ),
  ).join(" / ");
  const storedFileName = typeof value === "string" ? value : "";
  const displayedFileName =
    selectedFileName || storedFileName || formBuilderMessages.fileUploadNoFileSelected;
  const helperId = `${inputId}-helper`;
  const uploadErrorId = `${inputId}-upload-error`;
  const validationErrorId = `${inputId}-validation-error`;
  const describedBy = [helperId, uploadError ? uploadErrorId : "", error ? validationErrorId : ""]
    .filter(Boolean)
    .join(" ");
  const unavailable = disabled || uploadPending || !onUploadFile;

  return (
    <Box className={styles.pluginConfiguration__field}>
      <label htmlFor={inputId}>{field.label}</label>
      <input
        ref={inputRef}
        id={inputId}
        className={styles.pluginConfiguration__fileInput}
        type="file"
        accept={accept}
        disabled={unavailable}
        aria-invalid={Boolean(error || uploadError)}
        aria-describedby={describedBy}
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = "";
          if (!file || !onUploadFile) {
            return;
          }
          setSelectedFileName(file.name);
          void onUploadFile(file)
            .then((fileName) => onChange(fileName))
            .catch(() => undefined);
        }}
      />
      <Box className={styles.pluginConfiguration__fileUploadControl}>
        <Button
          type="button"
          variant={ButtonVariant.Secondary}
          className={styles.pluginConfiguration__fileUploadButton}
          disabled={unavailable}
          onClick={() => inputRef.current?.click()}
        >
          {uploadPending
            ? formBuilderMessages.fileUploadPending
            : formBuilderMessages.fileUploadChoose}
        </Button>
        <Typography as="span" className={styles.pluginConfiguration__fileName}>
          {displayedFileName}
        </Typography>
      </Box>
      <Typography as="small" id={helperId}>
        {acceptedTypes
          ? formBuilderMessages.fileUploadAcceptedTypes(acceptedTypes)
          : formBuilderMessages.fileUploadHint}
      </Typography>
      {storedFileName && !uploadPending ? (
        <Typography as="small" role="status">
          {formBuilderMessages.fileUploaded(storedFileName)}
        </Typography>
      ) : null}
      {uploadError ? (
        <Typography
          as="span"
          id={uploadErrorId}
          className={styles.pluginConfiguration__error}
          role="alert"
        >
          {formBuilderMessages.fileUploadFailed}
        </Typography>
      ) : null}
      {error ? (
        <Typography
          as="span"
          id={validationErrorId}
          className={styles.pluginConfiguration__error}
          role="alert"
        >
          {error}
        </Typography>
      ) : null}
    </Box>
  );
}
