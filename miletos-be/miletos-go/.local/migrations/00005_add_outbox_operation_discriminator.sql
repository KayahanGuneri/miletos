-- +goose Up

ALTER TABLE workflow_runtime.outbox_messages
    ADD COLUMN operation_discriminator TEXT;

-- +goose StatementBegin
DO $$
DECLARE
    legacy_workflow_event_count BIGINT;
BEGIN
    SELECT COUNT(*)
    INTO legacy_workflow_event_count
    FROM workflow_runtime.outbox_messages
    WHERE operation_kind = 'WORKFLOW_EVENT';

    IF legacy_workflow_event_count <> 0 THEN
        RAISE EXCEPTION USING
            ERRCODE = '23514',
            CONSTRAINT = 'outbox_messages_node_identity',
            MESSAGE = 'migration 00005 requires manual migration of legacy workflow-event outbox rows';
    END IF;
END;
$$;
-- +goose StatementEnd

ALTER TABLE workflow_runtime.outbox_messages
    DROP CONSTRAINT outbox_messages_node_identity;

ALTER TABLE workflow_runtime.outbox_messages
    ADD CONSTRAINT outbox_messages_node_identity
        CHECK (
            (
                operation_kind = 'WORKFLOW_EVENT'
                AND node_execution_id IS NULL
                AND node_id IS NULL
                AND attempt IS NULL
                AND operation_discriminator IS NOT NULL
                AND btrim(operation_discriminator) <> ''
                AND char_length(operation_discriminator) <= 512
            )
            OR (
                operation_kind IN ('NODE_COMMAND', 'NODE_RESULT')
                AND node_execution_id IS NOT NULL
                AND btrim(node_execution_id) <> ''
                AND node_id IS NOT NULL
                AND btrim(node_id) <> ''
                AND attempt > 0
                AND operation_discriminator IS NULL
            )
        );

-- +goose Down

ALTER TABLE workflow_runtime.outbox_messages
    DROP CONSTRAINT outbox_messages_node_identity;

ALTER TABLE workflow_runtime.outbox_messages
    ADD CONSTRAINT outbox_messages_node_identity
        CHECK (
            (
                operation_kind = 'WORKFLOW_EVENT'
                AND node_execution_id IS NULL
                AND node_id IS NULL
                AND attempt IS NULL
            )
            OR (
                operation_kind IN ('NODE_COMMAND', 'NODE_RESULT')
                AND node_execution_id IS NOT NULL
                AND btrim(node_execution_id) <> ''
                AND node_id IS NOT NULL
                AND btrim(node_id) <> ''
                AND attempt > 0
            )
        );

ALTER TABLE workflow_runtime.outbox_messages
    DROP COLUMN operation_discriminator;
