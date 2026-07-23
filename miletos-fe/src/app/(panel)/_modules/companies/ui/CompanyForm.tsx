"use client";

import { type FormEvent, useState } from "react";
import type { CompanyStatus } from "@/app/(panel)/_modules/companies/types/company-types";
import { COMPANY_STATUSES } from "@/app/(panel)/_modules/companies/types/company-management-types";
import { Box } from "@/components/lib/box/Box";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { Typography } from "@/components/lib/typography/Typography";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import styles from "./CompanyManagementPage.module.css";

export enum CompanyFormMode {
  Create = "CREATE",
  Edit = "EDIT",
}

interface CreateCompanyFormValues {
  name: string;
}

interface EditCompanyFormValues extends CreateCompanyFormValues {
  status: CompanyStatus;
}

type CompanyFormValues =
  | ({ mode: CompanyFormMode.Create } & CreateCompanyFormValues)
  | ({ mode: CompanyFormMode.Edit } & EditCompanyFormValues);

interface CompanyFormBaseProps {
  errorMessage?: string;
  isSubmitting: boolean;
  nameError?: string;
}

interface CreateCompanyFormProps extends CompanyFormBaseProps {
  mode: CompanyFormMode.Create;
  onSubmit: (values: CreateCompanyFormValues) => Promise<void>;
}

interface EditCompanyFormProps extends CompanyFormBaseProps {
  initialValues: EditCompanyFormValues;
  mode: CompanyFormMode.Edit;
  onCancel: () => void;
  onSubmit: (values: EditCompanyFormValues) => Promise<void>;
  statusError?: string;
}

type CompanyFormProps = CreateCompanyFormProps | EditCompanyFormProps;

export function CompanyForm(props: CompanyFormProps) {
  const [values, setValues] = useState<CompanyFormValues>(() =>
    props.mode === CompanyFormMode.Create
      ? { mode: CompanyFormMode.Create, name: "" }
      : { mode: CompanyFormMode.Edit, ...props.initialValues },
  );

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    try {
      if (values.mode === CompanyFormMode.Create && props.mode === CompanyFormMode.Create) {
        await props.onSubmit({ name: values.name.trim() });
        setValues({ mode: CompanyFormMode.Create, name: "" });
      }

      if (values.mode === CompanyFormMode.Edit && props.mode === CompanyFormMode.Edit) {
        await props.onSubmit({ name: values.name.trim(), status: values.status });
      }
    } catch {
      // Mutation state renders the normalized API error next to the form.
    }
  }

  const nameId = `company-name-${props.mode.toLowerCase()}`;
  const statusId = `company-status-${props.mode.toLowerCase()}`;

  return (
    <form
      noValidate
      className={
        props.mode === CompanyFormMode.Create
          ? styles.companyManagement__form
          : styles.companyManagement__editForm
      }
      onSubmit={handleSubmit}
    >
      {props.errorMessage ? (
        <Typography as="p" className={styles.companyManagement__error} role="alert">
          {props.errorMessage}
        </Typography>
      ) : null}

      <Box className={styles.companyManagement__field}>
        <label className={styles.companyManagement__label} htmlFor={nameId}>
          Company name
        </label>

        <input
          autoComplete="organization"
          className={styles.companyManagement__input}
          disabled={props.isSubmitting}
          id={nameId}
          name="name"
          placeholder="Enter a clear tenant name"
          type="text"
          value={values.name}
          onChange={(event) => setValues((current) => ({ ...current, name: event.target.value }))}
        />

        {props.nameError ? (
          <Typography as="p" className={styles.companyManagement__fieldError} role="alert">
            {props.nameError}
          </Typography>
        ) : null}
      </Box>

      {values.mode === CompanyFormMode.Edit && props.mode === CompanyFormMode.Edit ? (
        <Box className={styles.companyManagement__field}>
          <label className={styles.companyManagement__label} htmlFor={statusId}>
            Status
          </label>

          <select
            className={styles.companyManagement__input}
            disabled={props.isSubmitting}
            id={statusId}
            name="status"
            value={values.status}
            onChange={(event) =>
              setValues((current) =>
                current.mode === CompanyFormMode.Edit
                  ? { ...current, status: event.target.value as CompanyStatus }
                  : current,
              )
            }
          >
            {COMPANY_STATUSES.map((status) => (
              <option key={status} value={status}>
                {status}
              </option>
            ))}
          </select>

          {props.statusError ? (
            <Typography as="p" className={styles.companyManagement__fieldError} role="alert">
              {props.statusError}
            </Typography>
          ) : null}
        </Box>
      ) : null}

      <Box className={styles.companyManagement__rowActions}>
        {props.mode === CompanyFormMode.Edit ? (
          <Button
            disabled={props.isSubmitting}
            type="button"
            variant={ButtonVariant.Secondary}
            onClick={props.onCancel}
          >
            Cancel
          </Button>
        ) : null}

        <Button
          className={
            props.mode === CompanyFormMode.Create ? styles.companyManagement__submit : undefined
          }
          disabled={props.isSubmitting}
          type="submit"
          variant={ButtonVariant.Primary}
        >
          <Icon name={props.mode === CompanyFormMode.Create ? "plus" : "check"} />
          {props.isSubmitting
            ? props.mode === CompanyFormMode.Create
              ? "Creating company..."
              : "Saving..."
            : props.mode === CompanyFormMode.Create
              ? "Create company"
              : "Save"}
        </Button>
      </Box>
    </form>
  );
}
