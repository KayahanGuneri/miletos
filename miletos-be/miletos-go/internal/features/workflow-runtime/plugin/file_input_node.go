package plugin

import (
	"context"
	"encoding/csv"
	"strings"
)

func fileInputRegistration(runtime InputNodeRuntime) NodeRegistration {
	return NodeRegistration{
		Key:                     "core.file-input",
		InputMode:               NodeInputSingle,
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(0),
		OutputEdgeConstraint:    EdgeConstraint{},
		RoutingMode:             OutputRoutingBroadcast,
		Validator:               validateFileInput,
		Handler:                 fileInputNodeHandler(runtime),
		AllowedExecutionSources: []string{"MANUAL_DIRECT", ExecutionSourceDataArrival},
		RecordSource:            &RecordSource{Materialize: fileInputRecordMaterializer(runtime)},
		DataArrivalSource: fileArrivalSource(
			runtime,
			validateFileInput,
			func(content []byte, configuration map[string]any) ([]map[string]any, error) {
				fileName := configString(configuration, "fileName")
				if strings.HasSuffix(strings.ToLower(fileName), ".txt") {
					return []map[string]any{{"content": string(content)}}, nil
				}
				return csvRecordsFromContent(string(content))
			},
			"FILE_INPUT",
		),
	}
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

func fileInputNodeHandler(runtime InputNodeRuntime) NodeHandler {
	return func(nodeContext *Context) error {
		nodeContext.Lifecycles.OnRun(func() (any, error) {
			configuration := nodeContext.configuration
			if err := validateFileInput(configuration); err != nil {
				return nil, err
			}
			if payload, arrived := dataArrivalPayload(nodeContext.Payload); arrived {
				return requireSourceObject(payload, "FILE_INPUT_ARRIVAL_INVALID")
			}
			return nil, sourceDispatchRequired("FILE_INPUT")
		})
		return nil
	}
}

func fileInputRecordMaterializer(runtime InputNodeRuntime) RecordMaterializer {
	return func(
		ctx context.Context,
		companyID string,
		configuration map[string]any,
	) ([]map[string]any, error) {
		if err := validateFileInput(configuration); err != nil {
			return nil, err
		}
		fileName := configString(configuration, "fileName")
		content, err := readInputContent(
			ctx, runtime, companyID, configuration, fileName, "FILE_INPUT",
		)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(strings.ToLower(fileName), ".txt") {
			return []map[string]any{{"content": string(content)}}, nil
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
