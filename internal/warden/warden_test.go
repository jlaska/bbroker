package warden

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

type noopTerminator struct{ called bool }

func (n *noopTerminator) Terminate(_ context.Context) error {
	n.called = true
	return nil
}

func TestHealthEndpoint(t *testing.T) {
	term := &noopTerminator{}
	d := New("localhost:9222", 5*time.Minute, 30*time.Minute, term)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	d.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if body == "" {
		t.Fatal("expected non-empty health response")
	}
}

func TestIdleTimeoutTriggersTerminate(t *testing.T) {
	term := &noopTerminator{}
	d := New("localhost:9222", 50*time.Millisecond, 30*time.Minute, term)
	d.connected.Store(true)
	d.lastActivity.Store(time.Now().Add(-100 * time.Millisecond).UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	d.watchTimeouts(ctx)
	if !term.called {
		t.Fatal("expected Terminate to be called on idle timeout")
	}
}

func TestDuplicateConnectionRejected(t *testing.T) {
	term := &noopTerminator{}
	d := New("localhost:9222", 5*time.Minute, 30*time.Minute, term)
	d.connected.Store(true) // simulate active session

	srv := httptest.NewServer(http.HandlerFunc(d.handleWS))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] + "/"
	ctx := context.Background()
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("expected connection to be rejected")
	}
	if resp != nil && resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}
