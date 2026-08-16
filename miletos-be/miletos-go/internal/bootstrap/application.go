package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"

	"miletos-go/internal/config"
	crontrigger "miletos-go/internal/context-provider/cron-trigger"
	dataarrival "miletos-go/internal/context-provider/data-arrival"
	httptrigger "miletos-go/internal/context-provider/http-trigger"
	triggerlifecycle "miletos-go/internal/context-provider/trigger-lifecycle"
	"miletos-go/internal/features/workflow-runtime/execution"
	"miletos-go/internal/features/workflow-runtime/execution/queue"
	"miletos-go/internal/features/workflow-runtime/execution/repository"
	"miletos-go/internal/features/workflow-runtime/plugin"
	pluginstate "miletos-go/internal/features/workflow-runtime/plugin-state"
	"miletos-go/internal/features/workflow-runtime/workflow"
	"miletos-go/internal/health"
	"miletos-go/internal/shared/database"
	grpcserver "miletos-go/internal/shared/grpc"
	runtimev1 "miletos-go/internal/shared/grpc/generated/runtimev1"
	runtimehttp "miletos-go/internal/shared/http"
	"miletos-go/internal/shared/secrets"
	"miletos-go/internal/shared/security"
)

const shutdownTimeout = 10 * time.Second

type Application struct {
	configuration    config.Config
	logger           *slog.Logger
	database         *database.Client
	kafka            *queue.Kafka
	pluginKafka      *queue.Kafka
	httpServer       *http.Server
	grpcServer       *grpc.Server
	grpcListener     net.Listener
	nodeProcessor    *execution.NodeProcessor
	outboxDispatcher *execution.OutboxDispatcher
	reconciler       *execution.Reconciler
	cronScheduler    *crontrigger.Scheduler
	arrivalScheduler *dataarrival.Scheduler
	pluginEmitter    *execution.PluginEmitter
}

