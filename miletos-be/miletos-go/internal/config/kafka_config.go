package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	environmentKafkaEnabled         = environmentPrefix + "KAFKA_ENABLED"
	environmentKafkaBrokers         = environmentPrefix + "KAFKA_BROKERS"
	environmentKafkaEngineClientID  = environmentPrefix + "KAFKA_ENGINE_CLIENT_ID"
	environmentKafkaWorkerClientID  = environmentPrefix + "KAFKA_WORKER_CLIENT_ID"
	environmentKafkaCommandTopic    = environmentPrefix + "KAFKA_COMMAND_TOPIC"
	environmentKafkaEventTopic      = environmentPrefix + "KAFKA_EVENT_TOPIC"
	environmentKafkaWorkerGroupID   = environmentPrefix + "KAFKA_WORKER_GROUP_ID"
	environmentKafkaEngineGroupID   = environmentPrefix + "KAFKA_ENGINE_GROUP_ID"
	environmentKafkaMaxMessageBytes = environmentPrefix + "KAFKA_MAX_MESSAGE_BYTES"

	defaultKafkaEnabled         = false
	defaultKafkaBroker          = "127.0.0.1:9092"
	defaultKafkaEngineClientID  = "miletos-go-engine-v1"
	defaultKafkaWorkerClientID  = "miletos-go-worker-v1"
	defaultKafkaCommandTopic    = "miletos.workflow.node.commands.v1"
	defaultKafkaEventTopic      = "miletos.workflow.node.events.v1"
	defaultKafkaWorkerGroupID   = "miletos-go-workers-v1"
	defaultKafkaEngineGroupID   = "miletos-go-engine-results-v1"
	defaultKafkaMaxMessageBytes = 1024 * 1024
	minimumKafkaMaxMessageBytes = 1024
	maximumKafkaMaxMessageBytes = 16 * 1024 * 1024
	maximumKafkaNameLength      = 249
)

type KafkaConfig struct {
	Enabled         bool     `env:"MILETOS_RUNTIME_KAFKA_ENABLED"`
	Brokers         []string `env:"MILETOS_RUNTIME_KAFKA_BROKERS" envSeparator:","`
	EngineClientID  string   `env:"MILETOS_RUNTIME_KAFKA_ENGINE_CLIENT_ID"`
	WorkerClientID  string   `env:"MILETOS_RUNTIME_KAFKA_WORKER_CLIENT_ID"`
	CommandTopic    string   `env:"MILETOS_RUNTIME_KAFKA_COMMAND_TOPIC"`
	EventTopic      string   `env:"MILETOS_RUNTIME_KAFKA_EVENT_TOPIC"`
	WorkerGroupID   string   `env:"MILETOS_RUNTIME_KAFKA_WORKER_GROUP_ID"`
	EngineGroupID   string   `env:"MILETOS_RUNTIME_KAFKA_ENGINE_GROUP_ID"`
	MaxMessageBytes int      `env:"MILETOS_RUNTIME_KAFKA_MAX_MESSAGE_BYTES"`
}

func (configuration KafkaConfig) IsValid() bool {
	return validateKafkaConfig(configuration) == nil
}

func loadKafkaConfig(lookup environmentLookup) (KafkaConfig, error) {
	configuration := kafkaConfigDefaults()
	if err := parseEnvironment(&configuration, lookup, []string{
		environmentKafkaEnabled,
		environmentKafkaBrokers,
		environmentKafkaEngineClientID,
		environmentKafkaWorkerClientID,
		environmentKafkaCommandTopic,
		environmentKafkaEventTopic,
		environmentKafkaWorkerGroupID,
		environmentKafkaEngineGroupID,
		environmentKafkaMaxMessageBytes,
	}); err != nil {
		return KafkaConfig{}, err
	}
	normalizeKafkaConfig(&configuration)
	if err := validateKafkaConfig(configuration); err != nil {
		return KafkaConfig{}, err
	}
	return configuration, nil
}

func kafkaConfigDefaults() KafkaConfig {
	return KafkaConfig{
		Enabled:         defaultKafkaEnabled,
		Brokers:         []string{defaultKafkaBroker},
		EngineClientID:  defaultKafkaEngineClientID,
		WorkerClientID:  defaultKafkaWorkerClientID,
		CommandTopic:    defaultKafkaCommandTopic,
		EventTopic:      defaultKafkaEventTopic,
		WorkerGroupID:   defaultKafkaWorkerGroupID,
		EngineGroupID:   defaultKafkaEngineGroupID,
		MaxMessageBytes: defaultKafkaMaxMessageBytes,
	}
}

