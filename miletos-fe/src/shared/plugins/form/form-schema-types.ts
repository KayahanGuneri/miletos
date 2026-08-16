import type { PluginConfigurationValue } from "@/shared/plugins/contracts/plugin-configuration-types";

export type FormDataType = "STRING" | "NUMBER" | "BOOLEAN";

export type FormRenderType = "INPUT" | "DROPDOWN";

export type FormPrimitive = string | number | boolean;

export type FormFieldValue = PluginConfigurationValue;

export type FormConfiguration = Record<string, FormFieldValue | undefined>;

interface FormFieldBase {
  key: string;
  label: string;
  required?: boolean;
  placeholder?: string;
  when?: (data: FormConfiguration) => boolean;
}

export type FormField =
  | (FormFieldBase & {
      dataType: "STRING";
      renderType: "INPUT";
      inputType?: "text" | "password" | "file";
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
      renderType: "DROPDOWN";
      defaultValue?: boolean;
      options?: ReadonlyArray<{ label: string; value: boolean }>;
    });

export interface FlatFormSchema {
  fields: FormField[];
}

export function isFormFieldVisible(field: FormField, values: FormConfiguration): boolean {
  if (!field.when) {
    return true;
  }
  return field.when(values);
}
