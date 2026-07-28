package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
)

const createDefinitionSnapshotSQL = `
INSERT INTO workflow_definition_snapshots (
	snapshot_id,
	company_id,
	workflow_id,
	workflow_revision,
	workflow_name,
	definition_json,
	created_at
)
VALUES (
	$1,
	$2,
	$3,
	$4,
	$5,
	$6::jsonb,
	$7
)
`

const getDefinitionSnapshotByIDSQL = `
SELECT
	snapshot_id,
	company_id,
	workflow_id,
	workflow_revision,
	workflow_name,
	definition_json,
	created_at
FROM workflow_definition_snapshots
WHERE company_id = $1
  AND snapshot_id = $2
`

var _ repository.WorkflowSnapshotRepository = (*Store)(nil)

func (store *Store) Create(
	ctx context.Context,
	snapshot repository.DefinitionSnapshot,
) error {
	if !store.IsValid() {
		return fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("create definition snapshot context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !snapshot.IsValid() {
		return fmt.Errorf("definition snapshot must be valid")
	}
	_, err := store.pool.Exec(
		ctx,
		createDefinitionSnapshotSQL,
		snapshot.ID().String(),
		snapshot.CompanyID().String(),
		snapshot.WorkflowID().String(),
		int64(snapshot.WorkflowRevision()),
		snapshot.WorkflowName(),
		snapshot.DefinitionJSON().String(),
		snapshot.CreatedAt(),
	)
	if err != nil {
		return mapPostgreSQLError("create", "definition snapshot", err)
	}
	return nil
}

func (store *Store) GetByID(
	ctx context.Context,
	companyID workflow.CompanyID,
	snapshotID repository.DefinitionSnapshotID,
) (repository.DefinitionSnapshot, error) {
	if !store.IsValid() {
		return repository.DefinitionSnapshot{}, fmt.Errorf("PostgreSQL store must be valid")
	}
	if ctx == nil {
		return repository.DefinitionSnapshot{},
			fmt.Errorf("get definition snapshot context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return repository.DefinitionSnapshot{}, err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return repository.DefinitionSnapshot{},
			fmt.Errorf("company ID must be valid: %w", err)
	}
	normalizedSnapshotID, err := repository.NewDefinitionSnapshotID(snapshotID.String())
	if err != nil {
		return repository.DefinitionSnapshot{},
			fmt.Errorf("definition snapshot ID must be valid: %w", err)
	}
	snapshot, err := scanDefinitionSnapshot(
		store.pool.QueryRow(
			ctx,
			getDefinitionSnapshotByIDSQL,
			normalizedCompanyID.String(),
			normalizedSnapshotID.String(),
		),
	)
	if err != nil {
		return repository.DefinitionSnapshot{},
			mapPostgreSQLError("get", "definition snapshot", err)
	}
	return snapshot, nil
}

func scanDefinitionSnapshot(
	row pgx.Row,
) (repository.DefinitionSnapshot, error) {
	if row == nil {
		return repository.DefinitionSnapshot{},
			fmt.Errorf("definition snapshot row must not be nil")
	}
	var (
		snapshotID       string
		companyID        string
		workflowID       string
		workflowRevision int64
		workflowName     string
		definitionJSON   []byte
		createdAt        time.Time
	)
	if err := row.Scan(
		&snapshotID,
		&companyID,
		&workflowID,
		&workflowRevision,
		&workflowName,
		&definitionJSON,
		&createdAt,
	); err != nil {
		return repository.DefinitionSnapshot{}, err
	}
	if workflowRevision <= 0 {
		return repository.DefinitionSnapshot{},
			fmt.Errorf("definition snapshot workflow revision is invalid")
	}
	snapshot, err := repository.NewDefinitionSnapshot(
		repository.DefinitionSnapshotID(snapshotID),
		workflow.CompanyID(companyID),
		workflow.WorkflowID(workflowID),
		uint64(workflowRevision),
		workflowName,
		definitionJSON,
		createdAt,
	)
	if err != nil {
		return repository.DefinitionSnapshot{},
			fmt.Errorf("reconstruct definition snapshot: %w", err)
	}
	return snapshot, nil
}
