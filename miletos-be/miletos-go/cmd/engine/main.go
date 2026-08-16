package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"miletos-go/internal/bootstrap"
	"miletos-go/internal/config"
)

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		_, _ = fmt.Fprintln(os.Stderr, "environment loading failed:", err)
		os.Exit(1)
	}

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := bootstrap.Build(ctx, configuration, logger)
	if err == nil {
		err = application.Run(ctx)
	}
	if err != nil {
		logger.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
}
