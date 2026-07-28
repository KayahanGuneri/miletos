package config

import (
	slog "log/slog"
	os "os"
	reflect "reflect"
	strings "strings"
	testing "testing"
	time "time"
)

const testPostgreSQLPassword = "test-postgres-password"

func TestLoadUsesDefaults(t *testing.T) {
	actual, err := load(
		lookupFrom(
			withRequiredPostgreSQLPassword(nil),
		),
	)
	if err != nil {
		t.Fatalf(
			"load() returned an unexpected error: %v",
			err,
		)
	}

	expected := Config{
		ServiceName:           "miletos-go",
		Environment:           "local",
		HTTPHost:              "0.0.0.0",
		HTTPPort:              8081,
		HTTPReadHeaderTimeout: 5 * time.Second,
		HTTPReadTimeout:       15 * time.Second,
		HTTPWriteTimeout:      15 * time.Second,
		HTTPIdleTimeout:       60 * time.Second,
		ShutdownTimeout:       10 * time.Second,
		LogLevel:              slog.LevelInfo,
		PostgreSQL: PostgreSQLConfig{
			Host:           defaultPostgreSQLHost,
			Port:           defaultPostgreSQLPort,
			Database:       defaultPostgreSQLDatabase,
			Schema:         defaultPostgreSQLSchema,
			User:           defaultPostgreSQLUser,
			Password:       testPostgreSQLPassword,
			SSLMode:        defaultPostgreSQLSSLMode,
			MaxConnections: defaultPostgreSQLMaxConnections,
			MinConnections: defaultPostgreSQLMinConnections,
			ConnectTimeout: defaultPostgreSQLConnectTimeout,
			MigrationsPath: defaultPostgreSQLMigrationsPath,
		},
		Kafka:             defaultKafkaConfig(),
		WorkerConcurrency: defaultWorkerConcurrency,
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"load() defaults mismatch\nactual:   %#v\nexpected: %#v",
			actual,
			expected,
		)
	}

	if actual.HTTPAddress() != "0.0.0.0:8081" {
		t.Fatalf(
			"HTTPAddress() = %q, want %q",
			actual.HTTPAddress(),
			"0.0.0.0:8081",
		)
	}
}

func TestLoadAppliesAllOverrides(t *testing.T) {
	actual, err := load(
		lookupFrom(
			withRequiredPostgreSQLPassword(
				map[string]string{
					environmentServiceName:           " workflow-runtime ",
					environmentEnvironment:           " staging ",
					environmentHTTPHost:              " 127.0.0.1 ",
					environmentHTTPPort:              "18081",
					environmentHTTPReadHeaderTimeout: "2s",
					environmentHTTPReadTimeout:       "3s",
					environmentHTTPWriteTimeout:      "4s",
					environmentHTTPIdleTimeout:       "5s",
					environmentShutdownTimeout:       "6s",
					environmentLogLevel:              "DEBUG",
				},
			),
		),
	)
	if err != nil {
		t.Fatalf(
			"load() returned an unexpected error: %v",
			err,
		)
	}

	expected := Config{
		ServiceName:           "workflow-runtime",
		Environment:           "staging",
		HTTPHost:              "127.0.0.1",
		HTTPPort:              18081,
		HTTPReadHeaderTimeout: 2 * time.Second,
		HTTPReadTimeout:       3 * time.Second,
		HTTPWriteTimeout:      4 * time.Second,
		HTTPIdleTimeout:       5 * time.Second,
		ShutdownTimeout:       6 * time.Second,
		LogLevel:              slog.LevelDebug,
		PostgreSQL: PostgreSQLConfig{
			Host:           defaultPostgreSQLHost,
			Port:           defaultPostgreSQLPort,
			Database:       defaultPostgreSQLDatabase,
			Schema:         defaultPostgreSQLSchema,
			User:           defaultPostgreSQLUser,
			Password:       testPostgreSQLPassword,
			SSLMode:        defaultPostgreSQLSSLMode,
			MaxConnections: defaultPostgreSQLMaxConnections,
			MinConnections: defaultPostgreSQLMinConnections,
			ConnectTimeout: defaultPostgreSQLConnectTimeout,
			MigrationsPath: defaultPostgreSQLMigrationsPath,
		},
		Kafka:             defaultKafkaConfig(),
		WorkerConcurrency: defaultWorkerConcurrency,
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"load() overrides mismatch\nactual:   %#v\nexpected: %#v",
			actual,
			expected,
		)
	}
}

