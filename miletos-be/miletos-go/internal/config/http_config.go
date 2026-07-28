package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	environmentInternalServiceToken  = environmentPrefix + "INTERNAL_SERVICE_TOKEN"
	environmentHTTPHandlerTimeout    = environmentPrefix + "HTTP_HANDLER_TIMEOUT"
	environmentSwaggerEnabled        = environmentPrefix + "SWAGGER_ENABLED"
	defaultHTTPHandlerTimeout        = 30 * time.Second
	minimumInternalServiceTokenBytes = 32
)

type HTTPAPIConfig struct {
	InternalServiceToken string        `env:"MILETOS_RUNTIME_INTERNAL_SERVICE_TOKEN"`
	HandlerTimeout       time.Duration `env:"MILETOS_RUNTIME_HTTP_HANDLER_TIMEOUT"`
	SwaggerEnabled       bool          `env:"MILETOS_RUNTIME_SWAGGER_ENABLED"`
}

func LoadHTTPAPI() (HTTPAPIConfig, error) { return loadHTTPAPIConfig(os.LookupEnv) }

func loadHTTPAPIConfig(lookup environmentLookup) (HTTPAPIConfig, error) {
	if err := requireEnvironmentSecret(lookup, environmentInternalServiceToken); err != nil {
		return HTTPAPIConfig{}, err
	}
	if value, exists := lookup(environmentSwaggerEnabled); exists {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized != "true" && normalized != "false" {
			return HTTPAPIConfig{}, fmt.Errorf("%s must be true or false", environmentSwaggerEnabled)
		}
	}

	configuration := HTTPAPIConfig{HandlerTimeout: defaultHTTPHandlerTimeout}
	if err := parseEnvironment(
		&configuration,
		lookup,
		[]string{
			environmentInternalServiceToken,
			environmentHTTPHandlerTimeout,
			environmentSwaggerEnabled,
		},
	); err != nil {
		return HTTPAPIConfig{}, err
	}

	configuration.InternalServiceToken = strings.TrimSpace(configuration.InternalServiceToken)
	if err := validateHTTPAPIConfig(configuration); err != nil {
		return HTTPAPIConfig{}, err
	}
	return configuration, nil
}

func validateHTTPAPIConfig(configuration HTTPAPIConfig) error {
	if len(configuration.InternalServiceToken) < minimumInternalServiceTokenBytes {
		return fmt.Errorf(
			"%s must contain at least %d bytes",
			environmentInternalServiceToken,
			minimumInternalServiceTokenBytes,
		)
	}
	if strings.ContainsAny(configuration.InternalServiceToken, " \t\r\n") {
		return fmt.Errorf("%s must not contain whitespace", environmentInternalServiceToken)
	}
	if configuration.HandlerTimeout <= 0 {
		return fmt.Errorf("%s must be greater than zero", environmentHTTPHandlerTimeout)
	}
	return nil
}
