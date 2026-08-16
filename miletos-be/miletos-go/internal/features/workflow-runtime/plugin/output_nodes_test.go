package plugin

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOutputDestinationRegistration(t *testing.T) {
	registry := NewNodeRegistry()
	if err := RegisterOutputDestinationNodes(registry, OutputNodeRuntime{
		HTTPClient: &http.Client{}, OutputDirectory: t.TempDir(),
	}); err != nil {
		t.Fatalf("RegisterOutputDestinationNodes() error = %v", err)
	}

	for _, key := range []string{"core.rest-output", "core.database-output", "core.csv-output"} {
		registration, exists := registry.Get(key)
		if !exists {
			t.Fatalf("registration %q is missing", key)
		}
		assertSinkRegistration(t, registration)
		if registration.Validator == nil || registration.Handler == nil {
			t.Fatalf("registration %q must expose validator and handler", key)
		}
	}
}

func TestOutputDestinationRegistrationPropagatesFailure(t *testing.T) {
	registry := NewNodeRegistry()
	if err := registry.RegisterNode(restOutputRegistration(&http.Client{})); err != nil {
		t.Fatalf("seed registration error = %v", err)
	}
	if err := RegisterOutputDestinationNodes(registry, OutputNodeRuntime{}); err == nil {
		t.Fatal("RegisterOutputDestinationNodes() error = nil, want duplicate error")
	}
}

func TestRESTOutputValidation(t *testing.T) {
	tests := []struct {
		name          string
		configuration map[string]any
		wantCode      string
	}{
		{name: "missing URL", configuration: map[string]any{}, wantCode: "REST_OUTPUT_URL_REQUIRED"},
		{name: "malformed URL", configuration: map[string]any{"url": "://bad"}, wantCode: "REST_OUTPUT_URL_INVALID"},
		{name: "relative URL", configuration: map[string]any{"url": "/relative"}, wantCode: "REST_OUTPUT_URL_INVALID"},
		{name: "unsupported scheme", configuration: map[string]any{"url": "ftp://example.com"}, wantCode: "REST_OUTPUT_URL_INVALID"},
		{name: "embedded credentials", configuration: map[string]any{"url": "https://user:secret@example.com"}, wantCode: "REST_OUTPUT_URL_INVALID"},
		{name: "unsupported method", configuration: map[string]any{"url": "https://example.com", "method": "GET"}, wantCode: "REST_OUTPUT_METHOD_INVALID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertNodeErrorCode(t, validateRESTOutput(test.configuration), test.wantCode)
		})
	}
	for _, configuration := range []map[string]any{
		{"url": "https://example.com"},
		{"url": "https://example.com", "method": "POST"},
		{"url": "https://example.com", "method": "PUT"},
		{"url": "https://example.com", "method": "PATCH"},
	} {
		if err := validateRESTOutput(configuration); err != nil {
			t.Fatalf("validateRESTOutput(%#v) error = %v", configuration, err)
		}
	}
}

func TestRESTOutputMethodsAndJSONPayload(t *testing.T) {
	for _, method := range []string{"", http.MethodPost, http.MethodPut, http.MethodPatch} {
		name := method
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			var gotMethod string
			var gotContentType string
			var gotBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				gotMethod = request.Method
				gotContentType = request.Header.Get("Content-Type")
				gotBody, _ = io.ReadAll(request.Body)
				writer.WriteHeader(http.StatusCreated)
			}))
			defer server.Close()

			configuration := map[string]any{"url": server.URL}
			if method != "" {
				configuration["method"] = method
			}
			output, err := invokeOutputNode(
				t, restOutputRegistration(server.Client()), configuration,
				map[string]any{"name": "Ada"}, Infrastructure{}, context.Background(),
			)
			if err != nil {
				t.Fatalf("REST output error = %v", err)
			}
			wantMethod := method
			if wantMethod == "" {
				wantMethod = http.MethodPost
			}
			if gotMethod != wantMethod || gotContentType != "application/json" || string(gotBody) != `{"name":"Ada"}` {
				t.Fatalf("request = method %q, content type %q, body %q", gotMethod, gotContentType, gotBody)
			}
			if !reflect.DeepEqual(output, map[string]any{"statusCode": http.StatusCreated}) {
				t.Fatalf("output = %#v", output)
			}
		})
	}
}

