package execution

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"miletos-go/internal/features/workflow-runtime/execution/model"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
)

func TestMapGRPCNodeExecutionPreservesGenericNodeData(t *testing.T) {
	now := time.Now().UTC()
	mapped := mapGRPCNodeExecution(model.NodeExecution{
		ID: "node-exec", ExecutionID: "execution", NodeID: "node",
		Type: "plugin.example", Version: "v1", Status: model.NodeSucceeded,
		Attempt: 1, CreatedAt: now, UpdatedAt: now,
		Configuration: map[string]any{"setting": "configured"},
		Input:         map[string]any{"request": "value"},
		Output:        map[string]any{"result": "value"},
		Failure:       map[string]any{"code": "EXAMPLE"},
	})
	if mapped.GetConfiguration().AsMap()["setting"] != "configured" {
		t.Fatalf("configuration was not preserved: %v", mapped.GetConfiguration())
	}
	if mapped.GetInputSummary().AsMap()["request"] != "value" ||
		mapped.GetOutputSummary().AsMap()["result"] != "value" ||
		mapped.GetFailureSummary().AsMap()["code"] != "EXAMPLE" {
		t.Fatalf("generic node data was not preserved: %v", mapped)
	}
}

func TestMapGRPCErrorPreservesIdempotencyAndInternalContracts(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		code       codes.Code
		reason     string
		hasDetails bool
	}{
		{
			name: "request in progress", err: ErrIdempotencyRequestInProgress,
			code: codes.FailedPrecondition, reason: "IDEMPOTENCY_REQUEST_IN_PROGRESS",
			hasDetails: true,
		},
		{
			name: "key reused", err: repository.ErrIdempotencyConflict,
			code: codes.AlreadyExists, reason: "IDEMPOTENCY_KEY_REUSED",
			hasDetails: true,
		},
		{
			name: "unexpected internal", err: errors.New("database detail"),
			code: codes.Internal,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mapped := mapGRPCError(test.err)
			mappedStatus := status.Convert(mapped)
			if mappedStatus.Code() != test.code {
				t.Fatalf("gRPC code = %s, want %s", mappedStatus.Code(), test.code)
			}
			var reason string
			for _, detail := range mappedStatus.Details() {
				if errorInfo, ok := detail.(*errdetails.ErrorInfo); ok {
					reason = errorInfo.GetReason()
				}
			}
			if reason != test.reason {
				t.Fatalf("ErrorInfo reason = %q, want %q", reason, test.reason)
			}
			if test.hasDetails != (reason != "") {
				t.Fatalf("ErrorInfo presence = %v, want %v", reason != "", test.hasDetails)
			}
			if test.code == codes.Internal && mappedStatus.Message() != "an unexpected internal error occurred" {
				t.Fatalf("internal message = %q", mappedStatus.Message())
			}
		})
	}
}
