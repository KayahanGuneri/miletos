-- +goose Up

ALTER TABLE workflow_runtime.node_executions
    ADD COLUMN retry_max_attempts SMALLINT,
    ADD COLUMN retry_initial_backoff_ns BIGINT,
    ADD COLUMN retry_max_backoff_ns BIGINT,
    ADD COLUMN next_attempt_at TIMESTAMPTZ;

UPDATE workflow_runtime.node_executions
SET failure_summary = NULL
WHERE status = 'SUCCEEDED'
  AND failure_summary = '{}'::jsonb;

UPDATE workflow_runtime.node_executions
SET output_summary = NULL
WHERE status IN ('FAILED', 'TIMED_OUT', 'CANCELLED')
  AND output_summary = '{}'::jsonb;

UPDATE workflow_runtime.node_executions
SET
    output_summary = CASE
        WHEN output_summary = '{}'::jsonb THEN NULL
        ELSE output_summary
    END,
    failure_summary = CASE
        WHEN failure_summary = '{}'::jsonb THEN NULL
        ELSE failure_summary
    END
WHERE status IN (
    'PENDING',
    'READY',
    'QUEUED',
    'RUNNING',
    'SKIPPED'
)
AND (
    output_summary = '{}'::jsonb
    OR failure_summary = '{}'::jsonb
);

ALTER TABLE workflow_runtime.node_executions
    DROP CONSTRAINT node_exec_status_valid,
    ADD CONSTRAINT node_exec_status_valid CHECK (
        status IN (
            'PENDING',
            'READY',
            'QUEUED',
            'RUNNING',
            'RETRY_PENDING',
            'SUCCEEDED',
            'FAILED',
            'SKIPPED',
            'CANCELLED',
            'TIMED_OUT'
        )
    ),
    DROP CONSTRAINT IF EXISTS node_exec_summary_shape,
    ADD CONSTRAINT node_exec_summary_shape CHECK (
        CASE
            WHEN status = 'SUCCEEDED'
                THEN failure_summary IS NULL
            WHEN status IN ('RETRY_PENDING', 'FAILED', 'TIMED_OUT')
                THEN failure_summary IS NOT NULL AND output_summary IS NULL
            WHEN status = 'CANCELLED'
                THEN output_summary IS NULL
            WHEN status = 'SKIPPED'
                THEN output_summary IS NULL AND failure_summary IS NULL
            ELSE output_summary IS NULL AND failure_summary IS NULL
        END
    ),
    ADD CONSTRAINT node_exec_retry_policy_shape CHECK (
        (
            retry_max_attempts IS NULL
            AND retry_initial_backoff_ns IS NULL
            AND retry_max_backoff_ns IS NULL
        )
        OR
        (
            retry_max_attempts IS NOT NULL
            AND retry_initial_backoff_ns IS NOT NULL
            AND retry_max_backoff_ns IS NOT NULL
        )
    ),
    ADD CONSTRAINT node_exec_retry_policy_values CHECK (
        retry_max_attempts IS NULL
        OR
        (
            retry_max_attempts > 0
            AND retry_initial_backoff_ns > 0
            AND retry_max_backoff_ns >= retry_initial_backoff_ns
        )
    ),
    ADD CONSTRAINT node_exec_next_attempt_shape CHECK (
        (
            status = 'RETRY_PENDING'
            AND next_attempt_at IS NOT NULL
            AND next_attempt_at > updated_at
            AND started_at IS NOT NULL
            AND finished_at IS NULL
            AND retry_max_attempts IS NOT NULL
            AND attempt < retry_max_attempts
        )
        OR
        (
            status <> 'RETRY_PENDING'
            AND next_attempt_at IS NULL
        )
    );

ALTER TABLE workflow_runtime.execution_events
    DROP CONSTRAINT IF EXISTS execution_event_type_valid;

ALTER TABLE workflow_runtime.execution_events
    ADD CONSTRAINT execution_event_type_valid CHECK (
        event_type IN (
            'WORKFLOW_CREATED',
            'WORKFLOW_VALIDATING',
            'WORKFLOW_REJECTED',
            'WORKFLOW_QUEUED',
            'WORKFLOW_STARTED',
            'WORKFLOW_SUCCEEDED',
            'WORKFLOW_FAILED',
            'WORKFLOW_CANCELLED',
            'WORKFLOW_TIMED_OUT',
            'WORKFLOW_STALLED',
            'NODE_CREATED',
            'NODE_READY',
            'NODE_QUEUED',
            'NODE_STARTED',
            'NODE_RETRY_PENDING',
            'NODE_SUCCEEDED',
            'NODE_FAILED',
            'NODE_SKIPPED',
            'NODE_CANCELLED',
            'NODE_TIMED_OUT'
        )
    );

