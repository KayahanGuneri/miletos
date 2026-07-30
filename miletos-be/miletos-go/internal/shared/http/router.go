// Package http owns the runtime's shared HTTP transport.
package http

import (
	"log/slog"
	nethttp "net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"miletos-go/internal/shared/http/response"
	"miletos-go/internal/shared/security"
)

type RouteHandlers struct {
	Health                 nethttp.HandlerFunc
	PublicTrigger          nethttp.HandlerFunc
	ListPlugins            nethttp.HandlerFunc
	ListExecutions         nethttp.HandlerFunc
	ExecuteSync            nethttp.HandlerFunc
	ExecuteAsync           nethttp.HandlerFunc
	GetExecution           nethttp.HandlerFunc
	GetExecutionDefinition nethttp.HandlerFunc
	GetExecutionNodes      nethttp.HandlerFunc
	GetExecutionEvents     nethttp.HandlerFunc
	GetExecutionLogs       nethttp.HandlerFunc
	GetExecutionErrors     nethttp.HandlerFunc
	RecoverExecution       nethttp.HandlerFunc
}

func NewRouter(
	logger *slog.Logger,
	authentication security.Authentication,
	handlers RouteHandlers,
) nethttp.Handler {
	router := chi.NewRouter()

	router.Use(
		recoverRequests(logger),
		requestContext,
		securityHeaders,
		logRequests(logger),
	)

	router.Get("/health", handlers.Health)
	router.HandleFunc("/hooks/{token}", handlers.PublicTrigger)

	router.Route("/api/v1", func(api chi.Router) {
		api.Use(authentication.Handle)

		api.Get("/plugins", handlers.ListPlugins)

		api.Route("/executions", func(routes chi.Router) {
			routes.Use(companyContext)

			routes.Get("/", handlers.ListExecutions)
			routes.Post("/sync", handlers.ExecuteSync)
			routes.Post("/async", handlers.ExecuteAsync)

			routes.Get("/{executionID}", handlers.GetExecution)
			routes.Get("/{executionID}/definition", handlers.GetExecutionDefinition)
			routes.Get("/{executionID}/nodes", handlers.GetExecutionNodes)
			routes.Get("/{executionID}/events", handlers.GetExecutionEvents)
			routes.Get("/{executionID}/logs", handlers.GetExecutionLogs)
			routes.Get("/{executionID}/errors", handlers.GetExecutionErrors)
			routes.Post("/{executionID}/recover", handlers.RecoverExecution)
		})
	})

	router.NotFound(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		response.WriteAPIError(
			writer,
			request,
			nethttp.StatusNotFound,
			"NOT_FOUND",
			"The requested resource was not found.",
		)
	})

	router.MethodNotAllowed(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		response.WriteAPIError(
			writer,
			request,
			nethttp.StatusMethodNotAllowed,
			"METHOD_NOT_ALLOWED",
			"The requested method is not allowed for this resource.",
		)
	})

	return router
}

func recoverRequests(logger *slog.Logger) func(nethttp.Handler) nethttp.Handler {
	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(
			writer nethttp.ResponseWriter,
			request *nethttp.Request,
		) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error("HTTP handler panicked")
					writer.Header().Set("Content-Type", "application/json")
					writer.WriteHeader(nethttp.StatusInternalServerError)
					_, _ = writer.Write([]byte(
						`{"code":"INTERNAL_SERVER_ERROR","message":"An unexpected internal error occurred."}`,
					))
				}
			}()

			next.ServeHTTP(writer, request)
		})
	}
}

func securityHeaders(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(
		writer nethttp.ResponseWriter,
		request *nethttp.Request,
	) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")

		next.ServeHTTP(writer, request)
	})
}

func logRequests(logger *slog.Logger) func(nethttp.Handler) nethttp.Handler {
	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(
			writer nethttp.ResponseWriter,
			request *nethttp.Request,
		) {
			started := time.Now()

			next.ServeHTTP(writer, request)
			requestPath := request.URL.Path
			if strings.HasPrefix(requestPath, "/hooks/") {
				requestPath = "/hooks/{redacted}"
			}

			logger.Info(
				"HTTP request",
				"method",
				request.Method,
				"path",
				requestPath,
				"duration",
				time.Since(started),
			)
		})
	}
}
