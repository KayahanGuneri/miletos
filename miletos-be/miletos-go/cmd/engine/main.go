package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"miletos-go/internal/config"
	"miletos-go/internal/controller"
	"miletos-go/internal/engine"
	runtimehttp "miletos-go/internal/http"
	"miletos-go/internal/middleware"
	"miletos-go/internal/queue"
	"miletos-go/internal/repository"
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

	database, err := repository.Open(ctx, configuration.PostgreSQLURL)
	if err != nil {
		return err
	}
	defer database.Close()

	workflowRepository := repository.NewWorkflowRepository(database)
	executionRepository := repository.NewExecutionRepository(database)

	var nodeQueue queue.Queue
	var kafkaQueue *queue.Kafka
	if configuration.KafkaEnabled {
		kafkaQueue, err = queue.NewKafkaWithConcurrency(
			configuration.KafkaBrokers,
			configuration.KafkaClientID,
			configuration.KafkaConsumerGroup,
			configuration.NodeConcurrency,
		)
		if err != nil {
			return err
		}
		defer kafkaQueue.Close()
		nodeQueue = kafkaQueue
	}

	registry := engine.NewNodeRegistry()
	if err := engine.RegisterBuiltinNodes(registry); err != nil {
		return err
	}
	workflowService := engine.NewWorkflowService(workflowRepository, registry)
	scheduler := engine.NewScheduler(
		workflowRepository, executionRepository,
		nodeQueue, configuration.KafkaCommandTopic, registry,
	)
	nodeProcessor := engine.NewNodeProcessor(
		workflowRepository, executionRepository, registry, scheduler,
		nodeQueue, configuration.KafkaCommandTopic,
		configuration.RetryMaxAttempts, configuration.RetryDelay,
	)
	executionService := engine.NewExecutionService(
		workflowService, executionRepository, scheduler,
		nodeProcessor, configuration.KafkaEnabled,
	)
	recoveryService := engine.NewRecoveryService(
		workflowRepository, executionRepository, workflowService, scheduler,
	)

	healthController := &controller.HealthController{}
	pluginController := controller.NewPluginController(registry)
	executionController := controller.NewExecutionController(
		executionService, recoveryService, workflowRepository, executionRepository,
	)
	authentication := middleware.NewAuthentication(configuration.InternalServiceToken)
	router := runtimehttp.NewRouter(
		logger, authentication, healthController, pluginController, executionController,
	)
	server := runtimehttp.NewServer(configuration.HTTPAddress(), router)

	failures := make(chan error, 2)
	go func() {
		logger.Info("HTTP server started", "address", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failures <- fmt.Errorf("HTTP server: %w", err)
		}
	}()
	if configuration.KafkaEnabled {
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
	}

	select {
	case <-ctx.Done():
	case err := <-failures:
		stop()
		return err
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("shut down HTTP server: %w", err)
	}
	logger.Info("service stopped")
	return nil
}
