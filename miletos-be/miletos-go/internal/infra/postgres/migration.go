package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"miletos-go/internal/config"
)

const (
	migrationApplicationName  = "miletos-go-migrate"
	migrationVersionTableName = "workflow_runtime_goose_db_version"
)

func newMigrationConnectionConfig(
	configuration config.PostgreSQLConfig,
) (*pgx.ConnConfig, error) {
	poolConfiguration, err := NewPoolConfig(configuration)
	if err != nil {
		return nil, fmt.Errorf("create migration connection configuration: %w", err)
	}
	connectionConfiguration := poolConfiguration.ConnConfig.Copy()
	if connectionConfiguration.RuntimeParams == nil {
		connectionConfiguration.RuntimeParams = make(map[string]string)
	}
	delete(connectionConfiguration.RuntimeParams, "search_path")
	connectionConfiguration.RuntimeParams["application_name"] = migrationApplicationName
	return connectionConfiguration, nil
}

func OpenMigrationDatabase(
	parentContext context.Context,
	configuration config.PostgreSQLConfig,
) (*sql.DB, error) {
	if parentContext == nil {
		return nil, fmt.Errorf("migration database parent context must not be nil")
	}
	connectionConfiguration, err := newMigrationConnectionConfig(configuration)
	if err != nil {
		return nil, err
	}
	database := stdlib.OpenDB(*connectionConfiguration)
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	connectContext, cancelConnect := context.WithTimeout(
		parentContext,
		configuration.ConnectTimeout,
	)
	defer cancelConnect()
	if err := database.PingContext(connectContext); err != nil {
		closeErr := database.Close()
		if closeErr != nil {
			return nil, fmt.Errorf(
				"ping migration database: %v; close migration database: %w",
				err,
				closeErr,
			)
		}
		return nil, fmt.Errorf("ping migration database: %w", err)
	}
	return database, nil
}

type MigrationCommand string

const (
	MigrationCommandUp      MigrationCommand = "up"
	MigrationCommandStatus  MigrationCommand = "status"
	MigrationCommandVersion MigrationCommand = "version"
)

type MigrationResult struct {
	Version    int64
	HasVersion bool
}

func ParseMigrationCommand(value string) (MigrationCommand, error) {
	command := MigrationCommand(strings.ToLower(strings.TrimSpace(value)))
	if !command.IsValid() {
		return "", fmt.Errorf(
			"unsupported migration command %q; supported commands: up, status, version",
			value,
		)
	}
	return command, nil
}

func (command MigrationCommand) IsValid() bool {
	switch command {
	case MigrationCommandUp,
		MigrationCommandStatus,
		MigrationCommandVersion:
		return true
	default:
		return false
	}
}

func RunMigrationCommand(
	parentContext context.Context,
	database *sql.DB,
	migrationsPath string,
	command MigrationCommand,
) (MigrationResult, error) {
	if parentContext == nil {
		return MigrationResult{}, fmt.Errorf("migration parent context must not be nil")
	}
	if database == nil {
		return MigrationResult{}, fmt.Errorf("migration database must not be nil")
	}
	if !command.IsValid() {
		return MigrationResult{}, fmt.Errorf("unsupported migration command %q", command)
	}
	normalizedPath, err := validateMigrationsPath(migrationsPath)
	if err != nil {
		return MigrationResult{}, err
	}
	if err := configureGoose(); err != nil {
		return MigrationResult{}, err
	}
	switch command {
	case MigrationCommandUp:
		if err := goose.UpContext(parentContext, database, normalizedPath); err != nil {
			return MigrationResult{},
				fmt.Errorf("apply PostgreSQL migrations: %w", err)
		}
	case MigrationCommandStatus:
		if err := goose.StatusContext(parentContext, database, normalizedPath); err != nil {
			return MigrationResult{},
				fmt.Errorf("read PostgreSQL migration status: %w", err)
		}
	case MigrationCommandVersion:
	}
	version, err := goose.GetDBVersionContext(parentContext, database)
	if err != nil {
		return MigrationResult{},
			fmt.Errorf("read PostgreSQL migration version: %w", err)
	}
	return MigrationResult{Version: version, HasVersion: true}, nil
}

func validateMigrationsPath(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", fmt.Errorf("migrations path must not be empty")
	}
	info, err := os.Stat(normalized)
	if err != nil {
		return "", fmt.Errorf("inspect migrations path %q: %w", normalized, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("migrations path %q must be a directory", normalized)
	}
	return normalized, nil
}

func configureGoose() error {
	goose.SetTableName(migrationVersionTableName)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configure Goose PostgreSQL dialect: %w", err)
	}
	return nil
}
