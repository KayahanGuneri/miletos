package executionfeature

import (
	context "context"
	errors "errors"
	engine "miletos-go/internal/engine"
	execution "miletos-go/internal/features/execution"
	workflow "miletos-go/internal/features/workflow"
	repository "miletos-go/internal/ports/persistence"
	testing "testing"
	time "time"
)

func TestEdgeCountExpectationFormatsRanges(
	t *testing.T,
) {
	tests :=
		[]struct {
			name       string
			minimum    uint
			maximum    uint
			hasMaximum bool
			expected   string
		}{
			{
				name: "exact",

				minimum: 1,

				maximum: 1,

				hasMaximum: true,

				expected: "1",
			},
			{
				name: "range",

				minimum: 1,

				maximum: 3,

				hasMaximum: true,

				expected: "1..3",
			},
			{
				name: "minimum only",

				minimum: 2,

				expected: ">=2",
			},
			{
				name: "unbounded zero",

				expected: "",
			},
		}

	for _, test := range tests {

		t.Run(
			test.name,
			func(t *testing.T) {
				actual :=
					edgeCountExpectation(
						test.minimum,
						test.maximum,
						test.hasMaximum,
					)

				if actual !=
					test.expected {

					t.Fatalf(
						"expectation = %q, want %q",
						actual,
						test.expected,
					)
				}
			},
		)
	}
}

func TestSafeEngineValidationReasonDoesNotExposeTechnicalDetails(
	t *testing.T,
) {
	tests :=
		[]struct {
			code     engine.PreflightValidationIssueCode
			expected string
		}{
			{
				code: engine.
					IssueCodeTopologicalOrderUnavailable,

				expected: "workflow topological order is unavailable",
			},
			{
				code: engine.
					IssueCodeRuntimeEdgeInvalid,

				expected: "runtime edge configuration is invalid",
			},
			{
				code: engine.
					IssueCodeExecutorNotFound,

				expected: "required node executor is unavailable",
			},
			{
				code: engine.
					PreflightValidationIssueCode(
						"UNKNOWN",
					),

				expected: "execution plan validation failed",
			},
		}

	for _, test := range tests {

		actual :=
			safeEngineValidationReason(
				test.code,
			)

		if actual !=
			test.expected {

			t.Fatalf(
				"reason for %q = %q, want %q",
				test.code,
				actual,
				test.expected,
			)
		}
	}
}

const reconciliationFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type reconciliationPersistence struct {
	acceptanceEvidence bool
	acceptanceError    error
	markError          error
	getError           error

	acceptedRecord repository.HTTPIdempotencyRecord

	evidenceCalls int
	markCalls     int
	getCalls      int
}

func (
	persistence *reconciliationPersistence,
) ReserveHTTPIdempotency(
	context.Context,
	repository.HTTPIdempotencyReservation,
) (
	repository.HTTPIdempotencyRecord,
	bool,
	error,
) {
	return repository.HTTPIdempotencyRecord{},
		false,
		nil
}

func (
	persistence *reconciliationPersistence,
) MarkHTTPIdempotencyAccepted(
	context.Context,
	repository.HTTPIdempotencyAcceptance,
) (
	repository.HTTPIdempotencyRecord,
	error,
) {
	persistence.markCalls++

	if persistence.markError != nil {
		return repository.HTTPIdempotencyRecord{},
			persistence.markError
	}

	return persistence.acceptedRecord,
		nil
}

func (
	persistence *reconciliationPersistence,
) HasHTTPIdempotencyAcceptanceEvidence(
	context.Context,
	workflow.CompanyID,
	execution.WorkflowExecutionID,
) (
	bool,
	error,
) {
	persistence.evidenceCalls++

	if persistence.acceptanceError != nil {
		return false,
			persistence.acceptanceError
	}

	return persistence.acceptanceEvidence,
		nil
}

func (
	persistence *reconciliationPersistence,
) GetWorkflowExecution(
	context.Context,
	workflow.CompanyID,
	execution.WorkflowExecutionID,
) (
	repository.WorkflowExecutionRecord,
	error,
) {
	persistence.getCalls++

	if persistence.getError != nil {
		return repository.WorkflowExecutionRecord{},
			persistence.getError
	}

	return repository.WorkflowExecutionRecord{},
		nil
}

func TestReservedIdempotencyWithoutEvidenceRemainsInProgress(
	t *testing.T,
) {
	record :=
		mustReconciliationHTTPIdempotencyRecord(
			t,
			repository.HTTPIdempotencyStateReserved,
		)

	persistence :=
		&reconciliationPersistence{}

	application :=
		IdempotentExecutionApplication{
			persistence: persistence,
		}

	_, replayed, err :=
		application.
			replayHTTPIdempotency(
				context.Background(),
				record,
			)

	if !errors.Is(
		err,
		ErrIdempotencyRequestInProgress,
	) {
		t.Fatalf(
			"error = %v, want ErrIdempotencyRequestInProgress",
			err,
		)
	}

	if replayed {
		t.Fatal(
			"replayed = true, want false",
		)
	}

	if persistence.evidenceCalls != 1 {
		t.Fatalf(
			"evidence calls = %d, want 1",
			persistence.evidenceCalls,
		)
	}

	if persistence.markCalls != 0 {
		t.Fatalf(
			"mark calls = %d, want 0",
			persistence.markCalls,
		)
	}

	if persistence.getCalls != 0 {
		t.Fatalf(
			"get calls = %d, want 0",
			persistence.getCalls,
		)
	}
}

