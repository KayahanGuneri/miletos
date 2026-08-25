package plugin

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
)

func restOutputRegistration(client *http.Client) NodeRegistration {
	return NodeRegistration{
		Key:                  "core.rest-output",
		InputMode:            NodeInputSingle,
		InputPorts:           standardInputPorts(),
		InputEdgeConstraint:  fixedEdgeConstraint(1),
		OutputEdgeConstraint: fixedEdgeConstraint(0),
		RoutingMode:          OutputRoutingBroadcast,
		Validator:            validateRESTOutput,
		Handler:              restOutputNodeHandler(client),
	}
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
	if parsed.User != nil {
		return &NodeError{
			Category: "VALIDATION",
			Code:     "REST_OUTPUT_URL_INVALID",
			Message:  "configuration.url must not contain credentials",
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

func restOutputNodeHandler(client *http.Client) NodeHandler {
	return func(nodeContext *Context) error {
		nodeContext.Lifecycles.OnRun(func() (any, error) {
			return restOutputNode(client, nodeContext)
		})
		return nil
	}
}

func restOutputNode(client *http.Client, nodeContext *Context) (any, error) {
	configuration := nodeContext.configuration
	input, err := objectPayload(
		nodeContext.Payload,
		"REST_OUTPUT_PAYLOAD_INVALID",
		"REST output requires a JSON object payload.",
	)
	if err != nil {
		return nil, err
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
		nodeContext.runtime, method, rawURL, bytes.NewReader(encoded),
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
			CanRetry: isRetryableRESTTransportError(err),
			Cause:    err,
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

func isRetryableRESTTransportError(err error) bool {
	if errors.Is(err, driver.ErrBadConn) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EHOSTUNREACH) {
		return true
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return true
	}
	var temporary interface{ Temporary() bool }
	return errors.As(err, &temporary) && temporary.Temporary()
}
