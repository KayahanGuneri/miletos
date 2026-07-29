package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	ServiceName          string
	Environment          string
	HTTPHost             string
	HTTPPort             int
	LogLevel             slog.Level
	PostgreSQLURL        string
	KafkaEnabled         bool
	KafkaBrokers         []string
	KafkaClientID        string
	KafkaCommandTopic    string
	KafkaConsumerGroup   string
	InternalServiceToken string
	RetryMaxAttempts     int
	RetryDelay           time.Duration
	NodeConcurrency      int
}

func Load() (Config, error) {
	httpPort, err := integer("MILETOS_RUNTIME_HTTP_PORT", 8081)
	if err != nil {
		return Config{}, err
	}
	postgresqlURL, err := databaseURL()
	if err != nil {
		return Config{}, err
	}
	kafkaEnabled, err := boolean("MILETOS_RUNTIME_KAFKA_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	retryMaxAttempts, err := integer("MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS", 3)
	if err != nil {
		return Config{}, err
	}
	retryDelay, err := duration(
		"MILETOS_RUNTIME_RETRY_DELAY", 250*time.Millisecond,
	)
	if err != nil {
		return Config{}, err
	}
	nodeConcurrency, err := integer("MILETOS_RUNTIME_NODE_CONCURRENCY", 2)
	if err != nil {
		return Config{}, err
	}

	configuration := Config{
		ServiceName:   value("MILETOS_RUNTIME_SERVICE_NAME", "miletos-go"),
		Environment:   value("MILETOS_RUNTIME_ENVIRONMENT", "local"),
		HTTPHost:      value("MILETOS_RUNTIME_HTTP_HOST", "0.0.0.0"),
		HTTPPort:      httpPort,
		PostgreSQLURL: postgresqlURL,
		KafkaEnabled:  kafkaEnabled,
		KafkaBrokers:  commaSeparated("MILETOS_RUNTIME_KAFKA_BROKERS", "127.0.0.1:9092"),
		KafkaClientID: configuredValue(
			"MILETOS_RUNTIME_KAFKA_CLIENT_ID", "miletos-go-engine-v1",
		),
		KafkaCommandTopic: configuredValue(
			"MILETOS_RUNTIME_KAFKA_COMMAND_TOPIC", "miletos.workflow.node.commands.v1",
		),
		KafkaConsumerGroup: configuredValue(
			"MILETOS_RUNTIME_KAFKA_CONSUMER_GROUP", "miletos-go-engine-v1",
		),
		InternalServiceToken: strings.TrimSpace(os.Getenv(
			"MILETOS_RUNTIME_INTERNAL_SERVICE_TOKEN",
		)),
		RetryMaxAttempts: retryMaxAttempts,
		RetryDelay:       retryDelay,
		NodeConcurrency:  nodeConcurrency,
	}

	switch strings.ToLower(configuredValue("MILETOS_RUNTIME_LOG_LEVEL", "info")) {
	case "debug":
		configuration.LogLevel = slog.LevelDebug
	case "info":
		configuration.LogLevel = slog.LevelInfo
	case "warn":
		configuration.LogLevel = slog.LevelWarn
	case "error":
		configuration.LogLevel = slog.LevelError
	default:
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_LOG_LEVEL must be debug, info, warn, or error")
	}
	if configuration.HTTPPort < 1 || configuration.HTTPPort > 65535 {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_HTTP_PORT must be between 1 and 65535")
	}
	if len(configuration.InternalServiceToken) < 32 {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_INTERNAL_SERVICE_TOKEN must contain at least 32 bytes")
	}
	if configuration.KafkaEnabled {
		if len(configuration.KafkaBrokers) == 0 {
			return Config{}, fmt.Errorf("MILETOS_RUNTIME_KAFKA_BROKERS must not be empty when Kafka is enabled")
		}
		if configuration.KafkaClientID == "" {
			return Config{}, fmt.Errorf("MILETOS_RUNTIME_KAFKA_CLIENT_ID must not be empty when Kafka is enabled")
		}
		if configuration.KafkaCommandTopic == "" {
			return Config{}, fmt.Errorf("MILETOS_RUNTIME_KAFKA_COMMAND_TOPIC must not be empty when Kafka is enabled")
		}
		if configuration.KafkaConsumerGroup == "" {
			return Config{}, fmt.Errorf("MILETOS_RUNTIME_KAFKA_CONSUMER_GROUP must not be empty when Kafka is enabled")
		}
	}
	if configuration.RetryMaxAttempts < 1 {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS must be positive")
	}
	if configuration.RetryDelay <= 0 {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_RETRY_DELAY must be positive")
	}
	if configuration.RetryMaxAttempts > 32767 {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS cannot exceed 32767")
	}
	if configuration.NodeConcurrency < 2 || configuration.NodeConcurrency > 5 {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_NODE_CONCURRENCY must be between 2 and 5")
	}
	return configuration, nil
}

func (configuration Config) HTTPAddress() string {
	return net.JoinHostPort(configuration.HTTPHost, strconv.Itoa(configuration.HTTPPort))
}

func value(name, fallback string) string {
	if configured := strings.TrimSpace(os.Getenv(name)); configured != "" {
		return configured
	}
	return fallback
}

func configuredValue(name, fallback string) string {
	configured, exists := os.LookupEnv(name)
	if !exists {
		return fallback
	}
	return strings.TrimSpace(configured)
}

func integer(name string, fallback int) (int, error) {
	configured, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	configured = strings.TrimSpace(configured)
	parsed, err := strconv.Atoi(configured)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	return parsed, nil
}

func boolean(name string, fallback bool) (bool, error) {
	configured, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	configured = strings.TrimSpace(configured)
	parsed, err := strconv.ParseBool(configured)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return parsed, nil
}

func duration(name string, fallback time.Duration) (time.Duration, error) {
	configured, exists := os.LookupEnv(name)
	if !exists {
		return fallback, nil
	}
	configured = strings.TrimSpace(configured)
	parsed, err := time.ParseDuration(configured)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration", name)
	}
	return parsed, nil
}