func TestReservedIdempotencyWithEvidenceIsAcceptedAndReplayed(
	t *testing.T,
) {
	reserved :=
		mustReconciliationHTTPIdempotencyRecord(
			t,
			repository.HTTPIdempotencyStateReserved,
		)

	accepted :=
		mustReconciliationHTTPIdempotencyRecord(
			t,
			repository.HTTPIdempotencyStateAccepted,
		)

	persistence :=
		&reconciliationPersistence{
			acceptanceEvidence: true,

			acceptedRecord: accepted,
		}

	application :=
		IdempotentExecutionApplication{
			persistence: persistence,
		}

	_, replayed, err :=
		application.
			replayHTTPIdempotency(
				context.Background(),
				reserved,
			)

	if err != nil {
		t.Fatalf(
			"replayHTTPIdempotency() error = %v",
			err,
		)
	}

	if !replayed {
		t.Fatal(
			"replayed = false, want true",
		)
	}

	if persistence.evidenceCalls != 1 {
		t.Fatalf(
			"evidence calls = %d, want 1",
			persistence.evidenceCalls,
		)
	}

	if persistence.markCalls != 1 {
		t.Fatalf(
			"mark calls = %d, want 1",
			persistence.markCalls,
		)
	}

	if persistence.getCalls != 1 {
		t.Fatalf(
			"get calls = %d, want 1",
			persistence.getCalls,
		)
	}
}

func TestAcceptedIdempotencyReplaysWithoutEvidenceCheck(
	t *testing.T,
) {
	record :=
		mustReconciliationHTTPIdempotencyRecord(
			t,
			repository.HTTPIdempotencyStateAccepted,
		)

	persistence :=
		&reconciliationPersistence{}

	application :=
		IdempotentExecutionApplication{
			persistence: persistence,
		}

	_, replayed, err :=
		application.
			replayHTTPIdempotency(
				context.Background(),
				record,
			)

	if err != nil {
		t.Fatalf(
			"replayHTTPIdempotency() error = %v",
			err,
		)
	}

	if !replayed {
		t.Fatal(
			"replayed = false, want true",
		)
	}

	if persistence.evidenceCalls != 0 {
		t.Fatalf(
			"evidence calls = %d, want 0",
			persistence.evidenceCalls,
		)
	}

	if persistence.markCalls != 0 {
		t.Fatalf(
			"mark calls = %d, want 0",
			persistence.markCalls,
		)
	}

	if persistence.getCalls != 1 {
		t.Fatalf(
			"get calls = %d, want 1",
			persistence.getCalls,
		)
	}
}

func TestReservedIdempotencyEvidenceFailureStopsReconciliation(
	t *testing.T,
) {
	record :=
		mustReconciliationHTTPIdempotencyRecord(
			t,
			repository.HTTPIdempotencyStateReserved,
		)

	persistence :=
		&reconciliationPersistence{
			acceptanceError: errors.New(
				"evidence unavailable",
			),
		}

	application :=
		IdempotentExecutionApplication{
			persistence: persistence,
		}

	_, replayed, err :=
		application.
			replayHTTPIdempotency(
				context.Background(),
				record,
			)

	if err == nil {
		t.Fatal(
			"expected reconciliation error",
		)
	}

	if replayed {
		t.Fatal(
			"replayed = true, want false",
		)
	}

	if persistence.markCalls != 0 {
		t.Fatalf(
			"mark calls = %d, want 0",
			persistence.markCalls,
		)
	}

	if persistence.getCalls != 0 {
		t.Fatalf(
			"get calls = %d, want 0",
			persistence.getCalls,
		)
	}
}

func mustReconciliationHTTPIdempotencyRecord(
	t *testing.T,
	state repository.HTTPIdempotencyState,
) repository.HTTPIdempotencyRecord {
	t.Helper()

	createdAt :=
		time.Date(
			2026,
			time.July,
			20,
			12,
			0,
			0,
			0,
			time.UTC,
		)

	params :=
		repository.HTTPIdempotencyRecordParams{
			CompanyID: workflow.CompanyID(
				"company-reconciliation",
			),

			IdempotencyKey: "reconciliation-key",

			RequestFingerprint: reconciliationFingerprint,

			WorkflowExecutionID: execution.WorkflowExecutionID(
				"execution-reconciliation",
			),

			State: state,

			CreatedAt: createdAt,
		}

	if state ==
		repository.HTTPIdempotencyStateAccepted {

		params.AcceptedAt =
			createdAt.Add(
				time.Second,
			)
	}

	record, err :=
		repository.NewHTTPIdempotencyRecord(
			params,
		)

	if err != nil {
		t.Fatalf(
			"NewHTTPIdempotencyRecord() error = %v",
			err,
		)
	}

	return record
}
