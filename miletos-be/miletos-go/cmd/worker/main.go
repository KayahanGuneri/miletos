package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"miletos-go/internal/bootstrap"
	"miletos-go/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	container, err := bootstrap.NewWorkerContainer(ctx, configuration, logger)
	if err != nil {
		return err
	}
	defer container.Close()
	return container.Run(ctx)
}
