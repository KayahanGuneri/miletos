-- +goose Up

ALTER TABLE workflow_runtime.outbox_messages
    ADD COLUMN available_at TIMESTAMPTZ;

UPDATE workflow_runtime.outbox_messages
SET available_at = created_at;

ALTER TABLE workflow_runtime.outbox_messages
    ALTER COLUMN available_at SET NOT NULL,
    ADD CONSTRAINT outbox_messages_available_time
        CHECK (available_at >= created_at);

CREATE INDEX outbox_messages_publishable_pending_idx
    ON workflow_runtime.outbox_messages (
        available_at,
        created_at,
        message_id
    )
    WHERE publication_state = 'PENDING';

ALTER TABLE workflow_runtime.async_worker_results
    ADD COLUMN technical_detail TEXT,
    ADD CONSTRAINT async_worker_results_technical_detail
        CHECK (
            technical_detail IS NULL
            OR (
                btrim(technical_detail) <> ''
                AND char_length(technical_detail) <= 8000
            )
        );

-- +goose Down

DROP INDEX workflow_runtime.outbox_messages_publishable_pending_idx;

ALTER TABLE workflow_runtime.async_worker_results
    DROP CONSTRAINT async_worker_results_technical_detail;

ALTER TABLE workflow_runtime.outbox_messages
    DROP CONSTRAINT outbox_messages_available_time;

ALTER TABLE workflow_runtime.async_worker_results
    DROP COLUMN technical_detail;

ALTER TABLE workflow_runtime.outbox_messages
    DROP COLUMN available_at;
