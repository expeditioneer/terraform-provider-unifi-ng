// Package client provides an HTTP client for the UniFi Integration API.
// API docs: https://beez.ly/unifi-apis/
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is a UniFi Integration API client.
type Client struct {
	apiKey     string
	baseURL    string
	siteID     string
	httpClient *http.Client
}

// New creates a new UniFi API client.
func New(apiKey, apiURL, siteID string, allowInsecure bool) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if allowInsecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(apiURL, "/"),
		siteID:  siteID,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

// SiteID returns the configured site ID.
func (c *Client) SiteID() string { return c.siteID }

// do executes an HTTP request against the UniFi API.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	url := fmt.Sprintf("%s/proxy/network/integration%s", c.baseURL, path)

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing request %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, resp.StatusCode, nil
}

// Get performs a GET request and unmarshals the response into out.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	body, _, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// Post performs a POST request and unmarshals the response into out.
func (c *Client) Post(ctx context.Context, path string, in, out any) error {
	body, _, err := c.do(ctx, http.MethodPost, path, in)
	if err != nil {
		return err
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Put performs a PUT request and unmarshals the response into out.
func (c *Client) Put(ctx context.Context, path string, in, out any) error {
	body, _, err := c.do(ctx, http.MethodPut, path, in)
	if err != nil {
		return err
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	_, _, err := c.do(ctx, http.MethodDelete, path, nil)
	return err
}
