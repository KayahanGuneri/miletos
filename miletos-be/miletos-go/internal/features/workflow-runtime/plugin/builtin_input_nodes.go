package plugin

import (
	"bytes"
	"context"
	"database/sql/driver"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/ssh"

	"miletos-go/internal/shared/database"
)

type SFTPConfig struct {
	Host          string
	Port          int
	Username      string
	Password      string
	BaseDirectory string
	HostKeySHA256 string
}

type InputNodeRuntime struct {
	Database       *database.Client
	InputDirectory string
	Secrets        SecretDecryptor
}

// SecretDecryptor decrypts AES-GCM secrets stored by the Java control-plane.
type SecretDecryptor interface {
	Configured() bool
	Decrypt(encoded string) (string, error)
}

func RegisterInputSourceNodes(registry *NodeRegistry, runtime InputNodeRuntime) error {
	registrations := []NodeRegistration{
		fileInputRegistration(runtime),
		excelInputRegistration(runtime),
		databaseInputRegistration(runtime.Database),
	}
	for _, registration := range registrations {
		if err := registry.RegisterNode(registration); err != nil {
			return err
		}
	}
	return nil
}

func fileInputRegistration(runtime InputNodeRuntime) NodeRegistration {
	return builtinRegistration(builtinRegistrationSpec{
		Type:        "core.file-input",
		DisplayName: "File Input",
		Description: "Reads a TXT or CSV file from a local tenant input directory or configured SFTP source",
		Input:       false,
		Output:      true,
		OnRun:       fileInputHandler(runtime),
		Validator:   validateFileInput,
	})
}

func excelInputRegistration(runtime InputNodeRuntime) NodeRegistration {
	return builtinRegistration(builtinRegistrationSpec{
		Type:        "core.excel-input",
		DisplayName: "Excel Input",
		Description: "Reads an XLSX worksheet from a local tenant input directory or configured SFTP source",
		Input:       false,
		Output:      true,
		OnRun:       excelInputHandler(runtime),
		Validator:   validateExcelInput,
	})
}

func databaseInputRegistration(dbClient *database.Client) NodeRegistration {
	return builtinRegistration(builtinRegistrationSpec{
		Type:        "core.database-input",
		DisplayName: "Database Input",
		Description: "Runs a SELECT query against the configured read-only PostgreSQL input source",
		Input:       false,
		Output:      true,
		OnRun:       databaseInputHandler(dbClient),
		Validator:   validateDatabaseInput,
	})
}

func validateFileInput(configuration map[string]any) error {
	if err := validateInputSourceType(configuration, "FILE_INPUT"); err != nil {
		return err
	}
	fileName := configString(configuration, "fileName")
	if fileName == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "FILE_INPUT_FILENAME_REQUIRED",
			Message:  "configuration.fileName is required",
		}
	}
	if !isSafePathSegment(fileName) {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "FILE_INPUT_FILENAME_INVALID",
			Message:  "configuration.fileName must be a plain file name without path separators",
		}
	}
	lower := strings.ToLower(fileName)
	if !strings.HasSuffix(lower, ".txt") && !strings.HasSuffix(lower, ".csv") {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "FILE_INPUT_EXTENSION_UNSUPPORTED",
			Message:  "configuration.fileName must end with .txt or .csv",
		}
	}
	return validateSFTPConfiguration(configuration, "FILE_INPUT")
}

func fileInputHandler(runtime InputNodeRuntime) RunHandler {
	return func(nodeContext *Context) (any, error) {
		configuration := nodeContext.Configuration
		if err := validateFileInput(configuration); err != nil {
			return nil, err
		}
		fileName := configString(configuration, "fileName")
		content, err := readInputContent(
			nodeContext.Runtime,
			runtime,
			nodeContext.Execution.CompanyID,
			configuration,
			fileName,
			"FILE_INPUT",
		)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(strings.ToLower(fileName), ".txt") {
			return string(content), nil
		}
		return csvRecordsFromContent(string(content))
	}
}

