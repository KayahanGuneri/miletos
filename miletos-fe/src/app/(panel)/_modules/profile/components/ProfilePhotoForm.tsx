"use client";

import { type ChangeEvent, type FormEvent, useRef, useState } from "react";
import { ValidationError } from "yup";
import Button, { ButtonVariant } from "@/components/lib/button/Button";
import ProfileIcon from "./ProfileIcon";
import { useUpdateProfilePhotoMutation } from "../query/useUpdateProfilePhotoMutation";
import { profileMessages } from "../messages/profile-messages";
import { profilePhotoSchema, SUPPORTED_PROFILE_PHOTO_TYPES } from "../schema/profile-schemas";
import styles from "../ui/ProfilePagePanel.module.css";

interface ProfilePhotoFormProps {
  canUpdate: boolean;
}

interface ProfilePhotoFormState {
  clientError: string | null;
  file: File | null;
}

const INITIAL_PHOTO_FORM: ProfilePhotoFormState = { clientError: null, file: null };

const ProfilePhotoForm = ({ canUpdate }: ProfilePhotoFormProps) => {
  const mutation = useUpdateProfilePhotoMutation();
  const inputRef = useRef<HTMLInputElement | null>(null);
  const [formState, setFormState] = useState(INITIAL_PHOTO_FORM);
  const fieldErrors = mutation.error?.fieldErrors ?? {};

  const resetFileInput = () => {
    if (inputRef.current) {
      inputRef.current.value = "";
    }
  };

  const validateFile = async (file: File | null) => {
    try {
      await profilePhotoSchema.validate(file);
      return null;
    } catch (error) {
      return error instanceof ValidationError ? error.message : profileMessages.photo.required;
    }
  };

  const handleChange = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0] ?? null;
    mutation.reset();
    const clientError = file ? await validateFile(file) : null;

    setFormState({ clientError, file: clientError ? null : file });
    if (clientError) {
      resetFileInput();
    }
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();

    if (!canUpdate) {
      setFormState((current) => ({ ...current, clientError: profileMessages.photo.forbidden }));
      return;
    }

    const clientError = await validateFile(formState.file);
    if (clientError || !formState.file) {
      setFormState((current) => ({ ...current, clientError }));
      return;
    }

    mutation.mutate(formState.file, {
      onSuccess: () => {
        setFormState(INITIAL_PHOTO_FORM);
        resetFileInput();
      },
    });
  };

  return (
    <article className={styles.profilePage__card}>
      <div className={styles.profilePage__cardHeader}>
        <span className={styles.profilePage__cardIcon}>
          <ProfileIcon name="camera" />
        </span>
        <div>
          <p className={styles.profilePage__cardEyebrow}>{profileMessages.photo.eyebrow}</p>
          <h2 className={styles.profilePage__cardTitle}>{profileMessages.photo.title}</h2>
        </div>
      </div>

      {!canUpdate && (
        <div className={styles.profilePage__error} role="alert">
          {profileMessages.photo.forbidden}
        </div>
      )}
      {formState.clientError && (
        <div className={styles.profilePage__error} role="alert">
          {formState.clientError}
        </div>
      )}
      {mutation.error && (
        <div className={styles.profilePage__error} role="alert">
          {mutation.error.message}
        </div>
      )}
      {mutation.isSuccess && (
        <div className={styles.profilePage__success} role="status">
          {profileMessages.photo.success}
        </div>
      )}

      <form className={styles.profilePage__form} onSubmit={handleSubmit} noValidate>
        <div className={styles.profilePage__uploadZone}>
          <span className={styles.profilePage__uploadIcon}>
            <ProfileIcon name="upload" size={24} />
          </span>
          <div className={styles.profilePage__uploadCopy}>
            <strong>{formState.file?.name ?? profileMessages.photo.chooseFile}</strong>
            <span>{profileMessages.photo.supportedTypes}</span>
          </div>
          <label
            aria-disabled={!canUpdate || mutation.isPending}
            className={[
              styles.profilePage__fileButton,
              !canUpdate || mutation.isPending ? styles.profilePage__fileButtonDisabled : "",
            ]
              .filter(Boolean)
              .join(" ")}
            htmlFor="profilePhoto"
          >
            {profileMessages.photo.browse}
          </label>
          <input
            accept={SUPPORTED_PROFILE_PHOTO_TYPES.join(",")}
            className={styles.profilePage__fileInput}
            disabled={!canUpdate || mutation.isPending}
            id="profilePhoto"
            name="profilePhoto"
            onChange={handleChange}
            ref={inputRef}
            type="file"
          />
        </div>

        {fieldErrors.base64Content && (
          <p className={styles.profilePage__fieldError}>{fieldErrors.base64Content}</p>
        )}
        {fieldErrors.contentType && (
          <p className={styles.profilePage__fieldError}>{fieldErrors.contentType}</p>
        )}
        {fieldErrors.originalFilename && (
          <p className={styles.profilePage__fieldError}>{fieldErrors.originalFilename}</p>
        )}

        <Button
          className={styles.profilePage__submit}
          disabled={!canUpdate || mutation.isPending}
          type="submit"
          variant={ButtonVariant.Primary}
        >
          <ProfileIcon name="camera" size={18} />
          {mutation.isPending ? profileMessages.photo.submitting : profileMessages.photo.submit}
        </Button>
      </form>
    </article>
  );
};

export default ProfilePhotoForm;
