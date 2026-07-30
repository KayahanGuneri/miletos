package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"miletos-go/internal/config"
	"miletos-go/internal/features/workflowruntime/execution"
	"miletos-go/internal/features/workflowruntime/execution/queue"
	"miletos-go/internal/features/workflowruntime/execution/repository"
	"miletos-go/internal/features/workflowruntime/httptrigger"
	"miletos-go/internal/features/workflowruntime/plugin"
	"miletos-go/internal/features/workflowruntime/workflow"
	"miletos-go/internal/health"
	"miletos-go/internal/shared/database"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
	runtimehttp "miletos-go/internal/shared/http"
	"miletos-go/internal/shared/security"
)

const shutdownTimeout = 10 * time.Second

func main() {
	configuration, err := config.Load()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "configuration loading failed:", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: configuration.LogLevel,
	})).With(
		"service", configuration.ServiceName,
		"environment", configuration.Environment,
	)
	if err := run(configuration, logger); err != nil {
		logger.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(configuration config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbClient, err := database.Open(ctx, configuration.PostgreSQLURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := dbClient.Close(); err != nil {
			logger.Warn("close database connection", "error", err)
		}
	}()

	workflowRepository := workflow.NewWorkflowRepository(dbClient)
	executionRepository := repository.NewExecutionRepository(dbClient)
	outboxRepository := repository.NewOutboxRepository(dbClient)

	var nodeQueue queue.Queue
	var kafkaQueue *queue.Kafka
	if configuration.KafkaEnabled {
		kafkaQueue, err = queue.NewKafkaWithConcurrency(
			configuration.KafkaBrokers,
			configuration.KafkaClientID,
			configuration.KafkaConsumerGroup,
			configuration.NodeConcurrency,
			configuration.KafkaDeadLetterTopic,
		)
		if err != nil {
			return err
		}
		defer kafkaQueue.Close()
		nodeQueue = kafkaQueue
	}

	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		return err
	}
	workflowService := workflow.NewWorkflowService(workflowRepository, registry)
	scheduler := execution.NewScheduler(
		workflowRepository, executionRepository,
		nodeQueue, configuration.KafkaCommandTopic, registry,
	)
	nodeProcessor := execution.NewNodeProcessor(
		workflowRepository, executionRepository, registry, scheduler,
		nodeQueue, configuration.KafkaCommandTopic,
		configuration.RetryMaxAttempts, configuration.RetryDelay,
	)
	outboxDispatcher, err := execution.NewOutboxDispatcher(outboxRepository, nodeQueue)
	if err != nil {
		return err
	}
	reconciler := execution.NewReconciler(
		executionRepository,
		workflowRepository,
		scheduler,
		nodeProcessor,
		outboxRepository,
		configuration.KafkaCommandTopic,
		configuration.ReconciliationQueuedStale,
		configuration.ReconciliationRunningStale,
		configuration.ReconciliationRetryPendingStale,
	)
	executionService := execution.NewExecutionService(
		workflowService, executionRepository, scheduler,
		nodeProcessor, configuration.KafkaEnabled,
	)
	httpTriggerRepository := httptrigger.NewRepository(dbClient)
	httpTriggerService := httptrigger.NewService(
		configuration.PublicTriggerBaseURL,
		httpTriggerRepository,
		workflowRepository,
		workflowService,
		executionService,
	)
	recoveryService := execution.NewRecoveryService(
		workflowRepository, executionRepository, workflowService, scheduler,
	)

	healthController := &health.Controller{}
	pluginController := plugin.NewController(registry)
	executionController := execution.NewExecutionController(
		executionService, recoveryService, workflowRepository, executionRepository,
	)
	publicTriggerController := httptrigger.NewPublicController(httpTriggerService)
	authentication := security.NewAuthentication(configuration.InternalServiceToken)
	router := runtimehttp.NewRouter(
		logger,
		authentication,
		runtimehttp.RouteHandlers{
			Health:                 healthController.Get,
			PublicTrigger:          publicTriggerController.Invoke,
			ListPlugins:            pluginController.List,
			ListExecutions:         executionController.List,
			ExecuteSync:            executionController.ExecuteSync,
			ExecuteAsync:           executionController.ExecuteAsync,
			GetExecution:           executionController.Get,
			GetExecutionDefinition: executionController.GetDefinition,
			GetExecutionNodes:      executionController.GetNodes,
			GetExecutionEvents:     executionController.GetEvents,
			GetExecutionLogs:       executionController.GetLogs,
			GetExecutionErrors:     executionController.GetErrors,
			RecoverExecution:       executionController.Recover,
		},
	)
	server := runtimehttp.NewServer(configuration.HTTPAddress(), router)
	grpcListener, err := net.Listen("tcp", configuration.GRPCAddress())
	if err != nil {
		return fmt.Errorf("listen for gRPC: %w", err)
	}
	defer grpcListener.Close()
	grpcServer := grpcserver.NewServer(
		logger,
		security.NewInternalTokenValidator(configuration.InternalServiceToken),
	)
	runtimev1.RegisterPluginServiceServer(
		grpcServer, plugin.NewGRPCService(registry),
	)
	runtimev1.RegisterExecutionServiceServer(
		grpcServer,
		execution.NewGRPCService(
			executionService, recoveryService, workflowRepository, executionRepository,
		),
	)
	runtimev1.RegisterHTTPTriggerServiceServer(
		grpcServer, httptrigger.NewGRPCService(httpTriggerService),
	)

	if configuration.KafkaEnabled {
		if err := reconciler.RunOnce(ctx); err != nil {
			return fmt.Errorf("startup reconciliation: %w", err)
		}
	}

	failures := make(chan error, 4)
	go func() {
		logger.Info("HTTP server started", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failures <- fmt.Errorf("HTTP server: %w", err)
		}
	}()
	go func() {
		logger.Info("gRPC server started", "address", configuration.GRPCAddress())
		if err := grpcServer.Serve(grpcListener); err != nil {
			failures <- fmt.Errorf("gRPC server: %w", err)
		}
	}()
	if configuration.KafkaEnabled {
		go outboxDispatcher.Run(ctx)
		go func() {
			logger.Info(
				"node processor started",
				"topic", configuration.KafkaCommandTopic,
				"concurrency", configuration.NodeConcurrency,
			)
			if err := nodeProcessor.Run(ctx); err != nil && ctx.Err() == nil {
				failures <- fmt.Errorf("node processor: %w", err)
			}
		}()
		if configuration.ReconciliationEnabled {
			go func() {
				ticker := time.NewTicker(configuration.ReconciliationInterval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if err := reconciler.RunOnce(ctx); err != nil && ctx.Err() == nil {
							logger.Warn("workflow reconciliation failed", "error", err)
						}
					}
				}
			}()
		}
	}

	var serviceFailure error
	select {
	case <-ctx.Done():
	case err := <-failures:
		stop()
		serviceFailure = err
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		serviceFailure = errors.Join(serviceFailure, fmt.Errorf("shut down HTTP server: %w", err))
	}
	if err := stopGRPC(shutdownContext, grpcServer); err != nil {
		serviceFailure = errors.Join(serviceFailure, err)
	}
	logger.Info("service stopped")
	return serviceFailure
}

func stopGRPC(ctx context.Context, server *grpc.Server) error {
	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
		return nil
	case <-ctx.Done():
		server.Stop()
		<-stopped
		return fmt.Errorf("graceful gRPC shutdown exceeded deadline: %w", ctx.Err())
	}
}