func csvRecordsFromContent(content string) ([]map[string]any, error) {
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(content, "\uFEFF")))
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     "FILE_INPUT_CSV_INVALID",
			Message:  "The CSV input file is invalid.",
		}
	}
	return tabularRecordsFromRows(rows, "FILE_INPUT_CSV_INVALID")
}

func validateExcelInput(configuration map[string]any) error {
	if err := validateInputSourceType(configuration, "EXCEL_INPUT"); err != nil {
		return err
	}
	fileName := configString(configuration, "fileName")
	if fileName == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "EXCEL_INPUT_FILENAME_REQUIRED",
			Message:  "configuration.fileName is required",
		}
	}
	if !isSafePathSegment(fileName) {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "EXCEL_INPUT_FILENAME_INVALID",
			Message:  "configuration.fileName must be a plain file name without path separators",
		}
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".xlsx") {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "EXCEL_INPUT_EXTENSION_UNSUPPORTED",
			Message:  "configuration.fileName must end with .xlsx",
		}
	}
	if _, ok := configuration["sheetName"]; ok {
		if _, ok := configuration["sheetName"].(string); !ok {
			return &NodeError{
				Category: "VALIDATION",
				Code:     "EXCEL_INPUT_DATA_INVALID",
				Message:  "configuration.sheetName must be a string when provided",
			}
		}
	}
	return validateSFTPConfiguration(configuration, "EXCEL_INPUT")
}

func excelInputHandler(runtime InputNodeRuntime) RunHandler {
	return func(nodeContext *Context) (any, error) {
		configuration := nodeContext.Configuration
		if err := validateExcelInput(configuration); err != nil {
			return nil, err
		}
		fileName := configString(configuration, "fileName")
		sheetName := configString(configuration, "sheetName")
		content, err := readInputContent(
			nodeContext.Runtime,
			runtime,
			nodeContext.Execution.CompanyID,
			configuration,
			fileName,
			"EXCEL_INPUT",
		)
		if err != nil {
			return nil, err
		}
		workbook, err := excelize.OpenReader(bytes.NewReader(content))
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "EXCEL_INPUT_OPEN_FAILED",
				Message:  "The Excel input file could not be opened.",
			}
		}
		defer workbook.Close()
		sheets := workbook.GetSheetList()
		if len(sheets) == 0 {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "EXCEL_INPUT_SHEET_NOT_FOUND",
				Message:  "The Excel workbook does not contain any sheets.",
			}
		}
		if sheetName == "" {
			sheetName = sheets[0]
		} else if !slices.Contains(sheets, sheetName) {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "EXCEL_INPUT_SHEET_NOT_FOUND",
				Message:  "The configured Excel sheet was not found.",
			}
		}
		rows, err := workbook.GetRows(sheetName)
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "EXCEL_INPUT_DATA_INVALID",
				Message:  "The Excel sheet could not be read.",
			}
		}
		return tabularRecordsFromRows(rows, "EXCEL_INPUT_DATA_INVALID")
	}
}

func validateDatabaseInput(configuration map[string]any) error {
	query := configString(configuration, "query")
	if query == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_INPUT_QUERY_REQUIRED",
			Message:  "configuration.query is required",
		}
	}
	if !isConservativeSelectQuery(query) {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_INPUT_QUERY_INVALID",
			Message:  "configuration.query must be a single SELECT statement",
		}
	}
	return nil
}

