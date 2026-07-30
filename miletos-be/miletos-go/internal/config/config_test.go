package config

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

var configEnvironment = []string{
	"MILETOS_RUNTIME_SERVICE_NAME",
	"MILETOS_RUNTIME_ENVIRONMENT",
	"MILETOS_RUNTIME_HTTP_HOST",
	"MILETOS_RUNTIME_HTTP_PORT",
	"MILETOS_RUNTIME_LOG_LEVEL",
	"MILETOS_RUNTIME_POSTGRES_URL",
	"MILETOS_RUNTIME_POSTGRES_PASSWORD",
	"MILETOS_RUNTIME_POSTGRES_HOST",
	"MILETOS_RUNTIME_POSTGRES_PORT",
	"MILETOS_RUNTIME_POSTGRES_DATABASE",
	"MILETOS_RUNTIME_POSTGRES_USER",
	"MILETOS_RUNTIME_POSTGRES_SSL_MODE",
	"MILETOS_RUNTIME_KAFKA_ENABLED",
	"MILETOS_RUNTIME_KAFKA_BROKERS",
	"MILETOS_RUNTIME_KAFKA_CLIENT_ID",
	"MILETOS_RUNTIME_KAFKA_COMMAND_TOPIC",
	"MILETOS_RUNTIME_KAFKA_CONSUMER_GROUP",
	"MILETOS_RUNTIME_INTERNAL_SERVICE_TOKEN",
	"MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS",
	"MILETOS_RUNTIME_RETRY_DELAY",
}

func configureValidEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range configEnvironment {
		unsetEnvironment(t, name)
	}
	t.Setenv("MILETOS_RUNTIME_POSTGRES_PASSWORD", "test-password")
	t.Setenv("MILETOS_RUNTIME_INTERNAL_SERVICE_TOKEN", strings.Repeat("t", 32))
}

func unsetEnvironment(t *testing.T, name string) {
	t.Helper()
	previous, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, previous)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func TestLoadUsesDocumentedDefaults(t *testing.T) {
	configureValidEnvironment(t)

	configuration, err := Load()

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if configuration.ServiceName != "miletos-go" ||
		configuration.Environment != "local" ||
		configuration.HTTPAddress() != "0.0.0.0:8081" ||
		configuration.LogLevel != slog.LevelInfo ||
		configuration.KafkaEnabled ||
		configuration.RetryMaxAttempts != 3 ||
		configuration.RetryDelay != 250*time.Millisecond {
		t.Fatalf("unexpected defaults: %#v", configuration)
	}
	if !strings.Contains(configuration.PostgreSQLURL, "postgres://miletos:") {
		t.Fatalf("PostgreSQLURL = %q", configuration.PostgreSQLURL)
	}
}

func TestLoadAppliesIdentityAndHTTPOverrides(t *testing.T) {
	configureValidEnvironment(t)
	t.Setenv("MILETOS_RUNTIME_SERVICE_NAME", "runtime-test")
	t.Setenv("MILETOS_RUNTIME_ENVIRONMENT", "ci")
	t.Setenv("MILETOS_RUNTIME_HTTP_HOST", "127.0.0.1")
	t.Setenv("MILETOS_RUNTIME_HTTP_PORT", "9090")
	t.Setenv("MILETOS_RUNTIME_LOG_LEVEL", "debug")

	configuration, err := Load()

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if configuration.ServiceName != "runtime-test" ||
		configuration.Environment != "ci" ||
		configuration.HTTPAddress() != "127.0.0.1:9090" ||
		configuration.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected overrides: %#v", configuration)
	}
}

func TestLoadStrictParsedFields(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantError bool
		assert    func(*testing.T, Config)
	}{
		{
			name: "valid HTTP port", key: "MILETOS_RUNTIME_HTTP_PORT", value: "9090",
			assert: func(t *testing.T, configuration Config) {
				if configuration.HTTPPort != 9090 {
					t.Fatalf("HTTPPort = %d", configuration.HTTPPort)
				}
			},
		},
		{name: "malformed HTTP port", key: "MILETOS_RUNTIME_HTTP_PORT", value: "invalid", wantError: true},
		{name: "empty HTTP port", key: "MILETOS_RUNTIME_HTTP_PORT", value: "", wantError: true},
		{name: "HTTP port below range", key: "MILETOS_RUNTIME_HTTP_PORT", value: "0", wantError: true},
		{name: "HTTP port above range", key: "MILETOS_RUNTIME_HTTP_PORT", value: "65536", wantError: true},
		{
			name: "valid Kafka enabled", key: "MILETOS_RUNTIME_KAFKA_ENABLED", value: "true",
			assert: func(t *testing.T, configuration Config) {
				if !configuration.KafkaEnabled {
					t.Fatal("KafkaEnabled = false")
				}
			},
		},
		{name: "malformed Kafka enabled", key: "MILETOS_RUNTIME_KAFKA_ENABLED", value: "sometimes", wantError: true},
		{name: "empty Kafka enabled", key: "MILETOS_RUNTIME_KAFKA_ENABLED", value: "", wantError: true},
		{
			name: "valid retry attempts", key: "MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS", value: "5",
			assert: func(t *testing.T, configuration Config) {
				if configuration.RetryMaxAttempts != 5 {
					t.Fatalf("RetryMaxAttempts = %d", configuration.RetryMaxAttempts)
				}
			},
		},
		{name: "malformed retry attempts", key: "MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS", value: "many", wantError: true},
		{name: "empty retry attempts", key: "MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS", value: "", wantError: true},
		{name: "zero retry attempts", key: "MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS", value: "0", wantError: true},
		{name: "excessive retry attempts", key: "MILETOS_RUNTIME_RETRY_MAX_ATTEMPTS", value: "32768", wantError: true},
		{
			name: "valid retry delay", key: "MILETOS_RUNTIME_RETRY_DELAY", value: "2s",
			assert: func(t *testing.T, configuration Config) {
				if configuration.RetryDelay != 2*time.Second {
					t.Fatalf("RetryDelay = %v", configuration.RetryDelay)
				}
			},
		},
		{name: "malformed retry delay", key: "MILETOS_RUNTIME_RETRY_DELAY", value: "later", wantError: true},
		{name: "empty retry delay", key: "MILETOS_RUNTIME_RETRY_DELAY", value: "", wantError: true},
		{name: "zero retry delay", key: "MILETOS_RUNTIME_RETRY_DELAY", value: "0s", wantError: true},
		{name: "negative retry delay", key: "MILETOS_RUNTIME_RETRY_DELAY", value: "-1s", wantError: true},
		{name: "invalid log level", key: "MILETOS_RUNTIME_LOG_LEVEL", value: "verbose", wantError: true},
		{name: "empty log level", key: "MILETOS_RUNTIME_LOG_LEVEL", value: "", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureValidEnvironment(t)
			t.Setenv(test.key, test.value)

			configuration, err := Load()

			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), test.key) {
					t.Fatalf("Load() error = %v, want error identifying %s", err, test.key)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			test.assert(t, configuration)
		})
	}
}

