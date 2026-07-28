package executionfeature

import (
	"context"
	"fmt"

	"miletos-go/internal/engine"
	"miletos-go/internal/features/execution"
	"miletos-go/internal/features/workflow"
)

type PartialRecoveryApplication interface {
	Recover(
		context.Context,
		workflow.CompanyID,
		execution.WorkflowExecutionID,
		string,
		string,
	) (engine.PartialRecoveryOutcome, bool, error)
}

type EnginePartialRecoveryApplication struct {
	service engine.PartialRecoveryService
}

func NewEnginePartialRecoveryApplication(
	service engine.PartialRecoveryService,
) (EnginePartialRecoveryApplication, error) {
	if !service.IsValid() {
		return EnginePartialRecoveryApplication{},
			fmt.Errorf("partial recovery service must be valid")
	}
	return EnginePartialRecoveryApplication{service: service}, nil
}

func (application EnginePartialRecoveryApplication) Recover(
	ctx context.Context,
	companyID workflow.CompanyID,
	sourceExecutionID execution.WorkflowExecutionID,
	idempotencyKey string,
	requestFingerprint string,
) (engine.PartialRecoveryOutcome, bool, error) {
	return application.service.Recover(
		ctx, companyID, sourceExecutionID, idempotencyKey, requestFingerprint)
}