func databaseInputHandler(dbClient *database.Client) RunHandler {
	return func(nodeContext *Context) (any, error) {
		configuration := nodeContext.Configuration
		if err := validateDatabaseInput(configuration); err != nil {
			return nil, err
		}
		if dbClient == nil {
			return nil, &NodeError{
				Category: "INTERNAL",
				Code:     "DATABASE_INPUT_UNAVAILABLE",
				Message:  "Database input is not available in this runtime.",
			}
		}
		query := configString(configuration, "query")
		rows, err := dbClient.Query(nodeContext.Runtime, query)
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "DATABASE_INPUT_QUERY_FAILED",
				Message:  "The database input query failed.",
				CanRetry: isRetryableDatabaseInputError(err),
			}
		}
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "DATABASE_INPUT_READ_FAILED",
				Message:  "The database input result columns could not be read.",
				CanRetry: isRetryableDatabaseInputError(err),
			}
		}
		results := make([]map[string]any, 0)
		for rows.Next() {
			values := make([]any, len(columns))
			destinations := make([]any, len(columns))
			for index := range values {
				destinations[index] = &values[index]
			}
			if err := rows.Scan(destinations...); err != nil {
				return nil, &NodeError{
					Category: "EXECUTION",
					Code:     "DATABASE_INPUT_READ_FAILED",
					Message:  "A database input row could not be read.",
					CanRetry: false,
				}
			}
			record := make(map[string]any, len(columns))
			for index, column := range columns {
				record[column] = normalizeSQLValue(values[index])
			}
			results = append(results, record)
		}
		if err := rows.Err(); err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "DATABASE_INPUT_READ_FAILED",
				Message:  "The database input result stream failed.",
				CanRetry: isRetryableDatabaseInputError(err),
			}
		}
		return results, nil
	}
}

func isRetryableDatabaseInputError(err error) bool {
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return true
	}
	var postgresError interface{ SQLState() string }
	if !errors.As(err, &postgresError) {
		return false
	}
	sqlState := postgresError.SQLState()
	return strings.HasPrefix(sqlState, "08") ||
		sqlState == "57P01" ||
		sqlState == "57P02" ||
		sqlState == "57P03"
}

func validateInputSourceType(configuration map[string]any, codePrefix string) error {
	sourceType := resolveInputSourceType(configuration)
	if sourceType != "LOCAL" && sourceType != "SFTP" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     codePrefix + "_SOURCE_TYPE_INVALID",
			Message:  "configuration.sourceType must be LOCAL or SFTP when provided",
		}
	}
	return nil
}

func validateSFTPConfiguration(configuration map[string]any, codePrefix string) error {
	if resolveInputSourceType(configuration) != "SFTP" {
		return nil
	}
	if configString(configuration, "sftpHost") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_HOST_REQUIRED", Message: "configuration.sftpHost is required"}
	}
	port, ok := configInt(configuration, "sftpPort")
	if !ok || port < 1 || port > 65535 {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_PORT_INVALID", Message: "configuration.sftpPort must be an integer between 1 and 65535"}
	}
	if configString(configuration, "sftpUsername") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_USERNAME_REQUIRED", Message: "configuration.sftpUsername is required"}
	}
	if configString(configuration, "sftpBaseDirectory") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_BASE_DIRECTORY_REQUIRED", Message: "configuration.sftpBaseDirectory is required"}
	}
	if configString(configuration, "sftpHostKeySha256") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_HOST_KEY_REQUIRED", Message: "configuration.sftpHostKeySha256 is required"}
	}
	if configString(configuration, "sftpPasswordEncrypted") == "" {
		return &NodeError{Category: "VALIDATION", Code: codePrefix + "_SFTP_PASSWORD_REQUIRED", Message: "configuration.sftpPasswordEncrypted is required"}
	}
	return nil
}

func resolveInputSourceType(configuration map[string]any) string {
	if _, exists := configuration["sourceType"]; !exists {
		return "LOCAL"
	}
	value, ok := configuration["sourceType"].(string)
	if !ok {
		return ""
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "LOCAL"
	}
	return strings.ToUpper(trimmed)
}

func readInputContent(
	ctx context.Context,
	runtime InputNodeRuntime,
	companyID string,
	configuration map[string]any,
	fileName string,
	codePrefix string,
) ([]byte, error) {
	switch resolveInputSourceType(configuration) {
	case "LOCAL":
		targetPath, err := resolveTenantInputPath(runtime.InputDirectory, companyID, fileName)
		if err != nil {
			return nil, &NodeError{
				Category: "INTERNAL",
				Code:     codePrefix + "_READ_FAILED",
				Message:  "The input file could not be resolved for this tenant.",
			}
		}
		content, err := os.ReadFile(targetPath)
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     codePrefix + "_READ_FAILED",
				Message:  "The input file could not be read.",
			}
		}
		return content, nil
	case "SFTP":
		sftpConfig, err := resolveSFTPConfig(runtime, configuration)
		if err != nil {
			return nil, err
		}
		return readSFTPInputContent(ctx, sftpConfig, fileName, codePrefix)
	default:
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     codePrefix + "_SOURCE_TYPE_INVALID",
			Message:  "configuration.sourceType must be LOCAL or SFTP when provided",
		}
	}
}

