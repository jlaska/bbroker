package proxy

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestDiscoverCDPEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"webSocketDebuggerUrl":"ws://localhost:9222/devtools/browser/abc123"}`))
	}))
	defer srv.Close()

	// Parse host and port from test server URL (http://127.0.0.1:PORT)
	addr := strings.TrimPrefix(srv.URL, "http://")
	parts := strings.SplitN(addr, ":", 2)
	ip := parts[0]
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	got, err := DiscoverCDPEndpoint(ip, port)
	if err != nil {
		t.Fatalf("DiscoverCDPEndpoint: %v", err)
	}
	if !strings.Contains(got, "devtools/browser/abc123") {
		t.Errorf("unexpected CDP URL: %q", got)
	}
}

func TestRewriteHost(t *testing.T) {
	cases := []struct {
		in   string
		ip   string
		want string
	}{
		{"ws://localhost:9222/devtools/browser/abc", "10.0.0.5", "ws://10.0.0.5:9222/devtools/browser/abc"},
		{"ws://127.0.0.1:9222/devtools/page/xyz", "192.168.1.1", "ws://192.168.1.1:9222/devtools/page/xyz"},
	}
	for _, c := range cases {
		got := rewriteHost(c.in, c.ip)
		if got != c.want {
			t.Errorf("rewriteHost(%q, %q) = %q, want %q", c.in, c.ip, got, c.want)
		}
	}
}
