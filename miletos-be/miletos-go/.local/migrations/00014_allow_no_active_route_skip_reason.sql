-- +goose Up

ALTER TABLE workflow_runtime.node_executions
    DROP CONSTRAINT IF EXISTS node_exec_summary_shape,
    ADD CONSTRAINT node_exec_summary_shape CHECK (
        CASE
            WHEN status = 'SUCCEEDED'
                THEN failure_summary IS NULL

            WHEN status IN ('RETRY_PENDING', 'FAILED', 'TIMED_OUT')
                THEN failure_summary IS NOT NULL
                    AND output_summary IS NULL

            WHEN status = 'CANCELLED'
                THEN output_summary IS NULL

            WHEN status = 'SKIPPED'
                THEN output_summary IS NULL
                    AND (
                        failure_summary IS NULL
                        OR (
                            jsonb_typeof(failure_summary) = 'object'
                            AND failure_summary ? 'skipReason'
                            AND (failure_summary - 'skipReason') = '{}'::jsonb
                            AND failure_summary ->> 'skipReason' IN (
                                'OUT_OF_TRIGGER_SCOPE',
                                'DEPENDENCY_FAILED',
                                'NO_ACTIVE_ROUTE'
                            )
                        )
                    )

            ELSE
                output_summary IS NULL
                AND failure_summary IS NULL
        END
    );

-- +goose Down

ALTER TABLE workflow_runtime.node_executions
    DROP CONSTRAINT IF EXISTS node_exec_summary_shape,
    ADD CONSTRAINT node_exec_summary_shape CHECK (
        CASE
            WHEN status = 'SUCCEEDED'
                THEN failure_summary IS NULL

            WHEN status IN ('RETRY_PENDING', 'FAILED', 'TIMED_OUT')
                THEN failure_summary IS NOT NULL
                    AND output_summary IS NULL

            WHEN status = 'CANCELLED'
                THEN output_summary IS NULL

            WHEN status = 'SKIPPED'
                THEN output_summary IS NULL
                    AND (
                        failure_summary IS NULL
                        OR (
                            jsonb_typeof(failure_summary) = 'object'
                            AND failure_summary ? 'skipReason'
                            AND (failure_summary - 'skipReason') = '{}'::jsonb
                            AND failure_summary ->> 'skipReason' IN (
                                'OUT_OF_TRIGGER_SCOPE',
                                'DEPENDENCY_FAILED'
                            )
                        )
                    )

            ELSE
                output_summary IS NULL
                AND failure_summary IS NULL
        END
    );