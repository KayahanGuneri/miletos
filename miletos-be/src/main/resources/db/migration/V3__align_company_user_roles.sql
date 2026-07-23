ALTER TABLE app_users
    DROP CONSTRAINT IF EXISTS ck_app_users_superadmin_role_consistency;

ALTER TABLE app_users
    DROP CONSTRAINT IF EXISTS ck_app_users_company_scope;

ALTER TABLE app_users
    DROP CONSTRAINT IF EXISTS ck_app_users_role;

ALTER TABLE app_users
    ALTER COLUMN role DROP NOT NULL;

UPDATE app_users
SET is_super_admin = TRUE
WHERE role = 'SUPERADMIN';

UPDATE app_users
SET role = 'ADMIN'
WHERE role = 'COMPANY_ADMIN';

UPDATE app_users
SET role = 'MOD'
WHERE role = 'MOD_USER';

UPDATE app_users
SET role = NULL
WHERE is_super_admin = TRUE;

ALTER TABLE app_users
    ADD CONSTRAINT ck_app_users_role
    CHECK (
        role IS NULL
        OR role IN ('ADMIN', 'MOD', 'USER')
    );

ALTER TABLE app_users
    ADD CONSTRAINT ck_app_users_company_scope
    CHECK (
        (is_super_admin = TRUE AND company_id IS NULL AND role IS NULL)
        OR
        (is_super_admin = FALSE AND company_id IS NOT NULL AND role IS NOT NULL)
    );