func TestLoadAppliesPartialOverride(t *testing.T) {
	actual, err := load(
		lookupFrom(
			withRequiredPostgreSQLPassword(
				map[string]string{
					environmentHTTPPort: "9090",
				},
			),
		),
	)
	if err != nil {
		t.Fatalf(
			"load() returned an unexpected error: %v",
			err,
		)
	}

	if actual.HTTPPort != 9090 {
		t.Fatalf(
			"HTTPPort = %d, want %d",
			actual.HTTPPort,
			9090,
		)
	}

	if actual.ServiceName != defaultServiceName {
		t.Fatalf(
			"ServiceName = %q, want default %q",
			actual.ServiceName,
			defaultServiceName,
		)
	}
}

func TestLoadRejectsInvalidPorts(t *testing.T) {
	tests := map[string]string{
		"not an integer": "abc",
		"below range":    "0",
		"negative":       "-1",
		"above range":    "65536",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := load(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							environmentHTTPPort: value,
						},
					),
				),
			)

			requireErrorContains(
				t,
				err,
				environmentHTTPPort,
			)
		})
	}
}

func TestLoadRejectsInvalidDurations(t *testing.T) {
	tests := map[string]struct {
		key   string
		value string
	}{
		"invalid read header timeout": {
			key:   environmentHTTPReadHeaderTimeout,
			value: "invalid",
		},
		"zero read timeout": {
			key:   environmentHTTPReadTimeout,
			value: "0s",
		},
		"negative write timeout": {
			key:   environmentHTTPWriteTimeout,
			value: "-1s",
		},
		"invalid idle timeout": {
			key:   environmentHTTPIdleTimeout,
			value: "10",
		},
		"zero shutdown timeout": {
			key:   environmentShutdownTimeout,
			value: "0s",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := load(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							test.key: test.value,
						},
					),
				),
			)

			requireErrorContains(
				t,
				err,
				test.key,
			)
		})
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	_, err := load(
		lookupFrom(
			withRequiredPostgreSQLPassword(
				map[string]string{
					environmentLogLevel: "verbose",
				},
			),
		),
	)

	requireErrorContains(
		t,
		err,
		environmentLogLevel,
	)

	requireErrorContains(
		t,
		err,
		"debug, info, warn, error",
	)
}

func TestLoadRejectsEmptyRequiredStrings(t *testing.T) {
	tests := []string{
		environmentServiceName,
		environmentEnvironment,
		environmentHTTPHost,
	}

	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			_, err := load(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							key: "   ",
						},
					),
				),
			)

			requireErrorContains(
				t,
				err,
				key,
			)
		})
	}
}

func TestLoadReadsProcessEnvironment(t *testing.T) {
	clearRuntimeEnvironment(t)

	if err := os.Setenv(
		environmentHTTPPort,
		"19090",
	); err != nil {
		t.Fatalf(
			"os.Setenv() returned an unexpected error: %v",
			err,
		)
	}

	if err := os.Setenv(
		environmentPostgreSQLPassword,
		testPostgreSQLPassword,
	); err != nil {
		t.Fatalf(
			"os.Setenv() returned an unexpected error: %v",
			err,
		)
	}

	actual, err := Load()
	if err != nil {
		t.Fatalf(
			"Load() returned an unexpected error: %v",
			err,
		)
	}

	if actual.HTTPPort != 19090 {
		t.Fatalf(
			"HTTPPort = %d, want %d",
			actual.HTTPPort,
			19090,
		)
	}

	if actual.PostgreSQL.Password != testPostgreSQLPassword {
		t.Fatal(
			"PostgreSQL password was not loaded from the process environment",
		)
	}
}

func TestHTTPAddressSupportsIPv6(t *testing.T) {
	configuration := Config{
		HTTPHost: "::1",
		HTTPPort: 8081,
	}

	if actual := configuration.HTTPAddress(); actual != "[::1]:8081" {
		t.Fatalf(
			"HTTPAddress() = %q, want %q",
			actual,
			"[::1]:8081",
		)
	}
}

func lookupFrom(
	values map[string]string,
) environmentLookup {
	return func(name string) (string, bool) {
		value, exists := values[name]
		return value, exists
	}
}

func withRequiredPostgreSQLPassword(
	values map[string]string,
) map[string]string {
	result := make(
		map[string]string,
		len(values)+1,
	)

	for key, value := range values {
		result[key] = value
	}

	if _, exists := result[environmentPostgreSQLPassword]; !exists {
		result[environmentPostgreSQLPassword] = testPostgreSQLPassword
	}

	return result
}

