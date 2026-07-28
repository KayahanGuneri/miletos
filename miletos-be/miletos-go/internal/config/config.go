package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	env "github.com/caarlos0/env/v11"
)

const (
	environmentPrefix                = "MILETOS_RUNTIME_"
	environmentServiceName           = environmentPrefix + "SERVICE_NAME"
	environmentEnvironment           = environmentPrefix + "ENVIRONMENT"
	environmentHTTPHost              = environmentPrefix + "HTTP_HOST"
	environmentHTTPPort              = environmentPrefix + "HTTP_PORT"
	environmentHTTPReadHeaderTimeout = environmentPrefix + "HTTP_READ_HEADER_TIMEOUT"
	environmentHTTPReadTimeout       = environmentPrefix + "HTTP_READ_TIMEOUT"
	environmentHTTPWriteTimeout      = environmentPrefix + "HTTP_WRITE_TIMEOUT"
	environmentHTTPIdleTimeout       = environmentPrefix + "HTTP_IDLE_TIMEOUT"
	environmentShutdownTimeout       = environmentPrefix + "SHUTDOWN_TIMEOUT"
	environmentLogLevel              = environmentPrefix + "LOG_LEVEL"
	environmentWorkerConcurrency     = environmentPrefix + "WORKER_CONCURRENCY"

	defaultServiceName       = "miletos-go"
	defaultEnvironment       = "local"
	defaultHTTPHost          = "0.0.0.0"
	defaultHTTPPort          = 8081
	defaultWorkerConcurrency = 2
)

const (
	defaultHTTPReadHeaderTimeout = 5 * time.Second
	defaultHTTPReadTimeout       = 15 * time.Second
	defaultHTTPWriteTimeout      = 15 * time.Second
	defaultHTTPIdleTimeout       = 60 * time.Second
	defaultShutdownTimeout       = 10 * time.Second
	defaultLogLevel              = slog.LevelInfo
)

var configEnvironmentNames = []string{
	environmentServiceName,
	environmentEnvironment,
	environmentHTTPHost,
	environmentHTTPPort,
	environmentHTTPReadHeaderTimeout,
	environmentHTTPReadTimeout,
	environmentHTTPWriteTimeout,
	environmentHTTPIdleTimeout,
	environmentShutdownTimeout,
	environmentLogLevel,
	environmentWorkerConcurrency,
	environmentRetryMaxAttempts,
	environmentRetryInitialBackoff,
	environmentRetryMaxBackoff,
	environmentInterruptedAttemptAuditInterval,
	environmentKafkaEnabled,
	environmentKafkaBrokers,
	environmentKafkaEngineClientID,
	environmentKafkaWorkerClientID,
	environmentKafkaCommandTopic,
	environmentKafkaEventTopic,
	environmentKafkaWorkerGroupID,
	environmentKafkaEngineGroupID,
	environmentKafkaMaxMessageBytes,
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
}

type Config struct {
	ServiceName           string        `env:"MILETOS_RUNTIME_SERVICE_NAME"`
	Environment           string        `env:"MILETOS_RUNTIME_ENVIRONMENT"`
	HTTPHost              string        `env:"MILETOS_RUNTIME_HTTP_HOST"`
	HTTPPort              int           `env:"MILETOS_RUNTIME_HTTP_PORT"`
	HTTPReadHeaderTimeout time.Duration `env:"MILETOS_RUNTIME_HTTP_READ_HEADER_TIMEOUT"`
	HTTPReadTimeout       time.Duration `env:"MILETOS_RUNTIME_HTTP_READ_TIMEOUT"`
	HTTPWriteTimeout      time.Duration `env:"MILETOS_RUNTIME_HTTP_WRITE_TIMEOUT"`
	HTTPIdleTimeout       time.Duration `env:"MILETOS_RUNTIME_HTTP_IDLE_TIMEOUT"`
	ShutdownTimeout       time.Duration `env:"MILETOS_RUNTIME_SHUTDOWN_TIMEOUT"`
	LogLevel              slog.Level    `env:"MILETOS_RUNTIME_LOG_LEVEL"`
	PostgreSQL            PostgreSQLConfig
	Kafka                 KafkaConfig
	FaultTolerance        FaultToleranceConfig
	WorkerConcurrency     int `env:"MILETOS_RUNTIME_WORKER_CONCURRENCY"`
}

type environmentLookup func(string) (string, bool)

func Load() (Config, error) { return load(os.LookupEnv) }

