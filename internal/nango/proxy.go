package nango

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/usehivy/hivy/internal/logging"
)

// ProxyRequest makes a request through Nango's proxy to the provider's API.
// This allows making authenticated requests to the provider's API using the stored credentials.
func (c *Client) ProxyRequest(ctx context.Context, method, providerConfigKey, connectionID, path string, queryParams map[string]string, body any) (map[string]any, error) {
	return c.ProxyRequestWithHeaders(ctx, method, providerConfigKey, connectionID, path, queryParams, body, nil)
}

// ProxyRequestWithHeaders makes a request through Nango's proxy to the provider's API with custom headers.
// This allows making authenticated requests with provider-specific headers (e.g., Notion-Version).
func (c *Client) ProxyRequestWithHeaders(ctx context.Context, method, providerConfigKey, connectionID, path string, queryParams map[string]string, body any, headers map[string]string) (map[string]any, error) {
	var bodyReader io.Reader

	isEmptyBody := body == nil
	if !isEmptyBody {
		v := reflect.ValueOf(body)
		switch v.Kind() {
		case reflect.Ptr, reflect.Slice, reflect.Map:
			isEmptyBody = v.IsNil() || v.Len() == 0
		default:
			isEmptyBody = v.IsZero()
		}
	}

	if !isEmptyBody {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling proxy request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	query := ""
	if len(queryParams) > 0 {
		q := make([]string, 0, len(queryParams))
		for k, v := range queryParams {
			q = append(q, fmt.Sprintf("%s=%s", k, v))
		}
		query = "?" + strings.Join(q, "&")
	}

	fullURL := c.endpoint + "/proxy" + path + query

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return nil, err
	}

	if !isEmptyBody {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Provider-Config-Key", providerConfigKey)
	req.Header.Set("Connection-Id", connectionID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("nango proxy %s %s: %w", method, path, err))
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("nango proxy read %s %s: %w", method, path, err))
		return nil, err
	}

	if resp.StatusCode >= 400 {
		err := fmt.Errorf("nango proxy error %d: %s", resp.StatusCode, string(respBody))
		logging.Capture(ctx, fmt.Errorf("nango proxy %s %s: %w", method, path, err))
		return nil, err
	}

	if len(respBody) == 0 {
		return nil, nil
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return map[string]any{"_raw": string(respBody)}, nil
	}

	return result, nil
}

// RawProxyResponse holds the raw HTTP response from a Nango proxy request.
type RawProxyResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// RawProxyRequest makes a request through Nango's proxy and returns the raw response
// without parsing JSON or treating 4xx as errors. Only transport failures return an error.
func (c *Client) RawProxyRequest(ctx context.Context, method, providerConfigKey, connectionID, path, rawQuery string, body io.Reader, contentType string) (*RawProxyResponse, error) {
	headers := map[string]string{}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	return c.RawProxyRequestWithHeaders(ctx, method, providerConfigKey, connectionID, path, rawQuery, body, headers)
}

// RawProxyRequestWithHeaders makes a request through Nango's proxy and returns
// the raw response while allowing selected provider-specific headers.
func (c *Client) RawProxyRequestWithHeaders(ctx context.Context, method, providerConfigKey, connectionID, path, rawQuery string, body io.Reader, headers map[string]string) (*RawProxyResponse, error) {
	fullURL := c.endpoint + "/proxy" + path
	if rawQuery != "" {
		fullURL += "?" + rawQuery
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}

	for key, val := range headers {
		if key == "" || val == "" {
			continue
		}
		req.Header.Set(key, val)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Provider-Config-Key", providerConfigKey)
	req.Header.Set("Connection-Id", connectionID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &RawProxyResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       respBody,
	}, nil
}
