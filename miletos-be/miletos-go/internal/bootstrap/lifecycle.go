package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

func (container *EngineContainer) Run(ctx context.Context) error {
	if container == nil || container.server == nil || container.runtime == nil || container.logger == nil {
		return fmt.Errorf("engine container must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("engine runtime context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	runtimeContext, cancelRuntime := context.WithCancel(ctx)
	defer cancelRuntime()
	runtimeErrors := make(chan error, 2)
	go func() {
		container.logger.Info(
			"http server started",
			"address", container.server.Addr,
			"version", container.version,
			"kafka_enabled", container.configuration.Kafka.Enabled,
		)
		err := container.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			runtimeErrors <- fmt.Errorf("http server failed: %w", err)
		}
	}()
	if container.runtime.async != nil {
		go func() {
			if err := container.runtime.async.run(runtimeContext); err != nil {
				runtimeErrors <- fmt.Errorf("async runtime failed: %w", err)
			}
		}()
	}

	var runtimeError error
	select {
	case <-ctx.Done():
		container.logger.Info("shutdown signal received")
	case runtimeError = <-runtimeErrors:
		container.logger.Error("runtime component failed", "error", runtimeError)
	}
	cancelRuntime()

	shutdownContext, cancelShutdown := context.WithTimeout(
		context.Background(),
		container.configuration.ShutdownTimeout,
	)
	defer cancelShutdown()
	shutdownError := container.server.Shutdown(shutdownContext)
	if runtimeError != nil && shutdownError != nil {
		return errors.Join(
			runtimeError,
			fmt.Errorf("HTTP server shutdown failed: %w", shutdownError),
		)
	}
	if runtimeError != nil {
		return runtimeError
	}
	if shutdownError != nil {
		return fmt.Errorf("HTTP server shutdown failed: %w", shutdownError)
	}
	container.logger.Info("service stopped")
	return nil
}

func (container *EngineContainer) Close() {
	if container == nil || container.runtime == nil {
		return
	}
	container.runtime.close()
}

func (runtime *engineAsyncRuntime) run(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("asynchronous runtime context must not be nil")
	}
	errorsChannel := make(chan error, 3)
	go func() {
		errorsChannel <- runtime.publisher.Run(ctx)
	}()
	go func() {
		errorsChannel <- runtime.resultConsumer.Run(ctx)
	}()
	go func() {
		errorsChannel <- runtime.interruptionAuditor.Run(ctx)
	}()
	select {
	case <-ctx.Done():
		return nil
	case err := <-errorsChannel:
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return fmt.Errorf("asynchronous runtime component stopped unexpectedly")
		}
		return fmt.Errorf("engine async runtime stopped: %w", err)
	}
}

func (runtime *engineAsyncRuntime) close() {
	if runtime == nil {
		return
	}
	if runtime.resultConsumer != nil {
		runtime.resultConsumer.Close()
	}
	if runtime.producer != nil {
		runtime.producer.Close()
	}
}

func (runtime *engineRuntime) close() {
	if runtime == nil {
		return
	}
	if runtime.async != nil {
		runtime.async.close()
	}
	if runtime.kafkaProbe != nil {
		runtime.kafkaProbe.close()
	}
	if runtime.pool != nil {
		runtime.pool.Close()
	}
}

func (probe *kafkaReadinessProbe) Ping(ctx context.Context) error {
	if probe == nil || probe.client == nil {
		return fmt.Errorf("Kafka readiness client is unavailable")
	}
	if ctx == nil {
		return fmt.Errorf("Kafka readiness context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := probe.client.Ping(ctx); err != nil {
		return fmt.Errorf("ping Kafka broker: %w", err)
	}
	return nil
}

func (probe *kafkaReadinessProbe) close() {
	if probe == nil || probe.client == nil {
		return
	}
	probe.client.Close()
}

func (container *WorkerContainer) Run(ctx context.Context) error {
	if container == nil || container.worker == nil || container.logger == nil {
		return fmt.Errorf("worker container must be valid")
	}
	if ctx == nil {
		return fmt.Errorf("worker context must not be nil")
	}
	container.logger.Info(
		"worker started",
		"topic", container.configuration.Kafka.CommandTopic,
		"group", container.configuration.Kafka.WorkerGroupID,
		"concurrency", container.configuration.WorkerConcurrency,
	)
	return container.worker.Run(ctx)
}

func (container *WorkerContainer) Close() {
	if container == nil {
		return
	}
	if container.workerPool != nil {
		_ = container.workerPool.Shutdown(context.Background())
	}
	if container.consumer != nil {
		container.consumer.Close()
	}
	if container.pool != nil {
		container.pool.Close()
	}
}
