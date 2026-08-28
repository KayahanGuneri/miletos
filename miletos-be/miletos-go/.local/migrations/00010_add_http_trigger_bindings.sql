-- +goose Up

ALTER TABLE workflow_runtime.workflow_executions
    ADD COLUMN execution_origin TEXT NOT NULL DEFAULT 'MANUAL_DIRECT',
    ADD CONSTRAINT workflow_exec_origin_valid CHECK (
        execution_origin IN (
            'MANUAL_DIRECT',
            'HTTP_WEBHOOK',
            'KAFKA_EVENT',
            'MESSAGE_EVENT',
            'SCHEDULED',
            'CRON',
            'BACKGROUND_SYSTEM'
        )
    );

CREATE TABLE workflow_runtime.http_trigger_bindings (
    trigger_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    workflow_revision BIGINT NOT NULL,
    snapshot_id TEXT NOT NULL,
    trigger_node_id TEXT NOT NULL,
    http_method TEXT NOT NULL,
    token_hash BYTEA NOT NULL,
    status TEXT NOT NULL,
    resolved_mode TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    disabled_at TIMESTAMPTZ,
    lock_version BIGINT NOT NULL DEFAULT 0,

    CONSTRAINT http_trigger_bindings_pk
        PRIMARY KEY (trigger_id),
    CONSTRAINT http_trigger_bindings_tenant_uk
        UNIQUE (trigger_id, company_id),
    CONSTRAINT http_trigger_bindings_token_uk
        UNIQUE (token_hash),
    CONSTRAINT http_trigger_bindings_snapshot_fk
        FOREIGN KEY (snapshot_id, company_id)
        REFERENCES workflow_runtime.workflow_definition_snapshots (
            snapshot_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,
    CONSTRAINT http_trigger_bindings_ids_not_blank CHECK (
        btrim(trigger_id) <> ''
        AND btrim(company_id) <> ''
        AND btrim(workflow_id) <> ''
        AND btrim(snapshot_id) <> ''
        AND btrim(trigger_node_id) <> ''
    ),
    CONSTRAINT http_trigger_bindings_revision_positive
        CHECK (workflow_revision > 0),
    CONSTRAINT http_trigger_bindings_method_valid
        CHECK (http_method IN ('GET', 'POST', 'PUT', 'PATCH', 'DELETE')),
    CONSTRAINT http_trigger_bindings_token_hash_size
        CHECK (octet_length(token_hash) = 32),
    CONSTRAINT http_trigger_bindings_status_valid
        CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT http_trigger_bindings_mode_async
        CHECK (resolved_mode = 'ASYNC'),
    CONSTRAINT http_trigger_bindings_time_order CHECK (
        updated_at >= created_at
        AND (disabled_at IS NULL OR disabled_at >= created_at)
    ),
    CONSTRAINT http_trigger_bindings_disabled_shape CHECK (
        (status = 'ACTIVE' AND disabled_at IS NULL)
        OR (status = 'DISABLED' AND disabled_at IS NOT NULL)
    ),
    CONSTRAINT http_trigger_bindings_version_nonnegative
        CHECK (lock_version >= 0)
);

CREATE INDEX http_trigger_bindings_active_token_idx
    ON workflow_runtime.http_trigger_bindings (token_hash)
    WHERE status = 'ACTIVE';

CREATE INDEX http_trigger_bindings_company_workflow_idx
    ON workflow_runtime.http_trigger_bindings (
        company_id,
        workflow_id,
        created_at DESC
    );

-- +goose Down

DROP TABLE workflow_runtime.http_trigger_bindings;

ALTER TABLE workflow_runtime.workflow_executions
    DROP CONSTRAINT workflow_exec_origin_valid,
    DROP COLUMN execution_origin;