func readSFTPInputContent(
	ctx context.Context,
	sftpConfig SFTPConfig,
	fileName string,
	codePrefix string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSFTPRuntimeConfig(sftpConfig); err != nil {
		return nil, err
	}
	remotePath := path.Join(
		strings.TrimRight(strings.TrimSpace(sftpConfig.BaseDirectory), "/"),
		fileName,
	)
	if !strings.HasPrefix(remotePath, "/") {
		remotePath = "/" + remotePath
	}

	address := net.JoinHostPort(sftpConfig.Host, fmt.Sprintf("%d", sftpConfig.Port))
	tcpConnection, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_SFTP_CONNECT_FAILED",
			Message:  "The SFTP input source could not be reached.",
			CanRetry: true,
		}
	}
	defer tcpConnection.Close()
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = tcpConnection.Close()
	})
	defer stopCancellation()

	sshConnection, channels, requests, err := ssh.NewClientConn(tcpConnection, address, &ssh.ClientConfig{
		User: sftpConfig.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(sftpConfig.Password),
		},
		HostKeyCallback: fixedHostKeySHA256(sftpConfig.HostKeySHA256),
	})
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_SFTP_CONNECT_FAILED",
			Message:  "The SFTP input source could not be reached.",
			CanRetry: true,
		}
	}
	sshClient := ssh.NewClient(sshConnection, channels, requests)
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_SFTP_CONNECT_FAILED",
			Message:  "The SFTP input source could not be opened.",
			CanRetry: true,
		}
	}
	defer sftpClient.Close()

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_READ_FAILED",
			Message:  "The SFTP input file could not be read.",
		}
	}
	defer remoteFile.Close()

	content, err := io.ReadAll(remoteFile)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_READ_FAILED",
			Message:  "The SFTP input file could not be read.",
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return content, nil
}

func resolveSFTPConfig(
	runtime InputNodeRuntime,
	configuration map[string]any,
) (SFTPConfig, error) {
	port, _ := configInt(configuration, "sftpPort")
	resolved := SFTPConfig{
		Host:          configString(configuration, "sftpHost"),
		Port:          port,
		Username:      configString(configuration, "sftpUsername"),
		BaseDirectory: configString(configuration, "sftpBaseDirectory"),
		HostKeySHA256: configString(configuration, "sftpHostKeySha256"),
	}
	encrypted := configString(configuration, "sftpPasswordEncrypted")
	if encrypted != "" {
		if runtime.Secrets == nil || !runtime.Secrets.Configured() {
			return SFTPConfig{}, &NodeError{
				Category: "INTERNAL",
				Code:     "INPUT_SFTP_SECRET_UNAVAILABLE",
				Message:  "SFTP password decryption is not configured for this runtime.",
			}
		}
		password, err := runtime.Secrets.Decrypt(encrypted)
		if err != nil {
			return SFTPConfig{}, &NodeError{
				Category: "INTERNAL",
				Code:     "INPUT_SFTP_SECRET_DECRYPT_FAILED",
				Message:  "The SFTP password could not be decrypted.",
			}
		}
		resolved.Password = password
	}
	return resolved, nil
}

func configInt(configuration map[string]any, key string) (int, bool) {
	switch value := configuration[key].(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		parsed, err := strconv.Atoi(strconv.FormatInt(value, 10))
		return parsed, err == nil
	case float64:
		return exactFloatToInt(value)
	case float32:
		return exactFloatToInt(float64(value))
	default:
		return 0, false
	}
}

