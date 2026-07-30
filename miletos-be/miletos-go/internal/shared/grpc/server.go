package grpcserver

import (
	"context"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"miletos-go/internal/shared/requestcontext"
	"miletos-go/internal/shared/security"
)

func NewServer(
	logger *slog.Logger,
	tokenValidator security.InternalTokenValidator,
) *grpc.Server {
	return grpc.NewServer(grpc.ChainUnaryInterceptor(
		contextInterceptor(tokenValidator),
		recoveryInterceptor(logger),
	))
}

func contextInterceptor(
	tokenValidator security.InternalTokenValidator,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		request any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		incoming, ok := metadata.FromIncomingContext(ctx)
		if !ok || !tokenValidator.ValidateAuthorization(singleValue(incoming, "authorization")) {
			return nil, status.Error(
				codes.Unauthenticated,
				"valid internal service authentication is required",
			)
		}
		companyID := strings.TrimSpace(singleValue(
			incoming, requestcontext.HeaderCompanyID,
		))
		if companyID == "" || len(companyID) > 255 {
			return nil, status.Error(codes.PermissionDenied, "trusted company context is invalid")
		}
		values := requestcontext.Values{
			CompanyID: companyID,
			RequestID: requestcontext.NormalizeID(
				singleValue(incoming, requestcontext.HeaderRequestID), "req_",
			),
			CorrelationID: requestcontext.NormalizeID(
				singleValue(incoming, requestcontext.HeaderCorrelationID), "corr_",
			),
			IdempotencyKey: strings.TrimSpace(singleValue(
				incoming, requestcontext.HeaderIdempotencyKey,
			)),
		}
		return handler(requestcontext.WithValues(ctx, values), request)
	}
}

func recoveryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		request any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (response any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error(
					"gRPC handler panicked",
					"method", info.FullMethod,
					"requestId", requestcontext.RequestID(ctx),
				)
				response = nil
				err = status.Error(codes.Internal, "an unexpected internal error occurred")
			}
		}()
		return handler(ctx, request)
	}
}

func singleValue(values metadata.MD, name string) string {
	items := values.Get(name)
	if len(items) != 1 {
		return ""
	}
	return items[0]
}
