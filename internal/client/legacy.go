package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// The legacy controller API is undocumented. Use it only for settings the Integration API
// cannot write yet, and move each call to the Integration API once it can.

type legacyEnvelope struct {
	Meta struct {
		RC  string `json:"rc"`
		Msg string `json:"msg"`
	} `json:"meta"`
	Data json.RawMessage `json:"data"`
}

// LegacyGet performs a GET on a site-scoped legacy path such as "stat/device/<mac>".
func (c *Client) LegacyGet(ctx context.Context, path string, out any) error {
	return c.legacy(ctx, http.MethodGet, path, nil, out)
}

// LegacyPut performs a PUT on a site-scoped legacy path such as "rest/device/<id>".
func (c *Client) LegacyPut(ctx context.Context, path string, in, out any) error {
	return c.legacy(ctx, http.MethodPut, path, in, out)
}

func (c *Client) legacy(ctx context.Context, method, path string, in, out any) error {
	site, err := c.legacySiteName(ctx)
	if err != nil {
		return err
	}
	body, _, err := c.request(ctx, method, fmt.Sprintf("/proxy/network/api/s/%s/%s", site, path), in)
	if err != nil {
		return err
	}
	return decodeLegacy(body, out)
}

func decodeLegacy(body []byte, out any) error {
	var env legacyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("decoding legacy response: %w", err)
	}
	if env.Meta.RC != "ok" {
		return &APIError{StatusCode: http.StatusOK, Code: env.Meta.Msg, Body: string(body)}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// legacySiteName maps the Integration site ID to the legacy site name, e.g. "default".
func (c *Client) legacySiteName(ctx context.Context) (string, error) {
	c.legacyMu.Lock()
	defer c.legacyMu.Unlock()
	if c.legacySite != "" {
		return c.legacySite, nil
	}
	var sites struct {
		Data []struct {
			ID                string `json:"id"`
			InternalReference string `json:"internalReference"`
		} `json:"data"`
	}
	if err := c.Get(ctx, "/v1/sites?limit=200", &sites); err != nil {
		return "", fmt.Errorf("reading sites: %w", err)
	}
	for _, s := range sites.Data {
		if s.ID == c.siteID {
			c.legacySite = s.InternalReference
			return s.InternalReference, nil
		}
	}
	return "", fmt.Errorf("site %s not found", c.siteID)
}
