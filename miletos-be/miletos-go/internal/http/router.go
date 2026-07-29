package http

import (
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"miletos-go/internal/controller"
	"miletos-go/internal/middleware"
)

func NewRouter(
	logger *slog.Logger,
	authentication middleware.Authentication,
	health *controller.HealthController,
	plugins *controller.PluginController,
	executions *controller.ExecutionController,
) nethttp.Handler {
	router := chi.NewRouter()

	router.Use(
		recoverRequests(logger),
		middleware.RequestContext,
		securityHeaders,
		logRequests(logger),
	)

	router.Get("/health", health.Get)

	router.Route("/api/v1", func(api chi.Router) {
		api.Use(authentication.Handle)

		api.Get("/plugins", plugins.List)

		api.Route("/executions", func(routes chi.Router) {
			routes.Use(middleware.CompanyContext)

			routes.Get("/", executions.List)
			routes.Post("/sync", executions.ExecuteSync)
			routes.Post("/async", executions.ExecuteAsync)

			routes.Get("/{executionID}", executions.Get)
			routes.Get("/{executionID}/definition", executions.GetDefinition)
			routes.Get("/{executionID}/nodes", executions.GetNodes)
			routes.Get("/{executionID}/events", executions.GetEvents)
			routes.Get("/{executionID}/logs", executions.GetLogs)
			routes.Get("/{executionID}/errors", executions.GetErrors)
			routes.Post("/{executionID}/recover", executions.Recover)
		})
	})

	router.NotFound(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		controller.WriteAPIError(
			writer,
			request,
			nethttp.StatusNotFound,
			"NOT_FOUND",
			"The requested resource was not found.",
		)
	})

	router.MethodNotAllowed(func(writer nethttp.ResponseWriter, request *nethttp.Request) {
		controller.WriteAPIError(
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
					logger.Error(
						"HTTP handler panicked",
						"error",
						recovered,
					)

					nethttp.Error(
						writer,
						`{"code":"INTERNAL_SERVER_ERROR"}`,
						nethttp.StatusInternalServerError,
					)
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

			logger.Info(
				"HTTP request",
				"method",
				request.Method,
				"path",
				request.URL.Path,
				"duration",
				time.Since(started),
			)
		})
	}
}
