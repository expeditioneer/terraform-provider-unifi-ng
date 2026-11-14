package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newPinnedClient(t *testing.T, srv *httptest.Server, pin string) *Client {
	t.Helper()
	c, err := New("key", srv.URL, "site", false, pin)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPinnedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	sum := sha256.Sum256(srv.Certificate().Raw)
	colons := strings.ToUpper(hex.EncodeToString(sum[:]))
	var parts []string
	for i := 0; i < len(colons); i += 2 {
		parts = append(parts, colons[i:i+2])
	}

	var out map[string]any
	if err := newPinnedClient(t, srv, strings.Join(parts, ":")).Get(context.Background(), "/v1/sites", &out); err != nil {
		t.Errorf("matching pin rejected: %v", err)
	}

	wrong := strings.Repeat("00", sha256.Size)
	err := newPinnedClient(t, srv, wrong).Get(context.Background(), "/v1/sites", &out)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("wrong pin accepted or unexpected error: %v", err)
	}
}

func TestPinConflictsWithAllowInsecure(t *testing.T) {
	if _, err := New("key", "https://unifi", "site", true, strings.Repeat("00", sha256.Size)); err == nil {
		t.Error("expected an error")
	}
}

func TestInvalidPin(t *testing.T) {
	if _, err := New("key", "https://unifi", "site", false, "abc"); err == nil {
		t.Error("expected an error")
	}
}

func TestDecodeLegacy(t *testing.T) {
	var out []map[string]any
	if err := decodeLegacy([]byte(`{"meta":{"rc":"ok"},"data":[{"name":"ap"}]}`), &out); err != nil || len(out) != 1 {
		t.Errorf("got %v, %v", out, err)
	}
	if err := decodeLegacy([]byte(`{"meta":{"rc":"error","msg":"api.err.NotFound"},"data":[]}`), &out); err == nil {
		t.Error("expected an error for rc=error")
	}
}

func TestIsNotFound(t *testing.T) {
	// Bodies as returned by UniFi Network for an unknown network ID and an unknown device MAC.
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"integration", http.StatusNotFound, `{"statusCode":404,"statusName":"NOT_FOUND","code":"api.resource-not-found","message":"Requested resource does not exist"}`, true},
		{"legacy device", http.StatusBadRequest, `{"meta":{"rc":"error","msg":"api.err.UnknownDevice"},"data":[]}`, true},
		{"wrong path", http.StatusNotFound, `<html>Not Found</html>`, false},
		{"other error", http.StatusBadRequest, `{"statusCode":400,"code":"api.request.invalid"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			c, err := New("key", srv.URL, "site", false, "")
			if err != nil {
				t.Fatal(err)
			}
			err = c.Get(context.Background(), "/v1/sites/site/networks/x", &map[string]any{})
			if got := IsNotFound(err); got != tt.want {
				t.Errorf("IsNotFound(%v) = %v, want %v", err, got, tt.want)
			}
		})
	}
}
