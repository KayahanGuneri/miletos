package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"miletos-go/internal/config"
	"miletos-go/internal/engine"
	enginepersistence "miletos-go/internal/engine/lifecycle/persistence"
	"miletos-go/internal/engine/nodes/core"
	"miletos-go/internal/engine/plugin"
	engineruntime "miletos-go/internal/engine/runtime"
	"miletos-go/internal/engine/worker"
	executiondomain "miletos-go/internal/features/execution"
	executionfeature "miletos-go/internal/features/execution/application"
	healthfeature "miletos-go/internal/features/health"
	api "miletos-go/internal/infra/http"
	executionhttp "miletos-go/internal/infra/http/execution"
	"miletos-go/internal/infra/kafka"
	"miletos-go/internal/infra/kafka/protocol/kafkav1"
	"miletos-go/internal/infra/postgres"
	repository "miletos-go/internal/ports/persistence"
	sharedclock "miletos-go/internal/shared/clock"
)

const (
	enginePublisherBatchLimit               = 100
	enginePublisherPollInterval             = 100 * time.Millisecond
	enginePublisherStaleAfter               = 30 * time.Second
	engineInterruptedAttemptAuditBatchLimit = 100
)

type EngineContainer struct {
	configuration config.Config
	logger        *slog.Logger
	version       string
	server        *http.Server
	runtime       *engineRuntime
}

type engineRuntime struct {
	pool       *pgxpool.Pool
	store      *postgres.Store
	plugins    plugin.Registry
	execution  engine.ExecutionService
	async      *engineAsyncRuntime
	kafkaProbe *kafkaReadinessProbe
}

type engineAsyncRuntime struct {
	producer            *kafka.Producer
	resultConsumer      *engine.AsyncResultConsumer
	publisher           *kafka.OutboxPublisher
	execution           engine.ExecutionService
	coordinator         engine.AsyncCoordinator
	interruptionAuditor *engine.InterruptedAttemptAuditor
}

type kafkaReadinessProbe struct {
	client *kgo.Client
}

func NewEngineContainer(
	ctx context.Context,
	configuration config.Config,
	httpConfiguration config.HTTPAPIConfig,
	logger *slog.Logger,
	version string,
) (*EngineContainer, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	engineRuntime, err := newEngineRuntime(ctx, configuration)
	if err != nil {
		return nil, fmt.Errorf("initialize engine runtime: %w", err)
	}
	closeRuntimeOnError := true
	defer func() {
		if closeRuntimeOnError {
			engineRuntime.close()
		}
	}()

	authenticator, err := api.NewInternalAuthenticator(httpConfiguration.InternalServiceToken)
	if err != nil {
		return nil, fmt.Errorf("initialize internal HTTP authentication: %w", err)
	}
	engineExecutionApplication, err := executionfeature.NewEngineExecutionApplication(engineRuntime.execution)
	if err != nil {
		return nil, fmt.Errorf("initialize execution application: %w", err)
	}
	executionApplication, err := executionfeature.NewIdempotentExecutionApplication(
		engineExecutionApplication,
		engineRuntime.store,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize idempotent execution application: %w", err)
	}
	executionQueries, err := executionfeature.NewRepositoryExecutionQueries(engineRuntime.store)
	if err != nil {
		return nil, fmt.Errorf("initialize execution queries: %w", err)
	}
	var recoveryApplication executionfeature.PartialRecoveryApplication
	if engineRuntime.async != nil {
		recoveryService, err := engine.NewPartialRecoveryService(
			engineRuntime.store,
			engineRuntime.async.coordinator,
			sharedclock.System(),
		)
		if err != nil {
			return nil, fmt.Errorf("initialize partial recovery service: %w", err)
		}
		application, err := executionfeature.NewEnginePartialRecoveryApplication(recoveryService)
		if err != nil {
			return nil, fmt.Errorf("initialize partial recovery application: %w", err)
		}
		recoveryApplication = application
	}

	var kafkaProbe healthfeature.ReadinessProbe
	if engineRuntime.kafkaProbe != nil {
		kafkaProbe = engineRuntime.kafkaProbe
	}
	readiness := healthfeature.NewReadinessService(
		engineRuntime.pool,
		engineRuntime.plugins,
		kafkaProbe,
		configuration.Kafka.Enabled,
	)
	handler := api.NewHandler(configuration.ServiceName, version)
	router := api.NewRouterWithOptions(handler, api.RouterOptions{
		ExecutionHandler: executionhttp.NewHandler(
			executionApplication, executionQueries, recoveryApplication),
		HealthHandler:  api.NewHealthHandler(readiness),
		PluginHandler:  api.NewPluginHandler(engineRuntime.plugins),
		Logger:         logger,
		HandlerTimeout: httpConfiguration.HandlerTimeout,
		Authenticator:  &authenticator,
		SwaggerEnabled: httpConfiguration.SwaggerEnabled,
	})
	server := &http.Server{
		Addr:              configuration.HTTPAddress(),
		Handler:           router,
		ReadHeaderTimeout: configuration.HTTPReadHeaderTimeout,
		ReadTimeout:       configuration.HTTPReadTimeout,
		WriteTimeout:      configuration.HTTPWriteTimeout,
		IdleTimeout:       configuration.HTTPIdleTimeout,
	}

	closeRuntimeOnError = false
	return &EngineContainer{
		configuration: configuration,
		logger:        logger,
		version:       version,
		server:        server,
		runtime:       engineRuntime,
	}, nil
}

