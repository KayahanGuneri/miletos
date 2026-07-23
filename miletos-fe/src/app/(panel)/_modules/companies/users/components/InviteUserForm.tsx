"use client";

import { type FormEvent, useState } from "react";
import Icon from "@/app/(panel)/_modules/companies/components/Icon";
import { INVITABLE_USER_ROLES } from "@/shared/session/permissions/session-permissions";
import { type UserRole } from "@/shared/session/types/session-user-types";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import { useInviteUserMutation } from "@/app/(panel)/_modules/companies/users/query/useInviteUserMutation";
import styles from "./InviteUserForm.module.css";
import { Box } from "@/components/lib/box/Box";
import { Typography } from "@/components/lib/typography/Typography";
import { companyUsersMessages } from "../messages/company-users-messages";
import { formatCompanyUserLabel } from "../utils/company-user-utils";

enum InvitableUserRole {
  Admin = "ADMIN",
  Moderator = "MOD",
  User = "USER",
}

interface InviteUserFormState {
  clientError: string | null;
  email: string;
  firstName: string;
  lastName: string;
  role: InvitableUserRole;
}

const INITIAL_INVITE_USER_FORM: InviteUserFormState = {
  clientError: null,
  email: "",
  firstName: "",
  lastName: "",
  role: InvitableUserRole.User,
};

const ROLE_DESCRIPTIONS: Record<InvitableUserRole, string> = {
  [InvitableUserRole.Admin]: "Can manage users and company-level administration.",
  [InvitableUserRole.Moderator]: "Can support moderated operational workflows.",
  [InvitableUserRole.User]: "Receives standard access to the company workspace.",
};

interface InviteUserFormProps {
  companyId: number;
  isAllowed: boolean;
}