CREATE TABLE workflow_runtime.node_execution_attempts (
    company_id TEXT NOT NULL,
    workflow_execution_id TEXT NOT NULL,
    node_execution_id TEXT NOT NULL,
    attempt SMALLINT NOT NULL,
    attempt_status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    output_summary JSONB,
    failure_summary JSONB,
    retry_decision_kind TEXT,
    retry_decision_reason TEXT,
    retry_backoff_ns BIGINT,
    next_attempt_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT node_attempts_pk PRIMARY KEY (
        company_id,
        workflow_execution_id,
        node_execution_id,
        attempt
    ),
    CONSTRAINT node_attempts_node_fk FOREIGN KEY (
        node_execution_id,
        workflow_execution_id,
        company_id
    ) REFERENCES workflow_runtime.node_executions (
        node_execution_id,
        workflow_execution_id,
        company_id
    ) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT node_attempts_attempt_positive CHECK (attempt > 0),
    CONSTRAINT node_attempts_status_valid CHECK (
        attempt_status IN (
            'RUNNING',
            'SUCCEEDED',
            'FAILED',
            'CANCELLED',
            'TIMED_OUT'
        )
    ),
    CONSTRAINT node_attempts_time_order CHECK (
        updated_at >= created_at
        AND started_at >= created_at
        AND updated_at >= started_at
        AND (finished_at IS NULL OR finished_at >= started_at)
        AND (finished_at IS NULL OR updated_at >= finished_at)
    ),
    CONSTRAINT node_attempts_status_time_shape CHECK (
        (attempt_status = 'RUNNING' AND finished_at IS NULL)
        OR
        (attempt_status <> 'RUNNING' AND finished_at IS NOT NULL)
    ),
    CONSTRAINT node_attempts_summary_shape CHECK (
        (attempt_status = 'SUCCEEDED' OR output_summary IS NULL)
        AND (attempt_status <> 'SUCCEEDED' OR failure_summary IS NULL)
        AND (
            attempt_status NOT IN ('FAILED', 'TIMED_OUT')
            OR failure_summary IS NOT NULL
        )
    ),
    CONSTRAINT node_attempts_json_shape CHECK (
        (output_summary IS NULL OR jsonb_typeof(output_summary) = 'object')
        AND (failure_summary IS NULL OR jsonb_typeof(failure_summary) = 'object')
    ),
    CONSTRAINT node_attempts_retry_decision_shape CHECK (
        (
            retry_decision_kind IS NULL
            AND retry_decision_reason IS NULL
            AND retry_backoff_ns IS NULL
            AND next_attempt_at IS NULL
        )
        OR
        (
            attempt_status IN ('FAILED', 'TIMED_OUT')
            AND retry_decision_kind = 'RETRY'
            AND retry_decision_reason = 'RETRY_SCHEDULED'
            AND retry_backoff_ns > 0
            AND next_attempt_at > finished_at
            AND attempt < 32767
        )
        OR
        (
            retry_decision_kind = 'DO_NOT_RETRY'
            AND (
                (
                    attempt_status IN ('FAILED', 'TIMED_OUT')
                    AND retry_decision_reason IN (
                        'FAILURE_NOT_RETRYABLE',
                        'CATEGORY_NOT_RETRYABLE',
                        'DEADLINE_WOULD_BE_EXCEEDED'
                    )
                )
                OR
                (
                    attempt_status = 'CANCELLED'
                    AND retry_decision_reason = 'CATEGORY_NOT_RETRYABLE'
                )
            )
            AND retry_backoff_ns IS NULL
            AND next_attempt_at IS NULL
        )
        OR
        (
            attempt_status IN ('FAILED', 'TIMED_OUT')
            AND retry_decision_kind = 'EXHAUSTED'
            AND retry_decision_reason = 'MAX_ATTEMPTS_REACHED'
            AND retry_backoff_ns IS NULL
            AND next_attempt_at IS NULL
        )
    )
);

INSERT INTO workflow_runtime.node_execution_attempts (
    company_id,
    workflow_execution_id,
    node_execution_id,
    attempt,
    attempt_status,
    started_at,
    created_at,
    updated_at
)
SELECT
    source.company_id,
    source.workflow_execution_id,
    source.node_execution_id,
    source.attempt,
    'RUNNING',
    source.started_at,
    source.started_at,
    GREATEST(source.updated_at, source.started_at)
FROM workflow_runtime.node_executions AS source
WHERE source.status = 'RUNNING';

-- +goose Down

DROP TABLE workflow_runtime.node_execution_attempts;

ALTER TABLE workflow_runtime.execution_events
    DROP CONSTRAINT IF EXISTS execution_event_type_valid;

ALTER TABLE workflow_runtime.node_executions
    DROP CONSTRAINT node_exec_retry_policy_shape,
    DROP CONSTRAINT node_exec_retry_policy_values,
    DROP CONSTRAINT node_exec_next_attempt_shape,
    DROP CONSTRAINT node_exec_summary_shape,
    ADD CONSTRAINT node_exec_summary_shape CHECK (
        CASE
            WHEN status = 'SUCCEEDED'
                THEN failure_summary IS NULL
            WHEN status IN ('FAILED', 'TIMED_OUT')
                THEN failure_summary IS NOT NULL AND output_summary IS NULL
            WHEN status = 'CANCELLED'
                THEN output_summary IS NULL
            WHEN status = 'SKIPPED'
                THEN output_summary IS NULL AND failure_summary IS NULL
            ELSE output_summary IS NULL AND failure_summary IS NULL
        END
    ),
    DROP CONSTRAINT node_exec_status_valid,
    ADD CONSTRAINT node_exec_status_valid CHECK (
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
    DROP COLUMN next_attempt_at,
    DROP COLUMN retry_max_backoff_ns,
    DROP COLUMN retry_initial_backoff_ns,
    DROP COLUMN retry_max_attempts;
