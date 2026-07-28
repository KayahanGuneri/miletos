-- +goose Up

CREATE TABLE workflow_runtime.partial_recovery_requests (
    company_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    source_workflow_execution_id TEXT NOT NULL,
    recovery_workflow_execution_id TEXT NOT NULL,
    recovery_plan JSONB NOT NULL,
    preserved_node_count INTEGER NOT NULL,
    scheduled_node_count INTEGER NOT NULL,
    reset_node_count INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT partial_recovery_requests_pk
        PRIMARY KEY (company_id, idempotency_key),

    CONSTRAINT partial_recovery_requests_recovery_uk
        UNIQUE (recovery_workflow_execution_id),

    CONSTRAINT partial_recovery_requests_source_uk
        UNIQUE (company_id, source_workflow_execution_id),

    CONSTRAINT partial_recovery_requests_source_fk
        FOREIGN KEY (source_workflow_execution_id, company_id)
        REFERENCES workflow_runtime.workflow_executions (
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT partial_recovery_requests_recovery_fk
        FOREIGN KEY (recovery_workflow_execution_id, company_id)
        REFERENCES workflow_runtime.workflow_executions (
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT partial_recovery_requests_key_length
        CHECK (
            btrim(idempotency_key) <> ''
            AND char_length(idempotency_key) <= 255
        ),

    CONSTRAINT partial_recovery_requests_fingerprint_length
        CHECK (
            request_fingerprint ~ '^[0-9a-f]{64}$'
        ),

    CONSTRAINT partial_recovery_requests_distinct_execution
        CHECK (
            source_workflow_execution_id <> recovery_workflow_execution_id
        ),

    CONSTRAINT partial_recovery_requests_plan_object
        CHECK (
            jsonb_typeof(recovery_plan) = 'object'
            AND octet_length(recovery_plan::TEXT) <= 1048576
        ),

    CONSTRAINT partial_recovery_requests_node_counts
        CHECK (
            preserved_node_count >= 0
            AND scheduled_node_count > 0
            AND reset_node_count >= 0
        )
);

-- +goose Down

DROP TABLE workflow_runtime.partial_recovery_requests;
