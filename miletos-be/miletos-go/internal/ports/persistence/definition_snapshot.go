package repository

import (
	"bytes"
	"math"
	"time"

	"miletos-go/internal/features/workflow"
)

type DefinitionSnapshot struct {
	id               DefinitionSnapshotID
	companyID        workflow.CompanyID
	workflowID       workflow.WorkflowID
	workflowRevision uint64
	workflowName     string
	definitionJSON   JSONObject
	createdAt        time.Time
}

func NewDefinitionSnapshot(
	id DefinitionSnapshotID,
	companyID workflow.CompanyID,
	workflowID workflow.WorkflowID,
	workflowRevision uint64,
	workflowName string,
	definitionJSON []byte,
	createdAt time.Time,
) (DefinitionSnapshot, error) {
	normalizedID, err := NewDefinitionSnapshotID(id.String())
	if err != nil {
		return DefinitionSnapshot{}, err
	}
	normalizedCompanyID, err := workflow.NewCompanyID(companyID.String())
	if err != nil {
		return DefinitionSnapshot{}, newValidationError("companyID", err.Error())
	}
	normalizedWorkflowID, err := workflow.NewWorkflowID(workflowID.String())
	if err != nil {
		return DefinitionSnapshot{}, newValidationError("workflowID", err.Error())
	}
	if workflowRevision == 0 {
		return DefinitionSnapshot{},
			newValidationError("workflowRevision", "must be greater than zero")
	}
	if workflowRevision > uint64(math.MaxInt64) {
		return DefinitionSnapshot{},
			newValidationError(
				"workflowRevision",
				"must fit in a PostgreSQL BIGINT",
			)
	}
	normalizedWorkflowName, err := normalizeRequiredString("workflowName", workflowName)
	if err != nil {
		return DefinitionSnapshot{}, err
	}
	if len(bytes.TrimSpace(definitionJSON)) == 0 {
		return DefinitionSnapshot{},
			newValidationError("definitionJSON", "must not be empty")
	}
	normalizedDefinitionJSON, err := newJSONObject("definitionJSON", definitionJSON)
	if err != nil {
		return DefinitionSnapshot{}, err
	}
	if createdAt.IsZero() {
		return DefinitionSnapshot{},
			newValidationError("createdAt", "must not be zero")
	}
	return DefinitionSnapshot{
		id:               normalizedID,
		companyID:        normalizedCompanyID,
		workflowID:       normalizedWorkflowID,
		workflowRevision: workflowRevision,
		workflowName:     normalizedWorkflowName,
		definitionJSON:   normalizedDefinitionJSON,
		createdAt:        createdAt.UTC(),
	}, nil
}

func (snapshot DefinitionSnapshot) ID() DefinitionSnapshotID {
	return snapshot.id
}

func (snapshot DefinitionSnapshot) CompanyID() workflow.CompanyID {
	return snapshot.companyID
}

func (snapshot DefinitionSnapshot) WorkflowID() workflow.WorkflowID {
	return snapshot.workflowID
}

func (snapshot DefinitionSnapshot) WorkflowRevision() uint64 {
	return snapshot.workflowRevision
}

func (snapshot DefinitionSnapshot) WorkflowName() string {
	return snapshot.workflowName
}

func (snapshot DefinitionSnapshot) DefinitionJSON() JSONObject {
	return snapshot.definitionJSON
}

func (snapshot DefinitionSnapshot) CreatedAt() time.Time {
	return snapshot.createdAt
}

func (snapshot DefinitionSnapshot) IsValid() bool {
	_, err := NewDefinitionSnapshot(
		snapshot.id,
		snapshot.companyID,
		snapshot.workflowID,
		snapshot.workflowRevision,
		snapshot.workflowName,
		snapshot.definitionJSON.Bytes(),
		snapshot.createdAt,
	)
	return err == nil
}
