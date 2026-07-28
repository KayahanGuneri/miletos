package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"miletos-go/internal/bootstrap"
	"miletos-go/internal/config"
)

const version = "development"

func main() {
	bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	configuration, err := config.Load()
	if err != nil {
		bootstrapLogger.Error("configuration loading failed", "error", err)
		os.Exit(1)
	}
	httpConfiguration, err := config.LoadHTTPAPI()
	if err != nil {
		bootstrapLogger.Error("HTTP API configuration loading failed", "error", err)
		os.Exit(1)
	}
	logger := newLogger(configuration)
	if err := run(configuration, httpConfiguration, logger); err != nil {
		logger.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(
	configuration config.Config,
	httpConfiguration config.HTTPAPIConfig,
	logger *slog.Logger,
) error {
	if logger == nil {
		return fmt.Errorf("logger must not be nil")
	}
	ctx, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	container, err := bootstrap.NewEngineContainer(
		ctx,
		configuration,
		httpConfiguration,
		logger,
		version,
	)
	if err != nil {
		return err
	}
	defer container.Close()
	return container.Run(ctx)
}

func newLogger(configuration config.Config) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: configuration.LogLevel,
	})
	return slog.New(handler).With(
		"service", configuration.ServiceName,
		"environment", configuration.Environment,
	)
}