export function InviteUserForm({ companyId, isAllowed }: InviteUserFormProps) {
  const inviteUserMutation = useInviteUserMutation();
  const [form, setForm] = useState(INITIAL_INVITE_USER_FORM);

  const isSubmitting = inviteUserMutation.isPending;
  const fieldErrors = inviteUserMutation.error?.fieldErrors ?? {};
  const invitedUser = inviteUserMutation.data;

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setForm((current) => ({ ...current, clientError: null }));

    if (!isAllowed) {
      setForm((current) => ({ ...current, clientError: companyUsersMessages.invite.forbidden }));
      return;
    }

    if (!Number.isSafeInteger(companyId) || companyId <= 0) {
      setForm((current) => ({
        ...current,
        clientError: companyUsersMessages.invite.missingCompany,
      }));
      return;
    }

    inviteUserMutation.mutate(
      {
        companyId,
        email: form.email.trim(),
        firstName: form.firstName.trim(),
        lastName: form.lastName.trim(),
        role: form.role as UserRole,
      },
      {
        onSuccess: () => {
          setForm(INITIAL_INVITE_USER_FORM);
        },
      },
    );
  }

  return (
    <section className={styles.inviteUserForm}>
      <Box className={styles.inviteUserForm__intro}>
        <span className={styles.inviteUserForm__introIcon}>
          <Icon name="plus" />
        </span>
        <Box>
          <Typography as="p" className={styles.inviteUserForm__eyebrow}>
            New company member
          </Typography>
          <Typography as="h2" className={styles.inviteUserForm__title}>
            Invite user
          </Typography>
          <Typography as="p" className={styles.inviteUserForm__description}>
            Create an invitation and assign the role that matches the user&apos;s responsibilities.
          </Typography>
        </Box>
      </Box>

      {!isAllowed ? (
        <Box className={styles.inviteUserForm__error} role="alert">
          {companyUsersMessages.invite.forbidden}
        </Box>
      ) : null}
      {form.clientError ? (
        <Box className={styles.inviteUserForm__error} role="alert">
          {form.clientError}
        </Box>
      ) : null}
      {inviteUserMutation.error ? (
        <Box className={styles.inviteUserForm__error} role="alert">
          {inviteUserMutation.error.message}
        </Box>
      ) : null}
      {inviteUserMutation.isSuccess && invitedUser ? (
        <Box className={styles.inviteUserForm__success} role="status">
          Invitation created for {invitedUser.email}. {invitedUser.status} ·{" "}
          {invitedUser.onboardingStatus}
        </Box>
      ) : null}

      <form onSubmit={handleSubmit} noValidate>
        <Box className={styles.inviteUserForm__grid}>
          <Box className={`${styles.inviteUserForm__field} ${styles.inviteUserForm__fieldFull}`}>
            <label className={styles.inviteUserForm__label} htmlFor="inviteEmail">
              <Icon name="mail" size={16} />
              Email address
            </label>
            <input
              className={styles.inviteUserForm__input}
              id="inviteEmail"
              name="email"
              type="email"
              autoComplete="email"
              placeholder="name@company.com"
              value={form.email}
              disabled={isSubmitting || !isAllowed}
              onChange={(event) =>
                setForm((current) => ({ ...current, email: event.target.value }))
              }
            />
            {fieldErrors.email ? (
              <Typography as="p" className={styles.inviteUserForm__fieldError} role="alert">
                {fieldErrors.email}
              </Typography>
            ) : null}
          </Box>

          <Box className={styles.inviteUserForm__field}>
            <label className={styles.inviteUserForm__label} htmlFor="inviteFirstName">
              <Icon name="user" size={16} />
              First name
            </label>
            <input
              className={styles.inviteUserForm__input}
              id="inviteFirstName"
              name="firstName"
              type="text"
              autoComplete="given-name"
              placeholder="First name"
              value={form.firstName}
              disabled={isSubmitting || !isAllowed}
              onChange={(event) =>
                setForm((current) => ({ ...current, firstName: event.target.value }))
              }
            />
            {fieldErrors.firstName ? (
              <Typography as="p" className={styles.inviteUserForm__fieldError} role="alert">
                {fieldErrors.firstName}
              </Typography>
            ) : null}
          </Box>

          <Box className={styles.inviteUserForm__field}>
            <label className={styles.inviteUserForm__label} htmlFor="inviteLastName">
              <Icon name="user" size={16} />
              Last name
            </label>
            <input
              className={styles.inviteUserForm__input}
              id="inviteLastName"
              name="lastName"
              type="text"
              autoComplete="family-name"
              placeholder="Last name"
              value={form.lastName}
              disabled={isSubmitting || !isAllowed}
              onChange={(event) =>
                setForm((current) => ({ ...current, lastName: event.target.value }))
              }
            />
            {fieldErrors.lastName ? (
              <Typography as="p" className={styles.inviteUserForm__fieldError} role="alert">
                {fieldErrors.lastName}
              </Typography>
            ) : null}
          </Box>

          <Box className={`${styles.inviteUserForm__field} ${styles.inviteUserForm__fieldFull}`}>
            <label className={styles.inviteUserForm__label} htmlFor="inviteRole">
              <Icon name="shield" size={16} />
              Role
            </label>
            <select
              className={styles.inviteUserForm__select}
              id="inviteRole"
              name="role"
              value={form.role}
              disabled={isSubmitting || !isAllowed}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  role: event.target.value as InvitableUserRole,
                }))
              }
            >
              {INVITABLE_USER_ROLES.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </select>
            <Box className={styles.inviteUserForm__roleHint}>
              <Icon name="shield" size={17} />
              <span>
                <strong>{formatCompanyUserLabel(form.role)}</strong>
                <small>{ROLE_DESCRIPTIONS[form.role]}</small>
              </span>
            </Box>
            {fieldErrors.role ? (
              <Typography as="p" className={styles.inviteUserForm__fieldError} role="alert">
                {fieldErrors.role}
              </Typography>
            ) : null}
          </Box>
        </Box>

        <Box className={styles.inviteUserForm__actions}>
          <span className={styles.inviteUserForm__companyContext}>
            Invitation will be scoped to company <strong>{String(companyId).slice(0, 12)}…</strong>
          </span>
          <Button
            className={styles.inviteUserForm__submit}
            type="submit"
            variant={ButtonVariant.Primary}
            disabled={isSubmitting || !isAllowed}
          >
            <Icon name="send" />
            {isSubmitting
              ? companyUsersMessages.invite.submitting
              : companyUsersMessages.invite.submit}
          </Button>
        </Box>
      </form>
    </section>
  );
}
