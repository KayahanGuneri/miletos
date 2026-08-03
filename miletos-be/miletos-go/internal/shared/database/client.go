package database

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Client struct {
	db    *gorm.DB
	sqlDB *sql.DB
}

type Result struct {
	rowsAffected int64
}

func (result Result) RowsAffected() int64 {
	return result.rowsAffected
}

type Transaction struct {
	db *gorm.DB
}

func (client *Client) DB(ctx context.Context) *gorm.DB {
	return client.db.WithContext(ctx)
}

func (transaction *Transaction) DB(ctx context.Context) *gorm.DB {
	return transaction.db.WithContext(ctx)
}

var positionalParameter = regexp.MustCompile(`\$(\d+)`)

func normalizeQuery(query string, arguments []any) (string, []any) {
	normalizedArguments := make([]any, 0)
	normalizedQuery := positionalParameter.ReplaceAllStringFunc(
		query,
		func(parameter string) string {
			index, err := strconv.Atoi(parameter[1:])
			if err != nil || index < 1 || index > len(arguments) {
				return parameter
			}
			normalizedArguments = append(normalizedArguments, arguments[index-1])
			return "?"
		},
	)
	return normalizedQuery, normalizedArguments
}

func Open(ctx context.Context, connectionURL string) (*Client, error) {
	db, err := gorm.Open(postgres.Open(connectionURL), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obtain PostgreSQL connection: %w", err)
	}
	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingContext); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return &Client{db: db, sqlDB: sqlDB}, nil
}

func (client *Client) Close() error {
	if client == nil || client.sqlDB == nil {
		return nil
	}
	return client.sqlDB.Close()
}

func (client *Client) Exec(
	ctx context.Context,
	query string,
	arguments ...any,
) (Result, error) {
	query, arguments = normalizeQuery(query, arguments)
	result := client.db.WithContext(ctx).Exec(query, arguments...)
	return Result{rowsAffected: result.RowsAffected}, result.Error
}

func (client *Client) QueryRow(
	ctx context.Context,
	query string,
	arguments ...any,
) *sql.Row {
	query, arguments = normalizeQuery(query, arguments)
	return client.db.WithContext(ctx).Raw(query, arguments...).Row()
}

func (client *Client) Query(
	ctx context.Context,
	query string,
	arguments ...any,
) (*sql.Rows, error) {
	query, arguments = normalizeQuery(query, arguments)
	return client.db.WithContext(ctx).Raw(query, arguments...).Rows()
}

func (client *Client) Begin(ctx context.Context) (*Transaction, error) {
	transaction := client.db.WithContext(ctx).Begin()
	if transaction.Error != nil {
		return nil, transaction.Error
	}
	return &Transaction{db: transaction}, nil
}

func (transaction *Transaction) Exec(
	ctx context.Context,
	query string,
	arguments ...any,
) (Result, error) {
	query, arguments = normalizeQuery(query, arguments)
	result := transaction.db.WithContext(ctx).Exec(query, arguments...)
	return Result{rowsAffected: result.RowsAffected}, result.Error
}

func (transaction *Transaction) QueryRow(
	ctx context.Context,
	query string,
	arguments ...any,
) *sql.Row {
	query, arguments = normalizeQuery(query, arguments)
	return transaction.db.WithContext(ctx).Raw(query, arguments...).Row()
}

func (transaction *Transaction) Query(
	ctx context.Context,
	query string,
	arguments ...any,
) (*sql.Rows, error) {
	query, arguments = normalizeQuery(query, arguments)
	return transaction.db.WithContext(ctx).Raw(query, arguments...).Rows()
}

func (transaction *Transaction) Commit(_ context.Context) error {
	return transaction.db.Commit().Error
}

func (transaction *Transaction) Rollback(_ context.Context) error {
	return transaction.db.Rollback().Error
}
