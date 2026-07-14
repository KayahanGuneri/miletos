export interface PasswordFormState {
  clientError: string | null;
  currentPassword: string;
  newPassword: string;
  confirmNewPassword: string;
}

export const INITIAL_PASSWORD_FORM: PasswordFormState = {
  clientError: null,
  currentPassword: "",
  newPassword: "",
  confirmNewPassword: "",
};