func requireErrorContains(
	t *testing.T,
	err error,
	expected string,
) {
	t.Helper()

	if err == nil {
		t.Fatalf(
			"expected an error containing %q, got nil",
			expected,
		)
	}

	if !strings.Contains(err.Error(), expected) {
		t.Fatalf(
			"error = %q, want it to contain %q",
			err.Error(),
			expected,
		)
	}
}

func clearRuntimeEnvironment(
	t *testing.T,
) {
	t.Helper()

	keys := []string{
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
		environmentKafkaEnabled,
		environmentKafkaBrokers,
		environmentKafkaEngineClientID,
		environmentKafkaWorkerClientID,
		environmentKafkaCommandTopic,
		environmentKafkaEventTopic,
		environmentKafkaWorkerGroupID,
		environmentKafkaEngineGroupID,
		environmentKafkaMaxMessageBytes,
	}

	for _, key := range keys {
		value, exists := os.LookupEnv(key)

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf(
				"os.Unsetenv(%q) returned an error: %v",
				key,
				err,
			)
		}

		capturedKey := key
		capturedValue := value
		capturedExists := exists

		t.Cleanup(func() {
			if capturedExists {
				if err := os.Setenv(
					capturedKey,
					capturedValue,
				); err != nil {
					t.Errorf(
						"os.Setenv(%q) during cleanup returned an error: %v",
						capturedKey,
						err,
					)
				}

				return
			}

			if err := os.Unsetenv(capturedKey); err != nil {
				t.Errorf(
					"os.Unsetenv(%q) during cleanup returned an error: %v",
					capturedKey,
					err,
				)
			}
		})
	}
}

const testSwaggerInternalServiceToken = "abcdef0123456789abcdef0123456789"

