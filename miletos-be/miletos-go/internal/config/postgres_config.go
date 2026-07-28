package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	environmentPostgreSQLHost           = environmentPrefix + "POSTGRES_HOST"
	environmentPostgreSQLPort           = environmentPrefix + "POSTGRES_PORT"
	environmentPostgreSQLDatabase       = environmentPrefix + "POSTGRES_DATABASE"
	environmentPostgreSQLSchema         = environmentPrefix + "POSTGRES_SCHEMA"
	environmentPostgreSQLUser           = environmentPrefix + "POSTGRES_USER"
	environmentPostgreSQLPassword       = environmentPrefix + "POSTGRES_PASSWORD"
	environmentPostgreSQLSSLMode        = environmentPrefix + "POSTGRES_SSL_MODE"
	environmentPostgreSQLMaxConnections = environmentPrefix + "POSTGRES_MAX_CONNECTIONS"
	environmentPostgreSQLMinConnections = environmentPrefix + "POSTGRES_MIN_CONNECTIONS"
	environmentPostgreSQLConnectTimeout = environmentPrefix + "POSTGRES_CONNECT_TIMEOUT"
	environmentPostgreSQLMigrationsPath = environmentPrefix + "POSTGRES_MIGRATIONS_PATH"

	defaultPostgreSQLHost           = "127.0.0.1"
	defaultPostgreSQLPort           = 55432
	defaultPostgreSQLDatabase       = "miletos"
	defaultPostgreSQLSchema         = "workflow_runtime"
	defaultPostgreSQLUser           = "miletos"
	defaultPostgreSQLSSLMode        = "disable"
	defaultPostgreSQLMaxConnections = int32(10)
	defaultPostgreSQLMinConnections = int32(2)
	defaultPostgreSQLConnectTimeout = 5 * time.Second
	defaultPostgreSQLMigrationsPath = "./migrations"
)

type PostgreSQLConfig struct {
	Host           string        `env:"MILETOS_RUNTIME_POSTGRES_HOST"`
	Port           int           `env:"MILETOS_RUNTIME_POSTGRES_PORT"`
	Database       string        `env:"MILETOS_RUNTIME_POSTGRES_DATABASE"`
	Schema         string        `env:"MILETOS_RUNTIME_POSTGRES_SCHEMA"`
	User           string        `env:"MILETOS_RUNTIME_POSTGRES_USER"`
	Password       string        `env:"MILETOS_RUNTIME_POSTGRES_PASSWORD"`
	SSLMode        string        `env:"MILETOS_RUNTIME_POSTGRES_SSL_MODE"`
	MaxConnections int32         `env:"MILETOS_RUNTIME_POSTGRES_MAX_CONNECTIONS"`
	MinConnections int32         `env:"MILETOS_RUNTIME_POSTGRES_MIN_CONNECTIONS"`
	ConnectTimeout time.Duration `env:"MILETOS_RUNTIME_POSTGRES_CONNECT_TIMEOUT"`
	MigrationsPath string        `env:"MILETOS_RUNTIME_POSTGRES_MIGRATIONS_PATH"`
}

func LoadPostgreSQL() (PostgreSQLConfig, error) { return loadPostgreSQLConfig(os.LookupEnv) }

func (configuration PostgreSQLConfig) Address() string {
	return net.JoinHostPort(configuration.Host, strconv.Itoa(configuration.Port))
}

func (configuration PostgreSQLConfig) MaskedConnectionSummary() string {
	return fmt.Sprintf(
		"host=%s port=%d database=%s schema=%s user=%s sslmode=%s",
		configuration.Host,
		configuration.Port,
		configuration.Database,
		configuration.Schema,
		configuration.User,
		configuration.SSLMode,
	)
}

