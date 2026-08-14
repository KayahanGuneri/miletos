package plugin

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"miletos-go/internal/shared/database"
)

// OutputNodeRuntime carries the minimal runtime dependencies destination sink
// plugins need. Workflow JSON only stores safe destination metadata.
// OutputDirectory is owned and validated by the runtime configuration.
type OutputNodeRuntime struct {
	Database        *database.Client
	HTTPClient      *http.Client
	OutputDirectory string
}

func RegisterOutputDestinationNodes(registry *NodeRegistry, runtime OutputNodeRuntime) error {
	if runtime.HTTPClient == nil {
		runtime.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	registrations := []NodeRegistration{
		restOutputRegistration(runtime.HTTPClient),
		databaseOutputRegistration(runtime.Database),
		csvOutputRegistration(runtime.OutputDirectory),
	}
	for _, registration := range registrations {
		if err := registry.RegisterNode(registration); err != nil {
			return err
		}
	}
	return nil
}

func destinationOutputRegistration(spec builtinRegistrationSpec) NodeRegistration {
	registration := builtinRegistration(spec)
	// Destination sinks may finish workflows started from any execution origin.
	registration.AllowedExecutionSources = nil
	return registration
}

func restOutputRegistration(client *http.Client) NodeRegistration {
	return destinationOutputRegistration(builtinRegistrationSpec{
		Type:        "core.rest-output",
		DisplayName: "REST Output",
		Description: "Sends the incoming payload to an external HTTP endpoint as JSON",
		Input:       true,
		Output:      false,
		OnRun:       restOutputHandler(client),
		Validator:   validateRESTOutput,
	})
}

func databaseOutputRegistration(dbClient *database.Client) NodeRegistration {
	return destinationOutputRegistration(builtinRegistrationSpec{
		Type:        "core.database-output",
		DisplayName: "Database Output",
		Description: "Inserts the incoming payload into a configured PostgreSQL table as JSONB",
		Input:       true,
		Output:      false,
		OnRun:       databaseOutputHandler(dbClient),
		Validator:   validateDatabaseOutput,
	})
}

func csvOutputRegistration(outputDirectory string) NodeRegistration {
	return destinationOutputRegistration(builtinRegistrationSpec{
		Type:        "core.csv-output",
		DisplayName: "CSV Output",
		Description: "Exports the incoming payload as a CSV file under the runtime output directory",
		Input:       true,
		Output:      false,
		OnRun:       csvOutputHandler(outputDirectory),
		Validator:   validateCSVOutput,
	})
}

func validateRESTOutput(configuration map[string]any) error {
	rawURL, _ := configuration["url"].(string)
	if strings.TrimSpace(rawURL) == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "REST_OUTPUT_URL_REQUIRED",
			Message:  "configuration.url is required",
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "REST_OUTPUT_URL_INVALID",
			Message:  "configuration.url must be an absolute http or https URL with a host",
		}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "REST_OUTPUT_URL_INVALID",
			Message:  "configuration.url must use http or https",
		}
	}
	method, _ := configuration["method"].(string)
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodPost
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return nil
	default:
		return &NodeError{
			Category: "VALIDATION",
			Code:     "REST_OUTPUT_METHOD_INVALID",
			Message:  "configuration.method must be POST, PUT, or PATCH",
		}
	}
}

func restOutputHandler(client *http.Client) RunHandler {
	return func(nodeContext *Context) (any, error) {
		configuration := nodeContext.Configuration
		input := nodeContext.Payload
		if input == nil {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     "REST_OUTPUT_INPUT_REQUIRED",
				Message:  "REST output requires an incoming payload.",
			}
		}
		if err := validateRESTOutput(configuration); err != nil {
			return nil, err
		}
		rawURL := strings.TrimSpace(configuration["url"].(string))
		method, _ := configuration["method"].(string)
		method = strings.ToUpper(strings.TrimSpace(method))
		if method == "" {
			method = http.MethodPost
		}
		encoded, err := json.Marshal(input)
		if err != nil {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     "REST_OUTPUT_PAYLOAD_INVALID",
				Message:  "Incoming payload could not be serialized as JSON.",
			}
		}
		request, err := http.NewRequestWithContext(
			nodeContext.Runtime, method, rawURL, bytes.NewReader(encoded),
		)
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "REST_OUTPUT_REQUEST_FAILED",
				Message:  "The REST output request could not be created.",
			}
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "REST_OUTPUT_TRANSPORT_FAILED",
				Message:  "The REST output request failed.",
				CanRetry: true,
			}
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		if response.StatusCode < 200 || response.StatusCode > 299 {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "REST_OUTPUT_HTTP_STATUS",
				Message:  fmt.Sprintf("REST output received HTTP status %d.", response.StatusCode),
				CanRetry: response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests,
			}
		}
		return map[string]any{"statusCode": response.StatusCode}, nil
	}
}

var sqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateDatabaseOutput(configuration map[string]any) error {
	schema, _ := configuration["schema"].(string)
	table, _ := configuration["table"].(string)
	schema = strings.TrimSpace(schema)
	table = strings.TrimSpace(table)
	if schema == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_SCHEMA_REQUIRED",
			Message:  "configuration.schema is required",
		}
	}
	if table == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_TABLE_REQUIRED",
			Message:  "configuration.table is required",
		}
	}
	if !sqlIdentifierPattern.MatchString(schema) {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_SCHEMA_INVALID",
			Message:  "configuration.schema must be a simple PostgreSQL identifier",
		}
	}
	if !sqlIdentifierPattern.MatchString(table) {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "DATABASE_OUTPUT_TABLE_INVALID",
			Message:  "configuration.table must be a simple PostgreSQL identifier",
		}
	}
	return nil
}

func databaseOutputHandler(dbClient *database.Client) RunHandler {
	return func(nodeContext *Context) (any, error) {
		configuration := nodeContext.Configuration
		input := nodeContext.Payload
		if input == nil {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     "DATABASE_OUTPUT_INPUT_REQUIRED",
				Message:  "Database output requires an incoming payload.",
			}
		}
		if err := validateDatabaseOutput(configuration); err != nil {
			return nil, err
		}
		if dbClient == nil {
			return nil, &NodeError{
				Category: "INTERNAL",
				Code:     "DATABASE_OUTPUT_UNAVAILABLE",
				Message:  "Database output is not available in this runtime.",
			}
		}
		schema := strings.TrimSpace(configuration["schema"].(string))
		table := strings.TrimSpace(configuration["table"].(string))
		encoded, err := json.Marshal(input)
		if err != nil {
			return nil, &NodeError{
				Category: "VALIDATION",
				Code:     "DATABASE_OUTPUT_PAYLOAD_INVALID",
				Message:  "Incoming payload could not be serialized as JSON.",
			}
		}
		query := fmt.Sprintf(
			`INSERT INTO %s.%s (payload) VALUES ($1::jsonb)`,
			quotePostgreSQLIdentifier(schema),
			quotePostgreSQLIdentifier(table),
		)
		result, err := dbClient.Exec(nodeContext.Runtime, query, string(encoded))
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "DATABASE_OUTPUT_INSERT_FAILED",
				Message:  "The database output insert failed.",
				CanRetry: true,
			}
		}
		return map[string]any{
			"rowsAffected": result.RowsAffected(),
			"schema":       schema,
			"table":        table,
		}, nil
	}
}

func quotePostgreSQLIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func validateCSVOutput(configuration map[string]any) error {
	fileName, _ := configuration["fileName"].(string)
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "CSV_OUTPUT_FILENAME_REQUIRED",
			Message:  "configuration.fileName is required",
		}
	}
	if strings.ContainsAny(fileName, `/\:`) || strings.Contains(fileName, "..") {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "CSV_OUTPUT_FILENAME_INVALID",
			Message:  "configuration.fileName must be a plain .csv file name without path separators",
		}
	}
	if filepath.Base(fileName) != fileName {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "CSV_OUTPUT_FILENAME_INVALID",
			Message:  "configuration.fileName must be a plain .csv file name without path separators",
		}
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".csv") {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "CSV_OUTPUT_FILENAME_INVALID",
			Message:  "configuration.fileName must end with .csv",
		}
	}
	return nil
}