func TestLoadHTTPAPIConfigDisablesSwaggerByDefault(
	t *testing.T,
) {
	actual, err :=
		loadHTTPAPIConfig(
			lookupFrom(
				map[string]string{
					environmentInternalServiceToken: testSwaggerInternalServiceToken,
				},
			),
		)

	if err != nil {
		t.Fatalf(
			"loadHTTPAPIConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.SwaggerEnabled {
		t.Fatal(
			"SwaggerEnabled = true, want false by default",
		)
	}
}

func TestLoadHTTPAPIConfigEnablesSwaggerExplicitly(
	t *testing.T,
) {
	actual, err :=
		loadHTTPAPIConfig(
			lookupFrom(
				map[string]string{
					environmentInternalServiceToken: testSwaggerInternalServiceToken,
					environmentSwaggerEnabled:       "true",
				},
			),
		)

	if err != nil {
		t.Fatalf(
			"loadHTTPAPIConfig() returned an unexpected error: %v",
			err,
		)
	}

	if !actual.SwaggerEnabled {
		t.Fatal(
			"SwaggerEnabled = false, want true",
		)
	}
}

func TestLoadHTTPAPIConfigAcceptsExplicitSwaggerFalse(
	t *testing.T,
) {
	actual, err :=
		loadHTTPAPIConfig(
			lookupFrom(
				map[string]string{
					environmentInternalServiceToken: testSwaggerInternalServiceToken,
					environmentSwaggerEnabled:       "false",
				},
			),
		)

	if err != nil {
		t.Fatalf(
			"loadHTTPAPIConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.SwaggerEnabled {
		t.Fatal(
			"SwaggerEnabled = true, want false",
		)
	}
}

func TestLoadHTTPAPIConfigRejectsInvalidSwaggerValue(
	t *testing.T,
) {
	_, err :=
		loadHTTPAPIConfig(
			lookupFrom(
				map[string]string{
					environmentInternalServiceToken: testSwaggerInternalServiceToken,
					environmentSwaggerEnabled:       "enabled",
				},
			),
		)

	if err == nil {
		t.Fatal(
			"expected invalid Swagger configuration error",
		)
	}
}

const testHTTPAPIInternalServiceToken = "0123456789abcdef0123456789abcdef"

func TestLoadHTTPAPIConfigUsesDefaults(
	t *testing.T,
) {
	actual, err :=
		loadHTTPAPIConfig(
			lookupFrom(
				map[string]string{
					environmentInternalServiceToken: testHTTPAPIInternalServiceToken,
				},
			),
		)

	if err != nil {
		t.Fatalf(
			"loadHTTPAPIConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.InternalServiceToken !=
		testHTTPAPIInternalServiceToken {

		t.Fatalf(
			"InternalServiceToken mismatch",
		)
	}

	if actual.HandlerTimeout !=
		defaultHTTPHandlerTimeout {

		t.Fatalf(
			"HandlerTimeout = %v, want %v",
			actual.HandlerTimeout,
			defaultHTTPHandlerTimeout,
		)
	}
}

func TestLoadHTTPAPIConfigAppliesHandlerTimeout(
	t *testing.T,
) {
	actual, err :=
		loadHTTPAPIConfig(
			lookupFrom(
				map[string]string{
					environmentInternalServiceToken: testHTTPAPIInternalServiceToken,

					environmentHTTPHandlerTimeout: "45s",
				},
			),
		)

	if err != nil {
		t.Fatalf(
			"loadHTTPAPIConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.HandlerTimeout !=
		45*time.Second {

		t.Fatalf(
			"HandlerTimeout = %v, want %v",
			actual.HandlerTimeout,
			45*time.Second,
		)
	}
}

func TestLoadHTTPAPIConfigRequiresToken(
	t *testing.T,
) {
	_, err :=
		loadHTTPAPIConfig(
			lookupFrom(nil),
		)

	if err == nil {
		t.Fatal(
			"expected missing internal service token error",
		)
	}

	if !strings.Contains(
		err.Error(),
		environmentInternalServiceToken,
	) {
		t.Fatalf(
			"error = %q",
			err.Error(),
		)
	}
}

func TestLoadHTTPAPIConfigRejectsShortToken(
	t *testing.T,
) {
	_, err :=
		loadHTTPAPIConfig(
			lookupFrom(
				map[string]string{
					environmentInternalServiceToken: "too-short",
				},
			),
		)

	if err == nil {
		t.Fatal(
			"expected short token error",
		)
	}

	if !strings.Contains(
		err.Error(),
		"at least",
	) {
		t.Fatalf(
			"error = %q",
			err.Error(),
		)
	}
}

func TestLoadHTTPAPIConfigRejectsWhitespaceInToken(
	t *testing.T,
) {
	_, err :=
		loadHTTPAPIConfig(
			lookupFrom(
				map[string]string{
					environmentInternalServiceToken: "0123456789abcdef 0123456789abcdef",
				},
			),
		)

	if err == nil {
		t.Fatal(
			"expected token whitespace error",
		)
	}

	if !strings.Contains(
		err.Error(),
		"whitespace",
	) {
		t.Fatalf(
			"error = %q",
			err.Error(),
		)
	}
}

func TestLoadHTTPAPIConfigRejectsInvalidTimeout(
	t *testing.T,
) {
	tests := []string{
		"invalid",
		"0s",
		"-1s",
	}

	for _, value := range tests {
		t.Run(
			value,
			func(t *testing.T) {
				_, err :=
					loadHTTPAPIConfig(
						lookupFrom(
							map[string]string{
								environmentInternalServiceToken: testHTTPAPIInternalServiceToken,

								environmentHTTPHandlerTimeout: value,
							},
						),
					)

				if err == nil {
					t.Fatal(
						"expected invalid timeout error",
					)
				}

				if !strings.Contains(
					err.Error(),
					environmentHTTPHandlerTimeout,
				) {
					t.Fatalf(
						"error = %q",
						err.Error(),
					)
				}
			},
		)
	}
}

func TestLoadKafkaConfigUsesDisabledDefaults(t *testing.T) {
	actual, err := loadKafkaConfig(lookupFrom(nil))
	if err != nil {
		t.Fatalf("loadKafkaConfig() returned an error: %v", err)
	}

	expected := defaultKafkaConfig()
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Kafka defaults mismatch\nactual:   %#v\nexpected: %#v", actual, expected)
	}

	if actual.Enabled {
		t.Fatal("Kafka must be disabled by default")
	}

	if !actual.IsValid() {
		t.Fatal("default Kafka configuration must be valid")
	}
}

func TestLoadKafkaConfigAppliesOverrides(t *testing.T) {
	actual, err := loadKafkaConfig(
		lookupFrom(
			map[string]string{
				environmentKafkaEnabled:         "true",
				environmentKafkaBrokers:         " kafka-1:9092, [::1]:9093 ",
				environmentKafkaEngineClientID:  "engine-test-v1",
				environmentKafkaWorkerClientID:  "worker-test-v1",
				environmentKafkaCommandTopic:    "test.workflow.commands.v1",
				environmentKafkaEventTopic:      "test.workflow.events.v1",
				environmentKafkaWorkerGroupID:   "test-workers-v1",
				environmentKafkaEngineGroupID:   "test-engine-v1",
				environmentKafkaMaxMessageBytes: "2097152",
			},
		),
	)
	if err != nil {
		t.Fatalf("loadKafkaConfig() returned an error: %v", err)
	}

	expected := KafkaConfig{
		Enabled:         true,
		Brokers:         []string{"kafka-1:9092", "[::1]:9093"},
		EngineClientID:  "engine-test-v1",
		WorkerClientID:  "worker-test-v1",
		CommandTopic:    "test.workflow.commands.v1",
		EventTopic:      "test.workflow.events.v1",
		WorkerGroupID:   "test-workers-v1",
		EngineGroupID:   "test-engine-v1",
		MaxMessageBytes: 2097152,
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Kafka overrides mismatch\nactual:   %#v\nexpected: %#v", actual, expected)
	}
}

func TestLoadDoesNotRequireKafkaEnvironment(t *testing.T) {
	configuration, err := load(
		lookupFrom(withRequiredPostgreSQLPassword(nil)),
	)
	if err != nil {
		t.Fatalf("load() returned an error without Kafka environment: %v", err)
	}

	if configuration.Kafka.Enabled {
		t.Fatal("Kafka must remain disabled without explicit enablement")
	}
}

func TestLoadKafkaConfigRejectsInvalidBrokers(t *testing.T) {
	tests := map[string]string{
		"blank":            " ",
		"missing port":     "kafka",
		"missing host":     ":9092",
		"invalid port":     "kafka:invalid",
		"port below range": "kafka:0",
		"duplicate":        "kafka:9092,kafka:9092",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadKafkaConfig(
				lookupFrom(map[string]string{environmentKafkaBrokers: value}),
			)
			requireErrorContains(t, err, environmentKafkaBrokers)
		})
	}
}

func TestLoadKafkaConfigRejectsInvalidTopics(t *testing.T) {
	tests := map[string]string{
		"blank":       " ",
		"single dot":  ".",
		"double dot":  "..",
		"space":       "workflow commands",
		"slash":       "workflow/commands",
		"same topics": defaultKafkaEventTopic,
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadKafkaConfig(
				lookupFrom(map[string]string{environmentKafkaCommandTopic: value}),
			)
			if err == nil {
				t.Fatal("expected invalid Kafka topic to be rejected")
			}
		})
	}
}

func TestLoadKafkaConfigRejectsSharedConsumerGroup(t *testing.T) {
	_, err := loadKafkaConfig(
		lookupFrom(
			map[string]string{
				environmentKafkaWorkerGroupID: defaultKafkaEngineGroupID,
			},
		),
	)

	requireErrorContains(t, err, "must be different")
}

func TestLoadKafkaConfigRejectsInvalidMaximumMessageBytes(t *testing.T) {
	tests := []string{"invalid", "1023", "16777217"}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			_, err := loadKafkaConfig(
				lookupFrom(map[string]string{environmentKafkaMaxMessageBytes: value}),
			)
			requireErrorContains(t, err, environmentKafkaMaxMessageBytes)
		})
	}
}