func newEngineRuntime(ctx context.Context, configuration config.Config) (*engineRuntime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("engine runtime context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pool, err := postgres.OpenPool(ctx, configuration.PostgreSQL)
	if err != nil {
		return nil, fmt.Errorf("open engine PostgreSQL pool: %w", err)
	}
	closePoolOnError := true
	defer func() {
		if closePoolOnError {
			pool.Close()
		}
	}()

	store, err := postgres.NewStore(pool)
	if err != nil {
		return nil, fmt.Errorf("create engine PostgreSQL store: %w", err)
	}
	limits, err := engineruntime.NewRuntimeLimits(100, 100, configuration.Kafka.MaxMessageBytes)
	if err != nil {
		return nil, fmt.Errorf("create runtime limits: %w", err)
	}
	descriptors, err := core.CoreDescriptors()
	if err != nil {
		return nil, fmt.Errorf("create core plugin descriptors: %w", err)
	}
	plugins, err := plugin.NewRegistry(descriptors)
	if err != nil {
		return nil, fmt.Errorf("create plugin registry: %w", err)
	}
	registrations, err := core.DefaultExecutorRegistrations(limits)
	if err != nil {
		return nil, fmt.Errorf("create core executor registrations: %w", err)
	}
	executors, err := engineruntime.NewExecutorRegistry(registrations)
	if err != nil {
		return nil, fmt.Errorf("create executor registry: %w", err)
	}
	recorder, err := enginepersistence.NewRecorder(store)
	if err != nil {
		return nil, fmt.Errorf("create execution lifecycle recorder: %w", err)
	}
	dependencies, err := engine.NewEngineDependencies(plugins, executors, limits, sharedclock.System())
	if err != nil {
		return nil, fmt.Errorf("create engine dependencies: %w", err)
	}
	dependencies, err = dependencies.WithLifecycleRecorder(recorder)
	if err != nil {
		return nil, fmt.Errorf("attach lifecycle recorder: %w", err)
	}
	maxAttempts, err := executiondomain.NewAttemptNumber(
		configuration.FaultTolerance.RetryMaxAttempts,
	)
	if err != nil {
		return nil, fmt.Errorf("create retry policy max attempts: %w", err)
	}
	retryPolicy, err := executiondomain.NewRetryPolicy(
		maxAttempts,
		configuration.FaultTolerance.RetryInitialBackoff,
		configuration.FaultTolerance.RetryMaxBackoff,
	)
	if err != nil {
		return nil, fmt.Errorf("create retry policy: %w", err)
	}
	dependencies, err = dependencies.WithRetryPolicy(retryPolicy)
	if err != nil {
		return nil, fmt.Errorf("attach retry policy: %w", err)
	}
	syncRunner, err := engine.NewSyncRunner(dependencies)
	if err != nil {
		return nil, fmt.Errorf("create synchronous runner: %w", err)
	}
	executionService, err := engine.NewSyncExecutionService(syncRunner)
	if err != nil {
		return nil, fmt.Errorf("create synchronous execution service: %w", err)
	}
	asyncRuntime, err := newEngineAsyncRuntime(configuration, store, plugins, dependencies, syncRunner)
	if err != nil {
		return nil, fmt.Errorf("create asynchronous engine runtime: %w", err)
	}
	if asyncRuntime != nil {
		executionService = asyncRuntime.execution
	}
	kafkaProbe, err := newKafkaReadinessProbe(configuration.Kafka)
	if err != nil {
		if asyncRuntime != nil {
			asyncRuntime.close()
		}
		return nil, err
	}

	closePoolOnError = false
	return &engineRuntime{
		pool:       pool,
		store:      store,
		plugins:    plugins,
		execution:  executionService,
		async:      asyncRuntime,
		kafkaProbe: kafkaProbe,
	}, nil
}

func newEngineAsyncRuntime(
	configuration config.Config,
	store *postgres.Store,
	plugins plugin.Registry,
	dependencies engine.EngineDependencies,
	syncRunner engine.SyncRunner,
) (*engineAsyncRuntime, error) {
	if !configuration.Kafka.Enabled {
		return nil, nil
	}
	if configuration.PostgreSQL.MaxConnections < 2 {
		return nil, fmt.Errorf(
			"PostgreSQL max connections must be at least two while interrupted attempt ownership auditing is enabled",
		)
	}
	if store == nil || !store.IsValid() {
		return nil, fmt.Errorf("asynchronous engine store must be valid")
	}
	if plugins.Len() == 0 {
		return nil, fmt.Errorf("asynchronous engine plugin registry must not be empty")
	}
	if !dependencies.IsValid() {
		return nil, fmt.Errorf("asynchronous engine dependencies must be valid")
	}
	if !syncRunner.IsValid() {
		return nil, fmt.Errorf("asynchronous engine sync runner must be valid")
	}
	codec, err := kafkav1.NewCodec(
		configuration.Kafka.MaxMessageBytes,
		configuration.Kafka.MaxMessageBytes,
	)
	if err != nil {
		return nil, err
	}
	coordinator := engine.AsyncCoordinator{
		Outbox: store,
		Codec:  codec,
		Topic:  configuration.Kafka.CommandTopic,
	}
	asyncRunner, err := engine.NewAsyncRunner(dependencies, store, coordinator)
	if err != nil {
		return nil, err
	}
	executionService, err := engine.NewExecutionService(syncRunner, asyncRunner)
	if err != nil {
		return nil, err
	}
	consumerIdentity, err := repository.NewConsumerIdentity(configuration.Kafka.EngineGroupID)
	if err != nil {
		return nil, err
	}
	processor, err := engine.NewAsyncResultProcessor(
		codec,
		store,
		store,
		plugins,
		coordinator,
		consumerIdentity,
	)
	if err != nil {
		return nil, err
	}
	kafkaConsumer, err := kafka.NewConsumer(
		configuration.Kafka.Brokers,
		configuration.Kafka.EngineClientID,
		configuration.Kafka.EngineGroupID,
		configuration.Kafka.EventTopic,
	)
	if err != nil {
		return nil, err
	}
	resultConsumer, err := engine.NewAsyncResultConsumer(kafkaConsumer, processor)
	if err != nil {
		kafkaConsumer.Close()
		return nil, err
	}
	producer, err := kafka.NewProducer(
		configuration.Kafka.Brokers,
		configuration.Kafka.EngineClientID+"-outbox",
		configuration.Kafka.MaxMessageBytes,
	)
	if err != nil {
		resultConsumer.Close()
		return nil, err
	}
	publisher, err := kafka.NewOutboxPublisher(
		store,
		producer,
		configuration.Kafka.EngineClientID,
		enginePublisherBatchLimit,
		enginePublisherPollInterval,
		enginePublisherStaleAfter,
	)
	if err != nil {
		producer.Close()
		resultConsumer.Close()
		return nil, err
	}
	interruptionAuditor, err := engine.NewInterruptedAttemptAuditor(
		store,
		sharedclock.System(),
		configuration.FaultTolerance.InterruptedAttemptAuditInterval,
		engineInterruptedAttemptAuditBatchLimit,
	)
	if err != nil {
		producer.Close()
		resultConsumer.Close()
		return nil, err
	}
	return &engineAsyncRuntime{
		producer:            producer,
		resultConsumer:      resultConsumer,
		publisher:           publisher,
		execution:           executionService,
		coordinator:         coordinator,
		interruptionAuditor: interruptionAuditor,
	}, nil
}

func newKafkaReadinessProbe(configuration config.KafkaConfig) (*kafkaReadinessProbe, error) {
	if !configuration.Enabled {
		return nil, nil
	}
	client, err := kgo.NewClient(
		kgo.SeedBrokers(configuration.Brokers...),
		kgo.ClientID(configuration.EngineClientID+"-readiness"),
	)
	if err != nil {
		return nil, fmt.Errorf("create Kafka readiness client: %w", err)
	}
	return &kafkaReadinessProbe{client: client}, nil
}

type WorkerContainer struct {
	configuration config.Config
	logger        *slog.Logger
	pool          *pgxpool.Pool
	consumer      *kafka.Consumer
	workerPool    *worker.Pool
	worker        *worker.Worker
}

func NewWorkerContainer(
	ctx context.Context,
	configuration config.Config,
	logger *slog.Logger,
) (*WorkerContainer, error) {
	if ctx == nil {
		return nil, fmt.Errorf("worker context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}
	if configuration.PostgreSQL.MaxConnections < int32(configuration.WorkerConcurrency*2) {
		return nil, fmt.Errorf(
			"PostgreSQL max connections must be at least twice worker concurrency while session execution locks are enabled",
		)
	}

	pool, err := postgres.OpenPool(ctx, configuration.PostgreSQL)
	if err != nil {
		return nil, err
	}
	closePoolOnError := true
	defer func() {
		if closePoolOnError {
			pool.Close()
		}
	}()
	store, err := postgres.NewStore(pool)
	if err != nil {
		return nil, err
	}
	codec, err := kafkav1.NewCodec(
		configuration.Kafka.MaxMessageBytes,
		configuration.Kafka.MaxMessageBytes,
	)
	if err != nil {
		return nil, err
	}
	consumer, err := kafka.NewConsumer(
		configuration.Kafka.Brokers,
		configuration.Kafka.WorkerClientID,
		configuration.Kafka.WorkerGroupID,
		configuration.Kafka.CommandTopic,
	)
	if err != nil {
		return nil, err
	}
	closeConsumerOnError := true
	defer func() {
		if closeConsumerOnError {
			consumer.Close()
		}
	}()
	workerPool, err := worker.NewPool(
		ctx,
		configuration.WorkerConcurrency,
		configuration.WorkerConcurrency*2,
	)
	if err != nil {
		return nil, err
	}
	closeWorkerPoolOnError := true
	defer func() {
		if closeWorkerPoolOnError {
			_ = workerPool.Shutdown(context.Background())
		}
	}()
	identity, err := repository.NewConsumerIdentity(configuration.Kafka.WorkerGroupID)
	if err != nil {
		return nil, err
	}
	limits, err := engineruntime.NewRuntimeLimits(100, 100, configuration.Kafka.MaxMessageBytes)
	if err != nil {
		return nil, err
	}
	registrations, err := core.DefaultExecutorRegistrations(limits)
	if err != nil {
		return nil, err
	}
	registry, err := engineruntime.NewExecutorRegistry(registrations)
	if err != nil {
		return nil, err
	}
	descriptors, err := core.CoreDescriptors()
	if err != nil {
		return nil, err
	}
	pluginRegistry, err := plugin.NewRegistry(descriptors)
	if err != nil {
		return nil, err
	}
	executor := worker.RegistryExecutor{
		Registry: registry,
		Plugins:  pluginRegistry,
		Limits:   limits,
		Store:    store,
	}
	completionBuilder := worker.NewCompletionBuilder(codec, configuration.Kafka.EventTopic)
	runtimeWorker, err := worker.NewWorker(
		codec,
		store,
		store,
		executor,
		consumer,
		workerPool,
		identity,
		completionBuilder,
	)
	if err != nil {
		return nil, err
	}

	closeWorkerPoolOnError = false
	closeConsumerOnError = false
	closePoolOnError = false
	return &WorkerContainer{
		configuration: configuration,
		logger:        logger,
		pool:          pool,
		consumer:      consumer,
		workerPool:    workerPool,
		worker:        runtimeWorker,
	}, nil
}
