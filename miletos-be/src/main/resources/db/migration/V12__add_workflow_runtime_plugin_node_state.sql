CREATE TABLE workflow_runtime.plugin_node_state (
    company_id TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    workflow_revision BIGINT NOT NULL,
    node_id TEXT NOT NULL,
    state_key TEXT NOT NULL,
    state_value JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT plugin_node_state_pk PRIMARY KEY (
        company_id,
        workflow_id,
        workflow_revision,
        node_id,
        state_key
    ),
    CONSTRAINT plugin_node_state_identity_not_blank CHECK (
        btrim(company_id) <> ''
        AND btrim(workflow_id) <> ''
        AND btrim(node_id) <> ''
        AND btrim(state_key) <> ''
    ),
    CONSTRAINT plugin_node_state_revision_positive
        CHECK (workflow_revision > 0),
    CONSTRAINT plugin_node_state_key_length
        CHECK (char_length(state_key) <= 255),
    CONSTRAINT plugin_node_state_time_order
        CHECK (updated_at >= created_at)
);