func (configuration Config) HTTPAddress() string {
	return net.JoinHostPort(configuration.HTTPHost, strconv.Itoa(configuration.HTTPPort))
}

func load(lookup environmentLookup) (Config, error) {
	configuration := runtimeConfigDefaults()
	if err := parseEnvironment(&configuration, lookup, configEnvironmentNames); err != nil {
		return Config{}, err
	}
	normalizeConfig(&configuration)
	if err := validateConfig(configuration); err != nil {
		return Config{}, err
	}
	return configuration, nil
}

func runtimeConfigDefaults() Config {
	return Config{
		ServiceName:           defaultServiceName,
		Environment:           defaultEnvironment,
		HTTPHost:              defaultHTTPHost,
		HTTPPort:              defaultHTTPPort,
		HTTPReadHeaderTimeout: defaultHTTPReadHeaderTimeout,
		HTTPReadTimeout:       defaultHTTPReadTimeout,
		HTTPWriteTimeout:      defaultHTTPWriteTimeout,
		HTTPIdleTimeout:       defaultHTTPIdleTimeout,
		ShutdownTimeout:       defaultShutdownTimeout,
		LogLevel:              defaultLogLevel,
		PostgreSQL:            postgreSQLConfigDefaults(),
		Kafka:                 kafkaConfigDefaults(),
		FaultTolerance:        faultToleranceConfigDefaults(),
		WorkerConcurrency:     defaultWorkerConcurrency,
	}
}

func parseEnvironment(target any, lookup environmentLookup, names []string) error {
	if target == nil || lookup == nil {
		return fmt.Errorf("environment parser dependencies must not be nil")
	}
	for _, name := range names {
		value, exists := lookup(name)
		if !exists {
			continue
		}
		if name == environmentLogLevel {
			value = strings.ToLower(strings.TrimSpace(value))
			switch value {
			case "debug", "info", "warn", "error":
			default:
				return fmt.Errorf(
					"%s must be one of: debug, info, warn, error",
					environmentLogLevel,
				)
			}
		}
		if err := env.ParseWithOptions(target, env.Options{
			Environment: map[string]string{name: value},
		}); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func requireEnvironmentSecret(lookup environmentLookup, name string) error {
	if lookup == nil {
		return fmt.Errorf("environment lookup must not be nil")
	}
	value, exists := lookup(name)
	if !exists {
		return fmt.Errorf("%s must be set", name)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	return nil
}

func normalizeConfig(configuration *Config) {
	configuration.ServiceName = strings.TrimSpace(configuration.ServiceName)
	configuration.Environment = strings.TrimSpace(configuration.Environment)
	configuration.HTTPHost = strings.TrimSpace(configuration.HTTPHost)
	normalizeKafkaConfig(&configuration.Kafka)
	normalizePostgreSQLConfig(&configuration.PostgreSQL)
}

func validateConfig(configuration Config) error {
	for name, value := range map[string]string{
		environmentServiceName: configuration.ServiceName,
		environmentEnvironment: configuration.Environment,
		environmentHTTPHost:    configuration.HTTPHost,
	} {
		if value == "" {
			return fmt.Errorf("%s must not be empty", name)
		}
	}
	if err := validatePort(environmentHTTPPort, configuration.HTTPPort); err != nil {
		return err
	}
	for name, value := range map[string]time.Duration{
		environmentHTTPReadHeaderTimeout: configuration.HTTPReadHeaderTimeout,
		environmentHTTPReadTimeout:       configuration.HTTPReadTimeout,
		environmentHTTPWriteTimeout:      configuration.HTTPWriteTimeout,
		environmentHTTPIdleTimeout:       configuration.HTTPIdleTimeout,
		environmentShutdownTimeout:       configuration.ShutdownTimeout,
	} {
		if value <= 0 {
			return fmt.Errorf("%s must be greater than zero", name)
		}
	}
	switch configuration.LogLevel {
	case slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError:
	default:
		return fmt.Errorf("%s must be one of: debug, info, warn, error", environmentLogLevel)
	}
	if configuration.WorkerConcurrency < 2 || configuration.WorkerConcurrency > 5 {
		return fmt.Errorf("%s must be between 2 and 5", environmentWorkerConcurrency)
	}
	if err := validateFaultToleranceConfig(configuration.FaultTolerance); err != nil {
		return err
	}
	if err := validatePostgreSQLConfig(configuration.PostgreSQL); err != nil {
		return err
	}
	return validateKafkaConfig(configuration.Kafka)
}

func validatePort(name string, value int) error {
	if value < 1 || value > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", name)
	}
	return nil
}
