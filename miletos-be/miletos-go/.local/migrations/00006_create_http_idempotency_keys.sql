-- +goose Up

CREATE TABLE workflow_runtime.http_idempotency_keys (
    company_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,

    CONSTRAINT http_idempotency_keys_pk
        PRIMARY KEY (
            company_id,
            idempotency_key
        ),

    CONSTRAINT http_idempotency_keys_execution_uk
        UNIQUE (
            workflow_execution_id,
            company_id
        ),

    CONSTRAINT http_idempotency_keys_company_not_blank
        CHECK (
            btrim(company_id) <> ''
        ),

    CONSTRAINT http_idempotency_keys_key_length
        CHECK (
            char_length(btrim(idempotency_key))
                BETWEEN 1 AND 255
        ),

    CONSTRAINT http_idempotency_keys_fingerprint_format
        CHECK (
            request_fingerprint ~ '^[0-9a-f]{64}$'
        ),

    CONSTRAINT http_idempotency_keys_execution_not_blank
        CHECK (
            btrim(workflow_execution_id) <> ''
        ),

    CONSTRAINT http_idempotency_keys_state_valid
        CHECK (
            state IN (
                'RESERVED',
                'ACCEPTED'
            )
        ),

    CONSTRAINT http_idempotency_keys_acceptance_state
        CHECK (
            (
                state = 'RESERVED'
                AND accepted_at IS NULL
            )
            OR
            (
                state = 'ACCEPTED'
                AND accepted_at IS NOT NULL
                AND accepted_at >= created_at
            )
        )
);

-- +goose Down

DROP TABLE workflow_runtime.http_idempotency_keys;