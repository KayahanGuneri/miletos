package plugin

import (
	"context"
	"strings"
)

const defaultDatabaseSSLMode = "disable"

func validateDatabaseConnectionConfiguration(
	configuration map[string]any,
	codePrefix string,
) error {
	if configString(configuration, "databaseHost") == "" {
		return validationNodeError(
			codePrefix+"_DATABASE_HOST_REQUIRED",
			"configuration.databaseHost is required",
		)
	}
	port, ok := configInt(configuration, "databasePort")
	if !ok || port < 1 || port > 65535 {
		return validationNodeError(
			codePrefix+"_DATABASE_PORT_INVALID",
			"configuration.databasePort must be an integer between 1 and 65535",
		)
	}
	if configString(configuration, "databaseName") == "" {
		return validationNodeError(
			codePrefix+"_DATABASE_NAME_REQUIRED",
			"configuration.databaseName is required",
		)
	}
	if configString(configuration, "databaseUsername") == "" {
		return validationNodeError(
			codePrefix+"_DATABASE_USERNAME_REQUIRED",
			"configuration.databaseUsername is required",
		)
	}
	if configString(configuration, "databasePasswordEncrypted") == "" {
		return validationNodeError(
			codePrefix+"_DATABASE_PASSWORD_REQUIRED",
			"configuration.databasePasswordEncrypted is required",
		)
	}
	sslMode := databaseSSLMode(configuration)
	if !supportedDatabaseSSLMode(sslMode) {
		return validationNodeError(
			codePrefix+"_DATABASE_SSL_MODE_INVALID",
			"configuration.databaseSslMode must be disable, allow, prefer, require, verify-ca, or verify-full",
		)
	}
	return nil
}

func resolveDatabaseConnectionConfig(
	secrets SecretDecryptor,
	configuration map[string]any,
	codePrefix string,
) (DatabaseConnectionConfig, error) {
	if err := validateDatabaseConnectionConfiguration(configuration, codePrefix); err != nil {
		return DatabaseConnectionConfig{}, err
	}
	if secrets == nil || !secrets.Configured() {
		return DatabaseConnectionConfig{}, &NodeError{
			Category: "INTERNAL",
			Code:     codePrefix + "_DATABASE_SECRET_UNAVAILABLE",
			Message:  "Database password decryption is not configured for this runtime.",
		}
	}
	password, err := secrets.Decrypt(configString(configuration, "databasePasswordEncrypted"))
	if err != nil {
		return DatabaseConnectionConfig{}, &NodeError{
			Category: "INTERNAL",
			Code:     codePrefix + "_DATABASE_SECRET_DECRYPT_FAILED",
			Message:  "The database password could not be decrypted.",
		}
	}
	port, _ := configInt(configuration, "databasePort")
	return DatabaseConnectionConfig{
		Host:     configString(configuration, "databaseHost"),
		Port:     port,
		Database: configString(configuration, "databaseName"),
		Username: configString(configuration, "databaseUsername"),
		Password: password,
		SSLMode:  databaseSSLMode(configuration),
	}, nil
}

func openDatabaseConnection(
	ctx context.Context,
	database DatabaseInfrastructure,
	secrets SecretDecryptor,
	configuration map[string]any,
	codePrefix string,
) (DatabaseConnection, error) {
	resolved, err := resolveDatabaseConnectionConfig(secrets, configuration, codePrefix)
	if err != nil {
		return nil, err
	}
	return database.Open(ctx, resolved)
}

func databaseSSLMode(configuration map[string]any) string {
	sslMode := strings.ToLower(configString(configuration, "databaseSslMode"))
	if sslMode == "" {
		return defaultDatabaseSSLMode
	}
	return sslMode
}

func supportedDatabaseSSLMode(sslMode string) bool {
	switch sslMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}