func TestLoadKafkaConfigRejectsInvalidBoolean(t *testing.T) {
	_, err := loadKafkaConfig(
		lookupFrom(map[string]string{environmentKafkaEnabled: "sometimes"}),
	)

	requireErrorContains(t, err, environmentKafkaEnabled)
}

func TestKafkaConfigValidationRejectsMalformedValues(t *testing.T) {
	tests := map[string]func(KafkaConfig) KafkaConfig{
		"missing broker": func(value KafkaConfig) KafkaConfig {
			value.Brokers = nil
			return value
		},
		"same topic": func(value KafkaConfig) KafkaConfig {
			value.EventTopic = value.CommandTopic
			return value
		},
		"same group": func(value KafkaConfig) KafkaConfig {
			value.EngineGroupID = value.WorkerGroupID
			return value
		},
		"oversized client ID": func(value KafkaConfig) KafkaConfig {
			value.EngineClientID = strings.Repeat("x", maximumKafkaNameLength+1)
			return value
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if mutate(defaultKafkaConfig()).IsValid() {
				t.Fatal("malformed Kafka configuration was accepted")
			}
		})
	}
}

func TestKafkaConfigBrokersAreIndependent(t *testing.T) {
	configuration, err := loadKafkaConfig(lookupFrom(nil))
	if err != nil {
		t.Fatalf("loadKafkaConfig() returned an error: %v", err)
	}

	configuration.Brokers[0] = "mutated:9092"

	other, err := loadKafkaConfig(lookupFrom(nil))
	if err != nil {
		t.Fatalf("loadKafkaConfig() returned an error: %v", err)
	}

	if other.Brokers[0] != defaultKafkaBroker {
		t.Fatal("default Kafka brokers share mutable slice state")
	}
}