func commaSeparated(name, fallback string) []string {
	parts := strings.Split(configuredValue(name, fallback), ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if normalized := strings.TrimSpace(part); normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func databaseURL() (string, error) {
	const urlName = "MILETOS_RUNTIME_POSTGRES_URL"
	if configured, exists := os.LookupEnv(urlName); exists {
		configured = strings.TrimSpace(configured)
		return validateDatabaseURL(configured, urlName)
	}
	password := strings.TrimSpace(os.Getenv("MILETOS_RUNTIME_POSTGRES_PASSWORD"))
	if password == "" {
		return "", fmt.Errorf(
			"MILETOS_RUNTIME_POSTGRES_PASSWORD is required when %s is absent",
			urlName,
		)
	}
	host := value("MILETOS_RUNTIME_POSTGRES_HOST", "127.0.0.1")
	port := value("MILETOS_RUNTIME_POSTGRES_PORT", "55432")
	database := value("MILETOS_RUNTIME_POSTGRES_DATABASE", "miletos")
	user := value("MILETOS_RUNTIME_POSTGRES_USER", "miletos")
	sslMode := value("MILETOS_RUNTIME_POSTGRES_SSL_MODE", "disable")
	connection := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   database,
	}
	query := connection.Query()
	query.Set("sslmode", sslMode)
	connection.RawQuery = query.Encode()
	return validateDatabaseURL(connection.String(), "PostgreSQL connection configuration")
}

func validateDatabaseURL(connection, property string) (string, error) {
	if connection == "" {
		return "", fmt.Errorf("%s must not be empty", property)
	}
	if _, err := pgxpool.ParseConfig(connection); err != nil {
		return "", fmt.Errorf("%s is invalid", property)
	}
	return connection, nil
}
