package plugin

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
)

var csvOutputLocks sync.Map

func csvOutputRegistration() NodeRegistration {
	return NodeRegistration{
		Key:                  "core.csv-output",
		InputMode:            NodeInputSingle,
		InputPorts:           standardInputPorts(),
		InputEdgeConstraint:  fixedEdgeConstraint(1),
		OutputEdgeConstraint: fixedEdgeConstraint(0),
		RoutingMode:          OutputRoutingBroadcast,
		Validator:            validateCSVOutput,
		Handler:              csvOutputNodeHandler(),
	}
}

func validateCSVOutput(configuration map[string]any) error {
	destinationType := resolveCSVOutputDestinationType(configuration)
	if destinationType != "SFTP" {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "CSV_OUTPUT_DESTINATION_TYPE_INVALID",
			Message:  "configuration.destinationType must be SFTP; LOCAL output is no longer supported",
		}
	}
	fileName := configString(configuration, "fileName")
	if fileName == "" {
		return &NodeError{
			Category: "VALIDATION", Code: "CSV_OUTPUT_FILENAME_REQUIRED",
			Message: "configuration.fileName is required",
		}
	}
	if !isSafePathSegment(fileName) || !strings.HasSuffix(strings.ToLower(fileName), ".csv") {
		return &NodeError{
			Category: "VALIDATION", Code: "CSV_OUTPUT_FILENAME_INVALID",
			Message: "configuration.fileName must be a plain .csv file name without path separators",
		}
	}
	return validateSFTPConnectionConfiguration(configuration, "CSV_OUTPUT")
}

func resolveCSVOutputDestinationType(configuration map[string]any) string {
	raw, exists := configuration["destinationType"]
	if !exists {
		return "SFTP"
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "SFTP"
	}
	return value
}

func csvOutputNodeHandler() NodeHandler {
	return func(nodeContext *Context) error {
		nodeContext.Lifecycles.OnRun(func() (any, error) {
			return csvOutputNode(nodeContext)
		})
		return nil
	}
}

func csvOutputNode(nodeContext *Context) (any, error) {
	if err := validateCSVOutput(nodeContext.configuration); err != nil {
		return nil, err
	}
	object, err := objectPayload(
		nodeContext.Payload,
		"CSV_OUTPUT_PAYLOAD_INVALID",
		"CSV output requires a JSON object payload.",
	)
	if err != nil {
		return nil, err
	}
	row, columns, err := csvRowFromObject(object)
	if err != nil {
		return nil, err
	}
	sftpConfig, err := resolveSFTPConfig(
		nodeContext.Infra.Secrets, nodeContext.configuration, "CSV_OUTPUT",
	)
	if err != nil {
		return nil, err
	}
	fileName := configString(nodeContext.configuration, "fileName")
	if err := appendSFTPCSVRecord(
		nodeContext.runtime, sftpConfig, fileName, columns, row,
	); err != nil {
		return nil, err
	}
	return map[string]any{
		"destinationType": "SFTP", "fileName": fileName, "rowsWritten": 1,
	}, nil
}

func csvRowFromObject(object map[string]any) (map[string]string, []string, error) {
	if len(object) == 0 {
		return nil, nil, &NodeError{
			Category: "VALIDATION", Code: "CSV_OUTPUT_PAYLOAD_INVALID",
			Message: "CSV output requires at least one object field.",
		}
	}
	columns := make([]string, 0, len(object))
	for column := range object {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	row := make(map[string]string, len(columns))
	for _, column := range columns {
		value := object[column]
		if value == nil {
			row[column] = ""
			continue
		}
		switch typed := value.(type) {
		case string:
			row[column] = typed
		case bool, float64, json.Number,
			int, int8, int16, int32, int64,
			uint, uint8, uint16, uint32, uint64:
			row[column] = fmt.Sprint(typed)
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				return nil, nil, &NodeError{
					Category: "VALIDATION", Code: "CSV_OUTPUT_PAYLOAD_INVALID",
					Message: "CSV output could not encode a nested cell value.",
				}
			}
			row[column] = string(encoded)
		}
	}
	return row, columns, nil
}

