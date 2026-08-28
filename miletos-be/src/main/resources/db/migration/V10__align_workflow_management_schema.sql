ALTER TABLE workflows
    ADD COLUMN created_by_user_id BIGINT NULL,
    ADD COLUMN updated_by_user_id BIGINT NULL;

UPDATE workflows w
SET created_by_user_id = u.id
FROM public."user" u
WHERE LOWER(BTRIM(u.email)) = LOWER(BTRIM(w.created_by_email));

UPDATE workflows w
SET updated_by_user_id = u.id
FROM public."user" u
WHERE LOWER(BTRIM(u.email)) = LOWER(BTRIM(w.updated_by_email));

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM workflows
        WHERE created_by_user_id IS NULL
           OR updated_by_user_id IS NULL
    ) THEN
        RAISE EXCEPTION 'Workflow user backfill failed';
    END IF;
END
$$;

ALTER TABLE workflows
    ALTER COLUMN created_by_user_id SET NOT NULL,
    ALTER COLUMN updated_by_user_id SET NOT NULL;

ALTER TABLE workflows
    ADD CONSTRAINT fk_workflows_created_by_user
        FOREIGN KEY (created_by_user_id)
        REFERENCES public."user" (id),
    ADD CONSTRAINT fk_workflows_updated_by_user
        FOREIGN KEY (updated_by_user_id)
        REFERENCES public."user" (id);

ALTER TABLE workflows
    DROP CONSTRAINT IF EXISTS uk_workflows_company_normalized_name,
    DROP CONSTRAINT IF EXISTS ck_workflows_normalized_name;

ALTER TABLE workflows
    DROP COLUMN normalized_name,
    DROP COLUMN created_by_email,
    DROP COLUMN updated_by_email;

CREATE INDEX IF NOT EXISTS idx_workflows_company_status_updated
    ON workflows (company_id, status, updated_at DESC, id DESC);
