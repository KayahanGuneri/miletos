export type FormDataType = "STRING" | "NUMBER" | "BOOLEAN";

export type FormRenderType = "INPUT" | "DROPDOWN" | "CHECKBOX" | "FILE_UPLOAD" | "PASSWORD";

export type FormPrimitive = string | number | boolean;

export type FormConfiguration = Record<string, FormPrimitive | undefined>;

interface FormFieldBase {
  key: string;
  label: string;
  required?: boolean;
  placeholder?: string;
  visibleWhenKey?: string;
  visibleWhenValue?: FormPrimitive;
}

export type FormField =
  | (FormFieldBase & {
      dataType: "STRING";
      renderType: "INPUT" | "PASSWORD" | "FILE_UPLOAD";
      defaultValue?: string;
      accept?: string;
    })
  | (FormFieldBase & {
      dataType: "STRING";
      renderType: "DROPDOWN";
      defaultValue?: string;
      options: ReadonlyArray<{ label: string; value: string }>;
    })
  | (FormFieldBase & {
      dataType: "NUMBER";
      renderType: "INPUT";
      defaultValue?: number;
      min?: number;
      max?: number;
    })
  | (FormFieldBase & {
      dataType: "NUMBER";
      renderType: "DROPDOWN";
      defaultValue?: number;
      options: ReadonlyArray<{ label: string; value: number }>;
    })
  | (FormFieldBase & {
      dataType: "BOOLEAN";
      renderType: "CHECKBOX" | "DROPDOWN";
      defaultValue?: boolean;
      options?: ReadonlyArray<{ label: string; value: boolean }>;
    });

export interface FlatFormSchema {
  fields: FormField[];
}

export function isFormFieldVisible(field: FormField, values: FormConfiguration): boolean {
  if (!field.visibleWhenKey) {
    return true;
  }
  return values[field.visibleWhenKey] === field.visibleWhenValue;
}