func loadPostgreSQLConfig(lookup environmentLookup) (PostgreSQLConfig, error) {
	if err := requireEnvironmentSecret(lookup, environmentPostgreSQLPassword); err != nil {
		return PostgreSQLConfig{}, err
	}

	configuration := postgreSQLConfigDefaults()
	if err := parseEnvironment(&configuration, lookup, []string{
		environmentPostgreSQLHost,
		environmentPostgreSQLPort,
		environmentPostgreSQLDatabase,
		environmentPostgreSQLSchema,
		environmentPostgreSQLUser,
		environmentPostgreSQLPassword,
		environmentPostgreSQLSSLMode,
		environmentPostgreSQLMaxConnections,
		environmentPostgreSQLMinConnections,
		environmentPostgreSQLConnectTimeout,
		environmentPostgreSQLMigrationsPath,
	}); err != nil {
		return PostgreSQLConfig{}, err
	}
	normalizePostgreSQLConfig(&configuration)
	if err := validatePostgreSQLConfig(configuration); err != nil {
		return PostgreSQLConfig{}, err
	}
	return configuration, nil
}

func postgreSQLConfigDefaults() PostgreSQLConfig {
	return PostgreSQLConfig{
		Host:           defaultPostgreSQLHost,
		Port:           defaultPostgreSQLPort,
		Database:       defaultPostgreSQLDatabase,
		Schema:         defaultPostgreSQLSchema,
		User:           defaultPostgreSQLUser,
		SSLMode:        defaultPostgreSQLSSLMode,
		MaxConnections: defaultPostgreSQLMaxConnections,
		MinConnections: defaultPostgreSQLMinConnections,
		ConnectTimeout: defaultPostgreSQLConnectTimeout,
		MigrationsPath: defaultPostgreSQLMigrationsPath,
	}
}

func normalizePostgreSQLConfig(configuration *PostgreSQLConfig) {
	configuration.Host = strings.TrimSpace(configuration.Host)
	configuration.Database = strings.TrimSpace(configuration.Database)
	configuration.Schema = strings.TrimSpace(configuration.Schema)
	configuration.User = strings.TrimSpace(configuration.User)
	configuration.SSLMode = strings.ToLower(strings.TrimSpace(configuration.SSLMode))
	configuration.MigrationsPath = strings.TrimSpace(configuration.MigrationsPath)
}

func validatePostgreSQLConfig(configuration PostgreSQLConfig) error {
	for name, value := range map[string]string{
		environmentPostgreSQLHost:           configuration.Host,
		environmentPostgreSQLDatabase:       configuration.Database,
		environmentPostgreSQLUser:           configuration.User,
		environmentPostgreSQLMigrationsPath: configuration.MigrationsPath,
	} {
		if value == "" {
			return fmt.Errorf("%s must not be empty", name)
		}
	}
	if err := validatePort(environmentPostgreSQLPort, configuration.Port); err != nil {
		return err
	}
	if !isPostgreSQLIdentifier(configuration.Schema) {
		return fmt.Errorf(
			"%s must be a valid unquoted PostgreSQL identifier",
			environmentPostgreSQLSchema,
		)
	}
	if strings.TrimSpace(configuration.Password) == "" {
		return fmt.Errorf("%s must not be empty", environmentPostgreSQLPassword)
	}
	switch configuration.SSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf(
			"%s must be one of: disable, allow, prefer, require, verify-ca, verify-full",
			environmentPostgreSQLSSLMode,
		)
	}
	if configuration.MaxConnections <= 0 {
		return fmt.Errorf("%s must be greater than zero", environmentPostgreSQLMaxConnections)
	}
	if configuration.MinConnections < 0 {
		return fmt.Errorf("%s must not be negative", environmentPostgreSQLMinConnections)
	}
	if configuration.MinConnections > configuration.MaxConnections {
		return fmt.Errorf(
			"%s must be less than or equal to %s",
			environmentPostgreSQLMinConnections,
			environmentPostgreSQLMaxConnections,
		)
	}
	if configuration.ConnectTimeout <= 0 {
		return fmt.Errorf("%s must be greater than zero", environmentPostgreSQLConnectTimeout)
	}
	return nil
}

func isPostgreSQLIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if index == 0 {
			if !isASCIILetter(character) && character != '_' {
				return false
			}
			continue
		}
		if !isASCIILetter(character) && !isASCIIDigit(character) && character != '_' {
			return false
		}
	}
	return true
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