func exactFloatToInt(value float64) (int, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	parsed, err := strconv.Atoi(strconv.FormatFloat(value, 'f', -1, 64))
	return parsed, err == nil
}

func validateSFTPRuntimeConfig(sftpConfig SFTPConfig) error {
	if strings.TrimSpace(sftpConfig.Host) == "" ||
		sftpConfig.Port < 1 || sftpConfig.Port > 65535 ||
		strings.TrimSpace(sftpConfig.Username) == "" ||
		sftpConfig.Password == "" ||
		strings.TrimSpace(sftpConfig.BaseDirectory) == "" ||
		strings.TrimSpace(sftpConfig.HostKeySHA256) == "" {
		return &NodeError{
			Category: "INTERNAL",
			Code:     "INPUT_SFTP_NOT_CONFIGURED",
			Message:  "SFTP input is not configured for this runtime.",
		}
	}
	return nil
}

func fixedHostKeySHA256(expectedFingerprint string) ssh.HostKeyCallback {
	expected := strings.TrimPrefix(
		strings.TrimSpace(expectedFingerprint),
		"SHA256:",
	)
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		actual := strings.TrimPrefix(
			strings.TrimSpace(ssh.FingerprintSHA256(key)),
			"SHA256:",
		)
		if actual != expected {
			return fmt.Errorf("ssh host key fingerprint mismatch")
		}
		return nil
	}
}

func configString(configuration map[string]any, key string) string {
	value, _ := configuration[key].(string)
	return strings.TrimSpace(value)
}

func isSafePathSegment(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	if strings.ContainsAny(value, `/\:`) || strings.Contains(value, "..") {
		return false
	}
	return filepath.Base(value) == value
}

func resolveTenantInputPath(inputDirectory, companyID, fileName string) (string, error) {
	companyID = strings.TrimSpace(companyID)
	if !isSafePathSegment(companyID) {
		return "", fmt.Errorf("invalid company id for input path")
	}
	return filepath.Join(inputDirectory, companyID, fileName), nil
}

func tabularRecordsFromRows(rows [][]string, invalidCode string) ([]map[string]any, error) {
	if len(rows) == 0 {
		return nil, &NodeError{
			Category: "VALIDATION",
			Code:     invalidCode,
			Message:  "Tabular input requires a header row.",
		}
	}
	headers := make([]string, len(rows[0]))
	for index, header := range rows[0] {
		headers[index] = strings.TrimSpace(header)
	}
	if err := validateTabularHeaders(headers, invalidCode); err != nil {
		return nil, err
	}
	records := make([]map[string]any, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) > len(headers) {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     invalidCode,
				Message:  "Tabular input rows must not contain more values than headers.",
			}
		}
		record := make(map[string]any, len(headers))
		for index, header := range headers {
			value := ""
			if index < len(row) {
				value = row[index]
			}
			record[header] = value
		}
		records = append(records, record)
	}
	return records, nil
}

func validateTabularHeaders(headers []string, invalidCode string) error {
	if len(headers) == 0 {
		return &NodeError{
			Category: "VALIDATION",
			Code:     invalidCode,
			Message:  "Tabular input requires at least one header.",
		}
	}
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		if header == "" {
			return &NodeError{
				Category: "VALIDATION",
				Code:     invalidCode,
				Message:  "Tabular input headers must not be blank.",
			}
		}
		if _, exists := seen[header]; exists {
			return &NodeError{
				Category: "VALIDATION",
				Code:     invalidCode,
				Message:  "Tabular input headers must be unique.",
			}
		}
		seen[header] = struct{}{}
	}
	return nil
}

var selectSQLKeyword = regexp.MustCompile(`(?i)^SELECT\b`)

var forbiddenSQLKeyword = regexp.MustCompile(
	`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|TRUNCATE|MERGE|COPY|INTO)\b`,
)

func isConservativeSelectQuery(query string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" || strings.Contains(trimmed, ";") {
		return false
	}
	if !selectSQLKeyword.MatchString(trimmed) {
		return false
	}
	return !forbiddenSQLKeyword.MatchString(trimmed)
}

func normalizeSQLValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return value
	}
}
