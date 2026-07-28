-- +goose Up

CREATE TABLE workflow_runtime.outbox_messages (
    message_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    node_execution_id TEXT,
    node_id TEXT,
    attempt SMALLINT,

    operation_kind TEXT NOT NULL,
    operation_key TEXT NOT NULL,
    message_type TEXT NOT NULL,
    message_version INTEGER NOT NULL,
    destination TEXT NOT NULL,
    message_key TEXT NOT NULL,
    encoded_payload BYTEA NOT NULL,

    publication_state TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    claimed_at TIMESTAMPTZ,
    claim_owner TEXT,
    published_at TIMESTAMPTZ,
    lock_version BIGINT NOT NULL DEFAULT 0,

    CONSTRAINT outbox_messages_pk
        PRIMARY KEY (message_id),

    CONSTRAINT outbox_messages_operation_uk
        UNIQUE (
            company_id,
            workflow_execution_id,
            operation_key
        ),

    CONSTRAINT outbox_messages_execution_fk
        FOREIGN KEY (
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.workflow_executions (
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT outbox_messages_node_fk
        FOREIGN KEY (
            node_execution_id,
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.node_executions (
            node_execution_id,
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT outbox_messages_id_length
        CHECK (
            btrim(message_id) <> ''
            AND char_length(message_id) <= 200
        ),

    CONSTRAINT outbox_messages_company_not_blank
        CHECK (btrim(company_id) <> ''),

    CONSTRAINT outbox_messages_execution_not_blank
        CHECK (btrim(workflow_execution_id) <> ''),

    CONSTRAINT outbox_messages_node_identity
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
        ),

    CONSTRAINT outbox_messages_operation_kind_valid
        CHECK (
            operation_kind IN (
                'WORKFLOW_EVENT',
                'NODE_COMMAND',
                'NODE_RESULT'
            )
        ),

    CONSTRAINT outbox_messages_operation_key_length
        CHECK (
            btrim(operation_key) <> ''
            AND char_length(operation_key) <= 96
        ),

    CONSTRAINT outbox_messages_type_length
        CHECK (
            btrim(message_type) <> ''
            AND char_length(message_type) <= 128
        ),

    CONSTRAINT outbox_messages_version_positive
        CHECK (message_version > 0),

    CONSTRAINT outbox_messages_destination_length
        CHECK (
            btrim(destination) <> ''
            AND char_length(destination) <= 249
        ),

    CONSTRAINT outbox_messages_key_length
        CHECK (
            btrim(message_key) <> ''
            AND char_length(message_key) <= 512
        ),

    CONSTRAINT outbox_messages_payload_size
        CHECK (
            octet_length(encoded_payload) > 0
            AND octet_length(encoded_payload) <= 16777216
        ),

    CONSTRAINT outbox_messages_state_valid
        CHECK (
            publication_state IN (
                'PENDING',
                'PUBLISHING',
                'PUBLISHED'
            )
        ),

    CONSTRAINT outbox_messages_claim_state
        CHECK (
            (
                publication_state = 'PUBLISHING'
                AND claimed_at IS NOT NULL
                AND claimed_at >= created_at
                AND claim_owner IS NOT NULL
                AND btrim(claim_owner) <> ''
                AND char_length(claim_owner) <= 200
            )
            OR (
                publication_state <> 'PUBLISHING'
                AND claimed_at IS NULL
                AND claim_owner IS NULL
            )
        ),

    CONSTRAINT outbox_messages_published_state
        CHECK (
            (
                publication_state = 'PUBLISHED'
                AND published_at IS NOT NULL
                AND published_at >= created_at
            )
            OR (
                publication_state <> 'PUBLISHED'
                AND published_at IS NULL
            )
        ),

    CONSTRAINT outbox_messages_version_nonnegative
        CHECK (lock_version >= 0)
);

CREATE TABLE workflow_runtime.inbox_messages (
    consumer_name TEXT NOT NULL,
    message_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    node_execution_id TEXT,

    message_type TEXT NOT NULL,
    message_version INTEGER NOT NULL,
    processing_result TEXT NOT NULL,

    source_name TEXT,
    source_partition INTEGER,
    source_offset BIGINT,

    received_at TIMESTAMPTZ NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT inbox_messages_pk
        PRIMARY KEY (consumer_name, message_id),

    CONSTRAINT inbox_messages_execution_fk
        FOREIGN KEY (
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.workflow_executions (
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT inbox_messages_node_fk
        FOREIGN KEY (
            node_execution_id,
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.node_executions (
            node_execution_id,
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT inbox_messages_consumer_length
        CHECK (
            btrim(consumer_name) <> ''
            AND char_length(consumer_name) <= 200
        ),

    CONSTRAINT inbox_messages_id_length
        CHECK (
            btrim(message_id) <> ''
            AND char_length(message_id) <= 200
        ),

    CONSTRAINT inbox_messages_company_not_blank
        CHECK (btrim(company_id) <> ''),

    CONSTRAINT inbox_messages_execution_not_blank
        CHECK (btrim(workflow_execution_id) <> ''),

    CONSTRAINT inbox_messages_node_not_blank
        CHECK (
            node_execution_id IS NULL
            OR btrim(node_execution_id) <> ''
        ),

    CONSTRAINT inbox_messages_type_length
        CHECK (
            btrim(message_type) <> ''
            AND char_length(message_type) <= 128
        ),

    CONSTRAINT inbox_messages_version_positive
        CHECK (message_version > 0),

    CONSTRAINT inbox_messages_result_valid
        CHECK (processing_result IN ('APPLIED', 'IGNORED')),

    CONSTRAINT inbox_messages_source_position
        CHECK (
            (
                source_name IS NULL
                AND source_partition IS NULL
                AND source_offset IS NULL
            )
            OR (
                source_name IS NOT NULL
                AND btrim(source_name) <> ''
                AND char_length(source_name) <= 249
                AND source_partition >= 0
                AND source_offset >= 0
            )
        ),

    CONSTRAINT inbox_messages_time_order
        CHECK (processed_at >= received_at)
);

CREATE TABLE workflow_runtime.async_node_inputs (
    async_input_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    target_node_execution_id TEXT NOT NULL,
    source_node_execution_id TEXT NOT NULL,

    target_node_id TEXT NOT NULL,
    source_node_id TEXT NOT NULL,
    edge_id TEXT NOT NULL,
    source_output_port TEXT NOT NULL,
    target_input_port TEXT NOT NULL,
    source_attempt SMALLINT NOT NULL,

    payload_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT async_node_inputs_pk
        PRIMARY KEY (async_input_id),

    CONSTRAINT async_node_inputs_logical_uk
        UNIQUE (
            company_id,
            workflow_execution_id,
            edge_id,
            source_attempt
        ),

    CONSTRAINT async_node_inputs_execution_fk
        FOREIGN KEY (
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.workflow_executions (
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT async_node_inputs_target_fk
        FOREIGN KEY (
            target_node_execution_id,
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.node_executions (
            node_execution_id,
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT async_node_inputs_source_fk
        FOREIGN KEY (
            source_node_execution_id,
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.node_executions (
            node_execution_id,
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT async_node_inputs_id_length
        CHECK (
            btrim(async_input_id) <> ''
            AND char_length(async_input_id) <= 96
        ),

    CONSTRAINT async_node_inputs_identifiers_not_blank
        CHECK (
            btrim(company_id) <> ''
            AND btrim(workflow_execution_id) <> ''
            AND btrim(target_node_execution_id) <> ''
            AND btrim(source_node_execution_id) <> ''
            AND btrim(target_node_id) <> ''
            AND btrim(source_node_id) <> ''
            AND btrim(edge_id) <> ''
        ),

    CONSTRAINT async_node_inputs_port_length
        CHECK (
            btrim(source_output_port) <> ''
            AND char_length(source_output_port) <= 128
            AND btrim(target_input_port) <> ''
            AND char_length(target_input_port) <= 128
        ),

    CONSTRAINT async_node_inputs_attempt_positive
        CHECK (source_attempt > 0),

    CONSTRAINT async_node_inputs_payload_object
        CHECK (
            jsonb_typeof(payload_json) = 'object'
            AND octet_length(payload_json::TEXT) <= 16777216
        )
);

CREATE TABLE workflow_runtime.async_worker_results (
    worker_result_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    node_execution_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    attempt SMALLINT NOT NULL,

    result_status TEXT NOT NULL,
    routed_outputs_json JSONB NOT NULL DEFAULT '{}'::JSONB,
    terminal_output_json JSONB,
    context_changes_json JSONB NOT NULL DEFAULT '{}'::JSONB,
    failure_json JSONB,

    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT async_worker_results_pk
        PRIMARY KEY (worker_result_id),

    CONSTRAINT async_worker_results_node_attempt_uk
        UNIQUE (
            company_id,
            workflow_execution_id,
            node_execution_id,
            attempt
        ),

    CONSTRAINT async_worker_results_node_fk
        FOREIGN KEY (
            node_execution_id,
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.node_executions (
            node_execution_id,
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT async_worker_results_id_length
        CHECK (
            btrim(worker_result_id) <> ''
            AND char_length(worker_result_id) <= 96
        ),

    CONSTRAINT async_worker_results_identifiers_not_blank
        CHECK (
            btrim(company_id) <> ''
            AND btrim(workflow_execution_id) <> ''
            AND btrim(node_execution_id) <> ''
            AND btrim(node_id) <> ''
        ),

    CONSTRAINT async_worker_results_attempt_positive
        CHECK (attempt > 0),

    CONSTRAINT async_worker_results_status_valid
        CHECK (
            result_status IN (
                'SUCCEEDED',
                'FAILED',
                'CANCELLED',
                'TIMED_OUT'
            )
        ),

    CONSTRAINT async_worker_results_routed_outputs
        CHECK (
            jsonb_typeof(routed_outputs_json) = 'object'
            AND octet_length(routed_outputs_json::TEXT) <= 16777216
        ),

    CONSTRAINT async_worker_results_terminal_output
        CHECK (
            terminal_output_json IS NULL
            OR (
                jsonb_typeof(terminal_output_json) = 'object'
                AND octet_length(terminal_output_json::TEXT) <= 16777216
            )
        ),

    CONSTRAINT async_worker_results_context_changes
        CHECK (
            jsonb_typeof(context_changes_json) = 'object'
            AND octet_length(context_changes_json::TEXT) <= 16777216
        ),

    CONSTRAINT async_worker_results_failure
        CHECK (
            failure_json IS NULL
            OR (
                jsonb_typeof(failure_json) = 'object'
                AND octet_length(failure_json::TEXT) <= 16777216
            )
        ),

    CONSTRAINT async_worker_results_shape
        CHECK (
            (
                result_status = 'SUCCEEDED'
                AND failure_json IS NULL
                AND NOT (
                    routed_outputs_json <> '{}'::JSONB
                    AND terminal_output_json IS NOT NULL
                )
            )
            OR (
                result_status IN ('FAILED', 'CANCELLED', 'TIMED_OUT')
                AND routed_outputs_json = '{}'::JSONB
                AND terminal_output_json IS NULL
                AND context_changes_json = '{}'::JSONB
                AND failure_json IS NOT NULL
            )
        ),

    CONSTRAINT async_worker_results_time_order
        CHECK (
            finished_at >= started_at
            AND created_at >= finished_at
        )
);

CREATE TABLE workflow_runtime.async_context_variables (
    company_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    variable_key TEXT NOT NULL,
    value_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version BIGINT NOT NULL,

    CONSTRAINT async_context_variables_pk
        PRIMARY KEY (
            company_id,
            workflow_execution_id,
            variable_key
        ),

    CONSTRAINT async_context_variables_execution_fk
        FOREIGN KEY (
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.workflow_executions (
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT async_context_variables_key_length
        CHECK (
            btrim(variable_key) <> ''
            AND char_length(variable_key) <= 256
        ),

    CONSTRAINT async_context_variables_value_size
        CHECK (octet_length(value_json::TEXT) <= 1048576),

    CONSTRAINT async_context_variables_time_order
        CHECK (updated_at >= created_at),

    CONSTRAINT async_context_variables_version_positive
        CHECK (version > 0)
);

CREATE INDEX outbox_messages_pending_idx
    ON workflow_runtime.outbox_messages (
        created_at,
        message_id
    )
    WHERE publication_state = 'PENDING';

CREATE INDEX outbox_messages_workflow_idx
    ON workflow_runtime.outbox_messages (
        company_id,
        workflow_execution_id,
        created_at
    );

CREATE INDEX inbox_messages_workflow_idx
    ON workflow_runtime.inbox_messages (
        company_id,
        workflow_execution_id,
        processed_at
    );

CREATE INDEX async_node_inputs_target_idx
    ON workflow_runtime.async_node_inputs (
        company_id,
        workflow_execution_id,
        target_node_execution_id,
        created_at
    );

CREATE INDEX async_worker_results_workflow_idx
    ON workflow_runtime.async_worker_results (
        company_id,
        workflow_execution_id,
        created_at
    );

-- +goose Down

DROP TABLE workflow_runtime.async_context_variables;
DROP TABLE workflow_runtime.async_worker_results;
DROP TABLE workflow_runtime.async_node_inputs;
DROP TABLE workflow_runtime.inbox_messages;
DROP TABLE workflow_runtime.outbox_messages;
