ALTER TABLE auth_tokens
    DROP CONSTRAINT IF EXISTS uk_auth_tokens_token_hash;

ALTER TABLE auth_tokens
    ADD CONSTRAINT uk_auth_tokens_type_token_hash
    UNIQUE (type, token_hash);

INSERT INTO auth_tokens (
    type,
    company_id,
    user_id,
    created_by_user_id,
    token_hash,
    expires_at,
    used_at,
    created_at
)
SELECT
    'INVITE',
    company_id,
    user_id,
    created_by_user_id,
    token_hash,
    expires_at,
    used_at,
    created_at
FROM invite_tokens
ON CONFLICT (type, token_hash) DO NOTHING;

INSERT INTO auth_tokens (
    type,
    company_id,
    user_id,
    created_by_user_id,
    token_hash,
    expires_at,
    used_at,
    created_at
)
SELECT
    'PASSWORD_RESET',
    NULL,
    user_id,
    NULL,
    token_hash,
    expires_at,
    used_at,
    created_at
FROM password_reset_tokens
ON CONFLICT (type, token_hash) DO NOTHING;

DROP TABLE password_reset_tokens;
DROP TABLE invite_tokens;