func defaultKafkaConfig() KafkaConfig {
	return KafkaConfig{
		Enabled: false,

		Brokers: []string{defaultKafkaBroker},

		EngineClientID: defaultKafkaEngineClientID,
		WorkerClientID: defaultKafkaWorkerClientID,

		CommandTopic: defaultKafkaCommandTopic,
		EventTopic:   defaultKafkaEventTopic,

		WorkerGroupID: defaultKafkaWorkerGroupID,
		EngineGroupID: defaultKafkaEngineGroupID,

		MaxMessageBytes: defaultKafkaMaxMessageBytes,
	}
}

func TestLoadPostgreSQLReadsProcessEnvironment(
	t *testing.T,
) {
	clearRuntimeEnvironment(t)

	values := map[string]string{
		environmentPostgreSQLHost:           "127.0.0.1",
		environmentPostgreSQLPort:           "55479",
		environmentPostgreSQLDatabase:       "miletos",
		environmentPostgreSQLSchema:         "workflow_runtime",
		environmentPostgreSQLUser:           "runtime-user",
		environmentPostgreSQLPassword:       testPostgreSQLPassword,
		environmentPostgreSQLSSLMode:        "disable",
		environmentPostgreSQLMaxConnections: "8",
		environmentPostgreSQLMinConnections: "1",
		environmentPostgreSQLConnectTimeout: "7s",
		environmentPostgreSQLMigrationsPath: "./test-migrations",
	}

	for key, value := range values {
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf(
				"os.Setenv(%q) returned an unexpected error: %v",
				key,
				err,
			)
		}
	}

	actual, err := LoadPostgreSQL()
	if err != nil {
		t.Fatalf(
			"LoadPostgreSQL() returned an unexpected error: %v",
			err,
		)
	}

	if actual.Host != "127.0.0.1" {
		t.Fatalf(
			"Host = %q, want %q",
			actual.Host,
			"127.0.0.1",
		)
	}

	if actual.Port != 55479 {
		t.Fatalf(
			"Port = %d, want %d",
			actual.Port,
			55479,
		)
	}

	if actual.Database != "miletos" {
		t.Fatalf(
			"Database = %q, want %q",
			actual.Database,
			"miletos",
		)
	}

	if actual.Schema != "workflow_runtime" {
		t.Fatalf(
			"Schema = %q, want %q",
			actual.Schema,
			"workflow_runtime",
		)
	}

	if actual.Password != testPostgreSQLPassword {
		t.Fatal(
			"Password was not loaded from the process environment",
		)
	}

	if actual.MigrationsPath != "./test-migrations" {
		t.Fatalf(
			"MigrationsPath = %q, want %q",
			actual.MigrationsPath,
			"./test-migrations",
		)
	}
}

func TestLoadPostgreSQLConfigUsesDefaults(t *testing.T) {
	actual, err := loadPostgreSQLConfig(
		lookupFrom(
			map[string]string{
				environmentPostgreSQLPassword: testPostgreSQLPassword,
			},
		),
	)
	if err != nil {
		t.Fatalf(
			"loadPostgreSQLConfig() returned an unexpected error: %v",
			err,
		)
	}

	expected := PostgreSQLConfig{
		Host:           defaultPostgreSQLHost,
		Port:           defaultPostgreSQLPort,
		Database:       defaultPostgreSQLDatabase,
		Schema:         defaultPostgreSQLSchema,
		User:           defaultPostgreSQLUser,
		Password:       testPostgreSQLPassword,
		SSLMode:        defaultPostgreSQLSSLMode,
		MaxConnections: defaultPostgreSQLMaxConnections,
		MinConnections: defaultPostgreSQLMinConnections,
		ConnectTimeout: defaultPostgreSQLConnectTimeout,
		MigrationsPath: defaultPostgreSQLMigrationsPath,
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"PostgreSQL defaults mismatch\nactual:   %#v\nexpected: %#v",
			actual,
			expected,
		)
	}
}

