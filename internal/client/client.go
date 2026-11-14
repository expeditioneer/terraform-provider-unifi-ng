// Package client provides an HTTP client for the UniFi Integration API.
// API docs: https://beez.ly/unifi-apis/
package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// Client is a UniFi Integration API client.
type Client struct {
	apiKey     string
	baseURL    string
	siteID     string
	httpClient *http.Client

	legacyMu   sync.Mutex
	legacySite string
}

// New creates a new UniFi API client. A non-empty certSHA256 pins the controller certificate
// instead of verifying its chain and hostname.
func New(apiKey, apiURL, siteID string, allowInsecure bool, certSHA256 string) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	switch {
	case certSHA256 != "" && allowInsecure:
		return nil, fmt.Errorf("allow_insecure and tls_certificate_sha256 are mutually exclusive")
	case certSHA256 != "":
		pin, err := parseFingerprint(certSHA256)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = &tls.Config{
			// Self-signed controller certificates do not name the controller host; the pin replaces both checks.
			InsecureSkipVerify: true, //nolint:gosec
			VerifyConnection:   verifyPin(pin),
		}
	case allowInsecure:
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(apiURL, "/"),
		siteID:  siteID,
		httpClient: &http.Client{
			Transport: transport,
		},
	}, nil
}

// parseFingerprint accepts hex with or without colons, as printed by openssl x509 -fingerprint -sha256.
func parseFingerprint(s string) ([]byte, error) {
	pin, err := hex.DecodeString(strings.ReplaceAll(s, ":", ""))
	if err != nil || len(pin) != sha256.Size {
		return nil, fmt.Errorf("tls_certificate_sha256 must be a SHA-256 fingerprint in hex")
	}
	return pin, nil
}

// verifyPin uses VerifyConnection, which unlike VerifyPeerCertificate also runs on resumed sessions.
func verifyPin(pin []byte) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return fmt.Errorf("controller presented no certificate")
		}
		got := sha256.Sum256(cs.PeerCertificates[0].Raw)
		if !bytes.Equal(got[:], pin) {
			return fmt.Errorf("controller certificate SHA-256 %X does not match tls_certificate_sha256", got)
		}
		return nil
	}
}

// SiteID returns the configured site ID.
func (c *Client) SiteID() string { return c.siteID }

// do executes an HTTP request against the UniFi Integration API.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	return c.request(ctx, method, "/proxy/network/integration"+path, body)
}

func (c *Client) request(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	url := c.baseURL + path

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
		return nil, resp.StatusCode, &APIError{StatusCode: resp.StatusCode, Code: errorCode(respBody), Body: string(respBody)}
	}

	return respBody, resp.StatusCode, nil
}

// APIError is an error response of the Integration or the legacy API.
type APIError struct {
	StatusCode int
	// Integration "code" or legacy "meta.msg", e.g. "api.resource-not-found".
	Code string
	Body string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Body)
}

func errorCode(body []byte) string {
	var e struct {
		Code string `json:"code"`
		Meta struct {
			Msg string `json:"msg"`
		} `json:"meta"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	if e.Code != "" {
		return e.Code
	}
	return e.Meta.Msg
}

// IsNotFound reports whether the controller says the object does not exist. It checks the
// error code, not the status alone, because a wrong api_url also yields 404.
func IsNotFound(err error) bool {
	var e *APIError
	if !errors.As(err, &e) {
		return false
	}
	switch e.Code {
	case "api.err.UnknownDevice", "api.resource-not-found":
		return true
	}
	return false
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