func TestRESTOutputStatusAndTransportClassification(t *testing.T) {
	for _, test := range []struct {
		status    int
		retryable bool
	}{
		{status: http.StatusBadRequest, retryable: false},
		{status: http.StatusTooManyRequests, retryable: true},
		{status: http.StatusInternalServerError, retryable: true},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(strings.Repeat("x", 2048)))
			}))
			defer server.Close()
			_, err := invokeOutputNode(
				t, restOutputRegistration(server.Client()), map[string]any{"url": server.URL},
				map[string]any{"ok": true}, Infrastructure{}, context.Background(),
			)
			nodeError := requireNodeError(t, err)
			if nodeError.Code != "REST_OUTPUT_HTTP_STATUS" || nodeError.CanRetry != test.retryable {
				t.Fatalf("node error = %#v", nodeError)
			}
		})
	}

	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, driver.ErrBadConn
	})}
	_, err := invokeOutputNode(
		t, restOutputRegistration(client), map[string]any{"url": "https://example.com"},
		map[string]any{"ok": true}, Infrastructure{}, context.Background(),
	)
	nodeError := requireNodeError(t, err)
	if nodeError.Code != "REST_OUTPUT_TRANSPORT_FAILED" || !nodeError.CanRetry || !errors.Is(err, driver.ErrBadConn) {
		t.Fatalf("transport error = %#v", nodeError)
	}
}

func TestRESTOutputUsesExecutionContextAndClosesBody(t *testing.T) {
	type contextKey string
	const key contextKey = "request-marker"
	body := &trackingReadCloser{Reader: strings.NewReader("ok")}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Context().Value(key) != "expected" {
			t.Errorf("request context marker was not propagated")
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: body, Header: make(http.Header)}, nil
	})}
	output, err := invokeOutputNode(
		t, restOutputRegistration(client), map[string]any{"url": "https://example.com"},
		map[string]any{"ok": true}, Infrastructure{}, context.WithValue(context.Background(), key, "expected"),
	)
	if err != nil {
		t.Fatalf("REST output error = %v", err)
	}
	if !body.closed {
		t.Fatal("response body was not closed")
	}
	if !reflect.DeepEqual(output, map[string]any{"statusCode": http.StatusNoContent}) {
		t.Fatalf("output = %#v", output)
	}
}

func TestRESTOutputRejectsMissingOrUnserializablePayload(t *testing.T) {
	registration := restOutputRegistration(&http.Client{})
	configuration := map[string]any{"url": "https://example.com"}
	_, err := invokeOutputNode(t, registration, configuration, nil, Infrastructure{}, context.Background())
	assertNodeErrorCode(t, err, "REST_OUTPUT_INPUT_REQUIRED")
	_, err = invokeOutputNode(t, registration, configuration, make(chan int), Infrastructure{}, context.Background())
	assertNodeErrorCode(t, err, "REST_OUTPUT_PAYLOAD_INVALID")
}

func TestDatabaseOutputValidation(t *testing.T) {
	tests := []struct {
		configuration map[string]any
		wantCode      string
	}{
		{configuration: map[string]any{"table": "events"}, wantCode: "DATABASE_OUTPUT_SCHEMA_REQUIRED"},
		{configuration: map[string]any{"schema": "public"}, wantCode: "DATABASE_OUTPUT_TABLE_REQUIRED"},
		{configuration: map[string]any{"schema": "bad-name", "table": "events"}, wantCode: "DATABASE_OUTPUT_SCHEMA_INVALID"},
		{configuration: map[string]any{"schema": "public", "table": "events;drop"}, wantCode: "DATABASE_OUTPUT_TABLE_INVALID"},
	}
	for _, test := range tests {
		assertNodeErrorCode(t, validateDatabaseOutput(test.configuration), test.wantCode)
	}
}

