package selfupdate

import (
	"net/http"
	"testing"
	"time"
)

// TestDefaultClientTimeouts: DefaultHTTPClient bounds the dial, TLS and
// response headers, and not the whole request, so a large download is
// bounded by the context (0025-MADR D5). Each call returns a new client and
// a new transport, so a caller may adjust one (0031-MADR D1).
func TestDefaultClientTimeouts(t *testing.T) {
	c := DefaultHTTPClient()
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
	if tr.Proxy == nil {
		t.Fatal("the default client has no proxy function")
	}
	if tr == http.DefaultTransport {
		t.Fatal("the default client shares http.DefaultTransport")
	}
	c2 := DefaultHTTPClient()
	if c2 == c || c2.Transport == c.Transport {
		t.Fatal("two calls share a client or a transport")
	}
}
