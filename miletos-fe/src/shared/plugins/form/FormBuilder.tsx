"use client";

import type { ComponentType } from "react";
import {
  DropdownRenderer,
  InputRenderer,
  type FieldRendererProps,
} from "@/shared/plugins/form/field-renderers";
import {
  isFormFieldVisible,
  type FlatFormSchema,
  type FormConfiguration,
  type FormFieldValue,
  type FormRenderType,
} from "@/shared/plugins/form/form-schema-types";
import styles from "@/shared/plugins/configuration/PluginConfigurationDialog.module.css";

const RENDERERS: Record<FormRenderType, ComponentType<FieldRendererProps>> = {
  INPUT: InputRenderer,
  DROPDOWN: DropdownRenderer,
};

export interface FormBuilderProps {
  schema: FlatFormSchema;
  values: FormConfiguration;
  disabled: boolean;
  validationErrors: Record<string, string>;
  onChange: (values: FormConfiguration) => void;
  onUploadFile?: (file: File) => Promise<string>;
  uploadPending?: boolean;
  uploadError?: boolean;
}

export function FormBuilder({
  schema,
  values,
  disabled,
  validationErrors,
  onChange,
  onUploadFile,
  uploadPending,
  uploadError,
}: FormBuilderProps) {
  function updateField(key: string, value: FormFieldValue | undefined) {
    if (value === undefined) {
      const next = { ...values };
      delete next[key];
      onChange(next);
      return;
    }
    onChange({ ...values, [key]: value });
  }

  return (
    <section className={styles.pluginConfiguration__formSection}>
      {schema.fields.map((field, index) => {
        if (!isFormFieldVisible(field, values)) {
          return null;
        }
        const Renderer = RENDERERS[field.renderType];
        return (
          <Renderer
            key={`${field.key}:${field.renderType}:${index}`}
            field={field}
            value={values[field.key]}
            disabled={disabled}
            error={validationErrors[field.key]}
            onChange={(value) => updateField(field.key, value)}
            onUploadFile={onUploadFile}
            uploadPending={uploadPending}
            uploadError={uploadError}
          />
        );
      })}
    </section>
  );
}