func TestDatabaseOutputUsesParameterizedExternalExecCapability(t *testing.T) {
	database := &fakeDatabaseInfrastructure{result: staticSQLResult(2)}
	payload := map[string]any{"customer": "Ada", "active": true}
	output, err := invokeOutputNode(
		t, databaseOutputRegistration(), map[string]any{"schema": "public", "table": "events"},
		payload, Infrastructure{Database: database}, context.Background(),
	)
	if err != nil {
		t.Fatalf("database output error = %v", err)
	}
	if database.queryCalled || database.execCalls != 1 {
		t.Fatalf("Query called = %v, Exec calls = %d", database.queryCalled, database.execCalls)
	}
	if database.query != `INSERT INTO "public"."events" ("payload") VALUES ($1::jsonb)` {
		t.Fatalf("query = %q", database.query)
	}
	if len(database.arguments) != 1 || database.arguments[0] != `{"active":true,"customer":"Ada"}` {
		t.Fatalf("arguments = %#v", database.arguments)
	}
	want := map[string]any{
		"operation": "INSERT", "rowsAffected": int64(2), "schema": "public", "table": "events",
	}
	if !reflect.DeepEqual(output, want) {
		t.Fatalf("output = %#v, want %#v", output, want)
	}
}

func TestDatabaseOutputAcceptsJSONArrays(t *testing.T) {
	database := &fakeDatabaseInfrastructure{result: staticSQLResult(1)}
	_, err := invokeOutputNode(
		t, databaseOutputRegistration(), map[string]any{"schema": "public", "table": "events"},
		[]any{map[string]any{"id": float64(1)}}, Infrastructure{Database: database}, context.Background(),
	)
	if err != nil {
		t.Fatalf("database output array error = %v", err)
	}
	if database.arguments[0] != `[{"id":1}]` {
		t.Fatalf("payload argument = %#v", database.arguments[0])
	}
}

func TestDatabaseOutputCapabilityAndFailureClassification(t *testing.T) {
	configuration := map[string]any{"schema": "public", "table": "events"}
	payload := map[string]any{"id": 1}
	_, err := invokeOutputNode(
		t, databaseOutputRegistration(), configuration, payload, Infrastructure{}, context.Background(),
	)
	assertNodeErrorCode(t, err, "DATABASE_OUTPUT_UNAVAILABLE")

	for _, test := range []struct {
		name      string
		err       error
		retryable bool
	}{
		{name: "bad connection", err: driver.ErrBadConn, retryable: true},
		{name: "serialization failure", err: sqlStateError("40001"), retryable: true},
		{name: "constraint violation", err: sqlStateError("23505"), retryable: false},
		{name: "ordinary failure", err: errors.New("permission denied"), retryable: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			database := &fakeDatabaseInfrastructure{err: test.err}
			_, err := invokeOutputNode(
				t, databaseOutputRegistration(), configuration, payload,
				Infrastructure{Database: database}, context.Background(),
			)
			nodeError := requireNodeError(t, err)
			if nodeError.Code != "DATABASE_OUTPUT_WRITE_FAILED" || nodeError.CanRetry != test.retryable {
				t.Fatalf("node error = %#v", nodeError)
			}
		})
	}
}

func TestDatabaseOutputRejectsMissingOrUnserializablePayload(t *testing.T) {
	configuration := map[string]any{"schema": "public", "table": "events"}
	database := &fakeDatabaseInfrastructure{result: staticSQLResult(1)}
	_, err := invokeOutputNode(
		t, databaseOutputRegistration(), configuration, nil,
		Infrastructure{Database: database}, context.Background(),
	)
	assertNodeErrorCode(t, err, "DATABASE_OUTPUT_INPUT_REQUIRED")
	_, err = invokeOutputNode(
		t, databaseOutputRegistration(), configuration, make(chan int),
		Infrastructure{Database: database}, context.Background(),
	)
	assertNodeErrorCode(t, err, "DATABASE_OUTPUT_PAYLOAD_INVALID")
}

func TestCSVOutputValidation(t *testing.T) {
	for _, test := range []struct {
		configuration map[string]any
		wantCode      string
	}{
		{configuration: map[string]any{}, wantCode: "CSV_OUTPUT_FILENAME_REQUIRED"},
		{configuration: map[string]any{"fileName": "../result.csv"}, wantCode: "CSV_OUTPUT_FILENAME_INVALID"},
		{configuration: map[string]any{"fileName": `folder\result.csv`}, wantCode: "CSV_OUTPUT_FILENAME_INVALID"},
		{configuration: map[string]any{"fileName": "result.txt"}, wantCode: "CSV_OUTPUT_FILENAME_INVALID"},
	} {
		assertNodeErrorCode(t, validateCSVOutput(test.configuration), test.wantCode)
	}
}

