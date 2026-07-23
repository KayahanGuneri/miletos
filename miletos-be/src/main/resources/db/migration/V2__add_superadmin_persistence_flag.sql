ALTER TABLE app_users
    ADD COLUMN IF NOT EXISTS is_super_admin BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE app_users
SET is_super_admin = TRUE
WHERE role = 'SUPERADMIN';

CREATE INDEX IF NOT EXISTS idx_app_users_is_super_admin
    ON app_users (is_super_admin);

ALTER TABLE app_users
    ADD CONSTRAINT ck_app_users_superadmin_role_consistency
    CHECK (
        (is_super_admin = TRUE AND role = 'SUPERADMIN')
        OR
        (is_super_admin = FALSE AND role <> 'SUPERADMIN')
    );