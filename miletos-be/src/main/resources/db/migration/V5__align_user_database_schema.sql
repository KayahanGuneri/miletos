UPDATE app_users
SET email = normalized_email;

ALTER TABLE app_users
    DROP CONSTRAINT IF EXISTS uk_app_users_normalized_email;

ALTER TABLE app_users
    DROP COLUMN normalized_email;

ALTER TABLE app_users
    RENAME TO "user";

ALTER TABLE "user"
    RENAME CONSTRAINT fk_app_users_company TO fk_user_company;

ALTER TABLE "user"
    RENAME CONSTRAINT ck_app_users_status TO ck_user_status;

ALTER TABLE "user"
    RENAME CONSTRAINT ck_app_users_onboarding_status TO ck_user_onboarding_status;

ALTER TABLE "user"
    RENAME CONSTRAINT ck_app_users_role TO ck_user_role;

ALTER TABLE "user"
    RENAME CONSTRAINT ck_app_users_company_scope TO ck_user_company_scope;

ALTER TABLE "user"
    RENAME CONSTRAINT fk_app_users_profile_photo_file TO fk_user_profile_photo_file;

ALTER INDEX idx_app_users_company_id
    RENAME TO idx_user_company_id;

ALTER INDEX idx_app_users_role
    RENAME TO idx_user_role;

ALTER INDEX idx_app_users_status
    RENAME TO idx_user_status;

ALTER INDEX idx_app_users_is_super_admin
    RENAME TO idx_user_is_super_admin;

ALTER INDEX idx_app_users_profile_photo_file_id
    RENAME TO idx_user_profile_photo_file_id;

ALTER TABLE "user"
    ADD CONSTRAINT uk_user_email UNIQUE (email);

ALTER TABLE "user"
    ADD CONSTRAINT ck_user_email_normalized
    CHECK (email = LOWER(BTRIM(email)));
