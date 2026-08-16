package plugin

import "strings"

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