func TestLoadPostgreSQLConfigAppliesOverrides(t *testing.T) {
	actual, err := loadPostgreSQLConfig(
		lookupFrom(
			map[string]string{
				environmentPostgreSQLHost:           " ::1 ",
				environmentPostgreSQLPort:           "55479",
				environmentPostgreSQLDatabase:       " runtime_test ",
				environmentPostgreSQLSchema:         " runtime_schema ",
				environmentPostgreSQLUser:           " runtime_user ",
				environmentPostgreSQLPassword:       "custom password",
				environmentPostgreSQLSSLMode:        "VERIFY-FULL",
				environmentPostgreSQLMaxConnections: "24",
				environmentPostgreSQLMinConnections: "4",
				environmentPostgreSQLConnectTimeout: "9s",
				environmentPostgreSQLMigrationsPath: " ./database/migrations ",
			},
		),
	)
	if err != nil {
		t.Fatalf(
			"loadPostgreSQLConfig() returned an unexpected error: %v",
			err,
		)
	}

	expected := PostgreSQLConfig{
		Host:           "::1",
		Port:           55479,
		Database:       "runtime_test",
		Schema:         "runtime_schema",
		User:           "runtime_user",
		Password:       "custom password",
		SSLMode:        "verify-full",
		MaxConnections: 24,
		MinConnections: 4,
		ConnectTimeout: 9 * time.Second,
		MigrationsPath: "./database/migrations",
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"PostgreSQL overrides mismatch\nactual:   %#v\nexpected: %#v",
			actual,
			expected,
		)
	}
}

func TestLoadPostgreSQLConfigRequiresPassword(t *testing.T) {
	_, err := loadPostgreSQLConfig(
		lookupFrom(nil),
	)

	requireErrorContains(
		t,
		err,
		environmentPostgreSQLPassword,
	)

	requireErrorContains(
		t,
		err,
		"must be set",
	)
}

func TestLoadPostgreSQLConfigRejectsBlankPassword(t *testing.T) {
	_, err := loadPostgreSQLConfig(
		lookupFrom(
			map[string]string{
				environmentPostgreSQLPassword: "   ",
			},
		),
	)

	requireErrorContains(
		t,
		err,
		environmentPostgreSQLPassword,
	)

	requireErrorContains(
		t,
		err,
		"must not be empty",
	)
}

func TestLoadPostgreSQLConfigRejectsInvalidPort(t *testing.T) {
	tests := map[string]string{
		"not integer": "invalid",
		"zero":        "0",
		"negative":    "-1",
		"above range": "65536",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadPostgreSQLConfig(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							environmentPostgreSQLPort: value,
						},
					),
				),
			)

			requireErrorContains(
				t,
				err,
				environmentPostgreSQLPort,
			)
		})
	}
}

func TestLoadPostgreSQLConfigRejectsBlankRequiredStrings(
	t *testing.T,
) {
	tests := []string{
		environmentPostgreSQLHost,
		environmentPostgreSQLDatabase,
		environmentPostgreSQLUser,
		environmentPostgreSQLMigrationsPath,
	}

	for _, key := range tests {
		t.Run(key, func(t *testing.T) {
			_, err := loadPostgreSQLConfig(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							key: "   ",
						},
					),
				),
			)

			requireErrorContains(
				t,
				err,
				key,
			)
		})
	}
}

func TestLoadPostgreSQLConfigRejectsInvalidSchema(t *testing.T) {
	tests := map[string]string{
		"blank":                 " ",
		"starts with number":    "1runtime",
		"contains hyphen":       "workflow-runtime",
		"contains dot":          "workflow.runtime",
		"contains quoted value": `"workflow_runtime"`,
		"contains whitespace":   "workflow runtime",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadPostgreSQLConfig(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							environmentPostgreSQLSchema: value,
						},
					),
				),
			)

			requireErrorContains(
				t,
				err,
				environmentPostgreSQLSchema,
			)
		})
	}
}

func TestLoadPostgreSQLConfigAcceptsValidSchemas(t *testing.T) {
	tests := []string{
		"workflow_runtime",
		"_runtime",
		"Runtime1",
	}

	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			actual, err := loadPostgreSQLConfig(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							environmentPostgreSQLSchema: value,
						},
					),
				),
			)
			if err != nil {
				t.Fatalf(
					"loadPostgreSQLConfig() returned an unexpected error: %v",
					err,
				)
			}

			if actual.Schema != value {
				t.Fatalf(
					"Schema = %q, want %q",
					actual.Schema,
					value,
				)
			}
		})
	}
}

func TestLoadPostgreSQLConfigRejectsInvalidSSLMode(t *testing.T) {
	_, err := loadPostgreSQLConfig(
		lookupFrom(
			withRequiredPostgreSQLPassword(
				map[string]string{
					environmentPostgreSQLSSLMode: "invalid",
				},
			),
		),
	)

	requireErrorContains(
		t,
		err,
		environmentPostgreSQLSSLMode,
	)

	requireErrorContains(
		t,
		err,
		"disable, allow, prefer, require, verify-ca, verify-full",
	)
}

