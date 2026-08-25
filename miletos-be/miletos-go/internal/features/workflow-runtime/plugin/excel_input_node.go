package plugin

import (
	"bytes"
	"context"
	"slices"
	"strings"

	"github.com/xuri/excelize/v2"
)

func excelInputRegistration(runtime InputNodeRuntime) NodeRegistration {
	return NodeRegistration{
		Key:                     "core.excel-input",
		InputMode:               NodeInputSingle,
		OutputPorts:             standardOutputPorts(),
		InputEdgeConstraint:     fixedEdgeConstraint(0),
		OutputEdgeConstraint:    EdgeConstraint{},
		RoutingMode:             OutputRoutingBroadcast,
		Validator:               validateExcelInput,
		Handler:                 excelInputNodeHandler(runtime),
		AllowedExecutionSources: []string{"MANUAL_DIRECT", ExecutionSourceDataArrival},
		RecordSource:            &RecordSource{Materialize: excelInputRecordMaterializer(runtime)},
		DataArrivalSource: fileArrivalSource(
			runtime,
			validateExcelInput,
			func(content []byte, configuration map[string]any) ([]map[string]any, error) {
				return excelRecordsFromContent(content, configString(configuration, "sheetName"))
			},
			"EXCEL_INPUT",
		),
	}
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

func excelInputNodeHandler(runtime InputNodeRuntime) NodeHandler {
	return func(nodeContext *Context) error {
		nodeContext.Lifecycles.OnRun(func() (any, error) {
			configuration := nodeContext.configuration
			if err := validateExcelInput(configuration); err != nil {
				return nil, err
			}
			if payload, arrived := dataArrivalPayload(nodeContext.Payload); arrived {
				return requireSourceObject(payload, "EXCEL_INPUT_ARRIVAL_INVALID")
			}
			return nil, sourceDispatchRequired("EXCEL_INPUT")
		})
		return nil
	}
}

func excelInputRecordMaterializer(runtime InputNodeRuntime) RecordMaterializer {
	return func(
		ctx context.Context,
		companyID string,
		configuration map[string]any,
	) ([]map[string]any, error) {
		if err := validateExcelInput(configuration); err != nil {
			return nil, err
		}
		content, err := readInputContent(
			ctx,
			runtime,
			companyID,
			configuration,
			configString(configuration, "fileName"),
			"EXCEL_INPUT",
		)
		if err != nil {
			return nil, err
		}
		return excelRecordsFromContent(content, configString(configuration, "sheetName"))
	}
}

func excelRecordsFromContent(content []byte, sheetName string) ([]map[string]any, error) {
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
	records, err := tabularRecordsFromRows(rows, "EXCEL_INPUT_DATA_INVALID")
	if err != nil {
		return nil, err
	}
	return records, nil
}