func csvOutputHandler(outputDirectory string) RunHandler {
	return func(nodeContext *Context) (any, error) {
		configuration := nodeContext.Configuration
		input := nodeContext.Payload
		if err := validateCSVOutput(configuration); err != nil {
			return nil, err
		}
		fileName := strings.TrimSpace(configuration["fileName"].(string))
		rows, columns, err := csvRowsFromInput(input)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "CSV_OUTPUT_DIRECTORY_FAILED",
				Message:  "The CSV output directory could not be prepared.",
			}
		}
		targetPath := filepath.Join(outputDirectory, fileName)
		file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "CSV_OUTPUT_OPEN_FAILED",
				Message:  "The CSV output file could not be opened.",
			}
		}
		defer file.Close()
		writer := csv.NewWriter(file)
		if err := writer.Write(columns); err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "CSV_OUTPUT_WRITE_FAILED",
				Message:  "The CSV header could not be written.",
			}
		}
		for _, row := range rows {
			record := make([]string, len(columns))
			for index, column := range columns {
				record[index] = row[column]
			}
			if err := writer.Write(record); err != nil {
				return nil, &NodeError{
					Category: "EXECUTION",
					Code:     "CSV_OUTPUT_WRITE_FAILED",
					Message:  "A CSV row could not be written.",
				}
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return nil, &NodeError{
				Category: "EXECUTION",
				Code:     "CSV_OUTPUT_WRITE_FAILED",
				Message:  "The CSV output could not be flushed.",
			}
		}
		return map[string]any{
			"fileName":    fileName,
			"rowsWritten": len(rows),
		}, nil
	}
}

// normalizeCSVPayload wraps scalar payloads (string/number/bool/null) as a
// single-column object so they reuse the existing object CSV path.
func normalizeCSVPayload(input any) any {
	switch input.(type) {
	case map[string]any, []any:
		return input
	default:
		return map[string]any{"value": input}
	}
}

func csvRowsFromInput(input any) ([]map[string]string, []string, error) {
	input = normalizeCSVPayload(input)
	objects := make([]map[string]any, 0)
	switch typed := input.(type) {
	case map[string]any:
		objects = append(objects, typed)
	case []any:
		if len(typed) == 0 {
			return nil, nil, &NodeError{
				Category: "VALIDATION",
				Code:     "CSV_OUTPUT_PAYLOAD_INVALID",
				Message:  "CSV output array payload must contain at least one object.",
			}
		}
		for _, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, nil, &NodeError{
					Category: "VALIDATION",
					Code:     "CSV_OUTPUT_PAYLOAD_INVALID",
					Message:  "CSV output array items must be JSON objects.",
				}
			}
			objects = append(objects, object)
		}
	default:
		return nil, nil, &NodeError{
			Category: "VALIDATION",
			Code:     "CSV_OUTPUT_PAYLOAD_INVALID",
			Message:  "CSV output requires a JSON object or an array of JSON objects.",
		}
	}
	columnSet := make(map[string]struct{})
	for _, object := range objects {
		for key := range object {
			columnSet[key] = struct{}{}
		}
	}
	columns := make([]string, 0, len(columnSet))
	for column := range columnSet {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	rows := make([]map[string]string, 0, len(objects))
	for _, object := range objects {
		row := make(map[string]string, len(columns))
		for _, column := range columns {
			value, exists := object[column]
			if !exists || value == nil {
				row[column] = ""
				continue
			}
			switch typed := value.(type) {
			case string:
				row[column] = typed
			case bool, float64, json.Number:
				row[column] = fmt.Sprint(typed)
			default:
				encoded, err := json.Marshal(typed)
				if err != nil {
					return nil, nil, &NodeError{
						Category: "VALIDATION",
						Code:     "CSV_OUTPUT_PAYLOAD_INVALID",
						Message:  "CSV output could not encode a nested cell value.",
					}
				}
				row[column] = string(encoded)
			}
		}
		rows = append(rows, row)
	}
	return rows, columns, nil
}
