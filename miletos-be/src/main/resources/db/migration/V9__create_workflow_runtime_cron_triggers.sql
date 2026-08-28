CREATE SCHEMA IF NOT EXISTS workflow_runtime;

CREATE TABLE workflow_runtime.cron_trigger_bindings (
    trigger_id VARCHAR(128) PRIMARY KEY,
    company_id VARCHAR(128) NOT NULL,
    workflow_id VARCHAR(128) NOT NULL,
    workflow_revision BIGINT NOT NULL,
    snapshot_id VARCHAR(128) NOT NULL,
    trigger_node_id VARCHAR(128) NOT NULL,
    cron_expression VARCHAR(255) NOT NULL,
    timezone VARCHAR(128) NOT NULL,
    status VARCHAR(16) NOT NULL,
    next_fire_at TIMESTAMPTZ NOT NULL,
    last_scheduled_at TIMESTAMPTZ NULL,
    last_fired_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    disabled_at TIMESTAMPTZ NULL,
    lock_version BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT ck_cron_trigger_status CHECK (status IN ('ACTIVE', 'DISABLED')),
    CONSTRAINT ck_cron_trigger_lock_version CHECK (lock_version >= 0),
    CONSTRAINT ck_cron_trigger_revision CHECK (workflow_revision > 0)
);

CREATE INDEX idx_cron_trigger_company_workflow
    ON workflow_runtime.cron_trigger_bindings (company_id, workflow_id);
CREATE INDEX idx_cron_trigger_due_active
    ON workflow_runtime.cron_trigger_bindings (next_fire_at, trigger_id)
    WHERE status = 'ACTIVE';

CREATE TABLE workflow_runtime.cron_trigger_occurrences (
    occurrence_id VARCHAR(256) PRIMARY KEY,
    trigger_id VARCHAR(128) NOT NULL,
    company_id VARCHAR(128) NOT NULL,
    workflow_id VARCHAR(128) NOT NULL,
    snapshot_id VARCHAR(128) NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(16) NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    execution_id VARCHAR(128) NULL,
    failure_code VARCHAR(128) NULL,
    failure_message VARCHAR(512) NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    lock_version BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT uq_cron_trigger_occurrence UNIQUE (trigger_id, scheduled_at),
    CONSTRAINT ck_cron_occurrence_status CHECK (status IN ('PENDING', 'PROCESSING', 'SUCCEEDED', 'FAILED')),
    CONSTRAINT ck_cron_occurrence_attempt_count CHECK (attempt_count >= 0),
    CONSTRAINT ck_cron_occurrence_lock_version CHECK (lock_version >= 0)
);

CREATE INDEX idx_cron_occurrence_retry
    ON workflow_runtime.cron_trigger_occurrences (next_attempt_at, occurrence_id)
    WHERE status IN ('PENDING', 'FAILED');
