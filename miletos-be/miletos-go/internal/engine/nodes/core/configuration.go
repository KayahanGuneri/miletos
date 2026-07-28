package core

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"miletos-go/internal/engine/plugin"
	"miletos-go/internal/features/workflow"
)

const MaximumDelay time.Duration = 24 * time.Hour
const maximumDelayText = "24h"

type staticInputConfiguration struct {
	Value json.RawMessage `json:"value"`
}
type delayConfiguration struct {
	Delay string `json:"delay"`
}
type emptyConfiguration struct{}

func validateStaticInputConfiguration(configuration workflow.JSONObject) []plugin.ConfigurationIssue {
	decoded, decodeIssue := decodeStrictConfiguration[staticInputConfiguration](configuration)
	if decodeIssue != nil {
		return []plugin.ConfigurationIssue{
			*decodeIssue}
	}
	if len(bytes.TrimSpace(decoded.Value)) == 0 {
		return []plugin.ConfigurationIssue{
			{Field: "value", Reason: "is required"}}
	}
	return nil
}
func validatePassThroughConfiguration(configuration workflow.JSONObject,
) []plugin.ConfigurationIssue {
	return validateEmptyConfiguration(configuration)
}
func validateDelayConfiguration(configuration workflow.JSONObject) []plugin.ConfigurationIssue {
	decoded, decodeIssue := decodeStrictConfiguration[delayConfiguration](configuration)
	if decodeIssue != nil {
		return []plugin.ConfigurationIssue{
			*decodeIssue}
	}
	normalizedDelay := strings.TrimSpace(decoded.Delay)
	if normalizedDelay == "" {
		return []plugin.ConfigurationIssue{
			{Field: "delay", Reason: "is required"}}
	}
	duration, err := time.ParseDuration(normalizedDelay)
	if err != nil {
		return []plugin.ConfigurationIssue{
			{Field: "delay", Reason: "must be a valid duration"}}
	}
	if duration <= 0 {
		return []plugin.ConfigurationIssue{
			{Field: "delay", Reason: "must be greater than zero"}}
	}
	if duration > MaximumDelay {
		return []plugin.ConfigurationIssue{
			{Field: "delay", Reason: "must not exceed " +
				maximumDelayText}}
	}
	return nil
}
func validateTerminalConfiguration(
	configuration workflow.JSONObject) []plugin.ConfigurationIssue {
	return validateEmptyConfiguration(
		configuration)
}
func validateEmptyConfiguration(configuration workflow.JSONObject,
) []plugin.ConfigurationIssue {
	_, decodeIssue := decodeStrictConfiguration[emptyConfiguration](
		configuration)
	if decodeIssue != nil {
		return []plugin.ConfigurationIssue{*decodeIssue}
	}
	return nil
}
func decodeStrictConfiguration[T any](
	configuration workflow.JSONObject) (T,
	*plugin.ConfigurationIssue) {
	var decoded T
	decoder := json.NewDecoder(bytes.NewReader(
		configuration.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return decoded, &plugin.ConfigurationIssue{Field: "configuration", Reason: "must match the expected object schema"}
	}
	var trailingValue json.RawMessage
	if err := decoder.Decode(&trailingValue); err != io.EOF {
		return decoded, &plugin.ConfigurationIssue{Field: "configuration", Reason: "must contain exactly one JSON object"}
	}
	return decoded, nil
}
