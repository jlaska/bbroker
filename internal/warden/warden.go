package warden

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

// Defender manages a single browser session within a pod.
// It accepts exactly one WebSocket connection, proxies CDP frames to Chrome,
// and self-terminates the pod on idle or session timeout.
type Defender struct {
	cdpAddr          string // e.g. "localhost:9222"
	idleTimeout      time.Duration
	sessionTimeout   time.Duration
	connected        atomic.Bool
	lastActivity     atomic.Int64 // unix nano
	sessionStart     time.Time
	terminator       Terminator
}

// Terminator deletes the current pod (implemented differently in k8s vs test).
type Terminator interface {
	Terminate(ctx context.Context) error
}

func New(cdpAddr string, idleTimeout, sessionTimeout time.Duration, term Terminator) *Defender {
	d := &Defender{
		cdpAddr:        cdpAddr,
		idleTimeout:    idleTimeout,
		sessionTimeout: sessionTimeout,
		terminator:     term,
	}
	d.lastActivity.Store(time.Now().UnixNano())
	return d
}

// Run starts the HTTP server with WebSocket and health endpoints.
func (d *Defender) Run(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.handleWS)
	mux.HandleFunc("/health", d.handleHealth)

	srv := &http.Server{Addr: addr, Handler: mux}

	go d.watchTimeouts(ctx)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	slog.Info("warden listening", "addr", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (d *Defender) handleWS(w http.ResponseWriter, r *http.Request) {
	if d.connected.Swap(true) {
		http.Error(w, "session already active", http.StatusConflict)
		slog.Warn("rejected duplicate connection")
		return
	}
	defer d.connected.Store(false)
	d.sessionStart = time.Now()

	clientConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("ws accept", "err", err)
		return
	}
	defer clientConn.CloseNow()

	ctx := r.Context()
	slog.Info("client connected, dialing chrome CDP", "addr", d.cdpAddr)

	chromeConn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s/json", d.cdpAddr), nil)
	if err != nil {
		slog.Error("dial chrome", "err", err, "addr", d.cdpAddr)
		clientConn.Close(websocket.StatusInternalError, "could not connect to browser")
		return
	}
	defer chromeConn.CloseNow()

	slog.Info("session started")
	d.touch()

	errCh := make(chan error, 2)
	go d.relay(ctx, clientConn, chromeConn, "client→chrome", errCh)
	go d.relay(ctx, chromeConn, clientConn, "chrome→client", errCh)

	select {
	case err := <-errCh:
		if err != nil {
			slog.Info("relay ended", "reason", err)
		}
	case <-ctx.Done():
	}

	slog.Info("session ended", "duration", time.Since(d.sessionStart).Round(time.Second))
}

func (d *Defender) relay(ctx context.Context, src, dst *websocket.Conn, label string, errCh chan<- error) {
	for {
		msgType, data, err := src.Read(ctx)
		if err != nil {
			errCh <- fmt.Errorf("%s: %w", label, err)
			return
		}
		d.touch()
		if err := dst.Write(ctx, msgType, data); err != nil {
			errCh <- fmt.Errorf("%s write: %w", label, err)
			return
		}
	}
}

func (d *Defender) touch() {
	d.lastActivity.Store(time.Now().UnixNano())
}

func (d *Defender) watchTimeouts(ctx context.Context) {
	// Poll at 1/5 of the idle timeout (min 1s) so tests with short timeouts work.
	interval := d.idleTimeout / 5
	if interval < time.Second {
		interval = d.idleTimeout / 2
	}
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := time.Unix(0, d.lastActivity.Load())
			if d.connected.Load() && time.Since(last) > d.idleTimeout {
				slog.Warn("idle timeout exceeded, terminating pod")
				d.selfTerminate(ctx)
				return
			}
			if !d.sessionStart.IsZero() && time.Since(d.sessionStart) > d.sessionTimeout {
				slog.Warn("session timeout exceeded, terminating pod")
				d.selfTerminate(ctx)
				return
			}
		}
	}
}

func (d *Defender) selfTerminate(ctx context.Context) {
	if err := d.terminator.Terminate(ctx); err != nil {
		slog.Error("self-terminate failed", "err", err)
		// Fallback: exit the process so k8s restarts or cleans up.
		os.Exit(1)
	}
}

func (d *Defender) handleHealth(w http.ResponseWriter, r *http.Request) {
	connected := d.connected.Load()
	idleFor := time.Since(time.Unix(0, d.lastActivity.Load())).Round(time.Second)
	var duration time.Duration
	if !d.sessionStart.IsZero() {
		duration = time.Since(d.sessionStart).Round(time.Second)
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"connected":%v,"idleFor":%q,"sessionDuration":%q}`,
		connected, idleFor.String(), duration.String())
}
