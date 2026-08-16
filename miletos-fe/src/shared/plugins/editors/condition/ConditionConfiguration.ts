import { createFormPluginDefinition } from "@/shared/plugins/form/create-form-plugin-definition";
import { validateFormConfiguration } from "@/shared/plugins/form/form-configuration";
import type { FlatFormSchema } from "@/shared/plugins/form/form-schema-types";
import { pluginMessages } from "@/shared/plugins/messages/plugin-messages";

const messages = pluginMessages.condition;
const conditionFieldPathPattern = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/;
const conditionDecimalPattern = /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/;
const relationalConditionOperators = new Set<string>([
  "GREATER_THAN",
  "GREATER_THAN_OR_EQUAL",
  "LESS_THAN",
  "LESS_THAN_OR_EQUAL",
]);

function isFiniteConditionDecimal(value: string) {
  return conditionDecimalPattern.test(value) && Number.isFinite(Number(value));
}

export const CONDITION_OPERATORS = [
  "EQUALS",
  "NOT_EQUALS",
  "GREATER_THAN",
  "GREATER_THAN_OR_EQUAL",
  "LESS_THAN",
  "LESS_THAN_OR_EQUAL",
] as const;

const CONDITION_OPERATOR_LABELS: Record<(typeof CONDITION_OPERATORS)[number], string> = {
  EQUALS: "=",
  NOT_EQUALS: "!=",
  GREATER_THAN: ">",
  GREATER_THAN_OR_EQUAL: ">=",
  LESS_THAN: "<",
  LESS_THAN_OR_EQUAL: "<=",
};

export const conditionFormSchema: FlatFormSchema = {
  fields: [
    {
      key: "field",
      label: messages.fieldLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      placeholder: messages.fieldPlaceholder,
    },
    {
      key: "operator",
      label: messages.operatorLabel,
      dataType: "STRING",
      renderType: "DROPDOWN",
      required: true,
      defaultValue: "EQUALS",
      options: CONDITION_OPERATORS.map((operator) => ({
        label: CONDITION_OPERATOR_LABELS[operator],
        value: operator,
      })),
    },
    {
      key: "value",
      label: messages.valueLabel,
      dataType: "STRING",
      renderType: "INPUT",
      required: true,
      placeholder: messages.valuePlaceholder,
    },
  ],
};

function createConditionConfigurationDefinition(pluginType: "core.if" | "core.filter") {
  return createFormPluginDefinition({
    pluginType,
    schema: conditionFormSchema,
    validate: (values) => {
      const generic = validateFormConfiguration(conditionFormSchema, values);
      const errors = { ...generic.errors };
      const field = typeof values.field === "string" ? values.field.trim() : "";
      const operator = typeof values.operator === "string" ? values.operator.trim() : "";
      const value = typeof values.value === "string" ? values.value.trim() : "";
      if (!field) errors.field = messages.fieldRequired;
      else if (!conditionFieldPathPattern.test(field)) errors.field = messages.fieldInvalid;
      if (!CONDITION_OPERATORS.some((candidate) => candidate === operator)) {
        errors.operator = messages.operatorInvalid;
      }
      if (!value) errors.value = messages.valueRequired;
      else if (relationalConditionOperators.has(operator) && !isFiniteConditionDecimal(value)) {
        errors.value = messages.relationalValueInvalid;
      }
      return { valid: Object.keys(errors).length === 0, errors };
    },
  });
}

export const ifConfigurationDefinition = createConditionConfigurationDefinition("core.if");
export const filterConfigurationDefinition = createConditionConfigurationDefinition("core.filter");
