-- +goose Up

CREATE INDEX workflow_snapshots_history_idx
    ON workflow_runtime.workflow_definition_snapshots (
        company_id,
        workflow_id,
        workflow_revision DESC,
        created_at DESC
    );

CREATE INDEX workflow_exec_company_recent_idx
    ON workflow_runtime.workflow_executions (
        company_id,
        created_at DESC,
        workflow_execution_id DESC
    );

CREATE INDEX workflow_exec_company_status_idx
    ON workflow_runtime.workflow_executions (
        company_id,
        status,
        created_at DESC,
        workflow_execution_id DESC
    );

CREATE INDEX workflow_exec_company_workflow_idx
    ON workflow_runtime.workflow_executions (
        company_id,
        workflow_id,
        created_at DESC,
        workflow_execution_id DESC
    );

CREATE INDEX workflow_exec_correlation_idx
    ON workflow_runtime.workflow_executions (
        company_id,
        correlation_id
    )
    WHERE correlation_id IS NOT NULL;

CREATE INDEX node_exec_company_workflow_idx
    ON workflow_runtime.node_executions (
        company_id,
        workflow_execution_id,
        created_at,
        node_execution_id
    );

CREATE INDEX node_exec_company_status_idx
    ON workflow_runtime.node_executions (
        company_id,
        status,
        updated_at DESC
    );

CREATE INDEX execution_events_timeline_idx
    ON workflow_runtime.execution_events (
        company_id,
        workflow_execution_id,
        sequence_number
    );

CREATE INDEX execution_events_node_idx
    ON workflow_runtime.execution_events (
        company_id,
        workflow_execution_id,
        node_execution_id,
        sequence_number
    )
    WHERE node_execution_id IS NOT NULL;

CREATE INDEX execution_logs_timeline_idx
    ON workflow_runtime.execution_logs (
        company_id,
        workflow_execution_id,
        sequence_number
    );

CREATE INDEX execution_logs_node_idx
    ON workflow_runtime.execution_logs (
        company_id,
        workflow_execution_id,
        node_execution_id,
        sequence_number
    )
    WHERE node_execution_id IS NOT NULL;

CREATE INDEX execution_logs_level_idx
    ON workflow_runtime.execution_logs (
        company_id,
        workflow_execution_id,
        level,
        sequence_number
    );

CREATE INDEX execution_errors_timeline_idx
    ON workflow_runtime.execution_errors (
        company_id,
        workflow_execution_id,
        created_at,
        error_id
    );

CREATE INDEX execution_errors_node_idx
    ON workflow_runtime.execution_errors (
        company_id,
        workflow_execution_id,
        node_execution_id,
        created_at
    )
    WHERE node_execution_id IS NOT NULL;

CREATE INDEX execution_errors_category_idx
    ON workflow_runtime.execution_errors (
        company_id,
        category,
        created_at DESC
    );

-- +goose Down

DROP INDEX workflow_runtime.execution_errors_category_idx;
DROP INDEX workflow_runtime.execution_errors_node_idx;
DROP INDEX workflow_runtime.execution_errors_timeline_idx;

DROP INDEX workflow_runtime.execution_logs_level_idx;
DROP INDEX workflow_runtime.execution_logs_node_idx;
DROP INDEX workflow_runtime.execution_logs_timeline_idx;

DROP INDEX workflow_runtime.execution_events_node_idx;
DROP INDEX workflow_runtime.execution_events_timeline_idx;

DROP INDEX workflow_runtime.node_exec_company_status_idx;
DROP INDEX workflow_runtime.node_exec_company_workflow_idx;

DROP INDEX workflow_runtime.workflow_exec_correlation_idx;
DROP INDEX workflow_runtime.workflow_exec_company_workflow_idx;
DROP INDEX workflow_runtime.workflow_exec_company_status_idx;
DROP INDEX workflow_runtime.workflow_exec_company_recent_idx;

DROP INDEX workflow_runtime.workflow_snapshots_history_idx;