-- +goose Up
CREATE SCHEMA workflow_runtime;

COMMENT ON SCHEMA workflow_runtime IS
    'Durable workflow execution history owned by miletos-go';

-- +goose Down
DROP SCHEMA workflow_runtime;