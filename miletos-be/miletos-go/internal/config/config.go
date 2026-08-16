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
)

type Config struct {
	ServiceName                     string
	Environment                     string
	HTTPHost                        string
	HTTPPort                        int
	GRPCHost                        string
	GRPCPort                        int
	PublicTriggerBaseURL            string
	LogLevel                        slog.Level
	PostgreSQLURL                   string
	KafkaEnabled                    bool
	KafkaBrokers                    []string
	KafkaClientID                   string
	KafkaCommandTopic               string
	KafkaDeadLetterTopic            string
	KafkaConsumerGroup              string
	InternalServiceToken            string
	RetryMaxAttempts                int
	RetryDelay                      time.Duration
	NodeConcurrency                 int
	ReconciliationEnabled           bool
	ReconciliationInterval          time.Duration
	ReconciliationQueuedStale       time.Duration
	ReconciliationRunningStale      time.Duration
	ReconciliationRetryPendingStale time.Duration
	CronEnabled                     bool
	CronPollInterval                time.Duration
	CronBatchSize                   int
	InputDirectory                  string
	SecretsAESKey                   string
}

func Load() (Config, error) {
	httpPort, err := getEnvInt("MILETOS_RUNTIME_HTTP_PORT", 8081)
	if err != nil {
		return Config{}, err
	}
	grpcPort, err := getEnvInt("MILETOS_RUNTIME_GRPC_PORT", 9091)
	if err != nil {
		return Config{}, err
	}
	postgresqlURL, err := databaseURL()
	if err != nil {
		return Config{}, err
	}
	kafkaEnabled, err := getEnvBool("MILETOS_RUNTIME_KAFKA_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	retryMaxAttempts, err := getEnvInt("MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS", 3)
	if err != nil {
		return Config{}, err
	}
	retryDelay, err := getEnvDuration(
		"MILETOS_RUNTIME_RETRY_DELAY", 250*time.Millisecond,
	)
	if err != nil {
		return Config{}, err
	}
	nodeConcurrency, err := getEnvInt("MILETOS_RUNTIME_NODE_CONCURRENCY", 2)
	if err != nil {
		return Config{}, err
	}
	if nodeConcurrency < 2 || nodeConcurrency > 5 {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_NODE_CONCURRENCY must be between 2 and 5")
	}
	reconciliationEnabled, err := getEnvBool("MILETOS_RUNTIME_RECONCILIATION_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	reconciliationInterval, err := getEnvDuration("MILETOS_RUNTIME_RECONCILIATION_INTERVAL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	queuedStale, err := getEnvDuration("MILETOS_RUNTIME_RECONCILIATION_QUEUED_STALE", 2*time.Minute)
	if err != nil {
		return Config{}, err
	}
	runningStale, err := getEnvDuration("MILETOS_RUNTIME_RECONCILIATION_RUNNING_STALE", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}
	retryPendingStale, err := getEnvDuration("MILETOS_RUNTIME_RECONCILIATION_RETRY_PENDING_STALE", 2*time.Minute)
	if err != nil {
		return Config{}, err
	}
	cronEnabled, err := getEnvBool("MILETOS_RUNTIME_CRON_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	cronPollInterval, err := getEnvDuration("MILETOS_RUNTIME_CRON_POLL_INTERVAL", 15*time.Second)
	if err != nil || cronPollInterval <= 0 {
		if err != nil {
			return Config{}, err
		}
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_CRON_POLL_INTERVAL must be positive")
	}
	cronBatchSize, err := getEnvInt("MILETOS_RUNTIME_CRON_BATCH_SIZE", 50)
	if err != nil {
		return Config{}, err
	}
	if cronBatchSize < 1 || cronBatchSize > 1000 {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_CRON_BATCH_SIZE must be between 1 and 1000")
	}
	publicTriggerBaseURL, err := normalizedPublicTriggerBaseURL(
		getEnv("MILETOS_RUNTIME_PUBLIC_TRIGGER_BASE_URL", ""),
	)
	if err != nil {
		return Config{}, err
	}

	inputDirectory := getEnv("MILETOS_RUNTIME_INPUT_DIRECTORY", "input")
	if inputDirectory == "" {
		return Config{}, fmt.Errorf("MILETOS_RUNTIME_INPUT_DIRECTORY must not be blank")
	}

	configuration := Config{
		ServiceName:          getEnv("MILETOS_RUNTIME_SERVICE_NAME", "miletos-go"),
		Environment:          getEnv("MILETOS_RUNTIME_ENVIRONMENT", "local"),
		HTTPHost:             getEnv("MILETOS_RUNTIME_HTTP_HOST", "0.0.0.0"),
		HTTPPort:             httpPort,
		GRPCHost:             getEnv("MILETOS_RUNTIME_GRPC_HOST", "0.0.0.0"),
		GRPCPort:             grpcPort,
		PublicTriggerBaseURL: publicTriggerBaseURL,
		PostgreSQLURL:        postgresqlURL,
		InputDirectory:       inputDirectory,
		SecretsAESKey:        strings.TrimSpace(os.Getenv("MILETOS_SECRETS_AES_KEY")),
		KafkaEnabled:         kafkaEnabled,
		KafkaBrokers:         getEnvStrings("MILETOS_RUNTIME_KAFKA_BROKERS", "127.0.0.1:9092"),
		KafkaClientID: getEnv(
			"MILETOS_RUNTIME_KAFKA_CLIENT_ID", "miletos-go-engine-v1",
		),
		KafkaCommandTopic: getEnv(
			"MILETOS_RUNTIME_KAFKA_COMMAND_TOPIC", "miletos.workflow.node.commands.v1",
		),
		KafkaDeadLetterTopic: getEnv(
			"MILETOS_RUNTIME_KAFKA_DEAD_LETTER_TOPIC",
			"miletos.workflow.node.dead-letter.v1",
		),
		KafkaConsumerGroup: getEnv(
			"MILETOS_RUNTIME_KAFKA_CONSUMER_GROUP", "miletos-go-engine-v1",
		),
		InternalServiceToken: strings.TrimSpace(os.Getenv(
			"MILETOS_RUNTIME_INTERNAL_SERVICE_TOKEN",
		)),
		RetryMaxAttempts:                retryMaxAttempts,
		RetryDelay:                      retryDelay,
		NodeConcurrency:                 nodeConcurrency,
		ReconciliationEnabled:           reconciliationEnabled,
		ReconciliationInterval:          reconciliationInterval,
		ReconciliationQueuedStale:       queuedStale,
		ReconciliationRunningStale:      runningStale,
		ReconciliationRetryPendingStale: retryPendingStale,
		CronEnabled:                     cronEnabled,
		CronPollInterval:                cronPollInterval,
		CronBatchSize:                   cronBatchSize,
	}

	switch strings.ToLower(getEnv("MILETOS_RUNTIME_LOG_LEVEL", "info")) {
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
	return configuration, nil
}

func (configuration Config) HTTPAddress() string {
	return net.JoinHostPort(configuration.HTTPHost, strconv.Itoa(configuration.HTTPPort))
}

func (configuration Config) GRPCAddress() string {
	return net.JoinHostPort(configuration.GRPCHost, strconv.Itoa(configuration.GRPCPort))
}

func getEnv(name, fallback string) string {
	configured, exists := os.LookupEnv(name)
	if !exists {
		return fallback
	}
	return strings.TrimSpace(configured)
}

func getEnvInt(name string, fallback int) (int, error) {
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

func getEnvBool(name string, fallback bool) (bool, error) {
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

func getEnvDuration(name string, fallback time.Duration) (time.Duration, error) {
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

func getEnvStrings(name, fallback string) []string {
	parts := strings.Split(getEnv(name, fallback), ",")
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
	if configured := strings.TrimSpace(os.Getenv(urlName)); configured != "" {
		return configured, nil
	}
	password := strings.TrimSpace(os.Getenv("MILETOS_RUNTIME_POSTGRES_PASSWORD"))
	if password == "" {
		return "", fmt.Errorf(
			"MILETOS_RUNTIME_POSTGRES_PASSWORD is required when %s is absent",
			urlName,
		)
	}
	host := getEnv("MILETOS_RUNTIME_POSTGRES_HOST", "127.0.0.1")
	port := getEnv("MILETOS_RUNTIME_POSTGRES_PORT", "55432")
	database := getEnv("MILETOS_RUNTIME_POSTGRES_DATABASE", "miletos")
	user := getEnv("MILETOS_RUNTIME_POSTGRES_USER", "miletos")
	sslMode := getEnv("MILETOS_RUNTIME_POSTGRES_SSL_MODE", "disable")
	connection := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   database,
	}
	query := connection.Query()
	query.Set("sslmode", sslMode)
	connection.RawQuery = query.Encode()
	return connection.String(), nil
}

func normalizedPublicTriggerBaseURL(configured string) (string, error) {
	if configured == "" {
		return "", nil
	}
	parsed, err := url.Parse(configured)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", fmt.Errorf("MILETOS_RUNTIME_PUBLIC_TRIGGER_BASE_URL must be an absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("MILETOS_RUNTIME_PUBLIC_TRIGGER_BASE_URL must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("MILETOS_RUNTIME_PUBLIC_TRIGGER_BASE_URL must not contain credentials, query, or fragment")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
