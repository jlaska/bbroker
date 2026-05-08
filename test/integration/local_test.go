//go:build integration

// Package integration provides end-to-end tests that run against a real Chrome
// binary locally, bypassing the k8s pod creation layer.
//
// Run with: go test -tags=integration ./test/integration/ -chrome=/path/to/chrome
package integration

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

var chromePath = flag.String("chrome", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "path to Chrome binary")

// startChrome launches Chrome with a random debugging port and returns the port.
func startChrome(t *testing.T, port int) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(*chromePath,
		"--headless=new",
		"--no-sandbox",
		"--disable-gpu",
		"--disable-dev-shm-usage",
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start chrome: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill() })
	return cmd
}

// freePort finds an available TCP port.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// waitForChrome polls /json/version until Chrome is ready.
func waitForChrome(t *testing.T, port int) string {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var v struct {
			WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
		}
		if json.Unmarshal(body, &v) == nil && v.WebSocketDebuggerURL != "" {
			return v.WebSocketDebuggerURL
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("chrome did not become ready in time")
	return ""
}

// TestDefenderRelay tests the warden's WS proxy logic against a real Chrome.
func TestDefenderRelay(t *testing.T) {
	chromePort := freePort(t)
	startChrome(t, chromePort)
	cdpURL := waitForChrome(t, chromePort)
	// Rewrite localhost to 127.0.0.1 for dialing.
	cdpURL = strings.ReplaceAll(cdpURL, "localhost", "127.0.0.1")
	t.Logf("Chrome CDP: %s", cdpURL)

	// Start a simple WebSocket echo-proxy that mimics the warden:
	// Accept one connection and relay to Chrome CDP.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Logf("accept: %v", err)
			return
		}
		defer clientConn.CloseNow()

		ctx := r.Context()
		chromeConn, _, err := websocket.Dial(ctx, cdpURL, nil)
		if err != nil {
			t.Logf("dial chrome: %v", err)
			return
		}
		defer chromeConn.CloseNow()

		errCh := make(chan error, 2)
		relay := func(src, dst *websocket.Conn) {
			for {
				mt, data, err := src.Read(ctx)
				if err != nil {
					errCh <- err
					return
				}
				if err := dst.Write(ctx, mt, data); err != nil {
					errCh <- err
					return
				}
			}
		}
		go relay(clientConn, chromeConn)
		go relay(chromeConn, clientConn)
		<-errCh
	}))
	defer srv.Close()

	// Connect through the proxy and send a CDP command.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	proxyURL := "ws" + srv.URL[4:] + "/"
	conn, _, err := websocket.Dial(ctx, proxyURL, nil)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.CloseNow()

	// Send a CDP Browser.getVersion command.
	cmd := `{"id":1,"method":"Browser.getVersion","params":{}}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(cmd)); err != nil {
		t.Fatalf("write CDP: %v", err)
	}

	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read CDP response: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(msg, &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected response shape: %s", msg)
	}
	product, _ := result["product"].(string)
	if !strings.Contains(product, "Chrome") {
		t.Errorf("expected Chrome in product, got %q", product)
	}
	t.Logf("CDP relay works: product=%q", product)
}
