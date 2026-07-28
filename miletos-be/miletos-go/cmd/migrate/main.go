package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"miletos-go/internal/config"
	"miletos-go/internal/infra/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout,
		nil))
	signalContext, stopSignals := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := run(
		signalContext, os.Args[1:], logger,
	); err != nil {
		logger.Error("migration command failed",
			"error", err)
		os.Exit(1)
	}
}
func run(
	parentContext context.Context, arguments []string, logger *slog.Logger,
) error {
	if parentContext == nil {
		return fmt.Errorf(
			"migration parent context must not be nil")
	}
	if logger == nil {
		return fmt.Errorf(
			"migration logger must not be nil")
	}
	if len(arguments) != 1 {
		return fmt.Errorf(
			"usage: go run ./cmd/migrate <up|status|version>")
	}
	command, err := postgres.ParseMigrationCommand(arguments[0])
	if err != nil {
		return err
	}
	configuration, err := config.LoadPostgreSQL()
	if err != nil {
		return fmt.Errorf("load PostgreSQL configuration: %w",
			err)
	}
	logger.Info("opening migration database",
		"command", command, "postgres",
		configuration.MaskedConnectionSummary())
	database, err := postgres.OpenMigrationDatabase(parentContext, configuration)
	if err != nil {
		return err
	}
	defer database.Close()
	result, err := postgres.RunMigrationCommand(parentContext, database,
		configuration.MigrationsPath, command)
	if err != nil {
		return err
	}
	logger.Info("migration command completed",
		"command", command, "version",
		result.Version)
	return nil
}