func TestCSVOutputWritesDeterministicRowsAndNestedValues(t *testing.T) {
	directory := t.TempDir()
	payload := []any{
		map[string]any{"z": nil, "a": "first", "nested": map[string]any{"b": 2, "a": 1}},
		map[string]any{"a": "second", "extra": true, "nested": []any{1, "x"}},
	}
	output, err := invokeOutputNode(
		t, csvOutputRegistration(directory), map[string]any{"fileName": "result.csv"}, payload,
		Infrastructure{}, context.Background(),
	)
	if err != nil {
		t.Fatalf("CSV output error = %v", err)
	}
	records := readCSVRecords(t, filepath.Join(directory, "result.csv"))
	wantRecords := [][]string{
		{"a", "extra", "nested", "z"},
		{"first", "", `{"a":1,"b":2}`, ""},
		{"second", "true", `[1,"x"]`, ""},
	}
	if !reflect.DeepEqual(records, wantRecords) {
		t.Fatalf("records = %#v, want %#v", records, wantRecords)
	}
	wantOutput := map[string]any{
		"destinationType": "LOCAL", "fileName": "result.csv", "rowsWritten": 2,
	}
	if !reflect.DeepEqual(output, wantOutput) {
		t.Fatalf("output = %#v, want %#v", output, wantOutput)
	}
}

func TestCSVOutputWritesSingleObject(t *testing.T) {
	directory := t.TempDir()
	_, err := invokeOutputNode(
		t, csvOutputRegistration(directory), map[string]any{"fileName": "single.csv"},
		map[string]any{"id": 1, "name": "Ada"}, Infrastructure{}, context.Background(),
	)
	if err != nil {
		t.Fatalf("CSV output error = %v", err)
	}
	want := [][]string{{"id", "name"}, {"1", "Ada"}}
	if records := readCSVRecords(t, filepath.Join(directory, "single.csv")); !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want %#v", records, want)
	}
}

func TestCSVOutputRejectsMissingInvalidAndEmptyArrayPayloads(t *testing.T) {
	registration := csvOutputRegistration(t.TempDir())
	configuration := map[string]any{"fileName": "result.csv"}
	for _, payload := range []any{nil, 42, []any{}, []any{"not-an-object"}} {
		_, err := invokeOutputNode(
			t, registration, configuration, payload, Infrastructure{}, context.Background(),
		)
		if payload == nil {
			assertNodeErrorCode(t, err, "CSV_OUTPUT_INPUT_REQUIRED")
		} else {
			assertNodeErrorCode(t, err, "CSV_OUTPUT_PAYLOAD_INVALID")
		}
	}
}

func TestCSVOutputOverwritesExistingFile(t *testing.T) {
	directory := t.TempDir()
	registration := csvOutputRegistration(directory)
	configuration := map[string]any{"fileName": "result.csv"}
	if _, err := invokeOutputNode(
		t, registration, configuration, map[string]any{"old": "value"}, Infrastructure{}, context.Background(),
	); err != nil {
		t.Fatalf("first CSV output error = %v", err)
	}
	if _, err := invokeOutputNode(
		t, registration, configuration, map[string]any{"new": "value"}, Infrastructure{}, context.Background(),
	); err != nil {
		t.Fatalf("second CSV output error = %v", err)
	}
	want := [][]string{{"new"}, {"value"}}
	if records := readCSVRecords(t, filepath.Join(directory, "result.csv")); !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want overwrite %#v", records, want)
	}
}

func TestCSVOutputRejectsExistingSymbolicLink(t *testing.T) {
	directory := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.csv")
	if err := os.WriteFile(external, []byte("must remain unchanged"), 0o600); err != nil {
		t.Fatalf("seed external file error = %v", err)
	}
	link := filepath.Join(directory, "result.csv")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	_, err := invokeOutputNode(
		t, csvOutputRegistration(directory), map[string]any{"fileName": "result.csv"},
		map[string]any{"id": 1}, Infrastructure{}, context.Background(),
	)
	assertNodeErrorCode(t, err, "CSV_OUTPUT_FILENAME_INVALID")
	content, readErr := os.ReadFile(external)
	if readErr != nil || string(content) != "must remain unchanged" {
		t.Fatalf("external file = %q, error = %v", content, readErr)
	}
}

