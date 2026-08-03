package grpcserver

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"miletos-go/internal/shared/requestcontext"
	"miletos-go/internal/shared/security"
)

func TestContextInterceptorValidatesTokenAndPropagatesMetadata(t *testing.T) {
	const token = "internal-token-value-for-grpc-tests"
	validator, err := security.NewInternalTokenValidator(token)
	if err != nil {
		t.Fatalf("NewInternalTokenValidator() error = %v", err)
	}
	interceptor := contextInterceptor(validator)
	incoming := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization", "Bearer "+token,
		requestcontext.HeaderCompanyID, "company-7",
		requestcontext.HeaderRequestID, "request-7",
		requestcontext.HeaderCorrelationID, "correlation-7",
		requestcontext.HeaderIdempotencyKey, "idempotency-7",
	))

	_, err = interceptor(incoming, struct{}{}, &grpc.UnaryServerInfo{},
		func(ctx context.Context, _ any) (any, error) {
			if got := requestcontext.CompanyID(ctx); got != "company-7" {
				t.Fatalf("company ID = %q", got)
			}
			if got := requestcontext.RequestID(ctx); got != "request-7" {
				t.Fatalf("request ID = %q", got)
			}
			if got := requestcontext.CorrelationID(ctx); got != "correlation-7" {
				t.Fatalf("correlation ID = %q", got)
			}
			if got := requestcontext.IdempotencyKey(ctx); got != "idempotency-7" {
				t.Fatalf("idempotency key = %q", got)
			}
			return struct{}{}, nil
		})
	if err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
}

func TestContextInterceptorRejectsInvalidAuthenticationAndTenant(t *testing.T) {
	const token = "internal-token-value-for-grpc-tests"
	validator, validatorErr := security.NewInternalTokenValidator(token)
	if validatorErr != nil {
		t.Fatalf("NewInternalTokenValidator() error = %v", validatorErr)
	}
	interceptor := contextInterceptor(validator)
	handler := func(context.Context, any) (any, error) {
		t.Fatal("handler called for invalid metadata")
		return nil, nil
	}

	_, err := interceptor(context.Background(), struct{}{}, &grpc.UnaryServerInfo{}, handler)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing metadata code = %v, want Unauthenticated", status.Code(err))
	}

	authenticated := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"authorization", "Bearer "+token,
	))
	_, err = interceptor(authenticated, struct{}{}, &grpc.UnaryServerInfo{}, handler)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("missing tenant code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestRecoveryInterceptorConvertsPanicToInternal(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	interceptor := recoveryInterceptor(logger)

	response, err := interceptor(context.Background(), struct{}{},
		&grpc.UnaryServerInfo{FullMethod: "/runtime.Execution/Execute"},
		func(context.Context, any) (any, error) {
			panic("controlled test panic")
		})

	if response != nil {
		t.Fatalf("response = %#v, want nil", response)
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("panic status = %v, want Internal", status.Code(err))
	}
}