func normalizeKafkaConfig(configuration *KafkaConfig) {
	for index := range configuration.Brokers {
		configuration.Brokers[index] = strings.TrimSpace(configuration.Brokers[index])
	}
	configuration.EngineClientID = strings.TrimSpace(configuration.EngineClientID)
	configuration.WorkerClientID = strings.TrimSpace(configuration.WorkerClientID)
	configuration.CommandTopic = strings.TrimSpace(configuration.CommandTopic)
	configuration.EventTopic = strings.TrimSpace(configuration.EventTopic)
	configuration.WorkerGroupID = strings.TrimSpace(configuration.WorkerGroupID)
	configuration.EngineGroupID = strings.TrimSpace(configuration.EngineGroupID)
}

func validateKafkaConfig(configuration KafkaConfig) error {
	if len(configuration.Brokers) == 0 {
		return fmt.Errorf("%s must not be empty", environmentKafkaBrokers)
	}
	seen := make(map[string]struct{}, len(configuration.Brokers))
	for _, broker := range configuration.Brokers {
		if err := validateKafkaBroker(broker); err != nil {
			return fmt.Errorf("%s: %w", environmentKafkaBrokers, err)
		}
		if _, exists := seen[broker]; exists {
			return fmt.Errorf("%s must not contain duplicate broker %q", environmentKafkaBrokers, broker)
		}
		seen[broker] = struct{}{}
	}
	for field, value := range map[string]string{
		environmentKafkaEngineClientID: configuration.EngineClientID,
		environmentKafkaWorkerClientID: configuration.WorkerClientID,
		environmentKafkaWorkerGroupID:  configuration.WorkerGroupID,
		environmentKafkaEngineGroupID:  configuration.EngineGroupID,
	} {
		if err := validateKafkaName(field, value); err != nil {
			return err
		}
	}
	if err := validateKafkaTopic(configuration.CommandTopic); err != nil {
		return fmt.Errorf("%s: %w", environmentKafkaCommandTopic, err)
	}
	if err := validateKafkaTopic(configuration.EventTopic); err != nil {
		return fmt.Errorf("%s: %w", environmentKafkaEventTopic, err)
	}
	if configuration.CommandTopic == configuration.EventTopic {
		return fmt.Errorf(
			"%s and %s must be different",
			environmentKafkaCommandTopic,
			environmentKafkaEventTopic,
		)
	}
	if configuration.WorkerGroupID == configuration.EngineGroupID {
		return fmt.Errorf(
			"%s and %s must be different",
			environmentKafkaWorkerGroupID,
			environmentKafkaEngineGroupID,
		)
	}
	if configuration.MaxMessageBytes < minimumKafkaMaxMessageBytes ||
		configuration.MaxMessageBytes > maximumKafkaMaxMessageBytes {
		return fmt.Errorf(
			"%s must be between %d and %d",
			environmentKafkaMaxMessageBytes,
			minimumKafkaMaxMessageBytes,
			maximumKafkaMaxMessageBytes,
		)
	}
	return nil
}

func validateKafkaBroker(broker string) error {
	if strings.TrimSpace(broker) != broker || broker == "" {
		return fmt.Errorf("Kafka broker must be a non-blank normalized host:port")
	}
	host, port, err := net.SplitHostPort(broker)
	if err != nil {
		return fmt.Errorf("Kafka broker %q must use host:port format: %w", broker, err)
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("Kafka broker %q must contain a host", broker)
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		return fmt.Errorf("Kafka broker %q must contain a port between 1 and 65535", broker)
	}
	return nil
}

func validateKafkaName(field, value string) error {
	if strings.TrimSpace(value) != value || value == "" {
		return fmt.Errorf("%s must be non-blank and normalized", field)
	}
	if len(value) > maximumKafkaNameLength {
		return fmt.Errorf("%s must not exceed %d bytes", field, maximumKafkaNameLength)
	}
	return nil
}

func validateKafkaTopic(value string) error {
	if value == "." || value == ".." {
		return fmt.Errorf("topic must not be %q", value)
	}
	if err := validateKafkaName("topic", value); err != nil {
		return err
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return fmt.Errorf("topic contains unsupported character %q", character)
	}
	return nil
}