func Build(
	ctx context.Context,
	configuration config.Config,
	logger *slog.Logger,
) (_ *Application, buildErr error) {
	if err := validateServerPort("MILETOS_RUNTIME_HTTP_PORT", configuration.HTTPPort); err != nil {
		return nil, err
	}
	if err := validateServerPort("MILETOS_RUNTIME_GRPC_PORT", configuration.GRPCPort); err != nil {
		return nil, err
	}
	tokenValidator, err := security.NewInternalTokenValidator(
		configuration.InternalServiceToken,
	)
	if err != nil {
		return nil, err
	}
	dbClient, err := database.Open(ctx, configuration.PostgreSQLURL)
	if err != nil {
		return nil, err
	}
	application := &Application{
		configuration: configuration,
		logger:        logger,
		database:      dbClient,
	}
	defer func() {
		if buildErr != nil {
			application.closeResources()
		}
	}()

	workflowRepository := workflow.NewWorkflowRepository(dbClient)
	executionRepository := repository.NewExecutionRepository(dbClient)
	outboxRepository := repository.NewOutboxRepository(dbClient)

	var nodeQueue queue.Queue
	if configuration.KafkaEnabled {
		application.kafka, err = queue.NewKafkaWithConcurrency(
			configuration.KafkaBrokers,
			configuration.KafkaClientID,
			configuration.KafkaConsumerGroup,
			configuration.NodeConcurrency,
			configuration.KafkaCommandTopic,
			configuration.KafkaDeadLetterTopic,
		)
		if err != nil {
			return nil, err
		}
		nodeQueue = application.kafka
		application.pluginKafka, err = queue.NewKafkaWithConcurrency(
			configuration.KafkaBrokers,
			configuration.KafkaClientID+"-plugin-events",
			configuration.KafkaConsumerGroup+"-plugin-events",
			configuration.NodeConcurrency,
			execution.PluginEventTopic,
			configuration.KafkaDeadLetterTopic,
		)
		if err != nil {
			return nil, err
		}
	}
	application.pluginEmitter = execution.NewPluginEmitter(application.pluginKafka)

	registry := plugin.NewNodeRegistry()
	if err := plugin.RegisterBuiltinNodes(registry); err != nil {
		return nil, err
	}
	secretCipher, err := secrets.NewCipher(configuration.SecretsAESKey)
	if err != nil {
		return nil, err
	}
	databaseInfra := execution.NewDatabaseInfrastructure()
	inputRuntime := plugin.InputNodeRuntime{
		Database:       databaseInfra,
		InputDirectory: configuration.InputDirectory,
		Secrets:        secretCipher,
	}
	if err := plugin.RegisterInputSourceNodes(registry, inputRuntime); err != nil {
		return nil, err
	}
	runtimeHTTPClient := &http.Client{Timeout: 15 * time.Second}
	if err := plugin.RegisterOutputDestinationNodes(registry, plugin.OutputNodeRuntime{
		HTTPClient: runtimeHTTPClient,
	}); err != nil {
		return nil, err
	}
	if err := plugin.RegisterDataProcessingNodes(registry); err != nil {
		return nil, err
	}
	stateRepository := pluginstate.NewRepository(dbClient)
	workflowService := workflow.NewWorkflowService(workflowRepository, registry)
	scheduler := execution.NewScheduler(
		workflowRepository, executionRepository, nodeQueue,
		configuration.KafkaCommandTopic, registry,
	)
	application.nodeProcessor, err = execution.NewNodeProcessor(
		workflowRepository, executionRepository, registry, scheduler,
		nodeQueue, configuration.KafkaCommandTopic,
		configuration.RetryMaxAttempts, configuration.RetryDelay,
		stateRepository, application.pluginEmitter,
	)
	if err != nil {
		return nil, err
	}
	application.outboxDispatcher, err = execution.NewOutboxDispatcher(outboxRepository, nodeQueue)
	if err != nil {
		return nil, err
	}
	application.reconciler, err = execution.NewReconciler(
		executionRepository, workflowRepository, scheduler, application.nodeProcessor,
		outboxRepository, configuration.KafkaCommandTopic,
		configuration.ReconciliationInterval,
		configuration.ReconciliationQueuedStale,
		configuration.ReconciliationRunningStale,
		configuration.ReconciliationRetryPendingStale,
	)
	if err != nil {
		return nil, err
	}
	executionService := execution.NewExecutionService(
		workflowService, executionRepository, scheduler,
		application.nodeProcessor, stateRepository,
		configuration.KafkaEnabled, configuration.NodeConcurrency,
	)
	arrivalService := dataarrival.NewService(
		dataarrival.NewRepository(dbClient),
		workflowRepository,
		workflowService,
		executionService,
		registry,
		stateRepository,
	)
	application.arrivalScheduler = dataarrival.NewScheduler(arrivalService)
	application.nodeProcessor.SetWorkflowInfrastructure(
		execution.NewWorkflowInfrastructure(executionService),
	)
	application.nodeProcessor.SetDatabase(databaseInfra)
	application.nodeProcessor.SetSecrets(secretCipher)
	if err := application.pluginEmitter.RegisterSubscriptions(ctx, registry); err != nil {
		return nil, err
	}
	if configuration.CronEnabled && !configuration.KafkaEnabled {
		return nil, fmt.Errorf("cron scheduling requires Kafka-backed asynchronous execution")
	}
	cronService := crontrigger.NewService(
		crontrigger.NewRepository(dbClient), workflowRepository, workflowService, executionService, registry,
	)
	if configuration.CronEnabled {
		application.cronScheduler, err = crontrigger.NewScheduler(
			cronService, configuration.CronPollInterval, configuration.CronBatchSize,
		)
		if err != nil {
			return nil, err
		}
	}
	queryService := execution.NewExecutionQueryService(
		executionRepository, workflowRepository, registry,
	)
	recoveryService := execution.NewRecoveryService(
		workflowRepository, executionRepository, workflowService, scheduler,
	)
	httpTriggerService := httptrigger.NewService(
		configuration.PublicTriggerBaseURL,
		httptrigger.NewRepository(dbClient),
		workflowRepository,
		workflowService,
		executionService,
		registry,
		stateRepository,
		application.pluginEmitter,
	)

	executionController := execution.NewExecutionController(
		executionService, recoveryService, queryService,
	)
	router := runtimehttp.NewRouter(
		logger,
		security.NewAuthentication(tokenValidator),
		runtimehttp.RouteHandlers{
			Health:                 (&health.Controller{}).Get,
			PublicTrigger:          httptrigger.NewPublicController(httpTriggerService).Invoke,
			ListPlugins:            plugin.NewController(registry).List,
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
	application.httpServer = runtimehttp.NewServer(configuration.HTTPAddress(), router)
	application.grpcListener, err = net.Listen("tcp", configuration.GRPCAddress())
	if err != nil {
		return nil, fmt.Errorf("listen for gRPC: %w", err)
	}
	application.grpcServer = grpcserver.NewServer(
		logger,
		tokenValidator,
	)
	runtimev1.RegisterPluginServiceServer(application.grpcServer, plugin.NewGRPCService(registry))
	runtimev1.RegisterExecutionServiceServer(
		application.grpcServer,
		execution.NewGRPCService(executionService, recoveryService, queryService),
	)
	runtimev1.RegisterHTTPTriggerServiceServer(
		application.grpcServer, httptrigger.NewGRPCService(httpTriggerService),
	)
	runtimev1.RegisterCronTriggerServiceServer(
		application.grpcServer, crontrigger.NewGRPCService(cronService),
	)
	runtimev1.RegisterTriggerLifecycleServiceServer(
		application.grpcServer,
		triggerlifecycle.NewGRPCService(httpTriggerService, cronService, arrivalService),
	)
	return application, nil
}

func (application *Application) Run(ctx context.Context) error {
	defer application.closeResources()
	runContext, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	if application.configuration.KafkaEnabled {
		if err := application.reconciler.RunOnce(runContext); err != nil {
			return fmt.Errorf("startup reconciliation: %w", err)
		}
	}
	if application.cronScheduler != nil {
		application.cronScheduler.Start(runContext)
	}
	application.arrivalScheduler.Start(runContext)
	failures := make(chan error, 5)
	go application.runHTTP(failures)
	go application.runGRPC(failures)
	var workers sync.WaitGroup
	if application.configuration.KafkaEnabled {
		workers.Add(3)
		go func() {
			defer workers.Done()
			application.outboxDispatcher.Run(runContext)
		}()
		go func() {
			defer workers.Done()
			application.runNodeProcessor(runContext, failures)
		}()
		go func() {
			defer workers.Done()
			application.runPluginEmitter(runContext, failures)
		}()
		if application.configuration.ReconciliationEnabled {
			workers.Add(1)
			go func() {
				defer workers.Done()
				application.runReconciliation(runContext)
			}()
		}
	}

	var serviceFailure error
	select {
	case <-ctx.Done():
	case serviceFailure = <-failures:
	}
	cancelRun()
	if application.cronScheduler != nil {
		application.cronScheduler.Stop()
	}
	application.arrivalScheduler.Stop()
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	if err := application.httpServer.Shutdown(shutdownContext); err != nil {
		serviceFailure = errors.Join(serviceFailure, fmt.Errorf("shut down HTTP server: %w", err))
	}
	if err := stopGRPC(shutdownContext, application.grpcServer); err != nil {
		serviceFailure = errors.Join(serviceFailure, err)
	}
	workers.Wait()
	application.logger.Info("service stopped")
	return serviceFailure
}

func (application *Application) runHTTP(failures chan<- error) {
	application.logger.Info("HTTP server started", "address", application.httpServer.Addr)
	if err := application.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		failures <- fmt.Errorf("HTTP server: %w", err)
	}
}

func (application *Application) runGRPC(failures chan<- error) {
	application.logger.Info("gRPC server started", "address", application.configuration.GRPCAddress())
	if err := application.grpcServer.Serve(application.grpcListener); err != nil {
		failures <- fmt.Errorf("gRPC server: %w", err)
	}
}

func (application *Application) runNodeProcessor(ctx context.Context, failures chan<- error) {
	application.logger.Info(
		"node processor started",
		"topic", application.configuration.KafkaCommandTopic,
		"concurrency", application.configuration.NodeConcurrency,
	)
	if err := application.nodeProcessor.Run(ctx); err != nil && ctx.Err() == nil {
		failures <- fmt.Errorf("node processor: %w", err)
	}
}

func (application *Application) runPluginEmitter(ctx context.Context, failures chan<- error) {
	application.logger.Info(
		"plugin event dispatcher started",
		"topic", execution.PluginEventTopic,
	)
	if err := application.pluginEmitter.Run(ctx); err != nil && ctx.Err() == nil {
		failures <- fmt.Errorf("plugin event dispatcher: %w", err)
	}
}

func (application *Application) runReconciliation(ctx context.Context) {
	ticker := time.NewTicker(application.reconciler.Interval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := application.reconciler.RunOnce(ctx); err != nil && ctx.Err() == nil {
				application.logger.Warn("workflow reconciliation failed", "error", err)
			}
		}
	}
}

func validateServerPort(settingName string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", settingName)
	}
	return nil
}

func (application *Application) closeResources() {
	if application.grpcListener != nil {
		_ = application.grpcListener.Close()
	}
	if application.kafka != nil {
		application.kafka.Close()
	}
	if application.pluginKafka != nil {
		application.pluginKafka.Close()
	}
	if application.database != nil {
		if err := application.database.Close(); err != nil {
			application.logger.Warn("close database connection", "error", err)
		}
	}
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