func TestCSVOutputReportsDirectoryFailure(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(directory, []byte("file"), 0o600); err != nil {
		t.Fatalf("seed file error = %v", err)
	}
	_, err := invokeOutputNode(
		t, csvOutputRegistration(directory), map[string]any{"fileName": "result.csv"},
		map[string]any{"id": 1}, Infrastructure{}, context.Background(),
	)
	assertNodeErrorCode(t, err, "CSV_OUTPUT_DIRECTORY_FAILED")
}

func assertSinkRegistration(t *testing.T, registration NodeRegistration) {
	t.Helper()
	if registration.InputMode != NodeInputSingle || len(registration.InputPorts) != 1 ||
		registration.InputPorts[0].Name != "input" || len(registration.OutputPorts) != 0 {
		t.Fatalf("registration %q ports = input %#v, output %#v", registration.Key, registration.InputPorts, registration.OutputPorts)
	}
	if registration.InputEdgeConstraint.Minimum != 1 || registration.InputEdgeConstraint.Maximum == nil ||
		*registration.InputEdgeConstraint.Maximum != 1 || registration.OutputEdgeConstraint.Minimum != 0 ||
		registration.OutputEdgeConstraint.Maximum == nil || *registration.OutputEdgeConstraint.Maximum != 0 {
		t.Fatalf("registration %q constraints = input %#v, output %#v", registration.Key, registration.InputEdgeConstraint, registration.OutputEdgeConstraint)
	}
}

func invokeOutputNode(
	t *testing.T,
	registration NodeRegistration,
	configuration map[string]any,
	payload any,
	infrastructure Infrastructure,
	runtime context.Context,
) (any, error) {
	t.Helper()
	lifecycles := NewLifecycles()
	nodeContext := NewContext(ContextOptions{
		Runtime: runtime, Configuration: configuration, Payload: payload,
		Lifecycles: lifecycles, Infrastructure: infrastructure,
	})
	if err := registration.Handler(nodeContext); err != nil {
		return nil, err
	}
	return lifecycles.InvokeRun()
}

func requireNodeError(t *testing.T, err error) *NodeError {
	t.Helper()
	var nodeError *NodeError
	if !errors.As(err, &nodeError) {
		t.Fatalf("error = %v, want *NodeError", err)
	}
	return nodeError
}

func assertNodeErrorCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	nodeError := requireNodeError(t, err)
	if nodeError.Code != wantCode {
		t.Fatalf("error code = %q, want %q", nodeError.Code, wantCode)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (body *trackingReadCloser) Close() error {
	body.closed = true
	return nil
}

type fakeDatabaseInfrastructure struct {
	queryCalled bool
	execCalls   int
	query       string
	arguments   []any
	result      sql.Result
	err         error
}

func (database *fakeDatabaseInfrastructure) Open(
	context.Context,
	DatabaseConnectionConfig,
) (DatabaseConnection, error) {
	return database, nil
}

func (database *fakeDatabaseInfrastructure) Query(context.Context, string, ...any) (*sql.Rows, error) {
	database.queryCalled = true
	return nil, errors.New("unexpected Query call")
}

func (database *fakeDatabaseInfrastructure) Exec(
	_ context.Context,
	query string,
	arguments ...any,
) (sql.Result, error) {
	database.execCalls++
	database.query = query
	database.arguments = append([]any(nil), arguments...)
	return database.result, database.err
}

func (database *fakeDatabaseInfrastructure) Close() error { return nil }

type staticSQLResult int64

func (staticSQLResult) LastInsertId() (int64, error) { return 0, errors.New("unsupported") }
func (result staticSQLResult) RowsAffected() (int64, error) {
	return int64(result), nil
}

type sqlStateError string

func (err sqlStateError) Error() string    { return "database error" }
func (err sqlStateError) SQLState() string { return string(err) }

func readCSVRecords(t *testing.T, path string) [][]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open CSV: %v", err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	return records
}

func TestDatabaseOutputPayloadIsJSON(t *testing.T) {
	database := &fakeDatabaseInfrastructure{result: staticSQLResult(1)}
	_, err := invokeOutputNode(
		t, databaseOutputRegistration(), map[string]any{"schema": "public", "table": "events"},
		map[string]any{"value": json.Number("1.25")}, Infrastructure{Database: database}, context.Background(),
	)
	if err != nil {
		t.Fatalf("database output error = %v", err)
	}
	if database.arguments[0] != `{"value":1.25}` {
		t.Fatalf("payload argument = %#v", database.arguments[0])
	}
}