func appendSFTPCSVRecord(
	ctx context.Context,
	sftpConfig SFTPConfig,
	fileName string,
	columns []string,
	row map[string]string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateSFTPRuntimeConfig(sftpConfig); err != nil {
		return &NodeError{
			Category: "INTERNAL", Code: "CSV_OUTPUT_SFTP_UNAVAILABLE",
			Message: "SFTP output is not configured for this runtime.",
		}
	}
	remotePath, err := resolveSFTPRemotePath(sftpConfig.BaseDirectory, fileName)
	if err != nil {
		return &NodeError{
			Category: "VALIDATION", Code: "CSV_OUTPUT_SFTP_PATH_INVALID",
			Message: "The SFTP output path could not be resolved safely.",
		}
	}
	lockKey := strings.Join([]string{
		sftpConfig.Host, fmt.Sprint(sftpConfig.Port), sftpConfig.Username, remotePath,
	}, "\x00")
	lock, _ := csvOutputLocks.LoadOrStore(lockKey, &sync.Mutex{})
	mutex := lock.(*sync.Mutex)
	mutex.Lock()
	defer mutex.Unlock()

	client, closeClient, err := openSFTPClient(ctx, sftpConfig)
	if err != nil {
		return csvSFTPError("CSV_OUTPUT_SFTP_CONNECT_FAILED", "The SFTP output destination could not be reached.", err)
	}
	defer closeClient()
	releaseRemoteLock, err := acquireSFTPCSVLock(client, remotePath)
	if err != nil {
		return err
	}
	defer releaseRemoteLock()

	existingColumns, exists, err := readSFTPCSVHeader(client, remotePath)
	if err != nil {
		return err
	}
	if exists && !equalStrings(existingColumns, columns) {
		return &NodeError{
			Category: "VALIDATION", Code: "CSV_OUTPUT_HEADER_MISMATCH",
			Message: "The existing CSV header is not compatible with this record.",
		}
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	file, err := client.OpenFile(remotePath, flags)
	if err != nil {
		return csvSFTPError("CSV_OUTPUT_SFTP_WRITE_FAILED", "The SFTP output file could not be opened.", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	var encoded bytes.Buffer
	writer := csv.NewWriter(&encoded)
	if !exists {
		_ = writer.Write(columns)
	}
	record := make([]string, len(columns))
	for index, column := range columns {
		record[index] = row[column]
	}
	_ = writer.Write(record)
	writer.Flush()
	if err := writer.Error(); err != nil {
		return csvSFTPError("CSV_OUTPUT_WRITE_FAILED", "The CSV record could not be encoded.", err)
	}
	if _, err := io.Copy(file, &encoded); err != nil {
		return csvSFTPError("CSV_OUTPUT_SFTP_WRITE_FAILED", "The CSV record could not be appended.", err)
	}
	if err := file.Close(); err != nil {
		closed = true
		return csvSFTPError("CSV_OUTPUT_SFTP_WRITE_FAILED", "The SFTP output file could not be closed safely.", err)
	}
	closed = true
	return ctx.Err()
}

const csvOutputRemoteLockStaleAfter = 10 * time.Minute
const sftpStatusFileAlreadyExists = uint32(11)

func acquireSFTPCSVLock(client *sftp.Client, remotePath string) (func(), error) {
	lockPath := remotePath + ".miletos.lock"
	for attempt := 0; attempt < 2; attempt++ {
		lockFile, err := client.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
		if err == nil {
			if closeErr := lockFile.Close(); closeErr != nil {
				_ = client.Remove(lockPath)
				return nil, csvSFTPError(
					"CSV_OUTPUT_SFTP_LOCK_FAILED",
					"The SFTP output lock could not be established safely.",
					closeErr,
				)
			}
			return func() { _ = client.Remove(lockPath) }, nil
		}
		if !isSFTPAlreadyExists(err) {
			return nil, csvSFTPError(
				"CSV_OUTPUT_SFTP_LOCK_FAILED",
				"The SFTP output target could not be locked.",
				err,
			)
		}
		info, statErr := client.Stat(lockPath)
		if statErr != nil || time.Since(info.ModTime()) <= csvOutputRemoteLockStaleAfter {
			return nil, csvSFTPError(
				"CSV_OUTPUT_SFTP_BUSY",
				"The SFTP output target is currently being written by another execution.",
				err,
			)
		}
		if removeErr := client.Remove(lockPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, csvSFTPError(
				"CSV_OUTPUT_SFTP_LOCK_FAILED",
				"A stale SFTP output lock could not be recovered.",
				removeErr,
			)
		}
	}
	return nil, csvSFTPError(
		"CSV_OUTPUT_SFTP_BUSY",
		"The SFTP output target is currently being written by another execution.",
		os.ErrExist,
	)
}

func isSFTPAlreadyExists(err error) bool {
	if errors.Is(err, os.ErrExist) {
		return true
	}
	var statusError *sftp.StatusError
	return errors.As(err, &statusError) && statusError.Code == sftpStatusFileAlreadyExists
}

func readSFTPCSVHeader(client interface {
	Open(string) (*sftp.File, error)
	Stat(string) (os.FileInfo, error)
}, remotePath string) ([]string, bool, error) {
	info, err := client.Stat(remotePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, csvSFTPError("CSV_OUTPUT_SFTP_WRITE_FAILED", "The SFTP output file could not be inspected.", err)
	}
	if info.Size() == 0 {
		return nil, false, nil
	}
	file, err := client.Open(remotePath)
	if err != nil {
		return nil, false, csvSFTPError("CSV_OUTPUT_SFTP_WRITE_FAILED", "The existing CSV header could not be read.", err)
	}
	defer file.Close()
	header, err := csv.NewReader(file).Read()
	if err != nil {
		return nil, false, csvSFTPError("CSV_OUTPUT_SFTP_WRITE_FAILED", "The existing CSV header is invalid.", err)
	}
	return header, true, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func csvSFTPError(code string, message string, cause error) *NodeError {
	return &NodeError{
		Category: "EXECUTION", Code: code, Message: message, CanRetry: true, Cause: cause,
	}
}
