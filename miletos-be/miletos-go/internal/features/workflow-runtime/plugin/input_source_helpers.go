package plugin

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
	content, exists, err := readInputContentIfExists(
		ctx, runtime, companyID, configuration, fileName, codePrefix,
	)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, &NodeError{
			Category: "EXECUTION",
			Code:     codePrefix + "_READ_FAILED",
			Message:  "The input file could not be read.",
			Cause:    fs.ErrNotExist,
		}
	}
	return content, nil
}

func readInputContentIfExists(
	ctx context.Context,
	runtime InputNodeRuntime,
	companyID string,
	configuration map[string]any,
	fileName string,
	codePrefix string,
) ([]byte, bool, error) {
	switch resolveInputSourceType(configuration) {
	case "LOCAL":
		targetPath, err := resolveTenantInputPath(runtime.InputDirectory, companyID, fileName)
		if err != nil {
			return nil, false, &NodeError{
				Category: "INTERNAL",
				Code:     codePrefix + "_READ_FAILED",
				Message:  "The input file could not be resolved for this tenant.",
			}
		}
		content, err := os.ReadFile(targetPath)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, &NodeError{
				Category: "EXECUTION",
				Code:     codePrefix + "_READ_FAILED",
				Message:  "The input file could not be read.",
				Cause:    err,
			}
		}
		return content, true, nil
	case "SFTP":
		sftpConfig, err := resolveSFTPConfig(runtime.Secrets, configuration, codePrefix)
		if err != nil {
			return nil, false, err
		}
		content, err := readSFTPInputContent(ctx, sftpConfig, fileName, codePrefix)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return content, err == nil, err
	default:
		return nil, false, &NodeError{
			Category: "VALIDATION",
			Code:     codePrefix + "_SOURCE_TYPE_INVALID",
			Message:  "configuration.sourceType must be LOCAL or SFTP when provided",
		}
	}
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

func inputRuntimeForNode(runtime InputNodeRuntime, nodeContext *Context) InputNodeRuntime {
	runtime.Secrets = nodeContext.Infra.Secrets
	return runtime
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
