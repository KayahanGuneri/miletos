package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	executionhttp "miletos-go/internal/infra/http/execution"
)

const (
	defaultHTTPHandlerTimeout              = 30 * time.Second
	defaultExecutionRequestBodyLimit int64 = 1 << 20
)

type RouterOptions struct {
	Logger           *slog.Logger
	HandlerTimeout   time.Duration
	Authenticator    *InternalAuthenticator
	SwaggerEnabled   bool
	ExecutionHandler executionhttp.Handler
	HealthHandler    HealthHandler
	PluginHandler    PluginHandler
}

func NewRouter(handler Handler) http.Handler {
	return NewRouterWithOptions(handler, RouterOptions{})
}

func NewRouterWithOptions(handler Handler, options RouterOptions) http.Handler {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	handlerTimeout := options.HandlerTimeout
	if handlerTimeout <= 0 {
		handlerTimeout = defaultHTTPHandlerTimeout
	}

	router := chi.NewRouter()

	// Preserve the original global middleware order:
	// Recovery -> Request ID -> technical logging -> security headers
	// -> correlation ID -> route-specific middleware.
	router.Use(
		RecoveryMiddleware(logger),
		RequestIDMiddleware(),
		TechnicalLoggingMiddleware(logger),
		SecurityHeadersMiddleware(),
		CorrelationIDMiddleware(),
	)

	registerPublicRoutes(
		router,
		handler,
		options.HealthHandler,
		handlerTimeout,
		options.SwaggerEnabled,
	)
	registerProtectedRoutes(router, options, handlerTimeout)

	notFoundHandler := HandlerTimeoutMiddleware(handlerTimeout)(
		http.HandlerFunc(handler.NotFound),
	)
	router.NotFound(func(writer http.ResponseWriter, request *http.Request) {
		notFoundHandler.ServeHTTP(writer, request)
	})
	return router
}

func registerPublicRoutes(
	router chi.Router,
	handler Handler,
	healthHandler HealthHandler,
	handlerTimeout time.Duration,
	swaggerEnabled bool,
) {
	timedRoutes := router.With(HandlerTimeoutMiddleware(handlerTimeout))
	timedRoutes.Handle("/health", http.HandlerFunc(handler.Health))
	timedRoutes.Handle("/ready", http.HandlerFunc(healthHandler.Ready))
	timedRoutes.Handle("/openapi.json", http.HandlerFunc(openAPIHandler))

	if !swaggerEnabled {
		return
	}
	timedRoutes.Handle("/swagger", http.HandlerFunc(swaggerRedirectHandler))
	timedRoutes.Handle("/swagger/", http.HandlerFunc(swaggerUIHandler))

	// net/http ServeMux previously treated "/swagger/" as a subtree pattern.
	// Preserve that behavior explicitly.
	timedRoutes.Handle("/swagger/*", http.HandlerFunc(swaggerUIHandler))
}

func registerProtectedRoutes(
	router chi.Router,
	options RouterOptions,
	handlerTimeout time.Duration,
) {
	if options.Authenticator == nil {
		registerAuthenticationUnavailableRoutes(router, handlerTimeout)
		return
	}

	protectedRoutes := router.With(
		options.Authenticator.Middleware,
		HandlerTimeoutMiddleware(handlerTimeout),
	)
	protectedRoutes.Handle(
		"/api/v1/plugins",
		http.HandlerFunc(options.PluginHandler.Plugins),
	)

	// Preserve the original JSON tenant middleware order:
	// Correlation ID (global) -> body limit -> authentication
	// -> trusted company context -> timeout.
	tenantJSONRoutes := router.With(
		BodyLimitMiddleware(defaultExecutionRequestBodyLimit),
		options.Authenticator.Middleware,
		TrustedCompanyContextMiddleware(),
		HandlerTimeoutMiddleware(handlerTimeout),
	)
	tenantJSONRoutes.Handle(
		"/api/v1/executions/sync",
		http.HandlerFunc(options.ExecutionHandler.ExecuteSync),
	)
	tenantJSONRoutes.Handle(
		"/api/v1/executions/async",
		http.HandlerFunc(options.ExecutionHandler.ExecuteAsync),
	)
	tenantJSONRoutes.Handle(
		"/api/v1/executions/{executionID}/recover",
		http.HandlerFunc(options.ExecutionHandler.RecoverExecution),
	)

	// Preserve the original tenant middleware order:
	// Correlation ID (global) -> authentication -> trusted company context
	// -> timeout.
	tenantRoutes := router.With(
		options.Authenticator.Middleware,
		TrustedCompanyContextMiddleware(),
		HandlerTimeoutMiddleware(handlerTimeout),
	)
	executionRoutes := []struct {
		pattern string
		handler http.HandlerFunc
	}{
		{pattern: "/api/v1/executions", handler: options.ExecutionHandler.ListExecutions},
		{pattern: "/api/v1/executions/{executionID}", handler: options.ExecutionHandler.GetExecution},
		{
			pattern: "/api/v1/executions/{executionID}/definition",
			handler: options.ExecutionHandler.GetExecutionDefinition,
		},
		{
			pattern: "/api/v1/executions/{executionID}/nodes",
			handler: options.ExecutionHandler.GetExecutionNodes,
		},
		{
			pattern: "/api/v1/executions/{executionID}/events",
			handler: options.ExecutionHandler.GetExecutionEvents,
		},
		{
			pattern: "/api/v1/executions/{executionID}/logs",
			handler: options.ExecutionHandler.GetExecutionLogs,
		},
		{
			pattern: "/api/v1/executions/{executionID}/errors",
			handler: options.ExecutionHandler.GetExecutionErrors,
		},
		{pattern: "/api/v1/executions/", handler: options.ExecutionHandler.NotFound},
		{
			// net/http ServeMux previously treated "/api/v1/executions/" as a
			// subtree pattern. Chi uses an explicit wildcard for that behavior.
			pattern: "/api/v1/executions/*",
			handler: options.ExecutionHandler.NotFound,
		},
	}
	for _, route := range executionRoutes {
		tenantRoutes.Handle(route.pattern, route.handler)
	}
}

func registerAuthenticationUnavailableRoutes(router chi.Router, handlerTimeout time.Duration) {
	// Preserve existing nil-auth behavior: correlation ID is global and only
	// timeout runs before the 503 handler.
	unavailableRoutes := router.With(HandlerTimeoutMiddleware(handlerTimeout))
	authenticationUnavailable := http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			writeAPIError(
				writer,
				request,
				newAPIError(
					http.StatusServiceUnavailable,
					errorCodeServiceUnavailable,
					"Internal authentication is unavailable.",
				),
			)
		},
	)
	unavailablePatterns := []string{
		"/api/v1/plugins",
		"/api/v1/executions/sync",
		"/api/v1/executions/async",
		"/api/v1/executions",
		"/api/v1/executions/{executionID}",
		"/api/v1/executions/{executionID}/definition",
		"/api/v1/executions/{executionID}/nodes",
		"/api/v1/executions/{executionID}/events",
		"/api/v1/executions/{executionID}/logs",
		"/api/v1/executions/{executionID}/errors",
		"/api/v1/executions/{executionID}/recover",
		"/api/v1/executions/",
		"/api/v1/executions/*",
	}
	for _, pattern := range unavailablePatterns {
		unavailableRoutes.Handle(pattern, authenticationUnavailable)
	}
}
