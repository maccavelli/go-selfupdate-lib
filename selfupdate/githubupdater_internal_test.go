package selfupdate

import (
	"net/http"
	"testing"
	"time"
)

// TestDefaultClientTimeouts: NewGitHubUpdater's default client bounds the
// dial, TLS and response headers, and not the whole request, so a large
// download is bounded by the context (0025-MADR D5).
func TestDefaultClientTimeouts(t *testing.T) {
	c := defaultHTTPClient()
	if c.Timeout != 0 {
		t.Fatalf("Client.Timeout = %v, want none", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if tr.ResponseHeaderTimeout != 30*time.Second || tr.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("ResponseHeaderTimeout %v, TLSHandshakeTimeout %v; want 30s, 10s", tr.ResponseHeaderTimeout, tr.TLSHandshakeTimeout)
	}
	if tr == http.DefaultTransport {
		t.Fatal("the default client shares http.DefaultTransport")
	}
}
