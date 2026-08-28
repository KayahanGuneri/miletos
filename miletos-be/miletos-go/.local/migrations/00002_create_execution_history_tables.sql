-- +goose Up

CREATE TABLE workflow_runtime.workflow_definition_snapshots (
    snapshot_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    workflow_revision BIGINT NOT NULL,
    workflow_name TEXT NOT NULL,
    definition_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT workflow_snapshots_pk
        PRIMARY KEY (snapshot_id),

    CONSTRAINT workflow_snapshots_tenant_uk
        UNIQUE (snapshot_id, company_id),

    CONSTRAINT workflow_snapshots_id_not_blank
        CHECK (btrim(snapshot_id) <> ''),

    CONSTRAINT workflow_snapshots_company_not_blank
        CHECK (btrim(company_id) <> ''),

    CONSTRAINT workflow_snapshots_workflow_not_blank
        CHECK (btrim(workflow_id) <> ''),

    CONSTRAINT workflow_snapshots_revision_positive
        CHECK (workflow_revision > 0),

    CONSTRAINT workflow_snapshots_name_not_blank
        CHECK (btrim(workflow_name) <> ''),

    CONSTRAINT workflow_snapshots_definition_object
        CHECK (jsonb_typeof(definition_json) = 'object')
);

CREATE TABLE workflow_runtime.workflow_executions (
    workflow_execution_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    workflow_revision BIGINT NOT NULL,
    snapshot_id TEXT NOT NULL,
    mode TEXT NOT NULL,
    correlation_id TEXT,
    status TEXT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL,
    validating_at TIMESTAMPTZ,
    queued_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,

    terminal_outputs JSONB NOT NULL DEFAULT '{}'::JSONB,
    failure_summary JSONB,
    is_stalled BOOLEAN NOT NULL DEFAULT FALSE,

    next_sequence_number BIGINT NOT NULL DEFAULT 1,
    lock_version BIGINT NOT NULL DEFAULT 0,

    CONSTRAINT workflow_executions_pk
        PRIMARY KEY (workflow_execution_id),

    CONSTRAINT workflow_executions_tenant_uk
        UNIQUE (workflow_execution_id, company_id),

    CONSTRAINT workflow_exec_snapshot_fk
        FOREIGN KEY (snapshot_id, company_id)
        REFERENCES workflow_runtime.workflow_definition_snapshots (
            snapshot_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT workflow_exec_id_not_blank
        CHECK (btrim(workflow_execution_id) <> ''),

    CONSTRAINT workflow_exec_company_not_blank
        CHECK (btrim(company_id) <> ''),

    CONSTRAINT workflow_exec_workflow_not_blank
        CHECK (btrim(workflow_id) <> ''),

    CONSTRAINT workflow_exec_snapshot_not_blank
        CHECK (btrim(snapshot_id) <> ''),

    CONSTRAINT workflow_exec_revision_positive
        CHECK (workflow_revision > 0),

    CONSTRAINT workflow_exec_mode_valid
        CHECK (mode IN ('SYNC', 'ASYNC')),

    CONSTRAINT workflow_exec_correlation_not_blank
        CHECK (
            correlation_id IS NULL
            OR btrim(correlation_id) <> ''
        ),

    CONSTRAINT workflow_exec_status_valid
        CHECK (
            status IN (
                'CREATED',
                'VALIDATING',
                'REJECTED',
                'QUEUED',
                'RUNNING',
                'SUCCEEDED',
                'FAILED',
                'CANCELLED',
                'TIMED_OUT'
            )
        ),

    CONSTRAINT workflow_exec_validating_time_order
        CHECK (
            validating_at IS NULL
            OR validating_at >= created_at
        ),

    CONSTRAINT workflow_exec_queued_time_order
        CHECK (
            queued_at IS NULL
            OR queued_at >= created_at
        ),

    CONSTRAINT workflow_exec_started_time_order
        CHECK (
            started_at IS NULL
            OR started_at >= created_at
        ),

    CONSTRAINT workflow_exec_finished_time_order
        CHECK (
            finished_at IS NULL
            OR finished_at >= created_at
        ),

    CONSTRAINT workflow_exec_updated_time_order
        CHECK (updated_at >= created_at),

    CONSTRAINT workflow_exec_validating_has_time
        CHECK (
            status <> 'VALIDATING'
            OR validating_at IS NOT NULL
        ),

    CONSTRAINT workflow_exec_queued_has_time
        CHECK (
            status <> 'QUEUED'
            OR queued_at IS NOT NULL
        ),

    CONSTRAINT workflow_exec_running_has_time
        CHECK (
            status <> 'RUNNING'
            OR started_at IS NOT NULL
        ),

    CONSTRAINT workflow_exec_terminal_has_time
        CHECK (
            status NOT IN (
                'REJECTED',
                'SUCCEEDED',
                'FAILED',
                'CANCELLED',
                'TIMED_OUT'
            )
            OR finished_at IS NOT NULL
        ),

    CONSTRAINT workflow_exec_nonterminal_no_finish
        CHECK (
            status IN (
                'REJECTED',
                'SUCCEEDED',
                'FAILED',
                'CANCELLED',
                'TIMED_OUT'
            )
            OR finished_at IS NULL
        ),

    CONSTRAINT workflow_exec_outputs_object
        CHECK (jsonb_typeof(terminal_outputs) = 'object'),

    CONSTRAINT workflow_exec_failure_object
        CHECK (
            failure_summary IS NULL
            OR jsonb_typeof(failure_summary) = 'object'
        ),

    CONSTRAINT workflow_exec_sequence_positive
        CHECK (next_sequence_number > 0),

    CONSTRAINT workflow_exec_version_nonnegative
        CHECK (lock_version >= 0)
);

CREATE TABLE workflow_runtime.node_executions (
    node_execution_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    company_id TEXT NOT NULL,

    node_id TEXT NOT NULL,
    plugin_type TEXT NOT NULL,
    plugin_version TEXT NOT NULL,

    status TEXT NOT NULL,
    attempt SMALLINT NOT NULL DEFAULT 1,

    created_at TIMESTAMPTZ NOT NULL,
    ready_at TIMESTAMPTZ,
    queued_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL,

    input_summary JSONB,
    output_summary JSONB,
    failure_summary JSONB,

    lock_version BIGINT NOT NULL DEFAULT 0,

    CONSTRAINT node_executions_pk
        PRIMARY KEY (node_execution_id),

    CONSTRAINT node_exec_identity_uk
        UNIQUE (
            node_execution_id,
            workflow_execution_id,
            company_id
        ),

    CONSTRAINT node_exec_attempt_uk
        UNIQUE (
            workflow_execution_id,
            node_id,
            attempt
        ),

    CONSTRAINT node_exec_parent_fk
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

    CONSTRAINT node_exec_id_not_blank
        CHECK (btrim(node_execution_id) <> ''),

    CONSTRAINT node_exec_workflow_not_blank
        CHECK (btrim(workflow_execution_id) <> ''),

    CONSTRAINT node_exec_company_not_blank
        CHECK (btrim(company_id) <> ''),

    CONSTRAINT node_exec_node_not_blank
        CHECK (btrim(node_id) <> ''),

    CONSTRAINT node_exec_plugin_type_not_blank
        CHECK (btrim(plugin_type) <> ''),

    CONSTRAINT node_exec_plugin_version_not_blank
        CHECK (btrim(plugin_version) <> ''),

    CONSTRAINT node_exec_status_valid
        CHECK (
            status IN (
                'PENDING',
                'READY',
                'QUEUED',
                'RUNNING',
                'SUCCEEDED',
                'FAILED',
                'SKIPPED',
                'CANCELLED',
                'TIMED_OUT'
            )
        ),

    CONSTRAINT node_exec_attempt_positive
        CHECK (attempt > 0),

    CONSTRAINT node_exec_ready_time_order
        CHECK (
            ready_at IS NULL
            OR ready_at >= created_at
        ),

    CONSTRAINT node_exec_queued_time_order
        CHECK (
            queued_at IS NULL
            OR queued_at >= created_at
        ),

    CONSTRAINT node_exec_started_time_order
        CHECK (
            started_at IS NULL
            OR started_at >= created_at
        ),

    CONSTRAINT node_exec_finished_time_order
        CHECK (
            finished_at IS NULL
            OR finished_at >= created_at
        ),

    CONSTRAINT node_exec_updated_time_order
        CHECK (updated_at >= created_at),

    CONSTRAINT node_exec_ready_has_time
        CHECK (
            status <> 'READY'
            OR ready_at IS NOT NULL
        ),

    CONSTRAINT node_exec_queued_has_time
        CHECK (
            status <> 'QUEUED'
            OR (
                ready_at IS NOT NULL
                AND queued_at IS NOT NULL
            )
        ),

    CONSTRAINT node_exec_running_has_time
        CHECK (
            status <> 'RUNNING'
            OR started_at IS NOT NULL
        ),

    CONSTRAINT node_exec_terminal_has_time
        CHECK (
            status NOT IN (
                'SUCCEEDED',
                'FAILED',
                'SKIPPED',
                'CANCELLED',
                'TIMED_OUT'
            )
            OR finished_at IS NOT NULL
        ),

    CONSTRAINT node_exec_nonterminal_no_finish
        CHECK (
            status IN (
                'SUCCEEDED',
                'FAILED',
                'SKIPPED',
                'CANCELLED',
                'TIMED_OUT'
            )
            OR finished_at IS NULL
        ),

    CONSTRAINT node_exec_input_object
        CHECK (
            input_summary IS NULL
            OR jsonb_typeof(input_summary) = 'object'
        ),

    CONSTRAINT node_exec_output_object
        CHECK (
            output_summary IS NULL
            OR jsonb_typeof(output_summary) = 'object'
        ),

    CONSTRAINT node_exec_failure_object
        CHECK (
            failure_summary IS NULL
            OR jsonb_typeof(failure_summary) = 'object'
        ),

    CONSTRAINT node_exec_version_nonnegative
        CHECK (lock_version >= 0)
);

CREATE TABLE workflow_runtime.execution_events (
    event_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    node_execution_id TEXT,

    sequence_number BIGINT NOT NULL,
    event_type TEXT NOT NULL,

    previous_status TEXT,
    new_status TEXT,

    correlation_id TEXT,
    causation_id TEXT,

    safe_message TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT execution_events_pk
        PRIMARY KEY (event_id),

    CONSTRAINT execution_events_identity_uk
        UNIQUE (
            event_id,
            workflow_execution_id,
            company_id
        ),

    CONSTRAINT execution_events_sequence_uk
        UNIQUE (
            workflow_execution_id,
            sequence_number
        ),

    CONSTRAINT execution_events_parent_fk
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

    CONSTRAINT execution_events_node_fk
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

    CONSTRAINT execution_events_id_not_blank
        CHECK (btrim(event_id) <> ''),

    CONSTRAINT execution_events_workflow_not_blank
        CHECK (btrim(workflow_execution_id) <> ''),

    CONSTRAINT execution_events_company_not_blank
        CHECK (btrim(company_id) <> ''),

    CONSTRAINT execution_events_node_not_blank
        CHECK (
            node_execution_id IS NULL
            OR btrim(node_execution_id) <> ''
        ),

    CONSTRAINT execution_events_sequence_positive
        CHECK (sequence_number > 0),

    CONSTRAINT execution_events_type_not_blank
        CHECK (
            btrim(event_type) <> ''
            AND char_length(event_type) <= 128
        ),

    CONSTRAINT execution_events_previous_not_blank
        CHECK (
            previous_status IS NULL
            OR btrim(previous_status) <> ''
        ),

    CONSTRAINT execution_events_new_not_blank
        CHECK (
            new_status IS NULL
            OR btrim(new_status) <> ''
        ),

    CONSTRAINT execution_events_correlation_not_blank
        CHECK (
            correlation_id IS NULL
            OR btrim(correlation_id) <> ''
        ),

    CONSTRAINT execution_events_causation_not_blank
        CHECK (
            causation_id IS NULL
            OR btrim(causation_id) <> ''
        ),

    CONSTRAINT execution_events_message_length
        CHECK (
            safe_message IS NULL
            OR (
                btrim(safe_message) <> ''
                AND char_length(safe_message) <= 2000
            )
        ),

    CONSTRAINT execution_events_metadata_object
        CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE TABLE workflow_runtime.execution_logs (
    log_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    company_id TEXT NOT NULL,
    node_execution_id TEXT,

    sequence_number BIGINT NOT NULL,
    level TEXT NOT NULL,
    message TEXT NOT NULL,

    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT execution_logs_pk
        PRIMARY KEY (log_id),

    CONSTRAINT execution_logs_sequence_uk
        UNIQUE (
            workflow_execution_id,
            sequence_number
        ),

    CONSTRAINT execution_logs_parent_fk
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

    CONSTRAINT execution_logs_node_fk
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

    CONSTRAINT execution_logs_id_not_blank
        CHECK (btrim(log_id) <> ''),

    CONSTRAINT execution_logs_workflow_not_blank
        CHECK (btrim(workflow_execution_id) <> ''),

    CONSTRAINT execution_logs_company_not_blank
        CHECK (btrim(company_id) <> ''),

    CONSTRAINT execution_logs_node_not_blank
        CHECK (
            node_execution_id IS NULL
            OR btrim(node_execution_id) <> ''
        ),

    CONSTRAINT execution_logs_sequence_positive
        CHECK (sequence_number > 0),

    CONSTRAINT execution_logs_level_valid
        CHECK (
            level IN (
                'DEBUG',
                'INFO',
                'WARN',
                'ERROR'
            )
        ),

    CONSTRAINT execution_logs_message_length
        CHECK (
            btrim(message) <> ''
            AND char_length(message) <= 4000
        ),

    CONSTRAINT execution_logs_metadata_object
        CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE TABLE workflow_runtime.execution_errors (
    error_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    company_id TEXT NOT NULL,

    node_execution_id TEXT,
    related_event_id TEXT,

    category TEXT NOT NULL,
    code TEXT NOT NULL,
    safe_message TEXT NOT NULL,
    technical_detail TEXT,

    retryable BOOLEAN NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT execution_errors_pk
        PRIMARY KEY (error_id),

    CONSTRAINT execution_errors_parent_fk
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

    CONSTRAINT execution_errors_node_fk
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

    CONSTRAINT execution_errors_event_fk
        FOREIGN KEY (
            related_event_id,
            workflow_execution_id,
            company_id
        )
        REFERENCES workflow_runtime.execution_events (
            event_id,
            workflow_execution_id,
            company_id
        )
        ON UPDATE RESTRICT
        ON DELETE RESTRICT,

    CONSTRAINT execution_errors_id_not_blank
        CHECK (btrim(error_id) <> ''),

    CONSTRAINT execution_errors_workflow_not_blank
        CHECK (btrim(workflow_execution_id) <> ''),

    CONSTRAINT execution_errors_company_not_blank
        CHECK (btrim(company_id) <> ''),

    CONSTRAINT execution_errors_node_not_blank
        CHECK (
            node_execution_id IS NULL
            OR btrim(node_execution_id) <> ''
        ),

    CONSTRAINT execution_errors_event_not_blank
        CHECK (
            related_event_id IS NULL
            OR btrim(related_event_id) <> ''
        ),

    CONSTRAINT execution_errors_category_valid
        CHECK (
            category IN (
                'VALIDATION',
                'EXECUTION',
                'DEPENDENCY',
                'TIMEOUT',
                'CANCELED',
                'INTERNAL'
            )
        ),

    CONSTRAINT execution_errors_code_length
        CHECK (
            btrim(code) <> ''
            AND char_length(code) <= 128
        ),

    CONSTRAINT execution_errors_safe_message
        CHECK (
            btrim(safe_message) <> ''
            AND char_length(safe_message) <= 2000
        ),

    CONSTRAINT execution_errors_technical_length
        CHECK (
            technical_detail IS NULL
            OR char_length(technical_detail) <= 8000
        ),

    CONSTRAINT execution_errors_details_object
        CHECK (jsonb_typeof(details) = 'object')
);

-- +goose Down

DROP TABLE workflow_runtime.execution_errors;
DROP TABLE workflow_runtime.execution_logs;
DROP TABLE workflow_runtime.execution_events;
DROP TABLE workflow_runtime.node_executions;
DROP TABLE workflow_runtime.workflow_executions;
DROP TABLE workflow_runtime.workflow_definition_snapshots;