func TestLoadAcceptsConfiguredPostgreSQLURLWithoutPrevalidation(t *testing.T) {
	tests := []struct {
		name       string
		connection string
	}{
		{name: "URL", connection: "postgres://user:password@localhost/test_database?sslmode=disable"},
		{name: "URL with explicit port", connection: "postgres://user:password@127.0.0.1:6543/test_database"},
		{name: "keyword DSN", connection: "host=localhost port=6543 dbname=test_database user=test password=password sslmode=disable"},
		{name: "unreachable host is not contacted", connection: "postgres://user:password@does-not-exist.invalid:6543/test_database"},
		{name: "malformed URL", connection: "postgres://%zz"},
		{name: "malformed keyword DSN", connection: "host='unterminated"},
		{name: "invalid port", connection: "postgres://user:password@localhost:not-a-port/test_database"},
		{name: "malformed query value", connection: "postgres://user:password@localhost/test_database?connect_timeout=not-a-number"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configureValidEnvironment(t)
			t.Setenv("MILETOS_RUNTIME_POSTGRES_URL", test.connection)

			configuration, err := Load()

			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if configuration.PostgreSQLURL != test.connection {
				t.Fatalf("PostgreSQLURL = %q, want %q", configuration.PostgreSQLURL, test.connection)
			}
		})
	}
}

func TestLoadRequiresPostgreSQLPasswordForFallback(t *testing.T) {
	t.Run("missing URL and password", func(t *testing.T) {
		configureValidEnvironment(t)
		t.Setenv("MILETOS_RUNTIME_POSTGRES_PASSWORD", "")

		_, err := Load()

		if err == nil || !strings.Contains(err.Error(), "MILETOS_RUNTIME_POSTGRES_PASSWORD") {
			t.Fatalf("Load() error = %v", err)
		}
	})

}

func TestLoadKafkaFieldValidation(t *testing.T) {
	required := []string{
		"MILETOS_RUNTIME_KAFKA_BROKERS",
		"MILETOS_RUNTIME_KAFKA_CLIENT_ID",
		"MILETOS_RUNTIME_KAFKA_COMMAND_TOPIC",
		"MILETOS_RUNTIME_KAFKA_CONSUMER_GROUP",
	}
	for _, key := range required {
		t.Run("empty "+key, func(t *testing.T) {
			configureValidEnvironment(t)
			t.Setenv("MILETOS_RUNTIME_KAFKA_ENABLED", "true")
			t.Setenv(key, "")

			_, err := Load()

			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("Load() error = %v, want error identifying %s", err, key)
			}
		})
	}

	t.Run("disabled Kafka does not require fields", func(t *testing.T) {
		configureValidEnvironment(t)
		t.Setenv("MILETOS_RUNTIME_KAFKA_ENABLED", "false")
		for _, key := range required {
			t.Setenv(key, "")
		}

		if _, err := Load(); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
	})

	t.Run("valid values are trimmed and empty broker entries are filtered", func(t *testing.T) {
		configureValidEnvironment(t)
		t.Setenv("MILETOS_RUNTIME_KAFKA_ENABLED", "true")
		t.Setenv("MILETOS_RUNTIME_KAFKA_BROKERS", " broker-1:9092, ,broker-2:9092 ")
		t.Setenv("MILETOS_RUNTIME_KAFKA_CLIENT_ID", " client ")
		t.Setenv("MILETOS_RUNTIME_KAFKA_COMMAND_TOPIC", " commands ")
		t.Setenv("MILETOS_RUNTIME_KAFKA_CONSUMER_GROUP", " group ")

		configuration, err := Load()

		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if strings.Join(configuration.KafkaBrokers, ",") != "broker-1:9092,broker-2:9092" ||
			configuration.KafkaClientID != "client" ||
			configuration.KafkaCommandTopic != "commands" ||
			configuration.KafkaConsumerGroup != "group" {
			t.Fatalf("Kafka configuration = %#v", configuration)
		}
	})
}
