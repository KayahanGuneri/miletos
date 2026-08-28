-- +goose Up

ALTER TABLE workflow_runtime.workflow_executions
    DROP CONSTRAINT workflow_exec_origin_valid,
    ADD CONSTRAINT workflow_exec_origin_valid CHECK (
        execution_origin IN (
            'MANUAL_DIRECT',
            'HTTP_WEBHOOK',
            'KAFKA_EVENT',
            'MESSAGE_EVENT',
            'SCHEDULED',
            'CRON',
            'BACKGROUND_SYSTEM',
            'DATA_ARRIVAL'
        )
    );

CREATE TABLE workflow_runtime.data_arrival_source_bindings (
    company_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    workflow_revision BIGINT NOT NULL,
    snapshot_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    plugin_type TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    disabled_at TIMESTAMPTZ,

    CONSTRAINT data_arrival_source_bindings_pk PRIMARY KEY (
        company_id,
        workflow_id,
        workflow_revision,
        node_id
    ),
    CONSTRAINT data_arrival_source_bindings_snapshot_fk
        FOREIGN KEY (snapshot_id, company_id)
        REFERENCES workflow_runtime.workflow_definition_snapshots (
            snapshot_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,
    CONSTRAINT data_arrival_source_bindings_identity_not_blank CHECK (
        btrim(company_id) <> ''
        AND btrim(workflow_id) <> ''
        AND btrim(snapshot_id) <> ''
        AND btrim(node_id) <> ''
        AND btrim(plugin_type) <> ''
    ),
    CONSTRAINT data_arrival_source_bindings_revision_positive
        CHECK (workflow_revision > 0),
    CONSTRAINT data_arrival_source_bindings_status_valid
        CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT data_arrival_source_bindings_disabled_shape CHECK (
        (status = 'ACTIVE' AND disabled_at IS NULL)
        OR (status = 'DISABLED' AND disabled_at IS NOT NULL)
    ),
    CONSTRAINT data_arrival_source_bindings_time_order CHECK (
        updated_at >= created_at
        AND (disabled_at IS NULL OR disabled_at >= created_at)
    )
);

CREATE INDEX data_arrival_source_bindings_active_idx
    ON workflow_runtime.data_arrival_source_bindings (
        company_id,
        workflow_id,
        workflow_revision,
        node_id
    )
    WHERE status = 'ACTIVE';

-- +goose Down

DROP TABLE workflow_runtime.data_arrival_source_bindings;

UPDATE workflow_runtime.workflow_executions
SET execution_origin = 'BACKGROUND_SYSTEM'
WHERE execution_origin = 'DATA_ARRIVAL';

ALTER TABLE workflow_runtime.workflow_executions
    DROP CONSTRAINT workflow_exec_origin_valid,
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
