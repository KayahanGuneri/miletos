package execution

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"miletos-go/internal/features/workflow-runtime/plugin"
	"miletos-go/internal/shared/database"
)

type databaseInfrastructure struct{}

type databaseConnection struct {
	client *database.Client
}

type databaseConnectionError struct {
	cause error
}

type rowsAffectedResult struct {
	rowsAffected int64
}

func NewDatabaseInfrastructure() plugin.DatabaseInfrastructure {
	return &databaseInfrastructure{}
}

func (*databaseInfrastructure) Open(
	ctx context.Context,
	configuration plugin.DatabaseConnectionConfig,
) (plugin.DatabaseConnection, error) {
	connectionURL, err := databaseConnectionURL(configuration)
	if err != nil {
		return nil, err
	}
	client, err := database.Open(ctx, connectionURL)
	if err != nil {
		return nil, &databaseConnectionError{cause: err}
	}
	return &databaseConnection{client: client}, nil
}

func databaseConnectionURL(configuration plugin.DatabaseConnectionConfig) (string, error) {
	host := strings.TrimSpace(configuration.Host)
	databaseName := strings.TrimSpace(configuration.Database)
	username := strings.TrimSpace(configuration.Username)
	sslMode := strings.TrimSpace(configuration.SSLMode)
	if host == "" || databaseName == "" || username == "" || configuration.Password == "" ||
		configuration.Port < 1 || configuration.Port > 65535 || sslMode == "" {
		return "", errors.New("database connection configuration is invalid")
	}
	connection := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(username, configuration.Password),
		Host:   net.JoinHostPort(host, strconv.Itoa(configuration.Port)),
		Path:   databaseName,
	}
	query := connection.Query()
	query.Set("sslmode", sslMode)
	connection.RawQuery = query.Encode()
	return connection.String(), nil
}

func (connection *databaseConnection) Query(
	ctx context.Context,
	query string,
	arguments ...any,
) (*sql.Rows, error) {
	if connection == nil || connection.client == nil {
		return nil, plugin.ErrCapabilityUnavailable
	}
	return connection.client.Query(ctx, query, arguments...)
}

func (connection *databaseConnection) Exec(
	ctx context.Context,
	query string,
	arguments ...any,
) (sql.Result, error) {
	if connection == nil || connection.client == nil {
		return nil, plugin.ErrCapabilityUnavailable
	}
	result, err := connection.client.Exec(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	return rowsAffectedResult{rowsAffected: result.RowsAffected()}, nil
}

func (connection *databaseConnection) Close() error {
	if connection == nil || connection.client == nil {
		return nil
	}
	return connection.client.Close()
}

func (err *databaseConnectionError) Error() string {
	return "open PostgreSQL connection failed"
}

func (err *databaseConnectionError) Unwrap() error {
	return err.cause
}

func (result rowsAffectedResult) LastInsertId() (int64, error) {
	return 0, fmt.Errorf("LastInsertId is not supported")
}

func (result rowsAffectedResult) RowsAffected() (int64, error) {
	return result.rowsAffected, nil
}
