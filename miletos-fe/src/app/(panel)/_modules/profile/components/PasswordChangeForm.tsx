"use client";

import { type ChangeEvent, type FormEvent, useState } from "react";
import { ValidationError } from "yup";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { useChangePasswordMutation } from "../query/useChangePasswordMutation";
import { profileMessages } from "../messages/profile-messages";
import { passwordFormSchema } from "../schema/profile-schemas";
import { INITIAL_PASSWORD_FORM, type PasswordFormState } from "../types/profile-form-types";
import styles from "../ui/ProfilePagePanel.module.css";
import ProfileIcon from "./ProfileIcon";

interface PasswordChangeFormProps {
  canChange: boolean;
}

const PasswordChangeForm = ({ canChange }: PasswordChangeFormProps) => {
  const mutation = useChangePasswordMutation();
  const [form, setForm] = useState<PasswordFormState>(INITIAL_PASSWORD_FORM);
  const fieldErrors = mutation.error?.fieldErrors ?? {};

  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const { name, value } = event.target;
    setForm((current) => ({ ...current, clientError: null, [name]: value }));
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setForm((current) => ({ ...current, clientError: null }));

    if (!canChange) {
      setForm((current) => ({ ...current, clientError: profileMessages.password.forbidden }));
      return;
    }

    try {
      await passwordFormSchema.validate(form);
    } catch (error) {
      setForm((current) => ({
        ...current,
        clientError:
          error instanceof ValidationError
            ? error.message
            : profileMessages.common.validationFailed,
      }));
      return;
    }

    mutation.mutate(
      { currentPassword: form.currentPassword, newPassword: form.newPassword },
      { onSuccess: () => setForm(INITIAL_PASSWORD_FORM) },
    );
  };

  return (
    <article className={styles.profilePage__card}>
      <div className={styles.profilePage__cardHeader}>
        <span className={styles.profilePage__cardIcon}>
          <ProfileIcon name="key" />
        </span>
        <div>
          <p className={styles.profilePage__cardEyebrow}>{profileMessages.password.eyebrow}</p>
          <h2 className={styles.profilePage__cardTitle}>{profileMessages.password.title}</h2>
        </div>
      </div>

      {!canChange && (
        <div className={styles.profilePage__error} role="alert">
          {profileMessages.password.forbidden}
        </div>
      )}
      {form.clientError && (
        <div className={styles.profilePage__error} role="alert">
          {form.clientError}
        </div>
      )}
      {mutation.error && (
        <div className={styles.profilePage__error} role="alert">
          {mutation.error.message}
        </div>
      )}
      {mutation.isSuccess && (
        <div className={styles.profilePage__success} role="status">
          {profileMessages.password.success}
        </div>
      )}

      <form className={styles.profilePage__form} noValidate onSubmit={handleSubmit}>
        <div className={styles.profilePage__field}>
          <label className={styles.profilePage__label} htmlFor="currentPassword">
            {profileMessages.password.current}
          </label>
          <input
            autoComplete="current-password"
            className={styles.profilePage__input}
            disabled={!canChange || mutation.isPending}
            id="currentPassword"
            name="currentPassword"
            onChange={handleChange}
            type="password"
            value={form.currentPassword}
          />
          {fieldErrors.currentPassword && (
            <p className={styles.profilePage__fieldError}>{fieldErrors.currentPassword}</p>
          )}
        </div>

        <div className={styles.profilePage__fieldGroup}>
          <div className={styles.profilePage__field}>
            <label className={styles.profilePage__label} htmlFor="newPassword">
              {profileMessages.password.next}
            </label>
            <input
              autoComplete="new-password"
              className={styles.profilePage__input}
              disabled={!canChange || mutation.isPending}
              id="newPassword"
              name="newPassword"
              onChange={handleChange}
              type="password"
              value={form.newPassword}
            />
            {fieldErrors.newPassword && (
              <p className={styles.profilePage__fieldError}>{fieldErrors.newPassword}</p>
            )}
          </div>
          <div className={styles.profilePage__field}>
            <label className={styles.profilePage__label} htmlFor="confirmNewPassword">
              {profileMessages.password.confirm}
            </label>
            <input
              autoComplete="new-password"
              className={styles.profilePage__input}
              disabled={!canChange || mutation.isPending}
              id="confirmNewPassword"
              name="confirmNewPassword"
              onChange={handleChange}
              type="password"
              value={form.confirmNewPassword}
            />
          </div>
        </div>

        <Button
          className={styles.profilePage__submit}
          disabled={!canChange || mutation.isPending}
          type="submit"
          variant={ButtonVariant.Primary}
        >
          <ProfileIcon name="key" size={18} />
          {mutation.isPending
            ? profileMessages.password.submitting
            : profileMessages.password.submit}
        </Button>
      </form>
    </article>
  );
};

export default PasswordChangeForm;