func TestLoadPostgreSQLConfigAcceptsSupportedSSLModes(
	t *testing.T,
) {
	modes := []string{
		"disable",
		"allow",
		"prefer",
		"require",
		"verify-ca",
		"verify-full",
	}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			actual, err := loadPostgreSQLConfig(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							environmentPostgreSQLSSLMode: mode,
						},
					),
				),
			)
			if err != nil {
				t.Fatalf(
					"loadPostgreSQLConfig() returned an unexpected error: %v",
					err,
				)
			}

			if actual.SSLMode != mode {
				t.Fatalf(
					"SSLMode = %q, want %q",
					actual.SSLMode,
					mode,
				)
			}
		})
	}
}

func TestLoadPostgreSQLConfigRejectsInvalidConnectionCounts(
	t *testing.T,
) {
	tests := map[string]map[string]string{
		"max not integer": {
			environmentPostgreSQLMaxConnections: "invalid",
		},
		"max zero": {
			environmentPostgreSQLMaxConnections: "0",
		},
		"max negative": {
			environmentPostgreSQLMaxConnections: "-1",
		},
		"max above int32": {
			environmentPostgreSQLMaxConnections: "2147483648",
		},
		"min not integer": {
			environmentPostgreSQLMinConnections: "invalid",
		},
		"min negative": {
			environmentPostgreSQLMinConnections: "-1",
		},
		"min above max": {
			environmentPostgreSQLMinConnections: "11",
			environmentPostgreSQLMaxConnections: "10",
		},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadPostgreSQLConfig(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						values,
					),
				),
			)

			if err == nil {
				t.Fatal(
					"loadPostgreSQLConfig() returned nil error for invalid connection counts",
				)
			}
		})
	}
}

func TestLoadPostgreSQLConfigAllowsZeroMinimumConnections(
	t *testing.T,
) {
	actual, err := loadPostgreSQLConfig(
		lookupFrom(
			withRequiredPostgreSQLPassword(
				map[string]string{
					environmentPostgreSQLMinConnections: "0",
				},
			),
		),
	)
	if err != nil {
		t.Fatalf(
			"loadPostgreSQLConfig() returned an unexpected error: %v",
			err,
		)
	}

	if actual.MinConnections != 0 {
		t.Fatalf(
			"MinConnections = %d, want 0",
			actual.MinConnections,
		)
	}
}

func TestLoadPostgreSQLConfigRejectsInvalidConnectTimeout(
	t *testing.T,
) {
	tests := map[string]string{
		"invalid":  "value",
		"zero":     "0s",
		"negative": "-1s",
	}

	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := loadPostgreSQLConfig(
				lookupFrom(
					withRequiredPostgreSQLPassword(
						map[string]string{
							environmentPostgreSQLConnectTimeout: value,
						},
					),
				),
			)

			requireErrorContains(
				t,
				err,
				environmentPostgreSQLConnectTimeout,
			)
		})
	}
}

func TestPostgreSQLConfigAddressSupportsIPv6(t *testing.T) {
	configuration := PostgreSQLConfig{
		Host: "::1",
		Port: 5432,
	}

	if actual := configuration.Address(); actual != "[::1]:5432" {
		t.Fatalf(
			"Address() = %q, want %q",
			actual,
			"[::1]:5432",
		)
	}
}

func TestPostgreSQLConfigMaskedConnectionSummaryDoesNotExposePassword(
	t *testing.T,
) {
	configuration := PostgreSQLConfig{
		Host:     "localhost",
		Port:     5432,
		Database: "miletos",
		Schema:   "workflow_runtime",
		User:     "runtime-user",
		Password: "highly-sensitive-password",
		SSLMode:  "disable",
	}

	summary := configuration.MaskedConnectionSummary()

	if strings.Contains(
		summary,
		configuration.Password,
	) {
		t.Fatal(
			"MaskedConnectionSummary() exposed the PostgreSQL password",
		)
	}

	expectedParts := []string{
		"host=localhost",
		"port=5432",
		"database=miletos",
		"schema=workflow_runtime",
		"user=runtime-user",
		"sslmode=disable",
	}

	for _, expectedPart := range expectedParts {
		if !strings.Contains(summary, expectedPart) {
			t.Fatalf(
				"MaskedConnectionSummary() = %q, want it to contain %q",
				summary,
				expectedPart,
			)
		}
	}
}
